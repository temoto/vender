package evend

import (
	"context"
	"fmt"
	"time"

	"github.com/juju/errors"
	"github.com/temoto/vender/helpers"
	"github.com/temoto/vender/internal/engine"
	"github.com/temoto/vender/internal/state"
	"github.com/temoto/vender/internal/global"
)

type DeviceElevator struct { //nolint:maligned
	Generic

	// earlyPos    int16 // estimated
	// currentPos  int16 // estimated
	moveTimeout time.Duration
}

func (self *DeviceElevator) init(ctx context.Context) error {
	g := state.GetGlobal(ctx)
	config := &g.Config.Hardware.Evend.Elevator
	keepaliveInterval := helpers.IntMillisecondDefault(config.KeepaliveMs, 0)
	self.moveTimeout = helpers.IntSecondDefault(config.MoveTimeoutSec, 10*time.Second)
	self.Generic.Init(ctx, 0xd0, "elevator", proto1)

	g.Engine.Register(self.name+".move(?)",
		engine.FuncArg{Name: self.name + ".move", F: func(ctx context.Context, arg engine.Arg) error {
			return g.Engine.Exec(ctx, self.move(uint8(arg)))
		}})

	g.Engine.RegisterNewFunc(
		"elevator.status",
		func(ctx context.Context) error {
			g.Log.Infof("position:%s", global.GetEnv(self.name))
			return nil
		},
	)

	err := self.Generic.FIXME_initIO(ctx)
	if keepaliveInterval > 0 {
		go self.Generic.dev.Keepalive(keepaliveInterval, g.Alive.StopChan())
	}
	return errors.Annotate(err, self.name+".init")
}

func (self *DeviceElevator) move(position uint8) engine.Doer {
	cp := global.GetEnv(self.name)
	mp := fmt.Sprintf("%s->%d", cp, position)
	// self.currentPos = -1
	global.SetEnv(self.name,mp)
	tag := fmt.Sprintf("%s.move:%s", self.name, mp)
	// fmt.Printf("\n\033[41m ENV (%v) \033[0m\n\n",helpers.GetEnv("EL"))
	return engine.NewSeq(tag).
		Append(self.NewWaitReady(tag)).
		Append(self.Generic.NewAction(tag, 0x03, position, 0x64)).
		Append(self.NewWaitDone(tag, self.moveTimeout)).
		Append(engine.Func0{F: func() error { global.SetEnv(self.name,fmt.Sprintf("%d",position)); return nil }})
}
