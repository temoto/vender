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

	"github.com/temoto/alive"
	"github.com/temoto/errors"
	"github.com/temoto/vender/currency"
	"github.com/temoto/vender/hardware/money"
	"github.com/temoto/vender/head/tele"
	"github.com/temoto/vender/helpers"
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

var (
	ErrNeedMoreMoney        = errors.New("add-money")
	ErrChangeRetainOverflow = errors.New("ReturnChange(retain>total)")
)

func (self *MoneySystem) locked_credit(includeEscrow bool) currency.Amount {
	result := currency.Amount(0)
	if includeEscrow {
		result += self.bill.EscrowAmount()
	}
	result += self.dirty
	// result += self.billCredit.Total()
	// result += self.coinCredit.Total()
	result += self.giftCredit
	return result
}

func (self *MoneySystem) AcceptCredit(ctx context.Context, maxPrice currency.Amount, stopAccept <-chan struct{}, out chan<- Event) bool {
	const tag = "money.accept-credit"

	config := state.GetGlobal(ctx).Config
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

	err := self.setAcceptMax(ctx, limit)
	if err != nil || limit == 0 {
		return false // TODO unsure what is useful return value here
	}

	alive := alive.NewAlive()
	alive.Add(2)
	go self.bill.Run(ctx, alive, func(pi money.PollItem) bool {
		switch pi.Status {
		case money.StatusEscrow:
			if pi.DataCount == 1 {
				if err := self.bill.NewEscrow(true).Do(ctx); err != nil {
					self.Log.Errorf("money.bill escrow accept n=%s err=%v", currency.Amount(pi.DataNominal).FormatCtx(ctx), err)
				}
			}

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
			alive.Stop()
			if out != nil {
				out <- Event{created: itemTime, name: EventCredit, amount: pi.Amount()}
			}
		}
		return false
	})
	go self.coin.Run(ctx, alive, func(pi money.PollItem) bool {
		itemTime := time.Now()
		self.lk.Lock()
		defer self.lk.Unlock()

		switch pi.Status {
		case money.StatusDispensed:
			self.Log.Debugf("%s manual dispense: %s", tag, pi.String())
			_ = self.coin.DoTubeStatus.Do(ctx)
			_ = self.coin.CommandExpansionSendDiagStatus(nil)
		case money.StatusReturnRequest:
			alive.Stop()
			if out != nil {
				out <- Event{created: itemTime, name: EventAbort}
			}
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
			alive.Stop()
			if out != nil {
				out <- Event{created: itemTime, name: EventCredit, amount: pi.Amount()}
			}
		default:
			panic("unhandled coin POLL item: " + pi.String())
		}
		return false
	})

	select {
	case <-alive.StopChan():
		alive.Wait()
		return true
	case <-stopAccept:
		alive.Stop()
		alive.Wait()
		self.setAcceptMax(ctx, 0)
		return false
	}
}

func (self *MoneySystem) setAcceptMax(ctx context.Context, limit currency.Amount) error {
	errs := []error{
		self.bill.AcceptMax(limit).Do(ctx),
		self.coin.AcceptMax(limit).Do(ctx),
	}
	err := helpers.FoldErrors(errs)
	if err != nil {
		err = errors.Annotatef(err, "setAcceptMax limit=%s", limit.FormatCtx(ctx))
	}
	return err
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

	// TODO notify ui-front
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

	return nil
}

// Release bill escrow + inserted coins
// returns error *only* if unable to return all money
func (self *MoneySystem) Abort(ctx context.Context) error {
	const tag = "money-abort"
	self.lk.Lock()
	defer self.lk.Unlock()

	total := self.locked_credit(true)
	self.Log.Debugf("%s credit=%s", tag, total.FormatCtx(ctx))

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
