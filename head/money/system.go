package money

import (
	"context"
	"sync"
	"time"

	"github.com/juju/errors"
	"github.com/temoto/alive"
	"github.com/temoto/vender/currency"
	"github.com/temoto/vender/hardware/mdb/bill"
	"github.com/temoto/vender/hardware/mdb/coin"
	"github.com/temoto/vender/hardware/money"
	"github.com/temoto/vender/helpers"
	"github.com/temoto/vender/log2"
)

type MoneySystem struct {
	Log    *log2.Log
	lk     sync.Mutex
	events chan Event
	dirty  currency.Amount // uncommited

	bill       bill.BillValidator
	billCredit currency.NominalGroup
	billPoll   *alive.Alive

	coin       coin.CoinAcceptor
	coinCredit currency.NominalGroup
	coinPoll   *alive.Alive
}

func (self *MoneySystem) String() string                     { return "money" }
func (self *MoneySystem) Validate(ctx context.Context) error { return nil }
func (self *MoneySystem) Start(ctx context.Context) error {
	self.lk.Lock()
	defer self.lk.Unlock()
	if self.events != nil {
		panic("double Start()")
	}
	self.Log = log2.ContextValueLogger(ctx, log2.ContextKey)

	self.events = make(chan Event, 2)
	// TODO determine if combination of errors is fatal for money subsystem
	if err := self.billInit(ctx); err != nil {
		self.Log.Errorf("money.Start bill error=%v", errors.ErrorStack(err))
	}
	if err := self.coinInit(ctx); err != nil {
		self.Log.Errorf("money.Start coin error=%v", errors.ErrorStack(err))
	}

	return nil
}
func (self *MoneySystem) Stop(ctx context.Context) error {
	self.Log.Debugf("money.Stop")
	errs := make([]error, 0, 8)
	errs = append(errs, self.Abort(ctx))
	errs = append(errs, self.bill.AcceptMax(0).Do(ctx))
	errs = append(errs, self.coin.AcceptMax(0).Do(ctx))
	self.billPoll.Stop()
	self.coinPoll.Stop()
	self.billPoll.Wait()
	self.coinPoll.Wait()
	return helpers.FoldErrors(errs)
}

func (self *MoneySystem) billInit(ctx context.Context) error {
	if err := self.bill.Init(ctx); err != nil {
		return err
	}
	self.billCredit.SetValid(self.bill.SupportedNominals())

	self.billPoll = alive.NewAlive()
	go self.bill.Run(ctx, self.billPoll.StopChan(), func(pi money.PollItem) bool {
		switch pi.Status {
		case money.StatusCredit:
			itemTime := time.Now()
			self.lk.Lock()
			defer self.lk.Unlock()

			if err := self.billCredit.Add(pi.DataNominal, uint(pi.DataCount)); err != nil {
				self.Log.Errorf("money.bill credit.Add n=%v c=%d err=%v", pi.DataNominal, pi.DataCount, err)
				break
			}
			self.Log.Debugf("money.bill credit amount=%s bill=%s total=%s",
				pi.Amount().FormatCtx(ctx), self.billCredit.Total().FormatCtx(ctx), self.locked_credit(true).FormatCtx(ctx))
			self.dirty += pi.Amount()
			self.events <- Event{created: itemTime, name: EventCredit, amount: pi.Amount()}
			// maybe TODO escrow?
		}
		return false
	})

	return nil
}

func (self *MoneySystem) coinInit(ctx context.Context) error {
	const tag = "money.coin"

	self.coinPoll = alive.NewAlive()
	if err := self.coin.Init(ctx); err != nil {
		return err
	}
	self.coinCredit.SetValid(self.coin.SupportedNominals())

	go self.coin.Run(ctx, self.coinPoll.StopChan(), func(pi money.PollItem) bool {
		itemTime := time.Now()
		self.lk.Lock()
		defer self.lk.Unlock()

		switch pi.Status {
		case money.StatusDispensed:
			self.Log.Debugf("%s manual dispense: %s", tag, pi.String())
			self.coin.DoTubeStatus.Do(ctx)
			self.coin.CommandExpansionSendDiagStatus(nil)
			// TODO telemetry
		case money.StatusReturnRequest:
			self.events <- Event{created: itemTime, name: EventAbort}
		case money.StatusRejected:
			// TODO telemetry
		case money.StatusCredit:
			err := self.coinCredit.Add(pi.DataNominal, uint(pi.DataCount))
			if err != nil {
				self.Log.Errorf("%s credit.Add n=%v c=%d err=%v", tag, pi.DataNominal, pi.DataCount, err)
			}
			self.coin.DoTubeStatus.Do(ctx)
			self.coin.CommandExpansionSendDiagStatus(nil)
			self.dirty += pi.Amount()
			self.events <- Event{created: itemTime, name: EventCredit, amount: pi.Amount()}
		default:
			panic("unhandled coin POLL item: " + pi.String())
		}
		return false
	})

	return nil
}
