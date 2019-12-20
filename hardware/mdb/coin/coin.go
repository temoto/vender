package coin

import (
	"context"
	"encoding/binary"
	"fmt"
	"sync"
	"time"

	"github.com/juju/errors"
	"github.com/temoto/alive"
	"github.com/temoto/vender/currency"
	"github.com/temoto/vender/engine"
	"github.com/temoto/vender/hardware/mdb"
	"github.com/temoto/vender/hardware/money"
	"github.com/temoto/vender/helpers"
	"github.com/temoto/vender/state"
)

const (
	TypeCount              = 16
	defaultDispenseTimeout = 5 * time.Second
)

//go:generate stringer -type=CoinRouting -trimprefix=Routing
type CoinRouting uint8

const (
	RoutingCashBox CoinRouting = 0
	RoutingTubes   CoinRouting = 1
	RoutingNotUsed CoinRouting = 2
	RoutingReject  CoinRouting = 3
)

//go:generate stringer -type=Features -trimprefix=Feature
type Features uint32

const (
	FeatureAlternativePayout Features = 1 << iota
	FeatureExtendedDiagnostic
	FeatureControlledManualFillPayout
	FeatureFTL
)

type CoinAcceptor struct { //nolint:maligned
	mdb.Device
	giveSmart       bool
	dispenseTimeout time.Duration
	pollmu          sync.Mutex // isolate active/idle polling

	// parsed from SETUP
	featureLevel      uint8
	supportedFeatures Features
	nominals          [TypeCount]currency.Nominal // final values, including all scaling factors
	scalingFactor     uint8
	typeRouting       uint16

	// dynamic state useful for external code
	tubesmu sync.Mutex
	tubes   currency.NominalGroup
}

var (
	packetTubeStatus   = mdb.MustPacketFromHex("0a", true)
	packetExpIdent     = mdb.MustPacketFromHex("0f00", true)
	packetDiagStatus   = mdb.MustPacketFromHex("0f05", true)
	packetPayoutPoll   = mdb.MustPacketFromHex("0f04", true)
	packetPayoutStatus = mdb.MustPacketFromHex("0f03", true)
)

var (
	ErrNoCredit      = errors.Errorf("No Credit")
	ErrDoubleArrival = errors.Errorf("Double Arrival")
	ErrCoinRouting   = errors.Errorf("Coin Routing")
	ErrCoinJam       = errors.Errorf("Coin Jam")
	ErrSlugs         = errors.Errorf("Slugs")
)

func (self *CoinAcceptor) init(ctx context.Context) error {
	const tag = deviceName + ".init"

	g := state.GetGlobal(ctx)
	mdbus, err := g.Mdb()
	if err != nil {
		return errors.Annotate(err, tag)
	}
	self.Device.Init(mdbus, 0x08, "coin", binary.BigEndian)
	config := &g.Config.Hardware.Mdb.Coin
	self.giveSmart = config.GiveSmart || config.XXX_Deprecated_DispenseSmart
	self.dispenseTimeout = helpers.IntSecondDefault(config.DispenseTimeoutSec, defaultDispenseTimeout)
	self.scalingFactor = 1

	self.Device.DoInit = self.newIniter()

	// engine := state.GetGlobal(ctx).Engine
	// TODO register payout,etc

	// TODO (Enum idea) no IO in Init()
	err = self.Device.DoInit.Do(ctx)
	return errors.Annotate(err, tag)
}

func (self *CoinAcceptor) AcceptMax(max currency.Amount) engine.Doer {
	// config := state.GetConfig(ctx)
	enableBitset := uint16(0)

	if max != 0 {
		for i, n := range self.nominals {
			if n == 0 {
				continue
			}
			if currency.Amount(n) <= max {
				// TODO consult config
				// _ = config
				enableBitset |= 1 << uint(i)
			}
		}
	}

	return self.NewCoinType(enableBitset, 0xffff)
}

func (self *CoinAcceptor) SupportedNominals() []currency.Nominal {
	ns := make([]currency.Nominal, 0, TypeCount)
	for _, n := range self.nominals {
		if n != 0 {
			ns = append(ns, n)
		}
	}
	return ns
}

