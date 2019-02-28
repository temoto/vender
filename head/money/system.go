package money

import (
	"context"
	"sync"

	"github.com/juju/errors"
	"github.com/temoto/vender/log2"
)

type MoneySystem struct {
	Log    *log2.Log
	lk     sync.Mutex
	events chan Event
	bs     BillState
	cs     CoinState
}

func (self *MoneySystem) String() string                     { return "money" }
func (self *MoneySystem) Validate(ctx context.Context) error { return nil }
func (self *MoneySystem) Start(ctx context.Context) error {
	self.lk.Lock()
	defer self.lk.Unlock()
	if self.events != nil {
		panic("double Start()")
	}
	self.Log = log2.ContextValueLogger(ctx, log2.ContextKey)

	self.events = make(chan Event, 2)
	// TODO determine if combination of errors is fatal for money subsystem
	if err := self.bs.Init(ctx, self); err != nil {
		self.Log.Errorf("head/money Start bill error=%v", errors.ErrorStack(err))
	}
	// if err := self.cs.Init(ctx, self, m); err != nil {
	// 	self.Log.Errorf("head/money Start coin error=%v", errors.ErrorStack(err))
	// 	self.cs.Stop(ctx)
	// }

	return nil
}
func (self *MoneySystem) Stop(ctx context.Context) error {
	self.Log.Debugf("head/money Stop")
	self.Abort(ctx)
	// TODO return escrow
	self.bs.Stop(ctx)
	self.cs.Stop(ctx)
	return nil
}
