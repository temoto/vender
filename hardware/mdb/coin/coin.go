package coin

import (
	"context"
	"encoding/binary"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/temoto/alive"
	"github.com/temoto/vender/currency"
	"github.com/temoto/vender/hardware/mdb"
	"github.com/temoto/vender/hardware/money"
)

const (
	coinTypeCount = 16

	DelayErr  = 500 * time.Millisecond
	DelayNext = 200 * time.Millisecond

	RouteCashBox = 0
	RouteTubes   = 1
	RouteNotUsed = 2
	RouteReject  = 3
)

//go:generate stringer -type=Features
type Features uint32

const (
	FeatureAlternativePayout Features = 1 << iota
	FeatureExtendedDiagnostic
	FeatureControlledManualFillPayout
	FeatureFTL
)

type CoinAcceptor struct {
	mdb       mdb.Mdber
	byteOrder binary.ByteOrder

	// Indicates the value of the bill types 0 to 15.
	// These are final values including all scaling factors.
	coinTypeCredit []currency.Nominal

	featureLevel      uint8
	supportedFeatures Features

	internalScalingFactor int
	batch                 sync.Mutex
	ready                 chan struct{}
}

var (
	packetReset      = mdb.PacketFromHex("08")
	packetSetup      = mdb.PacketFromHex("09")
	packetTubeStatus = mdb.PacketFromHex("0a")
	packetPoll       = mdb.PacketFromHex("0b")
	packetExpIdent   = mdb.PacketFromHex("0f00")
)

var (
	ErrNoCredit      = fmt.Errorf("No Credit")
	ErrDoubleArrival = fmt.Errorf("Double Arrival")
	ErrCoinRouting   = fmt.Errorf("Coin Routing")
	ErrCoinJam       = fmt.Errorf("Coin Jam")
	ErrSlugs         = fmt.Errorf("Slugs")
)

// usage: defer coin.Batch()()
func (self *CoinAcceptor) Batch() func() {
	self.batch.Lock()
	return self.batch.Unlock
}

func (self *CoinAcceptor) Init(ctx context.Context, mdber mdb.Mdber) error {
	// TODO read config
	self.byteOrder = binary.BigEndian
	self.coinTypeCredit = make([]currency.Nominal, coinTypeCount)
	self.mdb = mdber
	self.internalScalingFactor = 1 // FIXME
	self.ready = make(chan struct{})
	// TODO maybe execute CommandReset?
	err := self.InitSequence()
	if err != nil {
		log.Printf("hardware/mdb/coin/InitSequence error=%s", err)
		// TODO maybe execute CommandReset?
	}
	return err
}

func (self *CoinAcceptor) SupportedNominals() []currency.Nominal {
	ns := make([]currency.Nominal, 0, len(self.coinTypeCredit))
	for _, n := range self.coinTypeCredit {
		if n > 0 {
			ns = append(ns, n)
		}
	}
	return ns
}

func (self *CoinAcceptor) Run(ctx context.Context, a *alive.Alive, ch chan<- money.PollResult) {
	stopch := a.StopChan()
	for a.IsRunning() {
		pr := self.CommandPoll()
		select {
		case ch <- pr:
		case <-stopch:
			return
		}
		select {
		case <-time.After(pr.Delay):
		case <-stopch:
			return
		}
	}
}

func (self *CoinAcceptor) InitSequence() error {
	defer self.Batch()()

	err := self.CommandSetup()
	if err != nil {
		return err
	}
	if err = self.CommandExpansionIdentification(); err != nil {
		return err
	}
	if err = self.CommandFeatureEnable(FeatureExtendedDiagnostic); err != nil {
		return err
	}
	if err = self.CommandExpansionSendDiagStatus(); err != nil {
		return err
	}
	if err = self.CommandTubeStatus(); err != nil {
		return err
	}
	// TODO read config
	if err = self.CommandCoinType(0xffff, 0xffff); err != nil {
		return err
	}
	return nil
}

func (self *CoinAcceptor) CommandReset() error {
	response := new(mdb.Packet)
	return self.mdb.Tx(packetReset, response)
}