func (self *CoinAcceptor) Run(ctx context.Context, alive *alive.Alive, fun func(money.PollItem) bool) {
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
		if err == nil {
			active, err = parse(response)
		}
		self.pollmu.Unlock()

		again = (alive != nil) && (alive.IsRunning()) && pd.Delay(&self.Device, active, err != nil, stopch)
		// TODO try pollmu.Unlock() here
	}
}
func (self *CoinAcceptor) pollFun(fun func(money.PollItem) bool) mdb.PollRequestFunc {
	const tag = deviceName + ".poll"

	return func(p mdb.Packet) (bool, error) {
		bs := p.Bytes()
		if len(bs) == 0 {
			return false, nil
		}

		var pi money.PollItem
		skip := false
		for i, b := range bs {
			if skip {
				skip = false
				continue
			}
			b2 := byte(0)
			if i+1 < len(bs) {
				b2 = bs[i+1]
			}
			pi, skip = self.parsePollItem(b, b2)
			switch pi.Status {
			case money.StatusInfo:
				self.Device.Log.Infof("%s/info: %s", tag, pi.String())
				// TODO telemetry
			case money.StatusError:
				self.Device.TeleError(errors.Annotate(pi.Error, tag))
			case money.StatusFatal:
				self.Device.TeleError(errors.Annotate(pi.Error, tag))
			case money.StatusBusy:
			case money.StatusWasReset:
				self.Device.Log.Infof("coin was reset")
				// TODO telemetry
			default:
				fun(pi)
			}
		}
		return true, nil
	}
}

func (self *CoinAcceptor) newIniter() engine.Doer {
	const tag = deviceName + ".init"
	return engine.NewSeq(tag).
		Append(self.Device.DoReset).
		Append(engine.Func{Name: tag + "/poll", F: func(ctx context.Context) error {
			self.Run(ctx, nil, func(money.PollItem) bool { return false })
			return nil
		}}).
		Append(self.newSetuper()).
		Append(engine.Func0{Name: tag + "/expid-diag", F: func() error {
			if err := self.CommandExpansionIdentification(); err != nil {
				return err
			}
			if err := self.CommandFeatureEnable(FeatureExtendedDiagnostic); err != nil {
				return err
			}
			diagResult := new(DiagResult)
			if err := self.ExpansionDiagStatus(diagResult); err != nil {
				return err
			}
			return nil
		}}).
		Append(engine.Func0{Name: tag + "/tube-status", F: self.TubeStatus})
}

func (self *CoinAcceptor) newSetuper() engine.Doer {
	const tag = deviceName + ".setup"
	return engine.Func{Name: tag, F: func(ctx context.Context) error {
		const expectLengthMin = 7
		if err := self.Device.TxSetup(); err != nil {
			return errors.Annotate(err, tag)
		}
		bs := self.Device.SetupResponse.Bytes()
		if len(bs) < expectLengthMin {
			return errors.Errorf("%s response=%s expected >= %d bytes",
				tag, self.Device.SetupResponse.Format(), expectLengthMin)
		}
		self.featureLevel = bs[0]
		self.scalingFactor = bs[3]
		self.typeRouting = self.Device.ByteOrder.Uint16(bs[5 : 5+2])
		for i, sf := range bs[7 : 7+TypeCount] {
			if sf == 0 {
				continue
			}
			n := currency.Nominal(sf) * currency.Nominal(self.scalingFactor)
			self.Device.Log.Debugf("%s [%d] sf=%d nominal=(%d)%s",
				tag, i, sf, n, currency.Amount(n).FormatCtx(ctx))
			self.nominals[i] = n
		}
		self.tubes.SetValid(self.nominals[:])

		self.Device.Log.Debugf("%s Changer Feature Level: %d", tag, self.featureLevel)
		self.Device.Log.Debugf("%s Country / Currency Code: %x", tag, bs[1:1+2])
		self.Device.Log.Debugf("%s Coin Scaling Factor: %d", tag, self.scalingFactor)
		// self.Device.Log.Debugf("%s Decimal Places: %d", tag, bs[4])
		self.Device.Log.Debugf("%s Coin Type Routing: %b", tag, self.typeRouting)
		self.Device.Log.Debugf("%s Coin Type Credit: %x %v", tag, bs[7:], self.nominals)
		return nil
	}}
}

