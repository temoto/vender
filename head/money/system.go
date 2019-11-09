package money

import (
	"context"
	"fmt"
	"sync"

	"github.com/golang/protobuf/proto"
	"github.com/juju/errors"
	"github.com/temoto/alive"
	"github.com/temoto/vender/currency"
	"github.com/temoto/vender/engine"
	"github.com/temoto/vender/hardware/mdb/bill"
	"github.com/temoto/vender/hardware/mdb/coin"
	tele_api "github.com/temoto/vender/head/tele/api"
	"github.com/temoto/vender/helpers"
	"github.com/temoto/vender/log2"
	"github.com/temoto/vender/state"
)

type MoneySystem struct { //nolint:maligned
	Log   *log2.Log
	lk    sync.Mutex
	dirty currency.Amount // uncommited

	bill        bill.BillValidator
	billCashbox currency.NominalGroup
	billCredit  currency.NominalGroup
	billPoll    *alive.Alive

	coin        coin.CoinAcceptor
	coinCashbox currency.NominalGroup
	coinCredit  currency.NominalGroup
	coinPoll    *alive.Alive

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
		err = errors.Annotate(err, "money.Start bill")
		g.Error(err)
	}
	self.billCashbox.SetValid(self.bill.SupportedNominals())
	self.billCredit.SetValid(self.bill.SupportedNominals())
	if err := self.coin.Init(ctx); err != nil {
		err = errors.Annotate(err, "money.Start coin")
		g.Error(err)
	}
	self.coinCashbox.SetValid(self.coin.SupportedNominals())
	self.coinCredit.SetValid(self.coin.SupportedNominals())

	g.Engine.RegisterNewFunc(
		"money.cashbox_zero",
		func(ctx context.Context) error {
			self.lk.Lock()
			defer self.lk.Unlock()
			self.billCashbox.Clear()
			self.coinCashbox.Clear()
			return nil
		},
	)
	g.Engine.RegisterNewFunc(
		"money.consume!",
		func(ctx context.Context) error {
			credit := self.Credit(ctx)
			err := self.WithdrawCommit(ctx, credit)
			return errors.Annotatef(err, "consume=%s", credit.FormatCtx(ctx))
		},
	)
	g.Engine.RegisterNewFunc(
		"money.commit",
		func(ctx context.Context) error {
			curPrice := GetCurrentPrice(ctx)
			err := self.WithdrawCommit(ctx, curPrice)
			return errors.Annotatef(err, "curPrice=%s", curPrice.FormatCtx(ctx))
		},
	)
	g.Engine.RegisterNewFunc("money.abort", self.Abort)

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

	doSetGiftCredit := engine.FuncArg{
		Name: "money.set_gift_credit(?)",
		F: func(ctx context.Context, arg engine.Arg) error {
			amount := g.Config.ScaleU(uint32(arg))
			self.SetGiftCredit(ctx, amount)
			return nil
		},
	}
	g.Engine.Register(doSetGiftCredit.Name, doSetGiftCredit)

	return nil
}

func (self *MoneySystem) Stop(ctx context.Context) error {
	const tag = "money.Stop"
	errs := make([]error, 0, 8)
	errs = append(errs, self.Abort(ctx))
	if self.bill.Device.State().Online() {
		errs = append(errs, self.bill.AcceptMax(0).Do(ctx))
	}
	if self.coin.Device.State().Online() {
		errs = append(errs, self.coin.AcceptMax(0).Do(ctx))
	}
	return errors.Annotate(helpers.FoldErrors(errs), tag)
}

// Stored in one-way cashbox Telemetry_Money
func (self *MoneySystem) TeleCashbox(ctx context.Context) *tele_api.Telemetry_Money {
	pb := &tele_api.Telemetry_Money{
		Bills: make(map[uint32]uint32, bill.TypeCount),
		Coins: make(map[uint32]uint32, coin.TypeCount),
	}
	self.lk.Lock()
	defer self.lk.Unlock()
	self.billCashbox.ToMapUint32(pb.Bills)
	self.coinCashbox.ToMapUint32(pb.Coins)
	self.Log.Debugf("TeleCashbox pb=%s", proto.CompactTextString(pb))
	return pb
}

// Dispensable Telemetry_Money
func (self *MoneySystem) TeleChange(ctx context.Context) *tele_api.Telemetry_Money {
	pb := &tele_api.Telemetry_Money{
		// TODO support bill recycler Bills: make(map[uint32]uint32, bill.TypeCount),
		Coins: make(map[uint32]uint32, coin.TypeCount),
	}
	if err := self.coin.DoTubeStatus.Do(ctx); err != nil {
		state.GetGlobal(ctx).Error(errors.Annotate(err, "TeleChange"))
	}
	self.coin.Tubes().ToMapUint32(pb.Coins)
	self.Log.Debugf("TeleChange pb=%s", proto.CompactTextString(pb))
	return pb
}

const currentPriceKey = "run/current-price"

func GetCurrentPrice(ctx context.Context) currency.Amount {
	v := ctx.Value(currentPriceKey)
	if v == nil {
		state.GetGlobal(ctx).Error(fmt.Errorf("code/config error money.GetCurrentPrice not set"))
		return 0
	}
	if p, ok := v.(currency.Amount); ok {
		return p
	}
	panic(fmt.Sprintf("code error ctx[currentPriceKey] expected=currency.Amount actual=%#v", v))
}
func SetCurrentPrice(ctx context.Context, p currency.Amount) context.Context {
	return context.WithValue(ctx, currentPriceKey, p)
}
