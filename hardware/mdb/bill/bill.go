// Package bill incapsulates work with bill validators.
package bill

import (
	"context"
	"encoding/binary"
	"fmt"

	"github.com/juju/errors"
	"github.com/temoto/alive"
	"github.com/temoto/vender/currency"
	"github.com/temoto/vender/engine"
	"github.com/temoto/vender/hardware/mdb"
	"github.com/temoto/vender/hardware/money"
	"github.com/temoto/vender/head/state"
	"github.com/temoto/vender/helpers/msync"
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

	DoIniter      engine.Doer
	DoConfigBills engine.Doer
}

var (
	packetEscrowAccept    = mdb.MustPacketFromHex("3501", true)
	packetEscrowReject    = mdb.MustPacketFromHex("3500", true)
	packetStacker         = mdb.MustPacketFromHex("36", true)
	packetExpIdent        = mdb.MustPacketFromHex("3700", true)
	packetExpIdentOptions = mdb.MustPacketFromHex("3702", true)
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
		self.Dev.Log.Errorf("mdb request=%s err=%v", request.Format(), r.E)
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
	self.Dev.Log.Debugf(self.String())
	return nil
}
func (self *CommandStacker) String() string {
	return fmt.Sprintf("STACKER done=%t full=%t count=%d", self.done, self.full, self.count)
}

func (self *BillValidator) Init(ctx context.Context) error {
	// TODO read config
	self.dev.Init(ctx, 0x30, "billvalidator", binary.BigEndian)

	// warning: hidden dependencies in order of following calls
	self.DoConfigBills = self.newConfigBills()
	self.DoIniter = self.newIniter()

	err := self.DoIniter.Do(ctx)
	return err
}

func (self *BillValidator) Run(ctx context.Context, a *alive.Alive, fun func(money.PollItem)) {
	self.dev.PollLoopPassive(ctx, a, self.newPoller(fun))
}

func (self *BillValidator) newPoller(fun func(money.PollItem)) mdb.PollParseFunc {
	return func(r mdb.PacketError) bool {
		if r.E != nil {
			return true
		}

		bs := r.P.Bytes()
		if len(bs) == 0 {
			return false
		}
		for _, b := range bs {
			pi := self.parsePollItem(b)
			fun(pi)
		}
		return true
	}
}

func (self *BillValidator) newIniter() engine.Doer {
	tx := engine.NewTransaction(self.dev.Name + ".initer")
	tx.Root.
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
		Append(&CommandStacker{Dev: &self.dev}).
		Append(engine.Sleep{Duration: self.dev.DelayNext}).
		Append(self.DoConfigBills)
	return tx
}

func (self *BillValidator) newConfigBills() engine.Doer {
	return engine.Func{
		Name: self.dev.Name + ".configbills",
		F: func(ctx context.Context) error {
			config := state.GetConfig(ctx)
			// TODO read enabled nominals from config
			_ = config
			return self.CommandBillType(0xffff, 0)
		},
	}
}

func (self *BillValidator) NewRestarter() engine.Doer {
	tx := engine.NewTransaction(self.dev.Name + ".restarter")
	tx.Root.
		Append(self.dev.NewDoReset()).
		Append(engine.Sleep{Duration: self.dev.DelayNext}).
		Append(self.newIniter())
	return tx
}

func (self *BillValidator) CommandBillType(accept, escrow uint16) error {
	buf := [5]byte{0x34}
	self.dev.ByteOrder.PutUint16(buf[1:], accept)
	self.dev.ByteOrder.PutUint16(buf[3:], escrow)
	request := mdb.MustPacketFromBytes(buf[:], true)
	err := self.dev.Tx(request).E
	self.dev.Log.Debugf("CommandBillType request=%s err=%v", request.Format(), err)
	return err
}

