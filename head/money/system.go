package money

import (
	"context"
	"fmt"
	"sync"

	"github.com/temoto/alive"
	"github.com/juju/errors"
	"github.com/temoto/vender/currency"
	"github.com/temoto/vender/engine"
	"github.com/temoto/vender/hardware/mdb/bill"
	"github.com/temoto/vender/hardware/mdb/coin"
	"github.com/temoto/vender/helpers"
	"github.com/temoto/vender/log2"
	"github.com/temoto/vender/state"
)

type MoneySystem struct { //nolint:maligned
	Log   *log2.Log
	lk    sync.Mutex
	dirty currency.Amount // uncommited

	bill       bill.BillValidator
	billCredit currency.NominalGroup
	billPoll   *alive.Alive

	coin       coin.CoinAcceptor
	coinCredit currency.NominalGroup
	coinPoll   *alive.Alive

	giftCredit currency.Amount
}

func GetGlobal(ctx context.Context) *MoneySystem {
	return state.GetGlobal(ctx).XXX_money.Load().(*MoneySystem)
}

func (self *MoneySystem) Start(ctx context.Context) error {
	g := state.GetGlobal(ctx)

	self.lk.Lock()
	defer self.lk.Unlock()
	self.Log = g.Log
	g.XXX_money.Store(self)

	// TODO wait for bill/coin inited by hardware.Enum()
	// TODO determine if combination of errors is fatal for money subsystem
	if err := self.bill.Init(ctx); err != nil {
		self.Log.Errorf("money.Start bill err=%v", errors.ErrorStack(err))
	}
	self.billCredit.SetValid(self.bill.SupportedNominals())
	if err := self.coin.Init(ctx); err != nil {
		self.Log.Errorf("money.Start coin err=%v", errors.ErrorStack(err))
	}
	self.coinCredit.SetValid(self.coin.SupportedNominals())

	doCommit := engine.Func{
		Name: "money.commit",
		F: func(ctx context.Context) error {
			curPrice := GetCurrentPrice(ctx)
			err := self.WithdrawCommit(ctx, curPrice)
			return errors.Annotatef(err, "curPrice=%s", curPrice.FormatCtx(ctx))
		},
	}
	g.Engine.Register(doCommit.String(), doCommit)
	doAbort := engine.Func{
		Name: "money.abort",
		F:    self.Abort,
	}
	g.Engine.Register(doAbort.String(), doAbort)
	doAccept := engine.FuncArg{
		Name: "money.accept(?)",
		F: func(ctx context.Context, arg engine.Arg) error {
			self.AcceptCredit(ctx, g.Config.ScaleU(uint32(arg)), nil, nil)
			return nil
		},
	}
	g.Engine.Register(doAccept.Name, doAccept)

	doDispense := engine.FuncArg{
		Name: "money.dispense(?)",
		F: func(ctx context.Context, arg engine.Arg) error {
			dispensed := currency.NominalGroup{}
			err := self.coin.NewDispenseSmart(g.Config.ScaleU(uint32(arg)), false, &dispensed).Do(ctx)
			self.Log.Infof("dispensed=%s", dispensed.String())
			return err
		}}
	g.Engine.Register(doDispense.Name, doDispense)

	return nil
}

func (self *MoneySystem) Stop(ctx context.Context) error {
	self.Log.Debugf("money.Stop")
	errs := make([]error, 0, 8)
	errs = append(errs, self.Abort(ctx))
	if self.billPoll != nil {
		errs = append(errs, self.bill.AcceptMax(0).Do(ctx))
		self.billPoll.Stop()
	}
	if self.coinPoll != nil {
		errs = append(errs, self.coin.AcceptMax(0).Do(ctx))
		self.coinPoll.Stop()
	}
	if self.billPoll != nil {
		self.billPoll.Wait()
	}
	if self.coinPoll != nil {
		self.coinPoll.Wait()
	}
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
