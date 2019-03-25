package ui

import (
	"fmt"
)

func (self *UISystem) Logf(format string, a ...interface{}) {
	s := fmt.Sprintf(format, a...)
	self.Log.Debugf("Log() %s", s)
	self.display.SetLine1(s)
}
