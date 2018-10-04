package money

import (
	"context"
	"log"
	"sync"
	"time"

	"github.com/temoto/alive"
	"github.com/temoto/vender/currency"
	"github.com/temoto/vender/hardware/mdb"
	"github.com/temoto/vender/hardware/mdb/coin"
	"github.com/temoto/vender/hardware/money"
)

type CoinState struct {
	lk    sync.Mutex
	alive *alive.Alive
	hw    coin.CoinAcceptor
}

func (self *CoinState) Init(ctx context.Context, mdber mdb.Mdber, events chan<- Event) error {
	self.lk.Lock()
	defer self.lk.Unlock()

	log.Printf("head/money/coin init")
	self.alive = alive.NewAlive()
	self.alive.Add(1)
	pch := make(chan money.PollResult, 2)
	if err := self.hw.Init(ctx, mdber); err != nil {
		return err
	}
	time.Sleep(coin.DelayNext)
	go self.hw.Run(ctx, self.alive, pch)
	go self.pollResultLoop(&Global, pch)
	return nil
}

func (self *CoinState) Stop(ctx context.Context) {
	self.alive.Stop()
	self.alive.Wait()
}

func (self *CoinState) pollResultLoop(m *MoneySystem, pch <-chan money.PollResult) {
	const logPrefix = "head/money/coin"
	h := func(m *MoneySystem, pr *money.PollResult, pi money.PollItem, hw Hardwarer) bool {
		switch pi.Status {
		case money.StatusReturnRequest:
			self.hw.CommandDispense(currency.Nominal(100), 2)
		case money.StatusRejected:
			// TODO telemetry
		case money.StatusWasReset:
			log.Printf("coin was reset")
			// TODO telemetry
			// self.hw.InitSequence()
		case money.StatusCredit:
			m.Events() <- Event{created: pr.Time, name: EventCredit, amount: pi.Amount()}
		default:
			return false
		}
		return true
	}
	onRefund := func(m *MoneySystem, hw Hardwarer) {
		// TODO
	}
	onRestart := func(m *MoneySystem, hw Hardwarer) {
		self.hw.CommandReset()
		self.hw.InitSequence()
	}
	pollResultLoop(m, pch, h, onRefund, onRestart, &self.hw, logPrefix)
}