func (self *CoinAcceptor) TubeStatus() error {
	const tag = deviceName + ".tubestatus"
	const expectLengthMin = 2

	response := mdb.Packet{}
	err := self.Device.TxKnown(packetTubeStatus, &response)
	if err != nil {
		return errors.Annotate(err, tag)
	}
	self.Device.Log.Debugf("%s response=(%d)%s", tag, response.Len(), response.Format())
	bs := response.Bytes()
	if len(bs) < expectLengthMin {
		return errors.Errorf("%s response=%s expected >= %d bytes",
			tag, response.Format(), expectLengthMin)
	}
	fulls := self.Device.ByteOrder.Uint16(bs[0:2])
	counts := bs[2:18]
	self.Device.Log.Debugf("%s fulls=%b counts=%v", tag, fulls, counts)

	self.tubesmu.Lock()
	defer self.tubesmu.Unlock()

	self.tubes.Clear()
	for i := uint8(0); i < TypeCount; i++ {
		full := (fulls & (1 << i)) != 0
		if full && counts[i] == 0 {
			self.Device.TeleError(fmt.Errorf("%s coin=%d problem (jam/sensor/etc)", tag, i+1))
		} else if counts[i] != 0 {
			nominal := self.coinTypeNominal(i)
			if err := self.tubes.Add(nominal, uint(counts[i])); err != nil {
				return err
			}
		}
	}
	self.Device.Log.Debugf("%s tubes=%s", tag, self.tubes.String())
	return nil
}
func (self *CoinAcceptor) Tubes() *currency.NominalGroup {
	self.tubesmu.Lock()
	result := self.tubes.Copy()
	self.tubesmu.Unlock()
	return result
}

func (self *CoinAcceptor) NewCoinType(accept, dispense uint16) engine.Doer {
	buf := [5]byte{0x0c}
	self.Device.ByteOrder.PutUint16(buf[1:], accept)
	self.Device.ByteOrder.PutUint16(buf[3:], dispense)
	request := mdb.MustPacketFromBytes(buf[:], true)
	return engine.Func0{Name: deviceName + ".CoinType", F: func() error {
		return self.Device.TxKnown(request, nil)
	}}
}

func (self *CoinAcceptor) CommandExpansionIdentification() error {
	const tag = deviceName + ".ExpId"
	const expectLength = 33
	request := packetExpIdent
	response := mdb.Packet{}
	err := self.Device.TxMaybe(request, &response)
	if err != nil {
		if errors.Cause(err) == mdb.ErrTimeout {
			self.Device.Log.Infof("%s request=%x not supported (timeout)", tag, request.Bytes())
			return nil
		}
		return errors.Annotate(err, tag)
	}
	self.Device.Log.Debugf("%s response=(%d)%s", tag, response.Len(), response.Format())
	bs := response.Bytes()
	if len(bs) < expectLength {
		return errors.Errorf("%s response=%s expected %d bytes", tag, response.Format(), expectLength)
	}
	self.supportedFeatures = Features(self.Device.ByteOrder.Uint32(bs[29 : 29+4]))
	self.Device.Log.Debugf("%s Manufacturer Code: %x", tag, bs[0:0+3])
	self.Device.Log.Debugf("%s Serial Number: '%s'", tag, string(bs[3:3+12]))
	self.Device.Log.Debugf("%s Model #/Tuning Revision: '%s'", tag, string(bs[15:15+12]))
	self.Device.Log.Debugf("%s Software Version: %x", tag, bs[27:27+2])
	self.Device.Log.Debugf("%s Optional Features: %b", tag, self.supportedFeatures)
	return nil
}

// CommandExpansionSendDiagStatus returns:
// - `nil` if command is not supported by device, result is not modified
// - otherwise returns nil or MDB/parse error, result set to valid DiagResult
func (self *CoinAcceptor) ExpansionDiagStatus(result *DiagResult) error {
	const tag = deviceName + ".ExpansionSendDiagStatus"

	if self.supportedFeatures&FeatureExtendedDiagnostic == 0 {
		self.Device.Log.Debugf("%s feature is not supported", tag)
		return nil
	}
	response := mdb.Packet{}
	err := self.Device.TxMaybe(packetDiagStatus, &response)
	if err != nil {
		if errors.Cause(err) == mdb.ErrTimeout {
			self.Device.Log.Infof("%s request=%x not supported (timeout)", tag, packetDiagStatus.Bytes())
			return nil
		}
		return errors.Annotate(err, tag)
	}
	dr, err := parseDiagResult(response.Bytes(), self.Device.ByteOrder)
	self.Device.Log.Debugf("%s result=%s", tag, dr.Error())
	if result != nil {
		*result = dr
	}
	return errors.Annotate(err, tag)
}

func (self *CoinAcceptor) CommandFeatureEnable(requested Features) error {
	const tag = deviceName + ".FeatureEnable"
	f := requested & self.supportedFeatures
	buf := [6]byte{0x0f, 0x01}
	self.Device.ByteOrder.PutUint32(buf[2:], uint32(f))
	request := mdb.MustPacketFromBytes(buf[:], true)
	err := self.Device.TxMaybe(request, nil)
	if errors.Cause(err) == mdb.ErrTimeout {
		self.Device.Log.Infof("%s request=%x not supported (timeout)", tag, request.Bytes())
		return nil
	}
	return errors.Annotate(err, tag)
}

