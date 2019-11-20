package tele

import (
	"context"
	"fmt"
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/juju/errors"
	tele_api "github.com/temoto/vender/head/tele/api"
	"github.com/temoto/vender/head/ui"
	"github.com/temoto/vender/state"
)

func (self *Tele) onCommandMessage(ctx context.Context, payload []byte) bool {
	cmd := new(tele_api.Command)
	err := proto.Unmarshal(payload, cmd)
	if err != nil {
		self.log.Errorf("tele command parse raw=%x err=%v", payload, err)
		// TODO reply error
		return true
	}
	self.log.Debugf("tele command raw=%x task=%#v", payload, cmd.String())
	self.dispatchCommand(ctx, cmd)
	return true
}

func (self *Tele) dispatchCommand(ctx context.Context, cmd *tele_api.Command) {
	var err error
	switch task := cmd.Task.(type) {
	case *tele_api.Command_Report:
		err = self.cmdReport(ctx, cmd)

	case *tele_api.Command_Lock:
		err = self.cmdLock(ctx, cmd, task.Lock)

	case *tele_api.Command_Exec:
		err = self.cmdExec(ctx, cmd, task.Exec)

	case *tele_api.Command_SetInventory:
		err = self.cmdSetInventory(ctx, cmd, task.SetInventory)

	default:
		err = fmt.Errorf("unknown command=%#v", cmd)
		self.log.Error(err.Error())
	}
	self.CommandReplyErr(cmd, err)
}

func (self *Tele) cmdReport(ctx context.Context, cmd *tele_api.Command) error {
	return errors.Annotate(self.Report(ctx, false), "cmdReport")
}

func (self *Tele) cmdLock(ctx context.Context, cmd *tele_api.Command, arg *tele_api.Command_ArgLock) error {
	if arg.Duration == 0 {
		ui.GetGlobal(ctx).LockEnd()
		return nil
	}
	ui.GetGlobal(ctx).LockDuration(time.Duration(arg.Duration) * time.Second)
	return nil
}

func (self *Tele) cmdExec(ctx context.Context, cmd *tele_api.Command, arg *tele_api.Command_ArgExec) error {
	g := state.GetGlobal(ctx)
	doer, err := g.Engine.ParseText("tele-exec", arg.Scenario)
	if err != nil {
		err = errors.Annotate(err, "parse")
		return err
	}
	err = doer.Validate()
	if err != nil {
		err = errors.Annotate(err, "validate")
		return err
	}

	// done := make(chan struct{})
	if arg.Lock {
		ui := ui.GetGlobal(ctx)
		if !ui.LockWait() {
			return errors.Errorf("ui.LockWait interrupted")
		}
		defer ui.LockDecrement()
	}
	err = doer.Do(ctx)
	err = errors.Annotate(err, "do")
	return err
}

func (self *Tele) cmdSetInventory(ctx context.Context, cmd *tele_api.Command, arg *tele_api.Command_ArgSetInventory) error {
	if arg == nil || arg.New == nil {
		return errors.Errorf("invalid arg")
	}

	g := state.GetGlobal(ctx)
	_, err := g.Inventory.SetTele(arg.New)
	return err
}
