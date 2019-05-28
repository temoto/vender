package money

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/juju/errors"
	"github.com/temoto/alive"
	"github.com/temoto/vender/currency"
	"github.com/temoto/vender/engine"
	"github.com/temoto/vender/hardware/mdb/bill"
	"github.com/temoto/vender/hardware/mdb/coin"
	"github.com/temoto/vender/hardware/money"
	"github.com/temoto/vender/head/tele"
	"github.com/temoto/vender/helpers"
	"github.com/temoto/vender/log2"
	"github.com/temoto/vender/state"
)

type MoneySystem struct {
	Log    *log2.Log
	lk     sync.Mutex
	subs   []EventFunc
	subsLk sync.Mutex
	dirty  currency.Amount // uncommited

	bill       bill.BillValidator
	billCredit currency.NominalGroup
	billPoll   *alive.Alive

	coin       coin.CoinAcceptor
	coinCredit currency.NominalGroup
	coinPoll   *alive.Alive

	giftCredit currency.Amount
}

func (self *MoneySystem) Start(ctx context.Context) error {
	self.lk.Lock()
	defer self.lk.Unlock()
	self.Log = log2.ContextValueLogger(ctx)

	// TODO determine if combination of errors is fatal for money subsystem
	if err := self.billInit(ctx); err != nil {
		self.Log.Errorf("money.Start bill err=%v", errors.ErrorStack(err))
	}
	if err := self.coinInit(ctx); err != nil {
		self.Log.Errorf("money.Start coin err=%v", errors.ErrorStack(err))
	}

	ponr := engine.Func{
		Name: "@money.ponr",
		F: func(ctx context.Context) error {
			curPrice := GetCurrentPrice(ctx)
			err := self.WithdrawCommit(ctx, curPrice)
			return errors.Annotatef(err, "curPrice=%s", curPrice.FormatCtx(ctx))
		},
	}
	engine.GetEngine(ctx).Register("@money.ponr", ponr)

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

const currentPriceKey = "run/current-price"

func GetCurrentPrice(ctx context.Context) currency.Amount {
	v := ctx.Value(currentPriceKey)
	if v == nil {
		panic("code error ctx[currentPriceKey]=nil")
	}
	if p, ok := v.(currency.Amount); ok {
		return p
	}
	panic(fmt.Sprintf("code error ctx[currentPriceKey] expected=currency.Amount actual=%#v", v))
}
func SetCurrentPrice(ctx context.Context, p currency.Amount) context.Context {
	return context.WithValue(ctx, currentPriceKey, p)
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
			self.EventFire(Event{created: itemTime, name: EventCredit, amount: pi.Amount()})
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
			_ = self.coin.DoTubeStatus.Do(ctx)
			_ = self.coin.CommandExpansionSendDiagStatus(nil)
		case money.StatusReturnRequest:
			self.EventFire(Event{created: itemTime, name: EventAbort})
		case money.StatusRejected:
			state.GetGlobal(ctx).Tele.StatModify(func(s *tele.Stat) { s.CoinRejected[uint32(pi.DataNominal)] += uint32(pi.DataCount) })
		case money.StatusCredit:
			err := self.coinCredit.Add(pi.DataNominal, uint(pi.DataCount))
			if err != nil {
				self.Log.Errorf("%s credit.Add n=%v c=%d err=%v", tag, pi.DataNominal, pi.DataCount, err)
			}
			_ = self.coin.DoTubeStatus.Do(ctx)
			_ = self.coin.CommandExpansionSendDiagStatus(nil)
			self.dirty += pi.Amount()
			self.EventFire(Event{created: itemTime, name: EventCredit, amount: pi.Amount()})
		default:
			panic("unhandled coin POLL item: " + pi.String())
		}
		return false
	})

	return nil
}
