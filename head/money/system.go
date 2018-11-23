package money

import (
	"context"
	"log"
	"sync"

	"github.com/juju/errors"
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
	// TODO determine if combination of errors is fatal for money subsystem
	if err := self.bs.Init(ctx, self, m); err != nil {
		self.bs.Stop(ctx)
		log.Printf("MoneySystem.Start bill error=%v", errors.ErrorStack(err))
	}
	if err := self.cs.Init(ctx, self, m); err != nil {
		self.cs.Stop(ctx)
		log.Printf("MoneySystem.Start coin error=%v", errors.ErrorStack(err))
	}
	return nil
}
func (self *MoneySystem) Stop(ctx context.Context) error {
	self.Abort(ctx)
	// TODO return escrow
	self.bs.Stop(ctx)
	self.cs.Stop(ctx)
	return nil
}
