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
	"fmt"
	"time"

	"github.com/temoto/errors"
	"github.com/temoto/vender/currency"
	"github.com/temoto/vender/state"
)

const (
	EventAbort  = "abort"
	EventCredit = "credit"
	// EventError = "error"
	EventPing = "ping"
)

type Event struct {
	created time.Time
	name    string
	amount  currency.Amount
	err     error
}

func (e *Event) Time() time.Time         { return e.created }
func (e *Event) Name() string            { return e.name }
func (e *Event) Amount() currency.Amount { return e.amount }
func (e *Event) Err() error              { return e.err }
func (e *Event) Error() string {
	if e.err == nil {
		return ""
	}
	return e.err.Error()
}
func (e *Event) String() string {
	return fmt.Sprintf("money.Event<name=%s err='%s' created=%s amount=%s>", e.name, e.Error(), e.created.Format(time.RFC3339Nano), e.amount.Format100I())
}

type EventFunc func(Event)

func (self *MoneySystem) EventFire(e Event) {
	// built-in handlers
	// switch e.name {
	// case EventAbort:
	// }

	self.subsLk.Lock()
	for _, fun := range self.subs {
		fun(e)
	}
	self.subsLk.Unlock()
}

func (self *MoneySystem) EventSubscribe(fun EventFunc) {
	self.subsLk.Lock()
	self.subs = append(self.subs, fun)
	self.subsLk.Unlock()
}

var (
	ErrNeedMoreMoney        = errors.New("add-money")
	ErrChangeRetainOverflow = errors.New("ReturnChange(retain>total)")
)

func (self *MoneySystem) locked_credit(includeEscrow bool) currency.Amount {
	result := currency.Amount(0)
	if includeEscrow {
		result += self.bill.EscrowAmount()
	}
	result += self.billCredit.Total()
	result += self.coinCredit.Total()
	result += self.giftCredit
	return result
}

func (self *MoneySystem) AcceptCredit(ctx context.Context, maxPrice currency.Amount) {
	const tag = "money.accept-credit"

	config := state.GetGlobal(ctx).Config()
	maxConfig := currency.Amount(config.Money.CreditMax)
	// Accept limit = lesser of: configured max credit or highest menu price.
	limit := maxConfig

	self.lk.Lock()
	available := self.locked_credit(true)
	self.lk.Unlock()
	if available != 0 && limit >= available {
		limit -= available
	}
	if available >= maxPrice {
		limit = 0
	}
	self.Log.Debugf("%s maxConfig=%s maxPrice=%s available=%s -> limit=%s",
		tag, maxConfig.FormatCtx(ctx), maxPrice.FormatCtx(ctx), available.FormatCtx(ctx), limit.FormatCtx(ctx))

	if err := self.bill.AcceptMax(limit).Do(ctx); err != nil {
		self.Log.Errorf("bill.AcceptMax err=%v", err)
	}
	if err := self.coin.AcceptMax(limit).Do(ctx); err != nil {
		self.Log.Errorf("coin.AcceptMax err=%v", err)
	}
}

func (self *MoneySystem) Credit(ctx context.Context) currency.Amount {
	self.lk.Lock()
	defer self.lk.Unlock()
	return self.locked_credit(true)
}

func (self *MoneySystem) SetGiftCredit(ctx context.Context, value currency.Amount) {
	const tag = "money.set-gift-credit"

	self.lk.Lock()
	// copy both values to release lock ASAP
	before, after := self.giftCredit, value
	self.giftCredit = after
	self.lk.Unlock()
	self.Log.Infof("%s before=%s after=%s", tag, before.FormatCtx(ctx), after.FormatCtx(ctx))
	go self.EventFire(Event{name: EventCredit, amount: value})
}

func (self *MoneySystem) WithdrawPrepare(ctx context.Context, amount currency.Amount) error {
	const tag = "money.withdraw-prepare"

	self.lk.Lock()
	defer self.lk.Unlock()

	self.Log.Debugf("%s amount=%s", tag, amount.FormatCtx(ctx))
	includeEscrow := true // TODO configurable
	available := self.locked_credit(includeEscrow)
	if available < amount {
		return ErrNeedMoreMoney
	}
	change := available - amount

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
			if err := self.bill.NewEscrow(true).Do(ctx); err != nil {
				err = errors.Annotate(err, tag)
				self.Log.Errorf("%s CRITICAL escrow release err=%v", tag, err)
				state.GetGlobal(ctx).Tele.Error(err)
			} else {
				self.dirty += billEscrowAmount
			}
		}

		if self.dirty != amount {
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
	if self.dirty != amount {
		self.Log.Errorf("%s CRITICAL amount=%s dirty=%s", tag, amount.FormatCtx(ctx), self.dirty.FormatCtx(ctx))
	}
	self.dirty = 0
	self.billCredit.Clear()
	self.coinCredit.Clear()
	self.giftCredit = 0
	go self.EventFire(Event{name: EventCredit})

	return nil
}

// Release bill escrow + inserted coins
// returns error *only* if unable to return all money
func (self *MoneySystem) Abort(ctx context.Context) error {
	const tag = "money-abort"
	self.lk.Lock()
	defer self.lk.Unlock()

	total := self.locked_credit(true)

	if err := self.locked_payout(ctx, total); err != nil {
		err = errors.Annotate(err, tag)
		state.GetGlobal(ctx).Tele.Error(err)
		return err
	}

	if self.dirty != 0 {
		self.Log.Errorf("%s CRITICAL (debt or code error) dirty=%s", tag, self.dirty.FormatCtx(ctx))
	}
	self.dirty = 0
	self.billCredit.Clear()
	self.coinCredit.Clear()
	self.giftCredit = 0
	go self.EventFire(Event{name: EventAbort})
	go self.EventFire(Event{name: EventCredit})

	return nil
}

func (self *MoneySystem) locked_payout(ctx context.Context, amount currency.Amount) error {
	const tag = "money.payout"
	var err error

	billEscrowAmount := self.bill.EscrowAmount()
	if billEscrowAmount != 0 && billEscrowAmount <= amount {
		if err = self.bill.NewEscrow(false).Do(ctx); err != nil {
			return errors.Annotate(err, tag)
		}
		amount -= billEscrowAmount
		if amount == 0 {
			return nil
		}
	}

	// TODO bill.recycler-release

	dispensed := new(currency.NominalGroup)
	err = self.coin.NewDispenseSmart(amount, true, dispensed).Do(ctx)
	// Warning: `dispensedAmount` may be more or less than `amount`
	dispensedAmount := dispensed.Total()
	self.Log.Debugf("%s coin total dispensed=%s", tag, dispensedAmount.FormatCtx(ctx))
	if dispensedAmount < amount {
		debt := amount - dispensedAmount
		err = errors.Annotatef(err, "debt=%s", debt.FormatCtx(ctx))
	}
	if dispensedAmount <= amount {
		self.dirty -= dispensedAmount
	} else {
		self.dirty -= amount
	}
	return err
}
