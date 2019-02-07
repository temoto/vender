// Package bill incapsulates work with bill validators.
package bill

import (
	"context"
	"encoding/binary"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/juju/errors"
	"github.com/temoto/alive"
	"github.com/temoto/vender/currency"
	"github.com/temoto/vender/hardware/mdb"
	"github.com/temoto/vender/hardware/money"
	"github.com/temoto/vender/helpers/msync"
)

const (
	billTypeCount = 16

	DelayErr      = 500 * time.Millisecond
	DelayNext     = 200 * time.Millisecond
	DelayIdle     = 700 * time.Millisecond
	IdleThreshold = 30 * time.Second
)

//go:generate stringer -type=Features -trimprefix=Feature
type Features uint32

const (
	FeatureFTL Features = 1 << iota
	FeatureRecycling
)

type BillValidator struct {
	mdb mdb.Mdber

	byteOrder binary.ByteOrder

	// Indicates the value of the bill types 0 to 15.
	// These are final values including all scaling factors.
	billTypeCredit []currency.Nominal

	featureLevel      uint8
	supportedFeatures Features

	// Escrow capability.
	escrowCap bool

	internalScalingFactor int
	batch                 sync.Mutex
	ready                 msync.Signal
}

var (
	packetReset           = mdb.PacketFromHex("30")
	packetSetup           = mdb.PacketFromHex("31")
	packetPoll            = mdb.PacketFromHex("33")
	packetEscrowAccept    = mdb.PacketFromHex("3501")
	packetEscrowReject    = mdb.PacketFromHex("3500")
	packetStacker         = mdb.PacketFromHex("36")
	packetExpIdent        = mdb.PacketFromHex("3700")
	packetExpIdentOptions = mdb.PacketFromHex("3702")
)

var (
	ErrDefectiveMotor   = fmt.Errorf("Defective Motor")
	ErrBillRemoved      = fmt.Errorf("Bill Removed")
	ErrEscrowImpossible = fmt.Errorf("An ESCROW command was requested for a bill not in the escrow position.")
	ErrAttempts         = fmt.Errorf("Attempts")
)

// usage: defer x.Batch()()
func (self *BillValidator) Batch() func() {
	self.batch.Lock()
	return self.batch.Unlock
}

func (self *BillValidator) Init(ctx context.Context, mdber mdb.Mdber) error {
	// TODO read config
	self.byteOrder = binary.BigEndian
	self.billTypeCredit = make([]currency.Nominal, billTypeCount)
	self.mdb = mdber
	self.ready = msync.NewSignal()
	// TODO maybe execute CommandReset?
	err := self.InitSequence()
	return err
}

func (self *BillValidator) Run(ctx context.Context, a *alive.Alive, ch chan<- money.PollResult) {
	defer a.Done()

	lastActive := time.Now()
	stopch := a.StopChan()
	for a.IsRunning() {
		delay := DelayNext
		// TODO to reuse single PollResult safely, must clone .Items before sending to chan
		pr := money.NewPollResult(mdb.PacketMaxLength)
		if err := self.CommandPoll(pr); err != nil {
			delay = DelayErr
		} else {
			select {
			case ch <- *pr:
			case <-stopch:
				return
			}
		}
		now := time.Now()
		if len(pr.Items) > 0 {
			lastActive = now
		} else {
			if delay == DelayNext && now.Sub(lastActive) > IdleThreshold {
				delay = DelayIdle
			}
		}
		select {
		case <-time.After(delay):
		case <-stopch:
			return
		}
	}
}

func (self *BillValidator) ReadyChan() <-chan msync.Nothing {
	return self.ready
}

func (self *BillValidator) InitSequence() error {
	defer self.Batch()()

	err := self.CommandSetup()
	if err != nil {
		return err
	}
	if err = self.CommandExpansionIdentificationOptions(); err != nil {
		if _, ok := err.(mdb.FeatureNotSupported); ok {
			if err = self.CommandExpansionIdentification(); err != nil {
				return err
			}
		} else {
			return err
		}
	}
	if err = self.CommandStacker(); err != nil {
		return err
	}

	// TODO if err
	// TODO read config
	// self.CommandBillType(0xffff, 0xffff)
	time.Sleep(DelayNext)
	if err = self.CommandBillType(0xffff, 0); err != nil {
		return err
	}
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
	log.Printf("CommandBillType request=%s err=%v", request.Format(), err)
	return err
}