func (self *CoinAcceptor) CommandSetup() error {
	const expectLengthMin = 7
	request := packetSetup
	response := new(mdb.Packet)
	err := self.mdb.Tx(request, response)
	if err != nil {
		log.Printf("mdb request=%s err=%v", request.Format(), err)
		return err
	}
	log.Printf("setup response=(%d)%s", response.Len(), response.Format())
	bs := response.Bytes()
	if len(bs) < expectLengthMin {
		return fmt.Errorf("hardware/mdb/coin SETUP response=%s expected >= %d bytes", response.Format(), expectLengthMin)
	}
	scalingFactor := bs[3]
	for i, sf := range bs[7:] {
		n := currency.Nominal(sf) * currency.Nominal(scalingFactor) * currency.Nominal(self.internalScalingFactor)
		log.Printf("i=%d sf=%d nominal=%s", i, sf, currency.Amount(n).Format100I())
		self.coinTypeCredit[i] = n
	}
	self.featureLevel = bs[0]
	log.Printf("Changer Feature Level: %d", self.featureLevel)
	log.Printf("Country / Currency Code: %x", bs[1:3])
	log.Printf("Coin Scaling Factor: %d", scalingFactor)
	log.Printf("Decimal Places: %d", bs[4])
	log.Printf("Coin Type Routing: %d", self.byteOrder.Uint16(bs[5:7]))
	log.Printf("Coin Type Credit: %x %#v", bs[7:], self.coinTypeCredit)
	return nil
}

func (self *CoinAcceptor) CommandPoll() (result money.PollResult) {
	defer func() {
		if result.Ready() {
			select {
			case self.ready <- struct{}{}:
			default:
			}
		}
	}()

	now := time.Now()
	response := new(mdb.Packet)
	savedebug := self.mdb.SetDebug(false)
	err := self.mdb.Tx(packetPoll, response)
	self.mdb.SetDebug(savedebug)
	result.Time = now
	result.Delay = DelayNext
	if err != nil {
		result.Error = err
		result.Delay = DelayErr
		return result
	}
	if response.Len() == 0 {
		return result
	}
	result.Items = make([]money.PollItem, 0, response.Len())
	log.Printf("poll response=%s", response.Format())
	bs := response.Bytes()
	pi := money.PollItem{}
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
		result.Items = append(result.Items, pi)
	}
	return result
}

func (self *CoinAcceptor) CommandTubeStatus() error {
	const expectLengthMin = 2
	request := packetTubeStatus
	response := new(mdb.Packet)
	err := self.mdb.Tx(request, response)
	if err != nil {
		log.Printf("mdb request=%s err=%v", request.Format(), err)
		return err
	}
	log.Printf("tubestatus response=(%d)%s", response.Len(), response.Format())
	bs := response.Bytes()
	if len(bs) < expectLengthMin {
		return fmt.Errorf("hardware/mdb/coin TUBE money.Status response=%s expected >= %d bytes", response.Format(), expectLengthMin)
	}
	full := self.byteOrder.Uint16(bs[0:2])
	counts := bs[2:18]
	log.Printf("tubestatus full=%b counts=%v", full, counts)
	// TODO use full,counts
	_ = full
	_ = counts
	return nil
}

func (self *CoinAcceptor) CommandCoinType(accept, dispense uint16) error {
	buf := [5]byte{0x0c}
	self.byteOrder.PutUint16(buf[1:], accept)
	self.byteOrder.PutUint16(buf[3:], dispense)
	request := mdb.PacketFromBytes(buf[:])
	response := new(mdb.Packet)
	err := self.mdb.Tx(request, response)
	if err != nil {
		log.Printf("mdb request=%s err=%v", request.Format(), err)
		return err
	}
	return nil
}

func (self *CoinAcceptor) CommandDispense(nominal currency.Nominal, count uint8) error {
	if count >= 16 {
		return fmt.Errorf("CommandDispense count=%d overflow >=16", count)
	}
	coinType, err := self.nominalCoinType(nominal)
	if err != nil {
		return err
	}

	response := new(mdb.Packet)
	request := mdb.PacketFromBytes([]byte{0x0d, (count << 4) + coinType})
	<-self.ready
	err = self.mdb.Tx(request, response)
	if err != nil {
		log.Printf("mdb request=%s err=%v", request.Format(), err)
		return err
	}
	return nil
}

func (self *CoinAcceptor) CommandExpansionIdentification() error {
	const expectLength = 33
	request := packetExpIdent
	response := new(mdb.Packet)
	err := self.mdb.Tx(request, response)
	if err != nil {
		log.Printf("mdb request=%s err=%v", request.Format(), err)
		return err
	}
	log.Printf("setup response=(%d)%s", response.Len(), response.Format())
	bs := response.Bytes()
	if len(bs) < expectLength {
		return fmt.Errorf("hardware/mdb/coin EXPANSION IDENTIFICATION response=%s expected %d bytes", response.Format(), expectLength)
	}
	self.supportedFeatures = Features(self.byteOrder.Uint32(bs[29:]))
	log.Printf("Supported features: %b", self.supportedFeatures)
	return nil
}

