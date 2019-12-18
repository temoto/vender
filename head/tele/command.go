package tele

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/juju/errors"
	tele_api "github.com/temoto/vender/head/tele/api"
	"github.com/temoto/vender/helpers"
	"github.com/temoto/vender/state"
)

func (self *tele) onCommandMessage(ctx context.Context, payload []byte) bool {
	cmd := new(tele_api.Command)
	err := proto.Unmarshal(payload, cmd)
	if err != nil {
		self.log.Errorf("tele command parse raw=%x err=%v", payload, err)
		// TODO reply error
		return true
	}
	self.log.Debugf("tele command raw=%x task=%#v", payload, cmd.String())
	// TODO store command in persistent queue, send MQTT ack, execute later
	self.dispatchCommand(ctx, cmd)
	return true
}

func (self *tele) dispatchCommand(ctx context.Context, cmd *tele_api.Command) {
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

	case *tele_api.Command_Stop:
		err = self.cmdStop(ctx, cmd, task.Stop)

	default:
		err = fmt.Errorf("unknown command=%#v", cmd)
		self.log.Error(err.Error())
	}
	self.CommandReplyErr(cmd, err)
}

func (self *tele) cmdReport(ctx context.Context, cmd *tele_api.Command) error {
	return errors.Annotate(self.Report(ctx, false), "cmdReport")
}

func (self *tele) cmdLock(ctx context.Context, cmd *tele_api.Command, arg *tele_api.Command_ArgLock) error {
	g := state.GetGlobal(ctx)
	return g.ScheduleSync(ctx, cmd.Priority, func(context.Context) error {
		time.Sleep(time.Duration(arg.Duration) * time.Second)
		return nil
	})
}

func (self *tele) cmdExec(ctx context.Context, cmd *tele_api.Command, arg *tele_api.Command_ArgExec) error {
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

	err = g.ScheduleSync(ctx, cmd.Priority, doer.Do)
	err = errors.Annotate(err, "schedule")
	return err
}

func (self *tele) cmdSetInventory(ctx context.Context, cmd *tele_api.Command, arg *tele_api.Command_ArgSetInventory) error {
	if arg == nil || arg.New == nil {
		return errors.Errorf("invalid arg")
	}

	g := state.GetGlobal(ctx)
	_, err := g.Inventory.SetTele(arg.New)
	return err
}

func (self *tele) cmdStop(ctx context.Context, cmd *tele_api.Command, arg *tele_api.Command_ArgStop) error {
	if arg == nil {
		return errors.Errorf("invalid arg")
	}

	g := state.GetGlobal(ctx)
	// go+delay to send transport ack before process exits
	// TODO store command in persistent queue, send MQTT ack, execute later
	go func() {
		delay := helpers.IntSecondDefault(g.Config.Tele.FIXME_stopDelaySec, 7*time.Second)
		g.Log.Debugf("cmdStop arg=%s crutch delay=%v", proto.MarshalTextString(arg), delay)
		time.Sleep(delay)

		g.ScheduleSync(ctx, cmd.Priority, func(context.Context) error {
			g.Stop()

			if arg.Timeout > 0 {
				timeout := time.Duration(arg.Timeout) * time.Second
				time.AfterFunc(timeout, func() {
					if !g.StopWait(timeout) {
						g.Log.Errorf("cmdStop timeout")
						os.Exit(1)
					}
				})
			}
			return nil
		})
	}()
	return nil
}
