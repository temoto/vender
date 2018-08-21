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
)

const (
	billTypeCount = 16

	delayShort = 100 * time.Millisecond
	delayErr   = 500 * time.Millisecond
	delayNext  = 200 * time.Millisecond
)

type BillValidator struct {
	bank   currency.NominalGroup
	escrow currency.NominalGroup
	mdb    mdb.Mdber

	byteOrder binary.ByteOrder

	// Indicates the value of the bill types 0 to 15.
	// These are final values including all scaling factors.
	billTypeCredit []currency.Nominal

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

func (self *BillValidator) Init(ctx context.Context) error {
	// TODO read config
	self.byteOrder = binary.BigEndian
	self.billTypeCredit = make([]currency.Nominal, billTypeCount)
	self.mdb = mdb.ContextValueMdber(ctx, "run/mdber")
	return nil
}

func (self *BillValidator) Loop(ctx context.Context, a *alive.Alive, ch chan<- PollResult) {
	self.mdb.TxDebug(packetReset, false)
	self.InitSequence()

	stopch := a.StopChan()
	for a.IsRunning() {
		pr := self.Poll()
		ch <- pr
		select {
		case <-stopch:
			return
		case <-time.After(pr.Delay):
		}
	}
}

func (self *BillValidator) billTypeNominal(b byte) currency.Nominal {
	if b >= billTypeCount {
		log.Printf("invalid bill type: %d", b)
		return 0
	}
	return self.billTypeCredit[b]
}

func (self *BillValidator) InitSequence() {
	self.Poll()
	self.Setup()
	self.mdb.TxDebug(mdb.PacketFromHex("3700"), true)       // 3700 EXPANSION IDENTIFICATION
	self.mdb.TxDebug(mdb.PacketFromHex("36"), true)         // 36 STACKER
	self.mdb.TxDebug(mdb.PacketFromHex("34ffffffff"), true) // 34 BILL TYPE
	for {
		pr := self.Poll()
		if pr.Ready {
			return
		}
		time.Sleep(pr.Delay)
	}
}

func (self *BillValidator) Setup() error {
	response := self.mdb.TxDebug(packetSetup, false)
	log.Printf("setup response=(%d)%s", response.Len(), response.Format())
	bs := response.Bytes()
	if len(bs) < 27 {
		return fmt.Errorf("bill validator SETUP response=%s expected 27 bytes", response.Format())
	}
	scalingFactor := self.byteOrder.Uint16(bs[3:5])
	for i, sf := range bs[11:27] {
		n := currency.Nominal(sf) * currency.Nominal(scalingFactor) * currency.Nominal(self.internalScalingFactor)
		log.Printf("i=%d sf=%d nominal=%s", i, sf, currency.Amount(n).Format100I())
		self.billTypeCredit[i] = n
	}
	self.escrowCap = bs[10] == 0xff
	log.Printf("Bill Validator Feature Level: %d", bs[0])
	log.Printf("Country / Currency Code: %x", bs[1:3])
	log.Printf("Bill Scaling Factor: %d", scalingFactor)
	log.Printf("Decimal Places: %d", bs[5])
	log.Printf("Stacker Capacity: %d", self.byteOrder.Uint16(bs[6:8]))
	log.Printf("Bill Security Levels: %016b", self.byteOrder.Uint16(bs[8:10]))
	log.Printf("Escrow/No Escrow: %t", self.escrowCap)
	log.Printf("Bill Type Credit: %x %#v", bs[11:27], self.billTypeCredit)
	return nil
}

func (self *BillValidator) parsePollItem(b byte) PollItem {
	switch b {
	case 0x01: // Defective Motor
		return PollItem{Status: StatusFatal, Error: fmt.Errorf("Defective Motor")}
	case 0x02: // Sensor Problem
		return PollItem{Status: StatusFatal, Error: fmt.Errorf("Sensor Problem")}
	case 0x03: // Validator Busy
		return PollItem{Status: StatusBusy}
	case 0x04: // ROM Checksum Error
		return PollItem{Status: StatusFatal, Error: fmt.Errorf("ROM Checksum Error")}
	case 0x05: // Validator Jammed
		return PollItem{Status: StatusFatal, Error: fmt.Errorf("Validator Jammed")}
	case 0x06: // Validator was reset
		return PollItem{Status: StatusWasReset}
	case 0x07: // Bill Removed
		return PollItem{Status: StatusError, Error: fmt.Errorf("Bill Removed")}
	case 0x08: // Cash Box out of position
		return PollItem{Status: StatusFatal, Error: fmt.Errorf("Cash Box out of position")}
	case 0x09: // Validator Disabled
		return PollItem{Status: StatusDisabled}
	case 0x0a: // Invalid Escrow request
		return PollItem{Status: StatusError, Error: fmt.Errorf("An ESCROW command was requested for a bill not in the escrow position.")}
	case 0x0b: // Bill Rejected
		return PollItem{Status: StatusRejected}
	case 0x0c: // Possible Credited Bill Removal
		return PollItem{Status: StatusError, Error: fmt.Errorf("There has been an attempt to remove a credited (stacked) bill.")}
	}

	if b&0x8f == b { // Bill Stacked
		amount := self.billTypeNominal(b & 0xf)
		return PollItem{Status: StatusCredit, Nominal: amount}
	}
	if b&0x9f == b { // Escrow Position
		amount := self.billTypeNominal(b & 0xf)
		log.Printf("bill escrow TODO packetEscrowAccept")
		return PollItem{Status: StatusEscrow, Nominal: amount}
	}
	if b&0x5f == b { // Number of attempts to input a bill while validator is disabled.
		attempts := b & 0x1f
		log.Printf("Number of attempts to input a bill while validator is disabled: %d", attempts)
		return PollItem{Status: StatusInfo, Attempts: attempts}
	}

	err := fmt.Errorf("parsePollItem unknown=%x", b)
	log.Print(err)
	return PollItem{Status: StatusFatal, Error: err}
}

func (self *BillValidator) Poll() PollResult {
	now := time.Now()
	response := self.mdb.TxDebug(packetPoll, false)
	result := PollResult{Time: now, Delay: delayNext}
	if response == nil {
		result.Error = fmt.Errorf("TODO use Tx(), if err !=nil result.Error = err")
		result.Delay = delayErr
		return result
	}
	if response.Len() == 0 {
		result.Ready = true
		return result
	}
	result.Items = make([]PollItem, response.Len())
	// log.Printf("poll response=%s", response.Format())
	for i, b := range response.Bytes() {
		result.Items[i] = self.parsePollItem(b)
	}
	return result
}
