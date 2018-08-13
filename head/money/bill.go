package money

import (
	"context"
	"sync"

	"github.com/temoto/alive"
)

type BillState struct {
	lk     sync.Mutex
	alive  *alive.Alive
	bank   NominalGroup
	escrow uint
}

var (
	bill BillState
)

func (self *BillState) Init(ctx context.Context) error {
	self.lk.Lock()
	defer self.lk.Unlock()
	self.alive = alive.NewAlive()
	go self.Loop(ctx)
	return nil
}

func (self *BillState) Loop(ctx context.Context) {
	defer self.alive.Done()
	for self.alive.IsRunning() {

	}
}

func (self *BillState) Stop(ctx context.Context) {
	self.alive.Stop()
	self.alive.Wait()
}
