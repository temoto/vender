package papa

import (
	"context"
	"log"

	"github.com/temoto/alive"
	"github.com/temoto/vender/head/state"
)

type PapaSystem struct {
	alive *alive.Alive
	// c *PapaClient
}

func (self *PapaSystem) String() string                     { return "papa" }
func (self *PapaSystem) Validate(ctx context.Context) error { return nil }
func (self *PapaSystem) Start(ctx context.Context) error {
	if self.alive != nil {
		panic("double Start()")
	}
	config := state.GetConfig(ctx)
	if !config.Papa.Enabled {
		log.Printf("head/papa system disabled in config")
		return nil
	}

	self.alive = alive.NewAlive()
	self.alive.Add(1)
	go self.netLoop(ctx)
	return nil
}
func (self *PapaSystem) Stop(ctx context.Context) error { return nil }
