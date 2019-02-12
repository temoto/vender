package money

import (
	"context"
	"log"
	"sync"
	"time"

	"github.com/juju/errors"
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

	self.events = make(chan Event, 2)
	// TODO determine if combination of errors is fatal for money subsystem
	if err := self.bs.Init(ctx, self); err != nil {
		log.Printf("head/money Start bill error=%v", errors.ErrorStack(err))
	}
	// if err := self.cs.Init(ctx, self, m); err != nil {
	// 	log.Printf("head/money Start coin error=%v", errors.ErrorStack(err))
	// 	self.cs.Stop(ctx)
	// }

	go func() {
		time.Sleep(10 * time.Second)
		log.Printf("!sim bill pause")
		self.bs.Stop(ctx)
		time.Sleep(10 * time.Second)
		log.Printf("!sim bill unpause")
		self.bs.Start(ctx, self)
	}()
	return nil
}
func (self *MoneySystem) Stop(ctx context.Context) error {
	log.Printf("head/money Stop")
	self.Abort(ctx)
	// TODO return escrow
	self.bs.Stop(ctx)
	self.cs.Stop(ctx)
	return nil
}
