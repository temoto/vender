package tele

const logMsgDisabled = "tele disabled"

func (self *Tele) Broken(flag bool) {
	if !self.enabled {
		self.log.Errorf(logMsgDisabled)
		return
	}

	newState := State_Problem
	if !flag {
		newState = State_Work
	}
	self.log.Infof("tele.Broken flag=%t state=%v", flag, newState)
	self.stateCh <- newState
}

func (self *Tele) Error(e error) {
	if !self.enabled {
		self.log.Errorf(logMsgDisabled)
		return
	}

	self.log.Errorf("tele.Error e=%v", e)
	tmerr := Telemetry_Error{
		Message: e.Error(),
	}
	if err := self.qpushTelemetry(&Telemetry{Error: &tmerr}); err != nil {
		self.log.Errorf("CRITICAL qpushTelemetry telemetry_error=%#v err=%v", tmerr, err)
	}
}

func (self *Tele) CommandChan() <-chan Command {
	if !self.enabled {
		self.log.Errorf(logMsgDisabled)
		return nil
	}

	return self.cmdCh
}

func (self *Tele) CommandReplyErr(c *Command, e error) {
	if !self.enabled {
		self.log.Errorf(logMsgDisabled)
		return
	}
	errText := ""
	if e != nil {
		errText = e.Error()
	}
	r := Response{
		CommandId: c.Id,
		Error:     errText,
	}
	err := self.qpushCommandResponse(c, &r)
	if err != nil {
		self.log.Errorf("CRITICAL command=%#v response=%#v err=%v", c, r, err)
	}
}

func (self *Tele) Service(msg string) {
	if !self.enabled {
		self.log.Errorf(logMsgDisabled)
		return
	}

	self.log.Infof("tele.Service msg=%s", msg)
	self.stateCh <- State_Service
	// FIXME send msg
}

func (self *Tele) StatModify(fun func(s *Stat)) {
	if !self.enabled {
		self.log.Errorf(logMsgDisabled)
		return
	}

	self.stat.Lock()
	fun(&self.stat)
	self.stat.Unlock()
}

func (self *Tele) Transaction(tx Telemetry_Transaction) {
	if !self.enabled {
		self.log.Errorf(logMsgDisabled)
		return
	}
	err := self.qpushTelemetry(&Telemetry{Transaction: &tx})
	if err != nil {
		self.log.Errorf("CRITICAL transaction=%#v err=%v", tx, err)
	}
}
