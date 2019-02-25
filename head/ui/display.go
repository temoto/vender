package ui

func (self *UISystem) displayInit() (err error) {
	self.Log.Debugf("display-init")
	if err := self.display.Init(); err != nil {
		return err
	}
	self.display.Init4()
	return
}
