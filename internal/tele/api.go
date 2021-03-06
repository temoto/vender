package tele

import (
	"context"
	"time"

	"github.com/juju/errors"
	"github.com/temoto/vender/internal/money"
	"github.com/temoto/vender/internal/state"
	tele_api "github.com/temoto/vender/tele"
)

const logMsgDisabled = "tele disabled"

func (self *tele) CommandReplyErr(c *tele_api.Command, e error) {
	if !self.config.Enabled {
		self.log.Infof(logMsgDisabled)
		return
	}
	errText := ""
	if e != nil {
		errText = e.Error()
	}
	r := tele_api.Response{
		CommandId: c.Id,
		Error:     errText,
	}
	err := self.qpushCommandResponse(c, &r)
	if err != nil {
		self.log.Error(errors.Annotatef(err, "CRITICAL command=%#v response=%#v", c, r))
	}
}

func (self *tele) Error(e error) {
	if !self.config.Enabled {
		self.log.Infof(logMsgDisabled)
		return
	}

	self.log.Debugf("tele.Error: " + errors.ErrorStack(e))
	tm := &tele_api.Telemetry{
		Error:        &tele_api.Telemetry_Error{Message: e.Error()},
		BuildVersion: self.config.BuildVersion,
	}
	if err := self.qpushTelemetry(tm); err != nil {
		self.log.Errorf("CRITICAL qpushTelemetry telemetry_error=%#v err=%v", tm.Error, err)
	}
}

func (self *tele) Report(ctx context.Context, serviceTag bool) error {
	if !self.config.Enabled {
		self.log.Infof(logMsgDisabled)
		return nil
	}

	g := state.GetGlobal(ctx)
	moneysys := money.GetGlobal(ctx)
	tm := &tele_api.Telemetry{
		Inventory:    g.Inventory.Tele(),
		MoneyCashbox: moneysys.TeleCashbox(ctx),
		MoneyChange:  moneysys.TeleChange(ctx),
		AtService:    serviceTag,
		BuildVersion: g.BuildVersion,
	}
	err := self.qpushTelemetry(tm)
	if err != nil {
		self.log.Errorf("CRITICAL qpushTelemetry tm=%#v err=%v", tm, err)
	}
	return err
}

func (self *tele) State(s tele_api.State) {
	if !self.config.Enabled {
		self.log.Infof(logMsgDisabled)
		return
	}

	// FIXME tests expecting blocking behavior and just select default: nothing
	tmr := time.NewTimer(100 * time.Millisecond)
	select {
	case self.stateCh <- s:
		self.log.Infof("tele.State s=%s", s)
		tmr.Stop()

	case <-tmr.C:
		self.log.Infof("tele.State s=%s chan busy, likely network problem", s)
	}
}

func (self *tele) StatModify(fun func(s *tele_api.Stat)) {
	if !self.config.Enabled {
		self.log.Infof(logMsgDisabled)
		return
	}

	self.stat.Lock()
	fun(&self.stat)
	self.stat.Unlock()
}

func (self *tele) Transaction(tx *tele_api.Telemetry_Transaction) {
	if !self.config.Enabled {
		self.log.Infof(logMsgDisabled)
		return
	}
	err := self.qpushTelemetry(&tele_api.Telemetry{Transaction: tx})
	if err != nil {
		self.log.Errorf("CRITICAL transaction=%#v err=%v", tx, err)
	}
}
