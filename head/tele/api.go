package tele

import (
	"context"

	"github.com/juju/errors"
	"github.com/temoto/vender/head/money"
	tele_api "github.com/temoto/vender/head/tele/api"
	"github.com/temoto/vender/state"
)

const logMsgDisabled = "tele disabled"

func (self *tele) CommandReplyErr(c *tele_api.Command, e error) {
	if !self.enabled {
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
	if !self.enabled {
		self.log.Infof(logMsgDisabled)
		return
	}

	self.log.Debugf("tele.Error: " + errors.ErrorStack(e))
	tmerr := tele_api.Telemetry_Error{
		Message: e.Error(),
	}
	if err := self.qpushTelemetry(&tele_api.Telemetry{Error: &tmerr}); err != nil {
		self.log.Errorf("CRITICAL qpushTelemetry telemetry_error=%#v err=%v", tmerr, err)
	}
}

func (self *tele) Report(ctx context.Context, serviceTag bool) error {
	if !self.enabled {
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
	if !self.enabled {
		self.log.Infof(logMsgDisabled)
		return
	}

	self.log.Infof("tele.State s=%v", s)
	self.stateCh <- s
}

func (self *tele) StatModify(fun func(s *tele_api.Stat)) {
	if !self.enabled {
		self.log.Infof(logMsgDisabled)
		return
	}

	self.stat.Lock()
	fun(&self.stat)
	self.stat.Unlock()
}

func (self *tele) Transaction(tx tele_api.Telemetry_Transaction) {
	if !self.enabled {
		self.log.Infof(logMsgDisabled)
		return
	}
	err := self.qpushTelemetry(&tele_api.Telemetry{Transaction: &tx})
	if err != nil {
		self.log.Errorf("CRITICAL transaction=%#v err=%v", tx, err)
	}
}
