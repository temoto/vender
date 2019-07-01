package coin

import (
	"context"
	"encoding/binary"
	"sync"
	"time"

	"github.com/temoto/errors"
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
	dev             mdb.Device
	dispenseSmart   bool
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

	doSetup      engine.Doer
	DoTubeStatus engine.Doer
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

func (self *CoinAcceptor) Init(ctx context.Context) error {
	const tag = "mdb.coin.Init"

	g := state.GetGlobal(ctx)
	m, err := g.Mdber()
	if err != nil {
		return errors.Annotate(err, tag)
	}
	self.dev.Init(m.Tx, g.Log, 0x08, "coin", binary.BigEndian)
	config := g.Config().Hardware.Mdb.Coin
	self.dispenseSmart = config.DispenseSmart
	self.dispenseTimeout = helpers.IntSecondDefault(config.DispenseTimeoutSec, defaultDispenseTimeout)
	self.scalingFactor = 1

	self.doSetup = self.newSetuper()
	self.DoTubeStatus = self.NewTubeStatus()

	engine := state.GetGlobal(ctx).Engine
	engine.Register("mdb.coin.restart", self.Restarter())

	// TODO (Enum idea) no IO in Init(), call Restarter() outside
	err = self.newIniter().Do(ctx)
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

func (self *CoinAcceptor) Run(ctx context.Context, stopch <-chan struct{}, fun func(money.PollItem) bool) {
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
func (self *CoinAcceptor) pollFun(fun func(money.PollItem) bool) mdb.PollFunc {
	const tag = "mdb.coin.poll"

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
				self.dev.Log.Infof("%s/info: %s", tag, pi.String())
				// TODO telemetry
			case money.StatusError:
				self.dev.Log.Errorf("%s/error: %v", tag, pi.String())
				// TODO telemetry
			case money.StatusFatal:
				self.dev.Log.Errorf("%s/fatal: %v", tag, pi.String())
				// TODO telemetry
			case money.StatusBusy:
			case money.StatusWasReset:
				self.dev.Log.Infof("coin was reset")
				// TODO telemetry
				// TODO enable coin types
			default:
				fun(pi)
			}
		}
		return true, nil
	}
}

func (self *CoinAcceptor) newIniter() engine.Doer {
	tag := self.dev.Name + ".initer"
	return engine.NewSeq(tag).
		Append(self.doSetup).
		Append(engine.Func0{Name: tag + "/expid-diag", F: func() error {
			var err error
			// FIXME Append(IgnoreTimeout(...))
			// timeout is unfortunately common "response" for unsupported commands
			if err = self.CommandExpansionIdentification(); err != nil && !errors.IsTimeout(err) {
				return err
			}
			if err = self.CommandFeatureEnable(FeatureExtendedDiagnostic); err != nil && !errors.IsTimeout(err) {
				return err
			}
			diagResult := new(DiagResult)
			if err = self.CommandExpansionSendDiagStatus(diagResult); err != nil && !errors.IsTimeout(err) {
				return err
			}
			return nil
		}}).
		Append(self.NewTubeStatus())
}

func (self *CoinAcceptor) Restarter() engine.Doer {
	return engine.NewSeq(self.dev.Name + ".restarter").
		Append(self.dev.DoReset).
		Append(self.newIniter())
}

func (self *CoinAcceptor) newSetuper() engine.Doer {
	const tag = "mdb.coin.setuper"
	return engine.Func{Name: tag, F: func(ctx context.Context) error {
		const expectLengthMin = 7
		err := self.dev.DoSetup(ctx)
		if err != nil {
			return errors.Annotate(err, tag)
		}
		bs := self.dev.SetupResponse.Bytes()
		if len(bs) < expectLengthMin {
			return errors.Errorf("%s response=%s expected >= %d bytes",
				tag, self.dev.SetupResponse.Format(), expectLengthMin)
		}
		self.featureLevel = bs[0]
		self.scalingFactor = bs[3]
		self.typeRouting = self.dev.ByteOrder.Uint16(bs[5 : 5+2])
		for i, sf := range bs[7 : 7+TypeCount] {
			if sf == 0 {
				continue
			}
			n := currency.Nominal(sf) * currency.Nominal(self.scalingFactor)
			self.dev.Log.Debugf("%s [%d] sf=%d nominal=(%d)%s",
				tag, i, sf, n, currency.Amount(n).FormatCtx(ctx))
			self.nominals[i] = n
		}
		self.tubes.SetValid(self.nominals[:])

		self.dev.Log.Debugf("%s Changer Feature Level: %d", tag, self.featureLevel)
		self.dev.Log.Debugf("%s Country / Currency Code: %x", tag, bs[1:1+2])
		self.dev.Log.Debugf("%s Coin Scaling Factor: %d", tag, self.scalingFactor)
		// self.dev.Log.Debugf("%s Decimal Places: %d", tag, bs[4])
		self.dev.Log.Debugf("%s Coin Type Routing: %b", tag, self.typeRouting)
		self.dev.Log.Debugf("%s Coin Type Credit: %x %v", tag, bs[7:], self.nominals)
		return nil
	}}
}

