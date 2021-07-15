package tele

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/golang/protobuf/proto"
	// "google.golang.org/protobuf/proto"

	"github.com/juju/errors"
	"github.com/skip2/go-qrcode"
	"github.com/temoto/vender/helpers"
	"github.com/temoto/vender/internal/global"
	"github.com/temoto/vender/internal/state"
	"github.com/temoto/vender/internal/types"
	tele_api "github.com/temoto/vender/tele"
)

var (
	errInvalidArg = fmt.Errorf("invalid arg")
)

func (self *tele) onCommandMessage(ctx context.Context, payload []byte) bool {
	if self.currentState == tele_api.State_Invalid || self.currentState == tele_api.State_Boot {
		return true
	}
	cmd := new(tele_api.Command)
	err := proto.Unmarshal(payload, cmd)
	if err != nil {
		self.log.Errorf("tele command parse raw=%x err=%v", payload, err)
		// TODO reply error
		return true
	}
	self.log.Debugf("tele command raw=%x task=%#v", payload, cmd.String())

	// now := time.Now().UnixNano()
	// if cmd.Deadline != 0 && now > cmd.Deadline {
	// 	self.CommandReplyErr(cmd, fmt.Errorf("deadline"))
	// } else {
	// 	// TODO store command in persistent queue, acknowledge now, execute later
	if err = self.dispatchCommand(ctx, cmd); err != nil {
		self.CommandReplyErr(cmd, err)
		return true
	}
	self.CommandReply(cmd, tele_api.CmdReplay_done)
	state.VmcUnLock(ctx)

	return true
}

func (self *tele) dispatchCommand(ctx context.Context, cmd *tele_api.Command) error {
	switch task := cmd.Task.(type) {
	case *tele_api.Command_Report:
		return self.cmdReport(ctx, cmd)

	// case *tele_api.Command_Lock:
	// 	return self.cmdLock(ctx, cmd, task.Lock)

	case *tele_api.Command_Exec:
		return self.cmdExec(ctx, cmd, task.Exec)

	case *tele_api.Command_SetInventory:
		return self.cmdSetInventory(ctx, cmd, task.SetInventory)

	case *tele_api.Command_Stop:
		return self.cmdStop(ctx, cmd, task.Stop)

	case *tele_api.Command_Show_QR:
		return self.cmdShowQR(ctx, cmd, task.Show_QR)

	default:
		err := fmt.Errorf("unknown command=%#v", cmd)
		self.log.Error(err)
		return err
	}
}

func (self *tele) cmdReport(ctx context.Context, cmd *tele_api.Command) error {
	return errors.Annotate(self.Report(ctx, false), "cmdReport")
}

// func (self *tele) cmdLock(ctx context.Context, cmd *tele_api.Command, arg *tele_api.Command_ArgLock) error {
// 	if err := state.CheckClientWorking(); err != nil {
// 		return err
// 	}
// 	g := state.GetGlobal(ctx)
// 	global.Log.Infof("precessing lock command")
// 	g.Hardware.Input.Enable(false)
// 	return g.ScheduleSync(ctx, cmd.Priority, func(context.Context) error {
// 		time.Sleep(time.Duration(arg.Duration) * time.Second)
// 		fmt.Printf("\n\033[41m lockenddd \033[0m\n\n")
// 		g.Hardware.Input.Enable(true)
// 		return nil
// 	})
// }

func (self *tele) cmdExec(ctx context.Context, cmd *tele_api.Command, arg *tele_api.Command_ArgExec) error {
	if arg.Scenario[:1] == "_" { // If the command contains the "_" prefix, then you ignore the client lock flag
		arg.Scenario = arg.Scenario[1:]
		types.VMC.Lock = false
	}
	if types.VMC.Lock {
		global.Log.Infof("ignore income remove command (locked) from: (%v) scenario: (%s)", cmd.Executer, arg.Scenario)
		self.CommandReply(cmd, tele_api.CmdReplay_busy)
		return nil
	}
	global.Log.Infof("income remove command from: (%v) scenario: (%s)", cmd.Executer, arg.Scenario)
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
	// if arg.Scenario {
	state.VmcLock(ctx)
	// }
	if cmd.Executer > 0 {
		self.CommandReply(cmd, tele_api.CmdReplay_accepted)
	}

	err = g.ScheduleSync(ctx, cmd.Priority, doer.Do)
	err = errors.Annotate(err, "schedule")
	return err
}

func (self *tele) cmdSetInventory(ctx context.Context, cmd *tele_api.Command, arg *tele_api.Command_ArgSetInventory) error {
	if arg == nil || arg.New == nil {
		return errInvalidArg
	}

	g := state.GetGlobal(ctx)
	_, err := g.Inventory.SetTele(arg.New)
	return err
}

func (self *tele) cmdStop(ctx context.Context, cmd *tele_api.Command, arg *tele_api.Command_ArgStop) error {
	if arg == nil {
		return errInvalidArg
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

func (self *tele) cmdShowQR(ctx context.Context, cmd *tele_api.Command, arg *tele_api.Command_ArgShowQR) error {
	if arg == nil {
		return errInvalidArg
	}

	g := state.GetGlobal(ctx)
	display, err := g.Display()
	if err != nil {
		return errors.Annotate(err, "display")
	}
	if display == nil {
		return fmt.Errorf("display is not configured")
	}
	// TODO display.Layout(arg.Layout)
	// TODO border,redundancy from layout/config
	global.Log.Infof("show QR:'%v'", arg.QrText)
	return display.QR(arg.QrText, true, qrcode.High)
}
