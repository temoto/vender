// Package money provides high-level interaction with money devices.
// Overview:
// - head->money: enable accepting coins and bills
//   inits required devices, starts polling
// - (parsed device status)
//   money->ui: X money inserted
// - head->money: (ready to serve product) secure transaction, release change
//   operate involved devices
package money

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/temoto/vender/currency"
)

const (
	InternalScalingFactor = 100
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

func (self *MoneySystem) Events() <-chan Event {
	return self.events
}

var (
	ErrNeedMoreMoney = errors.New("add-money")
)

func (self *MoneySystem) locked_credit(includeEscrow bool) currency.Amount {
	result := currency.Amount(0)
	// TODO loop over devices
	// result += self.bs.Credit()
	result += self.cs.credit.Total()
	return result
}

func (self *MoneySystem) Credit(ctx context.Context) currency.Amount {
	self.lk.Lock()
	defer self.lk.Unlock()
	return self.locked_credit(true)
}

func (self *MoneySystem) WithdrawPrepare(ctx context.Context, amount currency.Amount) error {
	self.lk.Lock()
	defer self.lk.Unlock()
	includeEscrow := true // TODO configurable
	available := self.locked_credit(includeEscrow)
	if available < amount {
		return ErrNeedMoreMoney
	}
	// TODO tx(ctx).log("money-lock", amount)
	return nil
}

// Store spending to durable memory, no user initiated return after this point.
func (self *MoneySystem) WithdrawCommit(ctx context.Context, amount currency.Amount) error {
	self.lk.Lock()
	defer self.lk.Unlock()
	// TODO tx(ctx).log("money-commit", amount)
	return nil
}

// Release bill escrow + inserted coins
// returns error *only* if unable to return all money
func (self *MoneySystem) Abort(ctx context.Context) error {
	self.lk.Lock()
	defer self.lk.Unlock()

	var err error
	total := self.locked_credit(true)

	// TODO bill.escrow-release

	// TODO read change strategy from config
	var coinsReturned currency.Amount
	if coinsReturned, err = self.cs.Dispense(ctx, &self.cs.credit); err != nil {
		// TODO telemetry high priority error
		self.Log.Errorf("MoneySystem.Abort err=%v", err)
		return err
	}
	self.Log.Debugf("MoneySystem.Abort coinsreturned=%v", coinsReturned.Format100I())
	self.cs.credit.Clear()
	total -= coinsReturned

	// TODO changer.drop(accumulated coins)
	// if bill escrow disabled -> changer.drop(accumulated rest)

	if total > 0 {
		return fmt.Errorf("MoneySystem.Abort yet to return %v", total.Format100I())
	}
	return nil
}
