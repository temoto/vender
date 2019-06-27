// Package bill incapsulates work with bill validators.
package bill

import (
	"context"
	"encoding/binary"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/temoto/errors"
	"github.com/temoto/vender/currency"
	"github.com/temoto/vender/engine"
	"github.com/temoto/vender/hardware/mdb"
	"github.com/temoto/vender/hardware/money"
	"github.com/temoto/vender/state"
)

const (
	TypeCount = 16
)

//go:generate stringer -type=Features -trimprefix=Feature
type Features uint32

const (
	FeatureFTL Features = 1 << iota
	FeatureRecycling
)

const DefaultEscrowTimeout = 30 * time.Second

type BillValidator struct {
	dev    mdb.Device
	pollmu sync.Mutex // isolate active/idle polling

	// parsed from SETUP
	featureLevel      uint8
	supportedFeatures Features
	escrowSupported   bool
	configScaling     uint16
	nominals          [TypeCount]currency.Nominal // final values, includes all scaling factors

	// dynamic state useful for external code
	escrowBill   currency.Nominal // assume only one bill may be in escrow position
	stackerFull  bool
	stackerCount uint32
}

var (
	packetEscrowAccept    = mdb.MustPacketFromHex("3501", true)
	packetEscrowReject    = mdb.MustPacketFromHex("3500", true)
	packetStacker         = mdb.MustPacketFromHex("36", true)
	packetExpIdent        = mdb.MustPacketFromHex("3700", true)
	packetExpIdentOptions = mdb.MustPacketFromHex("3702", true)
)

var (
	ErrDefectiveMotor   = errors.New("Defective Motor")
	ErrBillRemoved      = errors.New("Bill Removed")
	ErrEscrowImpossible = errors.New("An ESCROW command was requested for a bill not in the escrow position.")
	ErrAttempts         = errors.New("Attempts")
	ErrEscrowTimeout    = errors.New("ESCROW timeout")
)

func (self *BillValidator) Init(ctx context.Context) error {
	const tag = "mdb.bill.Init"

	g := state.GetGlobal(ctx)
	m, err := g.Mdber()
	if err != nil {
		return errors.Annotate(err, tag)
	}
	// TODO read settings from config
	self.configScaling = 100
	self.dev.Init(m.Tx, g.Log, 0x30, "bill", binary.BigEndian)

	doInit := self.newIniter()
	if err = doInit.Do(ctx); err != nil {
		return errors.Annotate(err, tag)
	}

	return nil
}

func (self *BillValidator) AcceptMax(max currency.Amount) engine.Doer {
	// config := state.GetConfig(ctx)
	enableBitset := uint16(0)
	escrowBitset := uint16(0)

	if max != 0 {
		for i, n := range self.nominals {
			if n == 0 {
				continue
			}
			if currency.Amount(n) <= max {
				// TODO consult config
				// _ = config
				enableBitset |= 1 << uint(i)
				// TODO consult config
				// escrowBitset |= 1 << uint(i)
			}
		}
	}

	return self.NewBillType(enableBitset, escrowBitset)
}

func (self *BillValidator) SupportedNominals() []currency.Nominal {
	ns := make([]currency.Nominal, 0, TypeCount)
	for _, n := range self.nominals {
		if n != 0 {
			ns = append(ns, n)
		}
	}
	return ns
}

