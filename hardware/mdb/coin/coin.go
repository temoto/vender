package coin

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
	coinTypeCount = 16

	delayShort = 100 * time.Millisecond
	delayErr   = 500 * time.Millisecond
	delayNext  = 200 * time.Millisecond

	RouteCashBox = 0
	RouteTubes   = 1
	RouteNotUsed = 2
	RouteReject  = 3
)

type CoinAcceptor struct {
	mdb       mdb.Mdber
	byteOrder binary.ByteOrder

	// Indicates the value of the bill types 0 to 15.
	// These are final values including all scaling factors.
	coinTypeCredit []currency.Nominal

	internalScalingFactor int
}

var (
	packetReset      = mdb.PacketFromHex("08")
	packetSetup      = mdb.PacketFromHex("09")
	packetTubeStatus = mdb.PacketFromHex("0a")
	packetPoll       = mdb.PacketFromHex("0b")
)

var (
	ErrNoCredit            = fmt.Errorf("No Credit")
	ErrDefectiveTubeSensor = fmt.Errorf("Defective Tube Sensor")
	ErrDoubleArrival       = fmt.Errorf("Double Arrival")
	ErrAcceptorUnplugged   = fmt.Errorf("Acceptor Unplugged")
	ErrTubeJam             = fmt.Errorf("Tube Jam")
	ErrROMChecksum         = fmt.Errorf("ROM checksum")
	ErrCoinRouting         = fmt.Errorf("Coin Routing")
	ErrCoinJam             = fmt.Errorf("Coin Jam")
	ErrFraud               = fmt.Errorf("Possible Credited Coin Removal")
)

func (self *CoinAcceptor) Init(ctx context.Context, m mdb.Mdber) error {
	// TODO read config
	self.byteOrder = binary.BigEndian
	self.coinTypeCredit = make([]currency.Nominal, coinTypeCount)
	self.mdb = m
	// TODO maybe execute CommandReset?
	err := self.InitSequence()
	if err != nil {
		log.Printf("hardware/mdb/coin/InitSequence error=%s", err)
		// TODO maybe execute CommandReset?
	}
	return err
}

func (self *CoinAcceptor) Run(ctx context.Context, a *alive.Alive, ch chan<- PollResult) {
	stopch := a.StopChan()
	for a.IsRunning() {
		pr := self.CommandPoll()
		ch <- pr
		select {
		case <-stopch:
			return
		case <-time.After(pr.Delay):
		}
	}
}

func (self *CoinAcceptor) InitSequence() error {
	err := self.CommandSetup()
	if err != nil {
		return err
	}
	self.mdb.TxDebug(mdb.PacketFromHex("0f00"), true) // 0f00 EXPANSION IDENTIFICATION
	self.mdb.TxDebug(mdb.PacketFromHex("0f01"), true) // 0f01 EXPANSION FEATURE ENABLE
	self.mdb.TxDebug(mdb.PacketFromHex("0f05"), true) // 0f05 EXPANSION SEND DIAG STATUS
	err = self.CommandTubeStatus()
	if err != nil {
		return err
	}
	err = self.CommandCoinType(0xffff, 0xffff) // TODO read config
	if err != nil {
		return err
	}
	return nil
}

func (self *CoinAcceptor) CommandReset() {
	self.mdb.TxDebug(packetReset, false)
}

func (self *CoinAcceptor) CommandSetup() error {
	const expectLength = 23
	response := new(mdb.Packet)
	err := self.mdb.Tx(packetSetup, response)
	if err != nil {
		log.Printf("mdb request=%s err: %s", packetSetup.Format(), err)
		return err
	}
	log.Printf("setup response=(%d)%s", response.Len(), response.Format())
	bs := response.Bytes()
	if len(bs) < expectLength {
		return fmt.Errorf("hardware/mdb/coin SETUP response=%s expected %d bytes", response.Format(), expectLength)
	}
	scalingFactor := bs[3]
	for i, sf := range bs[7:23] {
		n := currency.Nominal(sf) * currency.Nominal(scalingFactor) * currency.Nominal(self.internalScalingFactor)
		log.Printf("i=%d sf=%d nominal=%s", i, sf, currency.Amount(n).Format100I())
		self.coinTypeCredit[i] = n
	}
	log.Printf("Changer Feature Level: %d", bs[0])
	log.Printf("Country / Currency Code: %x", bs[1:3])
	log.Printf("Coin Scaling Factor: %d", scalingFactor)
	log.Printf("Decimal Places: %d", bs[4])
	log.Printf("Coin Type Routing: %d", self.byteOrder.Uint16(bs[5:7]))
	log.Printf("Coin Type Credit: %x %#v", bs[7:23], self.coinTypeCredit)
	return nil
}

