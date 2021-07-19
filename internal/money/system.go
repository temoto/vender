package money

import (
	"context"
	"fmt"
	"sync"

	"github.com/golang/protobuf/proto"
	"github.com/juju/errors"
	"github.com/temoto/vender/currency"
	"github.com/temoto/vender/hardware/mdb/bill"
	"github.com/temoto/vender/hardware/mdb/coin"
	"github.com/temoto/vender/helpers"
	"github.com/temoto/vender/internal/engine"
	"github.com/temoto/vender/internal/state"
	"github.com/temoto/vender/log2"
	tele_api "github.com/temoto/vender/tele"
)

type MoneySystem struct { //nolint:maligned
	Log   *log2.Log
	lk    sync.RWMutex
	dirty currency.Amount // uncommited

	bill        bill.Biller
	billCashbox currency.NominalGroup
	billCredit  currency.NominalGroup

	coin        coin.Coiner
	coinCashbox currency.NominalGroup
	coinCredit  currency.NominalGroup

	giftCredit currency.Amount
}

func GetGlobal(ctx context.Context) *MoneySystem {
	return state.GetGlobal(ctx).XXX_money.Load().(*MoneySystem)
}

func (ms *MoneySystem) AddDirty(dirty currency.Amount) {
	ms.dirty += dirty
}

func (ms *MoneySystem) SetDirty(dirty currency.Amount) {
	ms.dirty = dirty
}

func (ms *MoneySystem) GetDirty() currency.Amount {
	return ms.dirty
}

func (ms *MoneySystem) ResetMoney() {
	ms.locked_zero()
}

func (self *MoneySystem) Start(ctx context.Context) error {
	g := state.GetGlobal(ctx)

	self.lk.Lock()
	defer self.lk.Unlock()
	self.Log = g.Log
	g.XXX_money.Store(self)

	const devNameBill = "bill"
	const devNameCoin = "coin"
	self.bill = bill.Stub{}
	self.coin = coin.Stub{}
	errs := make([]error, 0, 2)
	if dev, err := g.GetDevice(devNameBill); err == nil {
		self.bill = dev.(bill.Biller)
	} else if errors.IsNotFound(err) {
		self.Log.Debugf("device=%s is not enabled in config", devNameBill)
	} else {
		errs = append(errs, errors.Annotatef(err, "device=%s", devNameBill))
	}
	if dev, err := g.GetDevice(devNameCoin); err == nil {
		self.coin = dev.(coin.Coiner)
	} else if errors.IsNotFound(err) {
		self.Log.Debugf("device=%s is not enabled in config", devNameCoin)
	} else {
		errs = append(errs, errors.Annotatef(err, "device=%s", devNameCoin))
	}
	if e := helpers.FoldErrors(errs); e != nil {
		return e
	}

	self.billCashbox.SetValid(self.bill.SupportedNominals())
	self.billCredit.SetValid(self.bill.SupportedNominals())
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

	doGive := engine.FuncArg{
		Name: "money.give(?)",
		F: func(ctx context.Context, arg engine.Arg) error {
			dispensed := currency.NominalGroup{}
			d := self.coin.NewGive(g.Config.ScaleU(uint32(arg)), false, &dispensed)
			err := g.Engine.Exec(ctx, d)
			self.Log.Infof("dispensed=%s", dispensed.String())
			return err
		}}
	g.Engine.Register(doGive.Name, doGive)
	g.Engine.Register("money.dispense(?)", doGive) // FIXME remove deprecated

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
	g := state.GetGlobal(ctx)
	errs := make([]error, 0, 8)
	errs = append(errs, self.Abort(ctx))
	errs = append(errs, g.Engine.Exec(ctx, self.bill.AcceptMax(0)))
	errs = append(errs, g.Engine.Exec(ctx, self.coin.AcceptMax(0)))
	return errors.Annotate(helpers.FoldErrors(errs), tag)
}

// TeleCashbox Stored in one-way cashbox Telemetry_Money
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

// TeleChange Dispensable Telemetry_Money
func (self *MoneySystem) TeleChange(ctx context.Context) *tele_api.Telemetry_Money {
	pb := &tele_api.Telemetry_Money{
		// TODO support bill recycler Bills: make(map[uint32]uint32, bill.TypeCount),
		Coins: make(map[uint32]uint32, coin.TypeCount),
	}
	if err := self.coin.TubeStatus(); err != nil {
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

func (self *MoneySystem) XXX_InjectCoin(n currency.Nominal) error {
	self.lk.Lock()
	defer self.lk.Unlock()
	self.Log.Debugf("XXX_InjectCoin n=%d", n)
	self.coinCredit.MustAdd(n, 1)
	self.dirty += currency.Amount(n)
	return nil
}
