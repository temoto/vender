// Package bill incapsulates work with bill validators.
package bill

import (
	"context"
	"encoding/binary"
	"fmt"
	"log"
	"time"

	"github.com/temoto/alive"
	"github.com/temoto/vender/currency"
	"github.com/temoto/vender/hardware/mdb"
	"github.com/temoto/vender/hardware/money"
)

const (
	billTypeCount = 16

	DelayShort = 100 * time.Millisecond
	DelayErr   = 500 * time.Millisecond
	DelayNext  = 200 * time.Millisecond
)

type BillValidator struct {
	mdb mdb.Mdber

	byteOrder binary.ByteOrder

	// Indicates the value of the bill types 0 to 15.
	// These are final values including all scaling factors.
	billTypeCredit []currency.Nominal

	featureLevel uint8

	// Escrow capability.
	escrowCap bool

	internalScalingFactor int
}

var (
	packetReset        = mdb.PacketFromHex("30")
	packetSetup        = mdb.PacketFromHex("31")
	packetPoll         = mdb.PacketFromHex("33")
	packetEscrowAccept = mdb.PacketFromHex("3501")
	packetEscrowReject = mdb.PacketFromHex("3500")
)

var (
	ErrDefectiveMotor   = fmt.Errorf("Defective Motor")
	ErrBillRemoved      = fmt.Errorf("Bill Removed")
	ErrEscrowImpossible = fmt.Errorf("An ESCROW command was requested for a bill not in the escrow position.")
	ErrAttempts         = fmt.Errorf("Attempts")
)

func (self *BillValidator) Init(ctx context.Context, mdber mdb.Mdber) error {
	// TODO read config
	self.byteOrder = binary.BigEndian
	self.billTypeCredit = make([]currency.Nominal, billTypeCount)
	self.mdb = mdber
	// TODO maybe execute CommandReset?
	self.InitSequence()
	return nil
}

func (self *BillValidator) Run(ctx context.Context, a *alive.Alive, ch chan<- money.PollResult) {
	stopch := a.StopChan()
	for a.IsRunning() {
		pr := self.CommandPoll()
		ch <- pr
		select {
		case <-stopch:
			return
		case <-time.After(pr.Delay):
		}
	}
}

func (self *BillValidator) InitSequence() error {
	err := self.CommandSetup()
	if err != nil {
		return err
	}
	// TODO if err
	self.mdb.TxDebug(mdb.PacketFromHex("3700"), true) // 3700 EXPANSION IDENTIFICATION
	// TODO if err
	self.mdb.TxDebug(mdb.PacketFromHex("36"), true) // 36 STACKER
	// TODO if err
	// TODO read config
	// self.CommandBillType(0xffff, 0xffff)
	self.CommandBillType(0xffff, 0)
	return nil
}

func (self *BillValidator) CommandReset() error {
	return self.mdb.Tx(packetReset, new(mdb.Packet))
}

func (self *BillValidator) CommandBillType(accept, escrow uint16) error {
	buf := [5]byte{0x34}
	self.byteOrder.PutUint16(buf[1:], accept)
	self.byteOrder.PutUint16(buf[3:], escrow)
	request := mdb.PacketFromBytes(buf[:])
	response := new(mdb.Packet)
	err := self.mdb.Tx(request, response)
	log.Printf("CommandBillType request=%s err=%s", request.Format(), err)
	return err
}

