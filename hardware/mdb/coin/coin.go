package coin

import (
	"context"
	"encoding/binary"
	"log"
	"time"

	"github.com/temoto/alive"
	"github.com/temoto/vender/currency"
	"github.com/temoto/vender/hardware/mdb"
)

const (
	coinTypeCount = 16
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
	packetCoinType   = mdb.PacketFromHex("0c")
	packetDispense   = mdb.PacketFromHex("0d")
)

func (self *CoinAcceptor) Init(ctx context.Context, m mdb.Mdber) error {
	// TODO read config
	self.byteOrder = binary.BigEndian
	self.coinTypeCredit = make([]currency.Nominal, coinTypeCount)
	self.mdb = m
	return nil
}

func (self *CoinAcceptor) Loop(ctx context.Context, a *alive.Alive, ch chan<- PollResult) {
	self.mdb.TxDebug(packetReset, false)
	self.InitSequence()

	stopch := a.StopChan()
	for a.IsRunning() {
		pr := self.Poll()
		ch <- pr
		select {
		case <-stopch:
			return
		case <-time.After(pr.Delay):
		}
	}
}

func (self *CoinAcceptor) InitSequence() {
	log.Printf("CoinAcceptor.InitSequence TODO")
}

func (self *CoinAcceptor) Poll() PollResult {
	log.Printf("CoinAcceptor.Poll TODO")
	return PollResult{}
}