func (self *BillValidator) Run(ctx context.Context, stopch <-chan struct{}, fun func(money.PollItem) bool) {
	pd := mdb.PollDelay{}
	var r mdb.PacketError
	parse := self.pollFun(fun)
	var active bool
	var err error
	for {
		self.pollmu.Lock()
		r = self.dev.Tx(self.dev.PacketPoll)
		if r.E == nil {
			active, err = parse(r.P)
		}
		self.pollmu.Unlock()

		if !pd.Delay(&self.dev, active, err != nil, stopch) {
			break
		}
	}
}
func (self *BillValidator) pollFun(fun func(money.PollItem) bool) mdb.PollFunc {
	const tag = "mdb.bill.poll"

	return func(p mdb.Packet) (bool, error) {
		bs := p.Bytes()
		if len(bs) == 0 {
			return false, nil
		}
		for _, b := range bs {
			pi := self.parsePollItem(b)

			switch pi.Status {
			case money.StatusInfo:
				self.dev.Log.Infof("%s/info: %s", tag, pi.String())
				// TODO telemetry
				// state.GetGlobal(ctx)
			case money.StatusError:
				self.dev.Log.Errorf("%s/error: %v", tag, pi.Error)
				// TODO telemetry
			case money.StatusFatal:
				self.dev.Log.Errorf("%s/fatal: %v", tag, pi.Error)
				// TODO telemetry
			case money.StatusBusy:
			default:
				if fun(pi) {
					return true, nil
				}
			}
		}
		return true, nil
	}
}

