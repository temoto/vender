// Package money provides high-level interaction with money devices.
// Overview:
// - head->money: enable accepting coins and bills
//   inits required devices, starts polling
// - (parsed device status)
//   money->ui: X money inserted
// - head->money: (ready to serve product) secure transaction, release change
package money

import (
	"context"
	"github.com/juju/errors"
	"github.com/temoto/vender/currency"
	"github.com/temoto/vender/internal/global"
	"github.com/temoto/vender/internal/state"
)

var (
	ErrNeedMoreMoney        = errors.New("add-money")
	ErrChangeRetainOverflow = errors.New("ReturnChange(retain>total)")
)

type creditFlag uint16

const (
	creditInvalid = creditFlag(0)
	creditCash    = creditFlag(1 << iota)
	creditEscrow
	creditGift
	creditAll = creditCash | creditEscrow | creditGift
)

func (cf creditFlag) Contains(sub creditFlag) bool { return cf&sub != 0 }

func (self *MoneySystem) locked_credit(flag creditFlag) currency.Amount {
	result := currency.Amount(0)
	if flag.Contains(creditEscrow) {
		result += self.bill.EscrowAmount()
	}
	if flag.Contains(creditCash) {
		// result += self.dirty
		result += self.GetDirty()
		// result += self.billCredit.Total()
		// result += self.coinCredit.Total()
	}
	if flag.Contains(creditGift) {
		result += self.giftCredit
	}
	return result
}

func (self *MoneySystem) Credit(ctx context.Context) currency.Amount {
	self.lk.RLock()
	defer self.lk.RUnlock()
	return self.locked_credit(creditAll)
}

// TODO replace with WithdrawPrepare() -> []Spending{Cash: ..., Gift: ...}
func (self *MoneySystem) GetGiftCredit() currency.Amount {
	self.lk.RLock()
	c := self.giftCredit
	self.lk.RUnlock()
	return c
}

func (self *MoneySystem) SetGiftCredit(ctx context.Context, value currency.Amount) {
	const tag = "money.set-gift-credit"

	self.lk.Lock()
	// copy both values to release lock ASAP
	before, after := self.giftCredit, value
	self.giftCredit = after
	self.lk.Unlock()
	self.Log.Infof("%s before=%s after=%s", tag, before.FormatCtx(ctx), after.FormatCtx(ctx))

	// TODO notify ui-front
}

func (self *MoneySystem) WithdrawPrepare(ctx context.Context, amount currency.Amount) error {
	const tag = "money.withdraw-prepare"
	g := state.GetGlobal(ctx)

	self.lk.Lock()
	defer self.lk.Unlock()

	self.Log.Debugf("%s amount=%s", tag, amount.FormatCtx(ctx))
	available := self.locked_credit(creditAll)
	if available < amount {
		return ErrNeedMoreMoney
	}

	change := currency.Amount(0)
	// Don't give change from gift money.
	if cash := self.locked_credit(creditCash | creditEscrow); cash > amount {
		change = cash - amount
	}

	go func() {
		self.lk.Lock()
		defer self.lk.Unlock()

		if err := self.locked_payout(ctx, change); err != nil {
			err = errors.Annotate(err, tag)
			self.Log.Errorf("%s CRITICAL change err=%v", tag, err)
			state.GetGlobal(ctx).Tele.Error(err)
		}

		billEscrowAmount := self.bill.EscrowAmount()
		if billEscrowAmount != 0 {
			if err := g.Engine.Exec(ctx, self.bill.EscrowAccept()); err != nil {
				err = errors.Annotate(err, tag+"CRITICAL EscrowAccept")
				self.Log.Error(err)
			} else {
				// self.dirty += billEscrowAmount
				self.AddDirty(billEscrowAmount)
			}
		}

		// if self.dirty != amount {
		if self.GetDirty() != amount {
			self.Log.Errorf("%s CRITICAL amount=%s dirty=%s", tag, amount.FormatCtx(ctx), self.dirty.FormatCtx(ctx))
		}
	}()

	return nil
}

