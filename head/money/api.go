package money

import (
	"context"
	"errors"
	"sync"
)

// head <-> money interface

var (
	lk sync.Mutex

	ErrNeedMoreMoney = errors.New("add-money")
)

func WithdrawPrepare(ctx context.Context, amount Amount) error {
	lk.Lock()
	defer lk.Unlock()
	available := Amount(0)
	// available += accumulated
	// available += bill escrow ! configurable
	if available < amount {
		return ErrNeedMoreMoney
	}
	// TODO tx(ctx).log("money-lock", amount)
	return nil
}

// Store spending to durable memory, no user initiated return after this point.
func WithdrawCommit(ctx context.Context, amount Amount) error {
	lk.Lock()
	defer lk.Unlock()
	// TODO tx(ctx).log("money-commit", amount)
	return nil
}

// Release bill escrow + inserted coins
// returns error *only* if unable to return all money
func Abort(ctx context.Context) error {
	lk.Lock()
	defer lk.Unlock()
	// TODO read change strategy from config
	// TODO bill.escrow-release
	// TODO changer.drop(accumulated coins)
	// if bill escrow disabled -> changer.drop(accumulated rest)
	return nil
}
