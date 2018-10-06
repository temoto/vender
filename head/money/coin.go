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
	lk     sync.Mutex
	alive  *alive.Alive
	hw     coin.CoinAcceptor
	credit currency.NominalGroup
}

func (self *CoinState) Init(ctx context.Context, mdber mdb.Mdber) error {
	self.lk.Lock()
	defer self.lk.Unlock()

	log.Printf("head/money/coin init")
	self.alive = alive.NewAlive()
	self.alive.Add(1)
	if err := self.hw.Init(ctx, mdber); err != nil {
		return err
	}
	self.credit.SetValid(self.hw.SupportedNominals())
	time.Sleep(coin.DelayNext)
	pch := make(chan money.PollResult, 2)
	go self.hw.Run(ctx, self.alive, pch)
	go self.pollResultLoop(&Global, pch)
	return nil
}

func (self *CoinState) Stop(ctx context.Context) {
	self.alive.Stop()
	self.alive.Wait()
}

func (self *CoinState) Dispense(ng *currency.NominalGroup) (currency.Amount, error) {
	sum := currency.Amount(0)
	err := ng.Iter(func(nominal currency.Nominal, count uint) error {
		log.Printf("Dispense n=%v c=%d", nominal, count)
		if count == 0 {
			return nil
		}
		err := self.hw.CommandDispense(nominal, uint8(count))
		if err == nil {
			sum += currency.Amount(nominal) * currency.Amount(count)
		}
		return err
	})
	return sum, err
}

func (self *CoinState) pollResultLoop(m *MoneySystem, pch <-chan money.PollResult) {
	const logPrefix = "head/money/coin"
	h := func(m *MoneySystem, pr *money.PollResult, pi money.PollItem, hw Hardwarer) bool {
		switch pi.Status {
		case money.StatusDispensed:
			log.Printf("manual dispense: %s", pi.String())
			// TODO telemetry
		case money.StatusReturnRequest:
			m.events <- Event{created: pr.Time, name: EventAbort}
		case money.StatusRejected:
			// TODO telemetry
		case money.StatusWasReset:
			log.Printf("coin was reset")
			// TODO telemetry
			// self.hw.InitSequence()
		case money.StatusCredit:
			err := self.credit.Add(pi.DataNominal, uint(pi.DataCount))
			if err != nil {
				log.Printf("coin credit.Add n=%v c=%d err=%v", pi.DataNominal, pi.DataCount, err)
			}
			m.events <- Event{created: pr.Time, name: EventCredit, amount: pi.Amount()}
		default:
			return false
		}
		return true
	}
	onRefund := func(m *MoneySystem, hw Hardwarer) { self.Dispense(&self.credit) }
	onRestart := func(m *MoneySystem, hw Hardwarer) {
		self.hw.CommandReset()
		self.hw.InitSequence()
	}
	pollResultLoop(m, pch, h, onRefund, onRestart, &self.hw, logPrefix)
}