func (self *CoinAcceptor) CommandFeatureEnable(requested Features) error {
	f := requested & self.supportedFeatures
	buf := [6]byte{0x0f, 0x01}
	self.byteOrder.PutUint32(buf[2:], uint32(f))
	request := mdb.PacketFromBytes(buf[:])
	response := new(mdb.Packet)
	err := self.mdb.Tx(request, response)
	if err != nil {
		log.Printf("mdb request=%s err=%v", request.Format(), err)
		return err
	}
	return nil
}

func (self *CoinAcceptor) CommandExpansionSendDiagStatus() error {
	if self.supportedFeatures&FeatureExtendedDiagnostic == 0 {
		log.Printf("CommandExpansionSendDiagStatus feature is not supported")
		return nil
	}
	self.mdb.TxDebug(mdb.PacketFromHex("0f05"), true) // 0f05 EXPANSION SEND DIAG money.Status
	return nil
}

func (self *CoinAcceptor) coinTypeNominal(b byte) currency.Nominal {
	if b >= coinTypeCount {
		log.Printf("invalid coin type: %d", b)
		return 0
	}
	return self.coinTypeCredit[b]
}

func (self *CoinAcceptor) nominalCoinType(nominal currency.Nominal) (byte, error) {
	for ct, n := range self.coinTypeCredit {
		if n == nominal {
			return byte(ct), nil
		}
	}
	return 0, fmt.Errorf("Unknown nominal %s", currency.Amount(nominal).Format100I())
}

func (self *CoinAcceptor) parsePollItem(b, b2 byte) (money.PollItem, bool) {
	switch b {
	case 0x01: // Escrow request
		return money.PollItem{Status: money.StatusReturnRequest}, false
	case 0x02: // Changer Payout Busy
		return money.PollItem{Status: money.StatusBusy}, false
	case 0x03: // No Credit
		return money.PollItem{Status: money.StatusError, Error: ErrNoCredit}, false
	case 0x04: // Defective Tube Sensor
		return money.PollItem{Status: money.StatusFatal, Error: money.ErrSensor}, false
	case 0x05: // Double Arrival
		return money.PollItem{Status: money.StatusInfo, Error: ErrDoubleArrival}, false
	case 0x06: // Acceptor Unplugged
		return money.PollItem{Status: money.StatusFatal, Error: money.ErrNoStorage}, false
	case 0x07: // Tube Jam
		return money.PollItem{Status: money.StatusFatal, Error: money.ErrJam}, false
	case 0x08: // ROM checksum error
		return money.PollItem{Status: money.StatusFatal, Error: money.ErrROMChecksum}, false
	case 0x09: // Coin Routing Error
		return money.PollItem{Status: money.StatusError, Error: ErrCoinRouting}, false
	case 0x0a: // Changer Busy
		return money.PollItem{Status: money.StatusBusy}, false
	case 0x0b: // Changer was Reset
		return money.PollItem{Status: money.StatusWasReset}, false
	case 0x0c: // Coin Jam
		return money.PollItem{Status: money.StatusFatal, Error: ErrCoinJam}, false
	case 0x0d: // Possible Credited Coin Removal
		return money.PollItem{Status: money.StatusError, Error: money.ErrFraud}, false
	}

	if b&0x80 != 0 { // Coins Dispensed Manually
		// b=1yyyxxxx b2=number of coins in tube
		// yyy = coins dispensed
		// xxxx = coin type
		count := (b >> 4) & 7
		nominal := self.coinTypeNominal(b & 0xf)
		return money.PollItem{Status: money.StatusDispensed, DataNominal: nominal, DataCount: count}, true
	}
	if b&0x7f == b { // Coins Deposited
		// b=01yyxxxx b2=number of coins in tube
		// yy = coin routing
		// xxxx = coin type
		routing := (b >> 4) & 3
		if routing > 3 {
			panic("code error")
		}
		nominal := self.coinTypeNominal(b & 0xf)
		return money.PollItem{Status: money.StatusCredit, DataNominal: nominal, DataCount: 1}, true
	}
	if b&0x3f == b { // Slug count
		slugs := b & 0x1f
		log.Printf("Number of slugs: %d", slugs)
		return money.PollItem{Status: money.StatusInfo, Error: ErrSlugs, DataCount: slugs}, false
	}

	err := fmt.Errorf("parsePollItem unknown=%x", b)
	log.Print(err)
	return money.PollItem{Status: money.StatusFatal, Error: err}, false
}