func (self *BillValidator) CommandSetup() error {
	const expectLength = 27
	response := new(mdb.Packet)
	err := self.mdb.Tx(packetSetup, response)
	if err != nil {
		log.Printf("mdb request=%s err=%v", packetSetup.Format(), err)
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

func (self *BillValidator) CommandPoll(result *money.PollResult) error {
	result.Error = nil
	result.Items = result.Items[:0]
	result.Time = time.Now()
	response := new(mdb.Packet)
	err := self.mdb.Tx(packetPoll, response)
	if err != nil {
		result.Error = err
		return errors.Annotate(err, "hardware/mdb/bill POLL")
	}
	bs := response.Bytes()
	if len(bs) == 0 {
		self.ready.Set()
		return nil
	}
	// log.Printf("poll response=%s", response.Format())
	for _, b := range bs {
		pi := self.parsePollItem(b)
		result.Items = append(result.Items, pi)
	}
	return nil
}

func (self *BillValidator) CommandStacker() error {
	request := packetStacker
	response := new(mdb.Packet)
	err := self.mdb.Tx(request, response)
	// if err != nil {
	// 	log.Printf("mdb request=%s err=%v", request.Format(), err)
	// 	return err
	// }
	log.Printf("mdb request=%s response=%s err=%v", request.Format(), response.Format(), err)
	return err
}

func (self *BillValidator) CommandExpansionIdentification() error {
	const expectLength = 29
	request := packetExpIdent
	response := new(mdb.Packet)
	err := self.mdb.Tx(request, response)
	if err != nil {
		log.Printf("mdb request=%s err=%v", request.Format(), err)
		return err
	}
	log.Printf("EXPANSION IDENTIFICATION response=(%d)%s", response.Len(), response.Format())
	bs := response.Bytes()
	if len(bs) < expectLength {
		return fmt.Errorf("hardware/mdb/bill EXPANSION IDENTIFICATION response=%s expected %d bytes", response.Format(), expectLength)
	}
	log.Printf("Manufacturer Code: %x", bs[0:0+3])
	log.Printf("Serial Number: '%s'", string(bs[3:3+12]))
	log.Printf("Model #/Tuning Revision: '%s'", string(bs[15:15+12]))
	log.Printf("Software Version: %x", bs[27:27+2])
	return nil
}

func (self *BillValidator) CommandFeatureEnable(requested Features) error {
	f := requested & self.supportedFeatures
	buf := [6]byte{0x37, 0x01}
	self.byteOrder.PutUint32(buf[2:], uint32(f))
	request := mdb.PacketFromBytes(buf[:])
	err := self.mdb.Tx(request, new(mdb.Packet))
	if err != nil {
		log.Printf("mdb request=%s err=%v", request.Format(), err)
	}
	return err
}

func (self *BillValidator) CommandExpansionIdentificationOptions() error {
	if self.featureLevel < 2 {
		return mdb.FeatureNotSupported("EXPANSION IDENTIFICATION WITH OPTION BITS is level 2+")
	}
	const expectLength = 33
	request := packetExpIdentOptions
	response := new(mdb.Packet)
	err := self.mdb.Tx(request, response)
	if err != nil {
		log.Printf("mdb request=%s err=%v", request.Format(), err)
		return err
	}
	log.Printf("EXPANSION IDENTIFICATION WITH OPTION BITS response=(%d)%s", response.Len(), response.Format())
	bs := response.Bytes()
	if len(bs) < expectLength {
		return fmt.Errorf("hardware/mdb/bill EXPANSION IDENTIFICATION WITH OPTION BITS response=%s expected %d bytes", response.Format(), expectLength)
	}
	self.supportedFeatures = Features(self.byteOrder.Uint32(bs[29 : 29+4]))
	log.Printf("Manufacturer Code: %x", bs[0:0+3])
	log.Printf("Serial Number: '%s'", string(bs[3:3+12]))
	log.Printf("Model #/Tuning Revision: '%s'", string(bs[15:15+12]))
	log.Printf("Software Version: %x", bs[27:27+2])
	log.Printf("Optional Features: %b", self.supportedFeatures)
	return nil
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