// Store spending to durable memory, no user initiated return after this point.
func (self *MoneySystem) WithdrawCommit(ctx context.Context, amount currency.Amount) error {
	const tag = "money.withdraw-commit"

	self.lk.Lock()
	defer self.lk.Unlock()

	self.Log.Debugf("%s amount=%s dirty=%s", tag, amount.FormatCtx(ctx), self.dirty.FormatCtx(ctx))
	// if self.dirty != amount {
	if self.GetDirty() != amount {
		self.Log.Errorf("%s CRITICAL amount=%s dirty=%s", tag, amount.FormatCtx(ctx), self.dirty.FormatCtx(ctx))
	}
	self.locked_zero()
	return nil
}

// Release bill escrow + inserted coins
// returns error *only* if unable to return all money
func (self *MoneySystem) Abort(ctx context.Context) error {
	const tag = "money-abort"
	self.lk.Lock()
	defer self.lk.Unlock()

	cash := self.locked_credit(creditCash | creditEscrow)
	self.Log.Debugf("%s cash=%s", tag, cash.FormatCtx(ctx))

	if err := self.locked_payout(ctx, cash); err != nil {
		err = errors.Annotate(err, tag)
		state.GetGlobal(ctx).Tele.Error(err)
		return err
	}

	// if self.dirty != 0 {
	if self.GetDirty() != 0 {
		self.Log.Errorf("%s CRITICAL (debt or code error) dirty=%s", tag, self.dirty.FormatCtx(ctx))
	}
	// self.dirty = 0
	self.SetDirty(0)
	self.billCredit.Clear()
	self.coinCredit.Clear()
	self.giftCredit = 0

	return nil
}

func (self *MoneySystem) locked_payout(ctx context.Context, amount currency.Amount) error {
	const tag = "money.payout"
	var err error
	g := state.GetGlobal(ctx)

	billEscrowAmount := self.bill.EscrowAmount()
	if billEscrowAmount != 0 && billEscrowAmount <= amount {
		if err = g.Engine.Exec(ctx, self.bill.EscrowReject()); err != nil {
			return errors.Annotate(err, tag)
		}
		amount -= billEscrowAmount
		if amount == 0 {
			return nil
		}
	}

	// TODO bill.recycler-release

	tubeBefore := self.coin.Tubes()
	global.Log.Infof("tubes before dispense (%v)", tubeBefore)
	dispensed := new(currency.NominalGroup)
	err = g.Engine.Exec(ctx, self.coin.NewGive(amount, true, dispensed))
	// Warning: `dispensedAmount` may be more or less than `amount`
	dispensedAmount := dispensed.Total()
	// self.Log.Debugf("%s coin total dispensed=%s", tag, dispensedAmount.FormatCtx(ctx))
	global.Log.Infof("coin total dispensed=%s", dispensedAmount.FormatCtx(ctx))
	self.coin.TubeStatus()
	tubeAfter := self.coin.Tubes()
	global.Log.Infof("tubes after dispense  (%v)", tubeAfter)
	// AlexM add check  if (tubeBefore - dispensed != tubeAfter) send tele error
	// self.g.Error(errors.Errorf("money timeout lost (%v)", credit))
	if dispensedAmount < amount {
		debt := amount - dispensedAmount
		err = errors.Annotatef(err, "debt=%s", debt.FormatCtx(ctx))
	}
	if dispensedAmount <= amount {
		// self.dirty -= dispensedAmount
		self.AddDirty(-dispensedAmount)
	} else {
		// self.dirty -= amount
		self.AddDirty(-amount)
	}
	return err
}

func (self *MoneySystem) locked_zero() {
	// self.dirty = 0
	self.SetDirty(0)
	self.billCredit.Clear()
	self.coinCredit.Clear()
	self.giftCredit = 0
}
