package money

import (
	"context"
	"log"
	"time"

	"github.com/temoto/alive"
	"github.com/temoto/vender/currency"
	"github.com/temoto/vender/hardware/mdb"
	"github.com/temoto/vender/hardware/mdb/coin"
	"github.com/temoto/vender/hardware/money"
)

type CoinState struct {
	alive  *alive.Alive
	hw     coin.CoinAcceptor
	credit currency.NominalGroup
}

func (self *CoinState) Init(ctx context.Context, parent *MoneySystem, mdber mdb.Mdber) error {
	log.Printf("head/money/coin/Init begin")
	self.alive = alive.NewAlive()
	if err := self.hw.Init(ctx, mdber); err != nil {
		return err
	}
	self.credit.SetValid(self.hw.SupportedNominals())
	time.Sleep(coin.DelayNext)
	pch := make(chan money.PollResult, 2)
	self.alive.Add(2)
	log.Printf("head/money/coin/Init end, running")
	go self.hw.Run(ctx, self.alive, pch)
	go self.pollResultLoop(ctx, parent, pch)
	return nil
}

func (self *CoinState) Stop(ctx context.Context) {
	self.alive.Stop()
	self.alive.Wait()
}

func (self *CoinState) Dispense(ng *currency.NominalGroup) (currency.Amount, error) {
	self.alive.Add(1)
	defer self.alive.Done()

	sum := currency.Amount(0)
	err := ng.Iter(func(nominal currency.Nominal, count uint) error {
		log.Printf("Dispense n=%v c=%d", nominal, count)
		self.hw.CommandTubeStatus()
		if count == 0 {
			return nil
		}
		err := self.hw.CommandDispense(nominal, uint8(count))
		// err := self.hw.CommandPayout(currency.Amount(nominal) * currency.Amount(count))
		log.Printf("dispense err=%v", err)
		if err == nil {
			sum += currency.Amount(nominal) * currency.Amount(count)
		}
		<-self.hw.ReadyChan()
		self.hw.CommandTubeStatus()
		self.hw.CommandExpansionSendDiagStatus(nil)
		log.Printf("Dispense end n=%v c=%d", nominal, count)
		return err
	})
	return sum, err
}

func (self *CoinState) pollResultLoop(ctx context.Context, m *MoneySystem, pch <-chan money.PollResult) {
	defer self.alive.Done()

	const logPrefix = "head/money/coin"
	h := func(m *MoneySystem, pr *money.PollResult, pi money.PollItem) bool {
		switch pi.Status {
		case money.StatusDispensed:
			log.Printf("manual dispense: %s", pi.String())
			self.hw.CommandTubeStatus()
			self.hw.CommandExpansionSendDiagStatus(nil)
			// TODO telemetry
		case money.StatusReturnRequest:
			m.events <- Event{created: pr.Time, name: EventAbort}
		case money.StatusRejected:
			// TODO telemetry
		case money.StatusWasReset:
			log.Printf("coin was reset")
			// TODO telemetry
		case money.StatusCredit:
			err := self.credit.Add(pi.DataNominal, uint(pi.DataCount))
			if err != nil {
				log.Printf("coin credit.Add n=%v c=%d err=%v", pi.DataNominal, pi.DataCount, err)
			}
			self.hw.CommandTubeStatus()
			self.hw.CommandExpansionSendDiagStatus(nil)
			m.events <- Event{created: pr.Time, name: EventCredit, amount: pi.Amount()}
		default:
			return false
		}
		return true
	}
	genericPollResultLoop(ctx, m, pch, h, self.hw.Restarter(), logPrefix)
}