func (self *CoinAcceptor) CommandPoll() PollResult {
	now := time.Now()
	response := new(mdb.Packet)
	err := self.mdb.Tx(packetPoll, response)
	result := PollResult{Time: now, Delay: delayNext}
	if err != nil {
		result.Error = err
		result.Delay = delayErr
		return result
	}
	if response.Len() == 0 {
		return result
	}
	result.Items = make([]PollItem, response.Len())
	// log.Printf("poll response=%s", response.Format())
	bs := response.Bytes()
	for i, b := range bs {
		b2 := byte(0)
		if i+1 < len(bs) {
			b2 = bs[i+1]
		}
		result.Items[i] = self.parsePollItem(b, b2)
	}
	return result
}

func (self *CoinAcceptor) CommandTubeStatus() error {
	const expectLength = 18
	response := new(mdb.Packet)
	err := self.mdb.Tx(packetTubeStatus, response)
	if err != nil {
		log.Printf("mdb request=%s err: %s", packetTubeStatus.Format(), err)
		return err
	}
	log.Printf("tubestatus response=(%d)%s", response.Len(), response.Format())
	bs := response.Bytes()
	if len(bs) < expectLength {
		return fmt.Errorf("hardware/mdb/coin TUBE STATUS response=%s expected %d bytes", response.Format(), expectLength)
	}
	full := self.byteOrder.Uint16(bs[0:2])
	counts := bs[2:18]
	// TODO use full,counts
	_ = full
	_ = counts
	return nil
}

func (self *CoinAcceptor) CommandCoinType(accept, dispense uint16) error {
	buf := [4]byte{}
	self.byteOrder.PutUint16(buf[0:2], accept)
	self.byteOrder.PutUint16(buf[2:4], dispense)
	var err error
	response := new(mdb.Packet)
	request := mdb.PacketFromBytes([]byte{0x0c})
	_, err = request.Write(buf[:])
	if err != nil {
		panic("code error")
	}
	err = self.mdb.Tx(request, response)
	if err != nil {
		log.Printf("mdb request=%s err: %s", request.Format(), err)
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
	err = self.mdb.Tx(request, response)
	if err != nil {
		log.Printf("mdb request=%s err: %s", request.Format(), err)
		return err
	}
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

func (self *CoinAcceptor) parsePollItem(b, b2 byte) PollItem {
	switch b {
	case 0x01: // Escrow request
		return PollItem{Status: StatusEscrowRequest}
	case 0x02: // Changer Payout Busy
		return PollItem{Status: StatusBusy}
	case 0x03: // No Credit
		return PollItem{Status: StatusError, Error: ErrNoCredit}
	case 0x04: // Defective Tube Sensor
		return PollItem{Status: StatusFatal, Error: ErrDefectiveTubeSensor}
	case 0x05: // Double Arrival
		return PollItem{Status: StatusError, Error: ErrDoubleArrival}
	case 0x06: // Acceptor Unplugged
		return PollItem{Status: StatusFatal, Error: ErrAcceptorUnplugged}
	case 0x07: // Tube Jam
		return PollItem{Status: StatusFatal, Error: ErrTubeJam}
	case 0x08: // ROM checksum error
		return PollItem{Status: StatusFatal, Error: ErrROMChecksum}
	case 0x09: // Coin Routing Error
		return PollItem{Status: StatusError, Error: ErrCoinRouting}
	case 0x0a: // Changer Busy
		return PollItem{Status: StatusBusy}
	case 0x0b: // Changer was Reset
		return PollItem{Status: StatusWasReset}
	case 0x0c: // Coin Jam
		return PollItem{Status: StatusFatal, Error: ErrCoinJam}
	case 0x0d: // Possible Credited Coin Removal
		return PollItem{Status: StatusError, Error: ErrFraud}
	}

	if b&0x80 != 0 { // Coins Dispensed Manually
		// b=1yyyxxxx b2=number of coins in tube
		// yyy = coins dispensed
		// xxxx = coin type
		count := (b >> 4) & 7
		nominal := self.coinTypeNominal(b & 0xf)
		return PollItem{Status: StatusDispensed, Nominal: nominal, Count: count}
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
		return PollItem{Status: StatusDeposited, Nominal: nominal, Count: 1}
	}
	if b&0x3f == b { // Slug count
		slugs := b & 0x1f
		log.Printf("Number of slugs: %d", slugs)
		return PollItem{Status: StatusSlugs, Count: slugs}
	}

	err := fmt.Errorf("parsePollItem unknown=%x", b)
	log.Print(err)
	return PollItem{Status: StatusFatal, Error: err}
}
