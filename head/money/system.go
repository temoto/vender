package money

import (
	"context"
	"sync"

	"github.com/temoto/vender/hardware/mdb"
)

type MoneySystem struct {
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

	m := mdb.ContextValueMdber(ctx, "run/mdber")
	self.events = make(chan Event, 2)
	_ = self.bs.Init(ctx, self, m)
	_ = self.cs.Init(ctx, self, m)
	return nil
}
func (self *MoneySystem) Stop(ctx context.Context) error {
	self.Abort(ctx)
	self.bs.Stop(ctx)
	self.cs.Stop(ctx)
	// TODO return escrow
	return nil
}
