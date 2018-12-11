// Package bill incapsulates work with bill validators.
package bill

import (
	"context"
	"encoding/binary"
	"fmt"
	"log"
	"time"

	"github.com/juju/errors"
	"github.com/temoto/alive"
	"github.com/temoto/vender/currency"
	"github.com/temoto/vender/hardware/mdb"
	"github.com/temoto/vender/hardware/money"
	"github.com/temoto/vender/head/state"
	"github.com/temoto/vender/helpers/msync"
)

const (
	TypeCount = 16

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
	dev mdb.Device

	featureLevel      uint8
	supportedFeatures Features
	// Escrow capability.
	escrowCap bool

	scalingFactor uint16
	billFactors   [TypeCount]uint8
	billNominals  [TypeCount]currency.Nominal

	stackerFull  bool
	stackerCount uint

	ready msync.Signal

	DoIniter      msync.Doer
	DoConfigBills msync.Doer
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

type CommandStacker struct {
	Dev   *mdb.Device
	done  bool
	full  bool
	count uint16
}

func (self *CommandStacker) Do(ctx context.Context) error {
	request := packetStacker
	r := self.Dev.Tx(request)
	if r.E != nil {
		log.Printf("mdb request=%s err=%v", request.Format(), r.E)
		return r.E
	}
	rb := r.P.Bytes()
	if len(rb) != 2 {
		return errors.Errorf("STACKER response length=%d expected=2", len(rb))
	}
	x := self.Dev.ByteOrder.Uint16(rb)
	self.full = (x & 0x8000) != 0
	self.count = x & 0x7fff
	self.done = true
	log.Printf(self.String())
	return nil
}
func (self *CommandStacker) String() string {
	return fmt.Sprintf("STACKER done=%t full=%t count=%d", self.done, self.full, self.count)
}

func (self *BillValidator) Init(ctx context.Context, mdber mdb.Mdber) error {
	// TODO read config
	self.dev.Address = 0x30
	self.dev.Name = "billvalidator"
	self.dev.ByteOrder = binary.BigEndian
	self.dev.Mdber = mdber

	// warning: hidden dependencies in order of following calls
	self.DoConfigBills = self.newConfigBills()
	self.DoIniter = self.newIniter()

	self.ready = msync.NewSignal()
	// TODO maybe execute CommandReset?
	err := self.DoIniter.Do(ctx)
	return err
}

func (self *BillValidator) Run(ctx context.Context, a *alive.Alive, ch chan<- money.PollResult) {
	defer a.Done()
	cmd := &CommandPoll{bv: self}
	defer func() { cmd.bv = nil; cmd = nil }() // help GC, redundant?

	lastActive := time.Now()
	stopch := a.StopChan()
	for a.IsRunning() {
		// FIXME maybe use cmd.R.Delay
		delay := DelayNext
		if err := cmd.Do(ctx); err != nil {
			delay = DelayErr
			log.Printf("billvalidator/Run/POLL err=%v", err)
		} else {
			select {
			case ch <- cmd.R:
			case <-stopch:
				return
			}
		}
		now := time.Now()
		if len(cmd.R.Items) > 0 {
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

func (self *BillValidator) newIniter() msync.Doer {
	tx := msync.NewTransaction(self.dev.Name + "-init")
	tx.Root.
		Append(&msync.DoFunc0{F: self.CommandSetup}).
		Append(&msync.DoFunc0{F: func() error {
			if err := self.CommandExpansionIdentificationOptions(); err != nil {
				if _, ok := err.(mdb.FeatureNotSupported); ok {
					if err = self.CommandExpansionIdentification(); err != nil {
						return err
					}
				} else {
					return err
				}
			}
			return nil
		}}).
		Append(&CommandStacker{Dev: &self.dev}).
		Append(&msync.DoSleep{DelayNext}).
		Append(self.DoConfigBills)
	return tx
}

func (self *BillValidator) newConfigBills() msync.Doer {
	return &msync.DoFunc{
		Name: "enable-bills-config",
		F: func(ctx context.Context) error {
			config := state.GetConfig(ctx)
			// TODO read enabled nominals from config
			_ = config
			return self.CommandBillType(0xffff, 0)
		},
	}
}

func (self *BillValidator) NewRestarter() msync.Doer {
	tx := msync.NewTransaction(self.dev.Name + "-restart")
	tx.Root.
		Append(&msync.DoFunc0{F: self.CommandReset}).
		Append(&msync.DoSleep{200 * time.Millisecond}).
		Append(self.newIniter())
	return tx
}

func (self *BillValidator) CommandReset() error {
	return self.dev.Tx(packetReset).E
}

func (self *BillValidator) CommandBillType(accept, escrow uint16) error {
	buf := [5]byte{0x34}
	self.dev.ByteOrder.PutUint16(buf[1:], accept)
	self.dev.ByteOrder.PutUint16(buf[3:], escrow)
	request := mdb.PacketFromBytes(buf[:])
	err := self.dev.Tx(request).E
	log.Printf("CommandBillType request=%s err=%v", request.Format(), err)
	return err
}

func (self *BillValidator) CommandSetup() error {
	const expectLength = 27
	r := self.dev.Tx(packetSetup)
	if r.E != nil {
		log.Printf("mdb request=%s err=%v", packetSetup.Format(), r.E)
		return r.E
	}
	log.Printf("setup response=(%d)%s", r.P.Len(), r.P.Format())
	bs := r.P.Bytes()
	if len(bs) < expectLength {
		return fmt.Errorf("bill validator SETUP response=%s expected %d bytes", r.P.Format(), expectLength)
	}

	self.featureLevel = bs[0]
	currencyCode := bs[1:3]
	self.scalingFactor = self.dev.ByteOrder.Uint16(bs[3:5])
	stackerCap := self.dev.ByteOrder.Uint16(bs[6:8])
	billSecurityLevels := self.dev.ByteOrder.Uint16(bs[8:10])
	self.escrowCap = bs[10] == 0xff

	log.Printf("Bill Type Scaling Factors: %3v", bs[11:])
	for i, sf := range bs[11:] {
		if i >= TypeCount {
			log.Printf("ERROR bill SETUP type factors count=%d more than expected=%d", len(bs[11:]), TypeCount)
			break
		}
		self.billFactors[i] = sf
		self.billNominals[i] = currency.Nominal(sf) * currency.Nominal(self.scalingFactor)
	}
	log.Printf("Bill Type calc. nominals:  %3v", self.billNominals)

	log.Printf("Bill Validator Feature Level: %d", self.featureLevel)
	log.Printf("Country / Currency Code: %x", currencyCode)
	log.Printf("Bill Scaling Factor: %d", self.scalingFactor)
	log.Printf("Decimal Places: %d", bs[5])
	log.Printf("Stacker Capacity: %d", stackerCap)
	log.Printf("Bill Security Levels: %016b", billSecurityLevels)
	log.Printf("Escrow/No Escrow: %t", self.escrowCap)
	log.Printf("Bill Type Credit: %x %#v", bs[11:], self.billNominals)
	return nil
}

type CommandPoll struct {
	bv *BillValidator
	R  money.PollResult
}

func (self *CommandPoll) Do(ctx context.Context) error {
	now := time.Now()
	r := self.bv.dev.Tx(packetPoll)
	// TODO avoid allocations
	self.R = money.PollResult{Time: now}
	if r.E != nil {
		self.R.Error = r.E
		return r.E
	}
	bs := r.P.Bytes()
	if len(bs) == 0 {
		// FIXME self.ready.Set()
		return nil
	}
	// TODO avoid allocations
	self.R.Items = make([]money.PollItem, len(bs))
	// log.Printf("poll response=%s", response.Format())
	for i, b := range bs {
		self.R.Items[i] = self.bv.parsePollItem(b)
	}
	return nil
}

func (self *BillValidator) CommandExpansionIdentification() error {
	const expectLength = 29
	request := packetExpIdent
	r := self.dev.Tx(request)
	if r.E != nil {
		log.Printf("mdb request=%s err=%v", request.Format(), r.E)
		return r.E
	}
	log.Printf("EXPANSION IDENTIFICATION response=(%d)%s", r.P.Len(), r.P.Format())
	bs := r.P.Bytes()
	if len(bs) < expectLength {
		return fmt.Errorf("hardware/mdb/bill EXPANSION IDENTIFICATION response=%s expected %d bytes", r.P.Format(), expectLength)
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
	self.dev.ByteOrder.PutUint32(buf[2:], uint32(f))
	request := mdb.PacketFromBytes(buf[:])
	err := self.dev.Tx(request).E
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
	r := self.dev.Tx(request)
	if r.E != nil {
		log.Printf("mdb request=%s err=%v", request.Format(), r.E)
		return r.E
	}
	log.Printf("EXPANSION IDENTIFICATION WITH OPTION BITS response=(%d)%s", r.P.Len(), r.P.Format())
	bs := r.P.Bytes()
	if len(bs) < expectLength {
		return fmt.Errorf("hardware/mdb/bill EXPANSION IDENTIFICATION WITH OPTION BITS response=%s expected %d bytes", r.P.Format(), expectLength)
	}
	self.supportedFeatures = Features(self.dev.ByteOrder.Uint32(bs[29 : 29+4]))
	log.Printf("Manufacturer Code: %x", bs[0:0+3])
	log.Printf("Serial Number: '%s'", string(bs[3:3+12]))
	log.Printf("Model #/Tuning Revision: '%s'", string(bs[15:15+12]))
	log.Printf("Software Version: %x", bs[27:27+2])
	log.Printf("Optional Features: %b", self.supportedFeatures)
	return nil
}

func (self *BillValidator) billNominal(b byte) currency.Nominal {
	if b >= TypeCount {
		log.Printf("invalid bill type: %d", b)
		return 0
	}
	return self.billNominals[b]
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
		amount := self.billNominal(b & 0xf)
		return money.PollItem{Status: money.StatusCredit, DataNominal: amount}
	}
	if b&0x9f == b { // Escrow Position
		amount := self.billNominal(b & 0xf)
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
