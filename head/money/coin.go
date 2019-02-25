package money

import (
	"context"
	"time"

	"github.com/temoto/alive"
	"github.com/temoto/vender/currency"
	"github.com/temoto/vender/hardware/mdb"
	"github.com/temoto/vender/hardware/mdb/coin"
	"github.com/temoto/vender/hardware/money"
	"github.com/temoto/vender/log2"
)

type CoinState struct {
	Log    *log2.Log
	alive  *alive.Alive
	hw     coin.CoinAcceptor
	credit currency.NominalGroup
}

func (self *CoinState) Init(ctx context.Context, parent *MoneySystem) error {
	self.Log = parent.Log
	self.Log.Debugf("head/money/coin/Init begin")
	self.alive = alive.NewAlive()
	if err := self.hw.Init(ctx); err != nil {
		return err
	}
	self.credit.SetValid(self.hw.SupportedNominals())
	time.Sleep(mdb.DefaultDelayNext)
	self.Log.Debugf("head/money/coin/Init end, running")
	go self.hw.Run(ctx, self.alive, func(pi money.PollItem) { self.handlePollItem(ctx, parent, pi) })
	return nil
}

func (self *CoinState) Stop(ctx context.Context) {
	self.alive.Stop()
	self.alive.Wait()
}

func (self *CoinState) Dispense(ctx context.Context, ng *currency.NominalGroup) (currency.Amount, error) {
	self.alive.Add(1)
	defer self.alive.Done()

	sum := currency.Amount(0)
	err := ng.Iter(func(nominal currency.Nominal, count uint) error {
		self.Log.Debugf("Dispense n=%v c=%d", nominal, count)
		self.hw.CommandTubeStatus()
		if count == 0 {
			return nil
		}
		err := self.hw.NewDispense(nominal, uint8(count)).Do(ctx)
		// err := self.hw.CommandPayout(currency.Amount(nominal) * currency.Amount(count))
		self.Log.Debugf("dispense err=%v", err)
		if err == nil {
			sum += currency.Amount(nominal) * currency.Amount(count)
		}
		self.hw.CommandTubeStatus()
		self.hw.CommandExpansionSendDiagStatus(nil)
		self.Log.Debugf("Dispense end n=%v c=%d", nominal, count)
		return err
	})
	return sum, err
}

func (self *CoinState) handlePollItem(ctx context.Context, m *MoneySystem, pi money.PollItem) {
	const logPrefix = "head/money/coin"
	itemTime := time.Now()

	switch pi.Status {
	case money.StatusDispensed:
		self.Log.Debugf("manual dispense: %s", pi.String())
		self.hw.CommandTubeStatus()
		self.hw.CommandExpansionSendDiagStatus(nil)
		// TODO telemetry
	case money.StatusReturnRequest:
		m.events <- Event{created: itemTime, name: EventAbort}
	case money.StatusRejected:
		// TODO telemetry
	case money.StatusWasReset:
		self.Log.Debugf("coin was reset")
		// TODO telemetry
	case money.StatusCredit:
		err := self.credit.Add(pi.DataNominal, uint(pi.DataCount))
		if err != nil {
			self.Log.Debugf("coin credit.Add n=%v c=%d err=%v", pi.DataNominal, pi.DataCount, err)
		}
		self.hw.CommandTubeStatus()
		self.hw.CommandExpansionSendDiagStatus(nil)
		m.events <- Event{created: itemTime, name: EventCredit, amount: pi.Amount()}
	default:
		m.handleGenericPollItem(ctx, pi, logPrefix)
	}
}