func (self *BillValidator) newIniter() engine.Doer {
	return engine.NewSeq(self.dev.Name + ".initer").
		// TODO maybe execute Reset?
		Append(engine.Func{Name: self.dev.Name + ".setup", F: self.CommandSetup}).
		Append(engine.Func0{Name: self.dev.Name + ".expid", F: func() error {
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
		Append(self.NewStacker()).
		Append(engine.Sleep{Duration: self.dev.DelayNext})
}

func (self *BillValidator) NewBillType(accept, escrow uint16) engine.Doer {
	buf := [5]byte{0x34}
	self.dev.ByteOrder.PutUint16(buf[1:], accept)
	self.dev.ByteOrder.PutUint16(buf[3:], escrow)
	request := mdb.MustPacketFromBytes(buf[:], true)
	return engine.Func0{Name: "mdb.bill.BillType", F: func() error {
		return self.dev.Tx(request).E
	}}
}

func (self *BillValidator) NewRestarter() engine.Doer {
	return engine.NewSeq(self.dev.Name + ".restarter").
		Append(self.dev.DoReset).
		Append(self.newIniter())
}

func (self *BillValidator) setEscrowBill(n currency.Nominal) {
	atomic.StoreUint32((*uint32)(&self.escrowBill), uint32(n))
}
func (self *BillValidator) EscrowAmount() currency.Amount {
	u32 := atomic.LoadUint32((*uint32)(&self.escrowBill))
	return currency.Amount(u32)
}

func (self *BillValidator) NewEscrow(accept bool) engine.Doer {
	var tag string
	var request mdb.Packet
	if accept {
		tag = "mdb.bill.escrow-accept"
		request = packetEscrowAccept
	} else {
		tag = "mdb.bill.escrow-reject"
		request = packetEscrowReject
	}

	// FIXME passive poll loop (`Run`) will wrongly consume response to this
	// TODO find a good way to isolate this code from `Run` loop
	// - silly `Mutex` will do the job
	// - serializing via channel on mdb.Device would be better

	return engine.Func{Name: tag, F: func(ctx context.Context) error {
		self.pollmu.Lock()
		defer self.pollmu.Lock()

		r := self.dev.Tx(request)
		if r.E != nil {
			return r.E
		}

		// > After an ESCROW command the bill validator should respond to a POLL command
		// > with the BILL STACKED, BILL RETURNED, INVALID ESCROW or BILL TO RECYCLER
		// > message within 30 seconds. If a bill becomes jammed in a position where
		// > the customer may be able to retrieve it, the bill validator
		// > should send a BILL RETURNED message.
		var result error
		fun := self.pollFun(func(pi money.PollItem) bool {
			code := pi.HardwareCode
			switch code {
			case StatusValidatorDisabled:
				self.dev.Log.Errorf("CRITICAL likely code error: escrow request while disabled")
				result = ErrEscrowImpossible
				return true
			case StatusInvalidEscrowRequest:
				self.dev.Log.Errorf("CRITICAL likely code error: escrow request invalid")
				result = ErrEscrowImpossible
				return true
			case StatusRoutingBillStacked, StatusRoutingBillReturned, StatusRoutingBillToRecycler:
				self.dev.Log.Infof("escrow result code=%02x", code) // TODO string
				return true
			default:
				return false
			}
		})
		err := self.dev.NewPollLoop(tag, self.dev.PacketPoll, DefaultEscrowTimeout, fun).Do(ctx)
		if err != nil {
			return err
		}
		return result
	}}
}

func (self *BillValidator) NewStacker() engine.Doer {
	const tag = "mdb.bill.stacker"

	return engine.Func{Name: tag, F: func(ctx context.Context) error {
		request := packetStacker
		r := self.dev.Tx(request)
		if r.E != nil {
			return errors.Annotate(r.E, tag)
		}
		rb := r.P.Bytes()
		if len(rb) != 2 {
			return errors.Errorf("%s response length=%d expected=2", tag, len(rb))
		}
		x := self.dev.ByteOrder.Uint16(rb)
		self.stackerFull = (x & 0x8000) != 0
		self.stackerCount = uint32(x & 0x7fff)
		self.dev.Log.Debugf("%s full=%t count=%d", tag, self.stackerFull, self.stackerCount)
		return nil
	}}
}

func (self *BillValidator) CommandSetup(ctx context.Context) error {
	const expectLength = 27
	var scalingFactor uint16
	var billFactors [TypeCount]uint8

	err := self.dev.DoSetup(ctx)
	if err != nil {
		return err
	}
	bs := self.dev.SetupResponse.Bytes()
	if len(bs) < expectLength {
		return fmt.Errorf("bill validator SETUP response=%s expected %d bytes", self.dev.SetupResponse.Format(), expectLength)
	}

	self.featureLevel = bs[0]
	currencyCode := bs[1:3]
	scalingFactor = self.dev.ByteOrder.Uint16(bs[3:5])
	stackerCap := self.dev.ByteOrder.Uint16(bs[6:8])
	billSecurityLevels := self.dev.ByteOrder.Uint16(bs[8:10])
	self.escrowSupported = bs[10] == 0xff

	self.dev.Log.Debugf("Bill Type Scaling Factors: %3v", bs[11:])
	for i, sf := range bs[11:] {
		if i >= TypeCount {
			self.dev.Log.Errorf("CRITICAL bill SETUP type factors count=%d > expected=%d", len(bs[11:]), TypeCount)
			break
		}
		billFactors[i] = sf
		self.nominals[i] = currency.Nominal(sf) * currency.Nominal(scalingFactor) * currency.Nominal(self.configScaling)
	}
	self.dev.Log.Debugf("Bill Type calc. nominals:  %3v", self.nominals)

	self.dev.Log.Debugf("Bill Validator Feature Level: %d", self.featureLevel)
	self.dev.Log.Debugf("Country / Currency Code: %x", currencyCode)
	self.dev.Log.Debugf("Bill Scaling Factor: %d", scalingFactor)
	// self.dev.Log.Debugf("Decimal Places: %d", bs[5])
	self.dev.Log.Debugf("Stacker Capacity: %d", stackerCap)
	self.dev.Log.Debugf("Bill Security Levels: %016b", billSecurityLevels)
	self.dev.Log.Debugf("Escrow/No Escrow: %t", self.escrowSupported)
	self.dev.Log.Debugf("Bill Type Credit: %x %v", bs[11:], self.nominals)
	return nil
}

func (self *BillValidator) CommandExpansionIdentification() error {
	const tag = "mdb.bill.ExpId"
	const expectLength = 29
	request := packetExpIdent
	r := self.dev.Tx(request)
	if r.E != nil {
		return errors.Annotate(r.E, tag)
	}
	bs := r.P.Bytes()
	self.dev.Log.Debugf("%s response=%x", tag, bs)
	if len(bs) < expectLength {
		return fmt.Errorf("%s response=%x length=%d expected=%d", tag, bs, len(bs), expectLength)
	}
	self.dev.Log.Debugf("%s Manufacturer Code: %x", tag, bs[0:0+3])
	self.dev.Log.Debugf("%s Serial Number: '%s'", tag, string(bs[3:3+12]))
	self.dev.Log.Debugf("%s Model #/Tuning Revision: '%s'", tag, string(bs[15:15+12]))
	self.dev.Log.Debugf("%s Software Version: %x", tag, bs[27:27+2])
	return nil
}

func (self *BillValidator) CommandFeatureEnable(requested Features) error {
	f := requested & self.supportedFeatures
	buf := [6]byte{0x37, 0x01}
	self.dev.ByteOrder.PutUint32(buf[2:], uint32(f))
	request := mdb.MustPacketFromBytes(buf[:], true)
	err := self.dev.Tx(request).E
	return errors.Annotate(err, "mdb.bill.FeatureEnable")
}

func (self *BillValidator) CommandExpansionIdentificationOptions() error {
	const tag = "mdb.bill.ExpIdOptions"
	if self.featureLevel < 2 {
		return mdb.FeatureNotSupported(tag + " is level 2+")
	}
	const expectLength = 33
	request := packetExpIdentOptions
	r := self.dev.Tx(request)
	if r.E != nil {
		return errors.Annotate(r.E, tag)
	}
	self.dev.Log.Debugf("%s response=(%d)%s", tag, r.P.Len(), r.P.Format())
	bs := r.P.Bytes()
	if len(bs) < expectLength {
		return fmt.Errorf("%s response=%s expected %d bytes", tag, r.P.Format(), expectLength)
	}
	self.supportedFeatures = Features(self.dev.ByteOrder.Uint32(bs[29 : 29+4]))
	self.dev.Log.Debugf("%s Manufacturer Code: %x", tag, bs[0:0+3])
	self.dev.Log.Debugf("%s Serial Number: '%s'", tag, string(bs[3:3+12]))
	self.dev.Log.Debugf("%s Model #/Tuning Revision: '%s'", tag, string(bs[15:15+12]))
	self.dev.Log.Debugf("%s Software Version: %x", tag, bs[27:27+2])
	self.dev.Log.Debugf("%s Optional Features: %b", tag, self.supportedFeatures)
	return nil
}

const (
	StatusDefectiveMotor       byte = 0x01
	StatusSensorProblem        byte = 0x02
	StatusValidatorBusy        byte = 0x03
	StatusROMChecksumError     byte = 0x04
	StatusValidatorJammed      byte = 0x05
	StatusValidatorWasReset    byte = 0x06
	StatusBillRemoved          byte = 0x07
	StatusCashboxOutOfPosition byte = 0x08
	StatusValidatorDisabled    byte = 0x09
	StatusInvalidEscrowRequest byte = 0x0a
	StatusBillRejected         byte = 0x0b
	StatusCreditedBillRemoval  byte = 0x0c
)

const (
	StatusRoutingBillStacked byte = 0x80 | (iota << 4)
	StatusRoutingEscrowPosition
	StatusRoutingBillReturned
	StatusRoutingBillToRecycler
	StatusRoutingDisabledBillRejected
	StatusRoutingBillToRecyclerManualFill
	StatusRoutingManualDispense
	StatusRoutingTransferredFromRecyclerToCashbox
)

func (self *BillValidator) parsePollItem(b byte) money.PollItem {
	const tag = "mdb.bill.poll-parse"

	switch b {
	case StatusDefectiveMotor:
		return money.PollItem{HardwareCode: b, Status: money.StatusFatal, Error: ErrDefectiveMotor}
	case StatusSensorProblem:
		return money.PollItem{HardwareCode: b, Status: money.StatusFatal, Error: money.ErrSensor}
	case StatusValidatorBusy:
		return money.PollItem{HardwareCode: b, Status: money.StatusBusy}
	case StatusROMChecksumError:
		return money.PollItem{HardwareCode: b, Status: money.StatusFatal, Error: money.ErrROMChecksum}
	case StatusValidatorJammed:
		return money.PollItem{HardwareCode: b, Status: money.StatusFatal, Error: money.ErrJam}
	case StatusValidatorWasReset:
		return money.PollItem{HardwareCode: b, Status: money.StatusWasReset}
	case StatusBillRemoved:
		return money.PollItem{HardwareCode: b, Status: money.StatusError, Error: ErrBillRemoved}
	case StatusCashboxOutOfPosition:
		return money.PollItem{HardwareCode: b, Status: money.StatusFatal, Error: money.ErrNoStorage}
	case StatusValidatorDisabled:
		return money.PollItem{HardwareCode: b, Status: money.StatusDisabled}
	case StatusInvalidEscrowRequest:
		return money.PollItem{HardwareCode: b, Status: money.StatusError, Error: ErrEscrowImpossible}
	case StatusBillRejected:
		return money.PollItem{HardwareCode: b, Status: money.StatusRejected}
	case StatusCreditedBillRemoval: // fishing attempt
		return money.PollItem{HardwareCode: b, Status: money.StatusError, Error: money.ErrFraud}
	}

	if b&0x80 != 0 { // Bill Routing
		status, billType := b&0xf0, b&0x0f
		result := money.PollItem{
			DataCount:    1,
			DataNominal:  self.nominals[billType],
			HardwareCode: status,
		}
		switch status {
		case StatusRoutingBillStacked:
			self.setEscrowBill(0)
			result.Status = money.StatusCredit
		case StatusRoutingEscrowPosition:
			if self.EscrowAmount() != 0 {
				self.dev.Log.Errorf("%s b=%b CRITICAL likely code error, ESCROW POSITION with EscrowAmount not empty", tag, b)
			}
			self.setEscrowBill(result.DataNominal)
			// self.dev.Log.Debugf("bill routing ESCROW POSITION")
			result.Status = money.StatusEscrow
		case StatusRoutingBillReturned:
			if self.EscrowAmount() == 0 {
				// most likely code error, but also may be rare case of boot up
				self.dev.Log.Errorf("%s b=%b CRITICAL likely code error, BILL RETURNED with EscrowAmount empty", tag, b)
			}
			self.setEscrowBill(0)
			// self.dev.Log.Debugf("bill routing BILL RETURNED")
			// TODO make something smarter than Status:Escrow,DataCount:0
			// maybe Status:Info is enough?
			result.Status = money.StatusEscrow
			result.DataCount = 0
		case StatusRoutingBillToRecycler:
			self.setEscrowBill(0)
			// self.dev.Log.Debugf("bill routing BILL TO RECYCLER")
			result.Status = money.StatusCredit
		case StatusRoutingDisabledBillRejected:
			// TODO maybe rejected?
			// result.Status = money.StatusRejected
			result.Status = money.StatusInfo
			result.Error = fmt.Errorf("bill routing DISABLED BILL REJECTED")
		case StatusRoutingBillToRecyclerManualFill:
			result.Status = money.StatusInfo
			result.Error = fmt.Errorf("bill routing BILL TO RECYCLER – MANUAL FILL")
		case StatusRoutingManualDispense:
			result.Status = money.StatusInfo
			result.Error = fmt.Errorf("bill routing MANUAL DISPENSE")
		case StatusRoutingTransferredFromRecyclerToCashbox:
			result.Status = money.StatusInfo
			result.Error = fmt.Errorf("bill routing TRANSFERRED FROM RECYCLER TO CASHBOX")
		default:
			panic("code error")
		}
		return result
	}

	if b&0x5f == b { // Number of attempts to input a bill while validator is disabled.
		attempts := b & 0x1f
		self.dev.Log.Debugf("%s b=%b Number of attempts to input a bill while validator is disabled: %d", tag, b, attempts)
		return money.PollItem{HardwareCode: 0x40, Status: money.StatusInfo, Error: ErrAttempts, DataCount: attempts}
	}

	if b&0x2f == b { // Bill Recycler (Only)
		err := errors.NotImplementedf("%s b=%b bill recycler", tag, b)
		self.dev.Log.Errorf(err.Error())
		return money.PollItem{HardwareCode: b, Status: money.StatusError, Error: err}
	}

	err := errors.Errorf("%s CRITICAL bill unknown b=%b", tag, b)
	self.dev.Log.Errorf(err.Error())
	return money.PollItem{HardwareCode: b, Status: money.StatusFatal, Error: err}
}