func (self *CoinAcceptor) NewTubeStatus() engine.Doer {
	const tag = "mdb.coin.tubestatus"
	return engine.Func{Name: tag, F: func(ctx context.Context) error {
		const expectLengthMin = 2

		r := self.dev.Tx(packetTubeStatus)
		if r.E != nil {
			return errors.Annotate(r.E, tag)
		}
		self.dev.Log.Debugf("%s response=(%d)%s", tag, r.P.Len(), r.P.Format())
		bs := r.P.Bytes()
		if len(bs) < expectLengthMin {
			return errors.Errorf("%s response=%s expected >= %d bytes",
				tag, r.P.Format(), expectLengthMin)
		}
		fulls := self.dev.ByteOrder.Uint16(bs[0:2])
		counts := bs[2:18]
		self.dev.Log.Debugf("%s fulls=%b counts=%v", tag, fulls, counts)

		self.tubesmu.Lock()
		defer self.tubesmu.Unlock()

		self.tubes.Clear()
		for i := uint8(0); i < TypeCount; i++ {
			full := (fulls & (1 << i)) != 0
			if full && counts[i] == 0 {
				// TODO telemetry
				self.dev.Log.Errorf("%s tube=%d problem (jam/sensor/etc)", tag, i+1)
			} else if counts[i] != 0 {
				nominal := self.coinTypeNominal(i)
				if err := self.tubes.Add(nominal, uint(counts[i])); err != nil {
					return err
				}
			}
		}
		self.dev.Log.Debugf("%s tubes=%s", tag, self.tubes.String())
		return nil
	}}
}
func (self *CoinAcceptor) Tubes() *currency.NominalGroup {
	self.tubesmu.Lock()
	result := self.tubes.Copy()
	self.tubesmu.Unlock()
	return result
}

func (self *CoinAcceptor) NewCoinType(accept, dispense uint16) engine.Doer {
	buf := [5]byte{0x0c}
	self.dev.ByteOrder.PutUint16(buf[1:], accept)
	self.dev.ByteOrder.PutUint16(buf[3:], dispense)
	request := mdb.MustPacketFromBytes(buf[:], true)
	return engine.Func0{Name: "mdb.coin.CoinType", F: func() error {
		return self.dev.Tx(request).E
	}}
}

func (self *CoinAcceptor) CommandExpansionIdentification() error {
	const tag = "mdb.coin.ExpId"
	const expectLength = 33
	request := packetExpIdent
	r := self.dev.Tx(request)
	if r.E != nil {
		return errors.Annotate(r.E, tag)
	}
	self.dev.Log.Debugf("%s response=(%d)%s", tag, r.P.Len(), r.P.Format())
	bs := r.P.Bytes()
	if len(bs) < expectLength {
		return errors.Errorf("%s response=%s expected %d bytes", tag, r.P.Format(), expectLength)
	}
	self.supportedFeatures = Features(self.dev.ByteOrder.Uint32(bs[29 : 29+4]))
	self.dev.Log.Debugf("%s Manufacturer Code: %x", tag, bs[0:0+3])
	self.dev.Log.Debugf("%s Serial Number: '%s'", tag, string(bs[3:3+12]))
	self.dev.Log.Debugf("%s Model #/Tuning Revision: '%s'", tag, string(bs[15:15+12]))
	self.dev.Log.Debugf("%s Software Version: %x", tag, bs[27:27+2])
	self.dev.Log.Debugf("%s Optional Features: %b", tag, self.supportedFeatures)
	return nil
}

// CommandExpansionSendDiagStatus returns:
// - `nil` if command is not supported by device, result is not modified
// - otherwise returns nil or MDB/parse error, result set to valid DiagResult
func (self *CoinAcceptor) CommandExpansionSendDiagStatus(result *DiagResult) error {
	const tag = "mdb.coin.ExpansionSendDiagStatus"

	if self.supportedFeatures&FeatureExtendedDiagnostic == 0 {
		self.dev.Log.Debugf("%s feature is not supported", tag)
		return nil
	}
	r := self.dev.Tx(packetDiagStatus)
	if r.E != nil {
		return errors.Annotate(r.E, tag)
	}
	dr, err := parseDiagResult(r.P.Bytes(), self.dev.ByteOrder)
	self.dev.Log.Debugf("%s result=%s", tag, dr.Error())
	if result != nil {
		*result = dr
	}
	return errors.Annotate(err, tag)
}

func (self *CoinAcceptor) CommandFeatureEnable(requested Features) error {
	f := requested & self.supportedFeatures
	buf := [6]byte{0x0f, 0x01}
	self.dev.ByteOrder.PutUint32(buf[2:], uint32(f))
	request := mdb.MustPacketFromBytes(buf[:], true)
	err := self.dev.Tx(request).E
	return errors.Annotate(err, "mdb.coin.FeatureEnable")
}

func (self *CoinAcceptor) coinTypeNominal(b byte) currency.Nominal {
	if b >= TypeCount {
		self.dev.Log.Errorf("invalid coin type: %d", b)
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
		self.dev.Log.Debugf("Number of slugs: %d", slugs)
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
		self.dev.Log.Debugf("deposited coinType=%d routing=%s pi=%s", coinType, routing.String(), pi.String())
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
