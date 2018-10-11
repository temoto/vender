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

type CoinAcceptor struct {
	mdb       mdb.Mdber
	byteOrder binary.ByteOrder

	// Indicates the value of the bill types 0 to 15.
	// These are final values including all scaling factors.
	coinTypeCredit []currency.Nominal

	coinTypeRouting uint16

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
	packetDiagStatus = mdb.PacketFromHex("0f05")
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
	// TODO maybe execute CommandReset then wait for StatusWasReset
	err := self.InitSequence()
	if err != nil {
		log.Printf("hardware/mdb/coin/InitSequence error=%s", err)
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
		// TODO to reuse single PollResult safely, must clone .Items before sending to chan
		pr := money.NewPollResult(mdb.PacketMaxLength)
		if err := self.CommandPoll(pr); err != nil {
			log.Printf("coin.Run CommandPoll err=%v", err)
			if pr.Delay == 0 {
				pr.Delay = DelayErr
			}
		} else {
			select {
			case ch <- *pr:
			case <-stopch:
				return
			}
		}
		select {
		case <-time.After(pr.Delay):
		case <-stopch:
			return
		}
	}
}

func (self *CoinAcceptor) ReadyChan() <-chan struct{} {
	return self.ready
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
	diagResult := new(DiagResult)
	if err = self.CommandExpansionSendDiagStatus(diagResult); err != nil {
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
	return self.mdb.Tx(packetReset, new(mdb.Packet))
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
	self.featureLevel = bs[0]
	scalingFactor := bs[3]
	self.coinTypeRouting = self.byteOrder.Uint16(bs[5 : 5+2])
	for i, sf := range bs[7 : 7+16] {
		n := currency.Nominal(sf) * currency.Nominal(scalingFactor) * currency.Nominal(self.internalScalingFactor)
		log.Printf("i=%d sf=%d nominal=%s", i, sf, currency.Amount(n).Format100I())
		self.coinTypeCredit[i] = n
	}
	log.Printf("Changer Feature Level: %d", self.featureLevel)
	log.Printf("Country / Currency Code: %x", bs[1:1+2])
	log.Printf("Coin Scaling Factor: %d", scalingFactor)
	log.Printf("Decimal Places: %d", bs[4])
	log.Printf("Coin Type Routing: %b", self.coinTypeRouting)
	log.Printf("Coin Type Credit: %x %#v", bs[7:], self.coinTypeCredit)
	return nil
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

func (self *CoinAcceptor) CommandPoll(result *money.PollResult) error {
	result.Delay = DelayNext
	result.Error = nil
	result.Items = result.Items[:0]
	result.Time = time.Now()
	response := new(mdb.Packet)
	err := self.mdb.Tx(packetPoll, response)
	if err != nil {
		result.Delay = DelayErr
		result.Error = err
		return err
	}
	bs := response.Bytes()
	if len(bs) == 0 {
		sendNothing(self.ready)
		return nil
	}
	log.Printf("poll response=%s", response.Format())
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
	if result.Ready() {
		sendNothing(self.ready)
	}
	return nil
}

func (self *CoinAcceptor) CommandCoinType(accept, dispense uint16) error {
	buf := [5]byte{0x0c}
	self.byteOrder.PutUint16(buf[1:], accept)
	self.byteOrder.PutUint16(buf[3:], dispense)
	request := mdb.PacketFromBytes(buf[:])
	err := self.mdb.Tx(request, new(mdb.Packet))
	if err != nil {
		log.Printf("mdb request=%s err=%v", request.Format(), err)
	}
	return err
}

func (self *CoinAcceptor) CommandDispense(nominal currency.Nominal, count uint8) error {
	if count >= 16 {
		return fmt.Errorf("CommandDispense count=%d overflow >=16", count)
	}
	coinType := self.nominalCoinType(nominal)
	if coinType < 0 {
		return fmt.Errorf("dispense not supported for nominal=%v", currency.Amount(nominal).Format100I())
	}

	request := mdb.PacketFromBytes([]byte{0x0d, (count << 4) + uint8(coinType)})
	<-self.ReadyChan()
	err := self.mdb.Tx(request, new(mdb.Packet))
	if err != nil {
		log.Printf("mdb request=%s err=%v", request.Format(), err)
	}
	return err
}

func (self *CoinAcceptor) CommandPayout(amount currency.Amount) error {
	request := mdb.PacketFromBytes([]byte{0x0f, 0x02, byte(int(amount) / 100 / self.internalScalingFactor)})
	<-self.ReadyChan()
	err := self.mdb.Tx(request, new(mdb.Packet))
	if err != nil {
		log.Printf("mdb request=%s err=%v", request.Format(), err)
	}
	return err
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
	log.Printf("EXPANSION IDENTIFICATION response=(%d)%s", response.Len(), response.Format())
	bs := response.Bytes()
	if len(bs) < expectLength {
		return fmt.Errorf("hardware/mdb/coin EXPANSION IDENTIFICATION response=%s expected %d bytes", response.Format(), expectLength)
	}
	self.supportedFeatures = Features(self.byteOrder.Uint32(bs[29 : 29+4]))
	log.Printf("Manufacturer Code: %x", bs[0:0+3])
	log.Printf("Serial Number: '%s'", string(bs[3:3+12]))
	log.Printf("Model #/Tuning Revision: '%s'", string(bs[15:15+12]))
	log.Printf("Software Version: %x", bs[27:27+2])
	log.Printf("Optional Features: %b", self.supportedFeatures)
	return nil
}

// CommandExpansionSendDiagStatus returns:
// - `nil` if command is not supported by device, result is not modified
// - otherwise returns nil or MDB/parse error, result set to valid DiagResult
func (self *CoinAcceptor) CommandExpansionSendDiagStatus(result *DiagResult) error {
	if self.supportedFeatures&FeatureExtendedDiagnostic == 0 {
		log.Printf("CommandExpansionSendDiagStatus feature is not supported")
		return nil
	}
	response := new(mdb.Packet)
	err := self.mdb.Tx(packetDiagStatus, response)
	if err != nil {
		return err
	}
	dr, err := parseDiagResult(response.Bytes(), self.byteOrder)
	log.Printf("DiagStatus=%s", dr.Error())
	if result != nil {
		*result = dr
	}
	return err
}

func (self *CoinAcceptor) CommandFeatureEnable(requested Features) error {
	f := requested & self.supportedFeatures
	buf := [6]byte{0x0f, 0x01}
	self.byteOrder.PutUint32(buf[2:], uint32(f))
	request := mdb.PacketFromBytes(buf[:])
	err := self.mdb.Tx(request, new(mdb.Packet))
	if err != nil {
		log.Printf("mdb request=%s err=%v", request.Format(), err)
	}
	return err
}

func (self *CoinAcceptor) coinTypeNominal(b byte) currency.Nominal {
	if b >= coinTypeCount {
		log.Printf("invalid coin type: %d", b)
		return 0
	}
	return self.coinTypeCredit[b]
}

func (self *CoinAcceptor) nominalCoinType(nominal currency.Nominal) int8 {
	for ct, n := range self.coinTypeCredit {
		if n == nominal && ((1<<uint(ct))&self.coinTypeRouting != 0) {
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
		log.Printf("Number of slugs: %d", slugs)
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
			pi.Error = fmt.Errorf("routing=notused b=%x pi=%s", b, pi.String())
		case RoutingReject:
			pi.Status = money.StatusRejected
		default:
			// pi.Status = money.StatusFatal
			panic(fmt.Errorf("code error b=%x routing=%b", b, routing))
		}
		log.Printf("deposited coinType=%d routing=%s pi=%s", coinType, routing.String(), pi.String())
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

	err := fmt.Errorf("parsePollItem unknown=%x", b)
	return money.PollItem{Status: money.StatusFatal, Error: err}, false
}

func sendNothing(ch chan<- struct{}) {
	select {
	case ch <- struct{}{}:
	default:
	}
}
