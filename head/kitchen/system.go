package kitchen

import (
	"context"
	"sync"

	"github.com/temoto/vender/hardware/mdb/evend"
	"github.com/temoto/vender/log2"
)

type KitchenSystem struct {
	log *log2.Log
	lk  sync.Mutex
}

func (self *KitchenSystem) String() string { return "kitchen" }
func (self *KitchenSystem) Start(ctx context.Context) error {
	self.lk.Lock()
	defer self.lk.Unlock()

	// TODO read config
	self.log = log2.ContextValueLogger(ctx, log2.ContextKey)

	// TODO func(dev Devicer) { dev.Init() && dev.Register() }
	// right now Enum does IO implicitly
	evend.Enum(ctx, nil)

	return nil
}
func (self *KitchenSystem) Validate(ctx context.Context) error { return nil }
func (self *KitchenSystem) Stop(ctx context.Context) error     { return nil }
