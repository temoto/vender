// Package bill incapsulates work with bill validators.
package bill

import (
	"context"
	"encoding/binary"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/juju/errors"
	"github.com/temoto/alive/v2"
	"github.com/temoto/vender/currency"
	"github.com/temoto/vender/hardware/mdb"
	"github.com/temoto/vender/hardware/money"
	"github.com/temoto/vender/internal/engine"
	"github.com/temoto/vender/internal/state"
	// "github.com/temoto/vender/helpers"
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

type BillValidator struct { //nolint:maligned
	mdb.Device
	pollmu        sync.Mutex // isolate active/idle polling
	configScaling uint16

	DoEscrowAccept engine.Func
	DoEscrowReject engine.Func
	DoStacker      engine.Func

	// parsed from SETUP
	featureLevel      uint8
	supportedFeatures Features
	escrowSupported   bool
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

func (self *BillValidator) init(ctx context.Context) error {
	const tag = deviceName + ".init"
	g := state.GetGlobal(ctx)
	mdbus, err := g.Mdb()
	if err != nil {
		return errors.Annotate(err, tag)
	}
	self.Device.Init(mdbus, 0x30, "bill", binary.BigEndian)
	config := g.Config.Hardware.Mdb.Bill
	self.configScaling = 100
	if config.ScalingFactor != 0 {
		self.configScaling = uint16(config.ScalingFactor)
	}

	self.DoEscrowAccept = self.newEscrow(true)
	self.DoEscrowReject = self.newEscrow(false)
	self.DoStacker = self.newStacker()
	g.Engine.Register(self.DoEscrowAccept.Name, self.DoEscrowAccept)
	g.Engine.Register(self.DoEscrowReject.Name, self.DoEscrowReject)

	self.Device.DoInit = self.newIniter()

	// TODO remove IO from Init()
	if err = g.Engine.Exec(ctx, self.Device.DoInit); err != nil {
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
				escrowBitset |= 1 << uint(i)
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

func (self *BillValidator) Run(ctx context.Context, alive *alive.Alive, fun func(money.PollItem) bool) {
	var stopch <-chan struct{}
	if alive != nil {
		defer alive.Done()
		stopch = alive.StopChan()
	}
	pd := mdb.PollDelay{}
	parse := self.pollFun(fun)
	var active bool
	var err error
	again := true
	for again {
		response := mdb.Packet{}
		self.pollmu.Lock()
		err = self.Device.TxKnown(self.Device.PacketPoll, &response)
		self.pollmu.Unlock()
		if err == nil {
			active, err = parse(response)
		}
		again = (alive != nil) && (alive.IsRunning()) && pd.Delay(&self.Device, active, err != nil, stopch)
		// self.Log.Debugf("bill.Run r.E=%v perr=%v pactive=%t alive_not_nil=%t alive_running=%t -> again=%t",
		// 	r.E, err, active, alive != nil, (alive != nil) && alive.IsRunning(), again)
		// TODO try pollmu.Unlock() here
	}
}
func (self *BillValidator) pollFun(fun func(money.PollItem) bool) mdb.PollRequestFunc {
	const tag = deviceName + ".poll"

	return func(p mdb.Packet) (bool, error) {
		bs := p.Bytes()
		if len(bs) == 0 {
			return false, nil
		}
		for _, b := range bs {
			pi := self.parsePollItem(b)

			switch pi.Status {
			case money.StatusInfo:
				self.Log.Infof("%s/info: %s", tag, pi.String())
				// TODO telemetry
			case money.StatusError:
				self.Device.TeleError(errors.Annotate(pi.Error, tag))
			case money.StatusFatal:
				self.Device.TeleError(errors.Annotate(pi.Error, tag))
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
	const tag = deviceName + ".init"
	return engine.NewSeq(tag).
		Append(self.Device.DoReset).
		Append(engine.Func{Name: tag + "/poll", F: func(ctx context.Context) error {
			self.Run(ctx, nil, func(money.PollItem) bool { return false })
			// POLL until it settles on empty response
			return nil
		}}).
		Append(engine.Func{Name: tag + "/setup", F: self.CommandSetup}).
		Append(engine.Func0{Name: tag + "/expid", F: func() error {
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
		Append(self.DoStacker).
		Append(engine.Sleep{Duration: self.Device.DelayNext})
}

func (self *BillValidator) NewBillType(accept, escrow uint16) engine.Doer {
	buf := [5]byte{0x34}
	self.Device.ByteOrder.PutUint16(buf[1:], accept)
	self.Device.ByteOrder.PutUint16(buf[3:], escrow)
	request := mdb.MustPacketFromBytes(buf[:], true)
	return engine.Func0{Name: deviceName + ".BillType", F: func() error {
		return self.Device.TxKnown(request, nil)
	}}
}

func (self *BillValidator) setEscrowBill(n currency.Nominal) {
	atomic.StoreUint32((*uint32)(&self.escrowBill), uint32(n))
}
func (self *BillValidator) EscrowAmount() currency.Amount {
	u32 := atomic.LoadUint32((*uint32)(&self.escrowBill))
	return currency.Amount(u32)
}

func (self *BillValidator) CommandSetup(ctx context.Context) error {
	const expectLength = 27
	var billFactors [TypeCount]uint8

	if err := self.Device.TxSetup(); err != nil {
		return errors.Trace(err)
	}
	bs := self.Device.SetupResponse.Bytes()
	if len(bs) < expectLength {
		return fmt.Errorf("bill validator SETUP response=%s expected %d bytes", self.Device.SetupResponse.Format(), expectLength)
	}

	self.featureLevel = bs[0]
	currencyCode := bs[1:3]
	scalingFactor := self.Device.ByteOrder.Uint16(bs[3:5])
	decimalPlaces := bs[5]
	scalingFinal := currency.Nominal(scalingFactor) * currency.Nominal(self.configScaling)
	for i := decimalPlaces; i > 0 && scalingFinal > 10; i-- {
		scalingFinal /= 10
	}
	stackerCap := self.Device.ByteOrder.Uint16(bs[6:8])
	billSecurityLevels := self.Device.ByteOrder.Uint16(bs[8:10])
	self.escrowSupported = bs[10] == 0xff

	self.Log.Debugf("Bill Type Scaling Factors: %3v", bs[11:])
	for i, sf := range bs[11:] {
		if i >= TypeCount {
			self.Log.Errorf("CRITICAL bill SETUP type factors count=%d > expected=%d", len(bs[11:]), TypeCount)
			break
		}
		billFactors[i] = sf
		self.nominals[i] = currency.Nominal(sf) * scalingFinal
	}
	self.Log.Debugf("Bill Type calc. nominals:  %3v", self.nominals)

	self.Log.Debugf("Bill Validator Feature Level: %d", self.featureLevel)
	self.Log.Debugf("Country / Currency Code: %x", currencyCode)
	self.Log.Debugf("Bill Scaling Factor: %d Decimal Places: %d final scaling: %d", scalingFactor, decimalPlaces, scalingFinal)
	self.Log.Debugf("Stacker Capacity: %d", stackerCap)
	self.Log.Debugf("Bill Security Levels: %016b", billSecurityLevels)
	self.Log.Debugf("Escrow/No Escrow: %t", self.escrowSupported)
	self.Log.Debugf("Bill Type Credit: %x %v", bs[11:], self.nominals)
	return nil
}

func (self *BillValidator) CommandExpansionIdentification() error {
	const tag = deviceName + ".ExpId"
	const expectLength = 29
	request := packetExpIdent
	response := mdb.Packet{}
	if err := self.Device.TxMaybe(request, &response); err != nil {
		return errors.Annotate(err, tag)
	}
	bs := response.Bytes()
	self.Log.Debugf("%s response=%x", tag, bs)
	if len(bs) < expectLength {
		return fmt.Errorf("%s response=%x length=%d expected=%d", tag, bs, len(bs), expectLength)
	}
	self.Log.Infof("%s Manufacturer Code: %x", tag, bs[0:0+3])
	self.Log.Infof("%s Serial Number: '%s'", tag, string(bs[3:3+12]))
	self.Log.Infof("%s Model #/Tuning Revision: '%s'", tag, string(bs[15:15+12]))
	self.Log.Infof("%s Software Version: %x", tag, bs[27:27+2])
	return nil
}

func (self *BillValidator) CommandFeatureEnable(requested Features) error {
	f := requested & self.supportedFeatures
	buf := [6]byte{0x37, 0x01}
	self.Device.ByteOrder.PutUint32(buf[2:], uint32(f))
	request := mdb.MustPacketFromBytes(buf[:], true)
	err := self.Device.TxMaybe(request, nil)
	return errors.Annotate(err, deviceName+".FeatureEnable")
}

func (self *BillValidator) CommandExpansionIdentificationOptions() error {
	const tag = deviceName + ".ExpIdOptions"
	if self.featureLevel < 2 {
		return mdb.FeatureNotSupported(tag + " is level 2+")
	}
	const expectLength = 33
	request := packetExpIdentOptions
	response := mdb.Packet{}
	err := self.Device.TxMaybe(request, &response)
	if err != nil {
		return errors.Annotate(err, tag)
	}
	self.Log.Debugf("%s response=(%d)%s", tag, response.Len(), response.Format())
	bs := response.Bytes()
	if len(bs) < expectLength {
		return fmt.Errorf("%s response=%s expected %d bytes", tag, response.Format(), expectLength)
	}
	self.supportedFeatures = Features(self.Device.ByteOrder.Uint32(bs[29 : 29+4]))
	self.Log.Infof("%s Manufacturer Code: %x", tag, bs[0:0+3])
	self.Log.Infof("%s Serial Number: '%s'", tag, string(bs[3:3+12]))
	self.Log.Infof("%s Model #/Tuning Revision: '%s'", tag, string(bs[15:15+12]))
	self.Log.Infof("%s Software Version: %x", tag, bs[27:27+2])
	self.Log.Infof("%s Optional Features: %b", tag, self.supportedFeatures)
	return nil
}

func (self *BillValidator) newEscrow(accept bool) engine.Func {
	var tag string
	var request mdb.Packet
	if accept {
		tag = deviceName + ".escrow-accept"
		request = packetEscrowAccept
	} else {
		tag = deviceName + ".escrow-reject"
		request = packetEscrowReject
	}

	// FIXME passive poll loop (`Run`) will wrongly consume response to this
	// TODO find a good way to isolate this code from `Run` loop
	// - silly `Mutex` will do the job
	// - serializing via channel on mdb.Device would be better

	return engine.Func{Name: tag, F: func(ctx context.Context) error {
		self.pollmu.Lock()
		defer self.pollmu.Unlock()

		if err := self.Device.TxKnown(request, nil); err != nil {
			return err
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
				if self.escrowBill != 0 {
					return false
				}
				self.Log.Errorf("CRITICAL likely code error: escrow request while disabled")
				result = ErrEscrowImpossible
				return true
			case StatusInvalidEscrowRequest:
				self.Log.Errorf("CRITICAL likely code error: escrow request invalid")
				result = ErrEscrowImpossible
				return true
			case StatusRoutingBillStacked, StatusRoutingBillReturned, StatusRoutingBillToRecycler:
				self.Log.Infof("escrow result code=%02x", code) // TODO string
				return true
			default:
				return false
			}
		})
		d := self.Device.NewPollLoop(tag, self.Device.PacketPoll, DefaultEscrowTimeout, fun)
		if err := engine.GetGlobal(ctx).Exec(ctx, d); err != nil {
			return err
		}
		return result
	}}
}

func (self *BillValidator) newStacker() engine.Func {
	const tag = deviceName + ".stacker"

	return engine.Func{Name: tag, F: func(ctx context.Context) error {
		request := packetStacker
		response := mdb.Packet{}
		err := self.Device.TxKnown(request, &response)
		if err != nil {
			return errors.Annotate(err, tag)
		}
		rb := response.Bytes()
		if len(rb) != 2 {
			return errors.Errorf("%s response length=%d expected=2", tag, len(rb))
		}
		x := self.Device.ByteOrder.Uint16(rb)
		self.stackerFull = (x & 0x8000) != 0
		self.stackerCount = uint32(x & 0x7fff)
		self.Log.Debugf("%s full=%t count=%d", tag, self.stackerFull, self.stackerCount)
		return nil
	}}
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
	const tag = deviceName + ".poll-parse"

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
			fmt.Printf("\n\033[41m StatusRoutingBillStackeddddd \033[0m\n\n")
			self.setEscrowBill(0)
			result.DataCashbox = true
			result.Status = money.StatusCredit
		case StatusRoutingEscrowPosition:
			fmt.Printf("\n\033[41m  StatusRoutingEscrowPositionnnnn \033[0m\n\n")
			if self.EscrowAmount() != 0 {
				self.Log.Errorf("%s b=%b CRITICAL likely code error, ESCROW POSITION with EscrowAmount not empty", tag, b)
			}
			dn := result.DataNominal
			fmt.Printf("\n\033[41m (%v) \033[0m\n\n",dn)
			self.setEscrowBill(dn)
			// self.Log.Debugf("bill routing ESCROW POSITION")
			result.Status = money.StatusEscrow
			result.DataCount = 1
		case StatusRoutingBillReturned:
			fmt.Printf("\n\033[41m StatusRoutingBillReturnedddd \033[0m\n\n")
			if self.EscrowAmount() == 0 {
				// most likely code error, but also may be rare case of boot up
				self.Log.Errorf("%s b=%b CRITICAL likely code error, BILL RETURNED with EscrowAmount empty", tag, b)
			}
			self.setEscrowBill(0)
			// self.Log.Debugf("bill routing BILL RETURNED")
			// TODO make something smarter than Status:Escrow,DataCount:0
			// maybe Status:Info is enough?
			result.Status = money.StatusEscrow
			result.DataCount = 0
		case StatusRoutingBillToRecycler:
			fmt.Printf("\n\033[41m StatusRoutingBillToRecyclerrrr \033[0m\n\n")
			self.setEscrowBill(0)
			// self.Log.Debugf("bill routing BILL TO RECYCLER")
			result.Status = money.StatusCredit
		case StatusRoutingDisabledBillRejected:
			fmt.Printf("\n\033[41m StatusRoutingDisabledBillRejectedddd \033[0m\n\n")
			// TODO maybe rejected?
			// result.Status = money.StatusRejected
			result.Status = money.StatusInfo
			result.Error = fmt.Errorf("bill routing DISABLED BILL REJECTED")
		case StatusRoutingBillToRecyclerManualFill:
			fmt.Printf("\n\033[41m StatusRoutingBillToRecyclerManualFilllll \033[0m\n\n")
			result.Status = money.StatusInfo
			result.Error = fmt.Errorf("bill routing BILL TO RECYCLER – MANUAL FILL")
		case StatusRoutingManualDispense:
			fmt.Printf("\n\033[41m StatusRoutingManualDispenseeee \033[0m\n\n")
			result.Status = money.StatusInfo
			result.Error = fmt.Errorf("bill routing MANUAL DISPENSE")
		case StatusRoutingTransferredFromRecyclerToCashbox:
			fmt.Printf("\n\033[41m StatusRoutingTransferredFromRecyclerToCashboxxxx \033[0m\n\n")
			result.Status = money.StatusInfo
			result.Error = fmt.Errorf("bill routing TRANSFERRED FROM RECYCLER TO CASHBOX")
		default:
			panic("code error")
		}
		return result
	}

	if b&0x5f == b { // Number of attempts to input a bill while validator is disabled.
		attempts := b & 0x1f
		self.Log.Debugf("%s b=%b Number of attempts to input a bill while validator is disabled: %d", tag, b, attempts)
		return money.PollItem{HardwareCode: 0x40, Status: money.StatusInfo, Error: ErrAttempts, DataCount: attempts}
	}

	if b&0x2f == b { // Bill Recycler (Only)
		err := errors.NotImplementedf("%s b=%b bill recycler", tag, b)
		self.Log.Errorf(err.Error())
		return money.PollItem{HardwareCode: b, Status: money.StatusError, Error: err}
	}

	err := errors.Errorf("%s CRITICAL bill unknown b=%b", tag, b)
	self.Log.Errorf(err.Error())
	return money.PollItem{HardwareCode: b, Status: money.StatusFatal, Error: err}
}