func (self *BillValidator) CommandSetup(ctx context.Context) error {
	const expectLength = 27
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
	self.scalingFactor = self.dev.ByteOrder.Uint16(bs[3:5])
	stackerCap := self.dev.ByteOrder.Uint16(bs[6:8])
	billSecurityLevels := self.dev.ByteOrder.Uint16(bs[8:10])
	self.escrowCap = bs[10] == 0xff

	self.dev.Log.Debugf("Bill Type Scaling Factors: %3v", bs[11:])
	for i, sf := range bs[11:] {
		if i >= TypeCount {
			self.dev.Log.Debugf("ERROR bill SETUP type factors count=%d more than expected=%d", len(bs[11:]), TypeCount)
			break
		}
		self.billFactors[i] = sf
		self.billNominals[i] = currency.Nominal(sf) * currency.Nominal(self.scalingFactor)
	}
	self.dev.Log.Debugf("Bill Type calc. nominals:  %3v", self.billNominals)

	self.dev.Log.Debugf("Bill Validator Feature Level: %d", self.featureLevel)
	self.dev.Log.Debugf("Country / Currency Code: %x", currencyCode)
	self.dev.Log.Debugf("Bill Scaling Factor: %d", self.scalingFactor)
	self.dev.Log.Debugf("Decimal Places: %d", bs[5])
	self.dev.Log.Debugf("Stacker Capacity: %d", stackerCap)
	self.dev.Log.Debugf("Bill Security Levels: %016b", billSecurityLevels)
	self.dev.Log.Debugf("Escrow/No Escrow: %t", self.escrowCap)
	self.dev.Log.Debugf("Bill Type Credit: %x %#v", bs[11:], self.billNominals)
	return nil
}

func (self *BillValidator) CommandExpansionIdentification() error {
	const expectLength = 29
	request := packetExpIdent
	r := self.dev.Tx(request)
	if r.E != nil {
		self.dev.Log.Errorf("mdb request=%s err=%v", request.Format(), r.E)
		return r.E
	}
	self.dev.Log.Debugf("EXPANSION IDENTIFICATION response=(%d)%s", r.P.Len(), r.P.Format())
	bs := r.P.Bytes()
	if len(bs) < expectLength {
		return fmt.Errorf("hardware/mdb/bill EXPANSION IDENTIFICATION response=%s expected %d bytes", r.P.Format(), expectLength)
	}
	self.dev.Log.Debugf("Manufacturer Code: %x", bs[0:0+3])
	self.dev.Log.Debugf("Serial Number: '%s'", string(bs[3:3+12]))
	self.dev.Log.Debugf("Model #/Tuning Revision: '%s'", string(bs[15:15+12]))
	self.dev.Log.Debugf("Software Version: %x", bs[27:27+2])
	return nil
}

func (self *BillValidator) CommandFeatureEnable(requested Features) error {
	f := requested & self.supportedFeatures
	buf := [6]byte{0x37, 0x01}
	self.dev.ByteOrder.PutUint32(buf[2:], uint32(f))
	request := mdb.MustPacketFromBytes(buf[:], true)
	err := self.dev.Tx(request).E
	if err != nil {
		self.dev.Log.Errorf("mdb request=%s err=%v", request.Format(), err)
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
		self.dev.Log.Errorf("mdb request=%s err=%v", request.Format(), r.E)
		return r.E
	}
	self.dev.Log.Debugf("EXPANSION IDENTIFICATION WITH OPTION BITS response=(%d)%s", r.P.Len(), r.P.Format())
	bs := r.P.Bytes()
	if len(bs) < expectLength {
		return fmt.Errorf("hardware/mdb/bill EXPANSION IDENTIFICATION WITH OPTION BITS response=%s expected %d bytes", r.P.Format(), expectLength)
	}
	self.supportedFeatures = Features(self.dev.ByteOrder.Uint32(bs[29 : 29+4]))
	self.dev.Log.Debugf("Manufacturer Code: %x", bs[0:0+3])
	self.dev.Log.Debugf("Serial Number: '%s'", string(bs[3:3+12]))
	self.dev.Log.Debugf("Model #/Tuning Revision: '%s'", string(bs[15:15+12]))
	self.dev.Log.Debugf("Software Version: %x", bs[27:27+2])
	self.dev.Log.Debugf("Optional Features: %b", self.supportedFeatures)
	return nil
}

func (self *BillValidator) billNominal(b byte) currency.Nominal {
	if b >= TypeCount {
		self.dev.Log.Errorf("invalid bill type: %d", b)
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
		self.dev.Log.Debugf("bill escrow TODO packetEscrowAccept")
		return money.PollItem{Status: money.StatusEscrow, DataNominal: amount}
	}
	if b&0x5f == b { // Number of attempts to input a bill while validator is disabled.
		attempts := b & 0x1f
		self.dev.Log.Debugf("Number of attempts to input a bill while validator is disabled: %d", attempts)
		return money.PollItem{Status: money.StatusInfo, Error: ErrAttempts, DataCount: attempts}
	}

	err := errors.Errorf("parsePollItem unknown=%x", b)
	self.dev.Log.Errorf(err.Error())
	return money.PollItem{Status: money.StatusFatal, Error: err}
}