func (self *CoinAcceptor) coinTypeNominal(b byte) currency.Nominal {
	if b >= TypeCount {
		self.Device.Log.Errorf("invalid coin type: %d", b)
		return 0
	}
	return self.nominals[b]
}

func (self *CoinAcceptor) nominalCoinType(nominal currency.Nominal) int8 {
	for ct, n := range self.nominals {
		if n == nominal && ((1<<uint(ct))&self.typeRouting != 0) {
			return int8(ct)
		}
	}
	return -1
}

func (self *CoinAcceptor) parsePollItem(b, b2 byte) (money.PollItem, bool) {
	switch b {
	case 0x01: // Escrow request
		return money.PollItem{Status: money.StatusReturnRequest}, false
	case 0x02: // Changer Payout Busy
		return money.PollItem{Status: money.StatusBusy}, false
	// high
	case 0x03: // No Credit
		return money.PollItem{Status: money.StatusError, Error: ErrNoCredit}, false
	// high
	case 0x04: // Defective Tube Sensor
		return money.PollItem{Status: money.StatusFatal, Error: money.ErrSensor}, false
	case 0x05: // Double Arrival
		return money.PollItem{Status: money.StatusInfo, Error: ErrDoubleArrival}, false
	// high
	case 0x06: // Acceptor Unplugged
		return money.PollItem{Status: money.StatusFatal, Error: money.ErrNoStorage}, false
	// high
	case 0x07: // Tube Jam
		return money.PollItem{Status: money.StatusFatal, Error: money.ErrJam}, false
	// high
	case 0x08: // ROM checksum error
		return money.PollItem{Status: money.StatusFatal, Error: money.ErrROMChecksum}, false
	// high
	case 0x09: // Coin Routing Error
		return money.PollItem{Status: money.StatusError, Error: ErrCoinRouting}, false
	case 0x0a: // Changer Busy
		return money.PollItem{Status: money.StatusBusy}, false
	case 0x0b: // Changer was Reset
		return money.PollItem{Status: money.StatusWasReset}, false
	// high
	case 0x0c: // Coin Jam
		return money.PollItem{Status: money.StatusFatal, Error: ErrCoinJam}, false
	case 0x0d: // Possible Credited Coin Removal
		return money.PollItem{Status: money.StatusError, Error: money.ErrFraud}, false
	}

	if b>>5 == 1 { // Slug count 001xxxxx
		slugs := b & 0x1f
		self.Device.Log.Debugf("Number of slugs: %d", slugs)
		return money.PollItem{Status: money.StatusInfo, Error: ErrSlugs, DataCount: slugs}, false
	}
	if b>>6 == 1 { // Coins Deposited
		// b=01yyxxxx b2=number of coins in tube
		// yy = coin routing
		// xxxx = coin type
		coinType := b & 0xf
		routing := CoinRouting((b >> 4) & 3)
		pi := money.PollItem{
			DataNominal: self.coinTypeNominal(coinType),
			DataCount:   1,
		}
		switch routing {
		case RoutingCashBox:
			pi.Status = money.StatusCredit
			pi.DataCashbox = true
		case RoutingTubes:
			pi.Status = money.StatusCredit
		case RoutingNotUsed:
			pi.Status = money.StatusError
			pi.Error = errors.Errorf("routing=notused b=%x pi=%s", b, pi.String())
		case RoutingReject:
			pi.Status = money.StatusRejected
		default:
			// pi.Status = money.StatusFatal
			panic(errors.Errorf("code error b=%x routing=%b", b, routing))
		}
		self.Device.Log.Debugf("deposited coinType=%d routing=%s pi=%s", coinType, routing.String(), pi.String())
		return pi, true
	}
	if b&0x80 != 0 { // Coins Dispensed Manually
		// b=1yyyxxxx b2=number of coins in tube
		// yyy = coins dispensed
		// xxxx = coin type
		count := (b >> 4) & 7
		nominal := self.coinTypeNominal(b & 0xf)
		return money.PollItem{Status: money.StatusDispensed, DataNominal: nominal, DataCount: count}, true
	}

	err := errors.Errorf("parsePollItem unknown=%x", b)
	return money.PollItem{Status: money.StatusFatal, Error: err}, false
}