func (self *BillValidator) CommandSetup() error {
	const expectLength = 27
	response := new(mdb.Packet)
	err := self.mdb.Tx(packetSetup, response)
	if err != nil {
		log.Printf("mdb request=%s err: %s", packetSetup.Format(), err)
		return err
	}
	log.Printf("setup response=(%d)%s", response.Len(), response.Format())
	bs := response.Bytes()
	if len(bs) < expectLength {
		return fmt.Errorf("bill validator SETUP response=%s expected %d bytes", response.Format(), expectLength)
	}
	scalingFactor := self.byteOrder.Uint16(bs[3:5])
	for i, sf := range bs[11:] {
		n := currency.Nominal(sf) * currency.Nominal(scalingFactor) * currency.Nominal(self.internalScalingFactor)
		log.Printf("i=%d sf=%d nominal=%s", i, sf, currency.Amount(n).Format100I())
		self.billTypeCredit[i] = n
	}
	self.escrowCap = bs[10] == 0xff
	self.featureLevel = bs[0]
	log.Printf("Bill Validator Feature Level: %d", self.featureLevel)
	log.Printf("Country / Currency Code: %x", bs[1:3])
	log.Printf("Bill Scaling Factor: %d", scalingFactor)
	log.Printf("Decimal Places: %d", bs[5])
	log.Printf("Stacker Capacity: %d", self.byteOrder.Uint16(bs[6:8]))
	log.Printf("Bill Security Levels: %016b", self.byteOrder.Uint16(bs[8:10]))
	log.Printf("Escrow/No Escrow: %t", self.escrowCap)
	log.Printf("Bill Type Credit: %x %#v", bs[11:], self.billTypeCredit)
	return nil
}

func (self *BillValidator) CommandPoll() money.PollResult {
	now := time.Now()
	response := new(mdb.Packet)
	err := self.mdb.Tx(packetPoll, response)
	result := money.PollResult{Time: now, Delay: DelayNext}
	if err != nil {
		result.Error = err
		result.Delay = DelayErr
		return result
	}
	if response.Len() == 0 {
		return result
	}
	result.Items = make([]money.PollItem, response.Len())
	// log.Printf("poll response=%s", response.Format())
	for i, b := range response.Bytes() {
		result.Items[i] = self.parsePollItem(b)
	}
	return result
}

func (self *BillValidator) billTypeNominal(b byte) currency.Nominal {
	if b >= billTypeCount {
		log.Printf("invalid bill type: %d", b)
		return 0
	}
	return self.billTypeCredit[b]
}

func (self *BillValidator) parsePollItem(b byte) money.PollItem {
	switch b {
	case 0x01: // Defective Motor
		return money.PollItem{Status: money.StatusFatal, Error: ErrDefectiveMotor}
	case 0x02: // Sensor Problem
		return money.PollItem{Status: money.StatusFatal, Error: money.ErrSensor}
	case 0x03: // Validator Busy
		return money.PollItem{Status: money.StatusBusy}
	case 0x04: // ROM Checksum Error
		return money.PollItem{Status: money.StatusFatal, Error: money.ErrROMChecksum}
	case 0x05: // Validator Jammed
		return money.PollItem{Status: money.StatusFatal, Error: money.ErrJam}
	case 0x06: // Validator was reset
		return money.PollItem{Status: money.StatusWasReset}
	case 0x07: // Bill Removed
		return money.PollItem{Status: money.StatusError, Error: ErrBillRemoved}
	case 0x08: // Cash Box out of position
		return money.PollItem{Status: money.StatusFatal, Error: money.ErrNoStorage}
	case 0x09: // Validator Disabled
		return money.PollItem{Status: money.StatusDisabled}
	case 0x0a: // Invalid Escrow request
		return money.PollItem{Status: money.StatusError, Error: ErrEscrowImpossible}
	case 0x0b: // Bill Rejected
		return money.PollItem{Status: money.StatusRejected}
	case 0x0c: // Possible Credited Bill Removal
		return money.PollItem{Status: money.StatusError, Error: money.ErrFraud}
	}

	if b&0x8f == b { // Bill Stacked
		amount := self.billTypeNominal(b & 0xf)
		return money.PollItem{Status: money.StatusCredit, DataNominal: amount}
	}
	if b&0x9f == b { // Escrow Position
		amount := self.billTypeNominal(b & 0xf)
		log.Printf("bill escrow TODO packetEscrowAccept")
		return money.PollItem{Status: money.StatusEscrow, DataNominal: amount}
	}
	if b&0x5f == b { // Number of attempts to input a bill while validator is disabled.
		attempts := b & 0x1f
		log.Printf("Number of attempts to input a bill while validator is disabled: %d", attempts)
		return money.PollItem{Status: money.StatusInfo, Error: ErrAttempts, DataCount: attempts}
	}

	err := fmt.Errorf("parsePollItem unknown=%x", b)
	log.Print(err)
	return money.PollItem{Status: money.StatusFatal, Error: err}
}
