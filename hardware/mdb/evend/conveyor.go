package evend

import (
	"context"
	"fmt"
	"time"

	"github.com/temoto/vender/engine"
)

type DeviceConveyor struct {
	Generic

	minSpeed     uint16
	calibTimeout time.Duration
	posCup       uint16
	posHoppers   [16]uint16
	posElevator  uint16

	currentPos uint16 // estimated
}

func (self *DeviceConveyor) Init(ctx context.Context) error {
	// TODO read config
	self.calibTimeout = 15 * time.Second
	self.minSpeed = 200
	self.posCup = 1560
	self.posHoppers[0] = 250
	self.posHoppers[1] = 570
	self.posHoppers[2] = 890
	self.posHoppers[3] = 1210
	self.posElevator = 1895
	err := self.Generic.Init(ctx, 0xd8, "conveyor", proto2)

	engine := engine.ContextValueEngine(ctx, engine.ContextKey)
	engine.Register("mdb.evend.conveyor_move_zero", self.NewMoveSync(0))
	engine.Register("mdb.evend.conveyor_move_mixer", self.NewMoveSync(1))
	engine.Register("mdb.evend.conveyor_move_cup", self.NewMoveSync(self.posCup))
	// TODO single action with parameter hopper index
	for i, value := range self.posHoppers {
		engine.Register(fmt.Sprintf("mdb.evend.conveyor_move_hopper(%d)", i+1), self.NewMoveSync(value))
	}
	engine.Register("mdb.evend.conveyor_move_elevator", self.NewMoveSync(self.posElevator))

	return err
}

func (self *DeviceConveyor) NewMove(position uint16) engine.Doer {
	tag := fmt.Sprintf("%s.move:%d", self.dev.Name, position)
	return engine.Func{Name: tag, F: func(ctx context.Context) error {
		// exception byte order
		arg := []byte{0x01, byte(position & 0xff), byte(position >> 8)}
		return self.CommandAction(arg)
	}}
}
func (self *DeviceConveyor) NewMoveSync(position uint16) engine.Doer {
	tag := fmt.Sprintf("%s.move_sync:%d", self.dev.Name, position)
	tx := engine.NewTransaction(tag)
	tx.Root.
		Append(self.DoWaitReady(tag)).
		Append(self.NewMove(position)).
		Append(engine.Func{Name: tag + "/custom-wait-done", F: func(ctx context.Context) error {
			var timeout time.Duration
			if position == 0 {
				timeout = self.calibTimeout
			} else {
				distance := absDiffU16(self.currentPos, position)
				eta := time.Duration(float32(distance)/float32(self.minSpeed)*1000) * time.Millisecond
				timeout = eta * 2
			}
			self.dev.Log.Debugf("%s position current=%d target=%d timeout=%s", self.logPrefix, self.currentPos, position, timeout)
			err := self.DoWaitDone(tag, timeout).Do(ctx)
			if err == nil {
				self.currentPos = position
			}
			return err
		}})
	return tx
}

func absDiffU16(a, b uint16) uint16 {
	if a >= b {
		return a - b
	}
	return b - a
}
