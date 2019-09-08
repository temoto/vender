package tele

import (
	tele_api "github.com/temoto/vender/head/tele/api"
)

const logMsgDisabled = "tele disabled"

func (self *Tele) State(s tele_api.State) {
	if !self.enabled {
		self.log.Errorf(logMsgDisabled)
		return
	}

	self.log.Infof("tele.State s=%v", s)
	self.stateCh <- s
}

func (self *Tele) Error(e error) {
	if !self.enabled {
		self.log.Errorf(logMsgDisabled)
		return
	}

	self.log.Errorf("tele.Error e=%v", e)
	tmerr := tele_api.Telemetry_Error{
		Message: e.Error(),
	}
	if err := self.qpushTelemetry(&tele_api.Telemetry{Error: &tmerr}); err != nil {
		self.log.Errorf("CRITICAL qpushTelemetry telemetry_error=%#v err=%v", tmerr, err)
	}
}

func (self *Tele) CommandReplyErr(c *tele_api.Command, e error) {
	if !self.enabled {
		self.log.Errorf(logMsgDisabled)
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
		self.log.Errorf("CRITICAL command=%#v response=%#v err=%v", c, r, err)
	}
}

func (self *Tele) StatModify(fun func(s *tele_api.Stat)) {
	if !self.enabled {
		self.log.Errorf(logMsgDisabled)
		return
	}

	self.stat.Lock()
	fun(&self.stat)
	self.stat.Unlock()
}

func (self *Tele) Transaction(tx tele_api.Telemetry_Transaction) {
	if !self.enabled {
		self.log.Errorf(logMsgDisabled)
		return
	}
	err := self.qpushTelemetry(&tele_api.Telemetry{Transaction: &tx})
	if err != nil {
		self.log.Errorf("CRITICAL transaction=%#v err=%v", tx, err)
	}
}
