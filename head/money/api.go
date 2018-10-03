// head <-> money interface
package money

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/temoto/vender/currency"
)

const (
	EventCredit = "credit"
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

const (
	EventPing   = "ping"
	EventAmount = "amount"
	EventError  = "error"
)

type MoneySystem struct {
	events chan Event
	bs     BillState
	cs     CoinState
}

func (self *MoneySystem) Events() chan Event {
	return self.events
}

var (
	apiLock sync.Mutex
	Global  = MoneySystem{
		events: make(chan Event, 2),
	}
)

var (
	ErrNeedMoreMoney = errors.New("add-money")
)

func Credit(ctx context.Context) currency.Amount {
	apiLock.Lock()
	defer apiLock.Unlock()
	result := currency.Amount(0)
	return result
}

func WithdrawPrepare(ctx context.Context, amount currency.Amount) error {
	apiLock.Lock()
	defer apiLock.Unlock()
	available := currency.Amount(0)
	// available += accumulated
	// available += bill escrow ! configurable
	if available < amount {
		return ErrNeedMoreMoney
	}
	// TODO tx(ctx).log("money-lock", amount)
	return nil
}

// Store spending to durable memory, no user initiated return after this point.
func WithdrawCommit(ctx context.Context, amount currency.Amount) error {
	apiLock.Lock()
	defer apiLock.Unlock()
	// TODO tx(ctx).log("money-commit", amount)
	return nil
}

// Release bill escrow + inserted coins
// returns error *only* if unable to return all money
func Abort(ctx context.Context) error {
	apiLock.Lock()
	defer apiLock.Unlock()
	// TODO read change strategy from config
	// TODO bill.escrow-release
	// TODO changer.drop(accumulated coins)
	// if bill escrow disabled -> changer.drop(accumulated rest)
	return nil
}
