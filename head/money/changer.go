package money

import (
	"context"
	"sync"

	"github.com/temoto/alive"
)

type ChangerState struct {
	lk    sync.Mutex
	alive *alive.Alive
	bank  NominalGroup
}

var (
	changer ChangerState
)

func (self *ChangerState) Init(ctx context.Context) error {
	self.lk.Lock()
	defer self.lk.Unlock()
	self.alive = alive.NewAlive()
	self.alive.Add(1)
	go self.Loop(ctx)
	return nil
}

func (self *ChangerState) Loop(ctx context.Context) {
	defer self.alive.Done()
	for self.alive.IsRunning() {

	}
}

func (self *ChangerState) Stop(ctx context.Context) {
	self.alive.Stop()
	self.alive.Wait()
}
