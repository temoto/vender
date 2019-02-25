package ui

import (
	"fmt"
)

func (self *UISystem) Logf(format string, a ...interface{}) {
	s := fmt.Sprintf(format, a...)
	self.Log.Debugf("Log() %s", s)
	self.display.WriteString(s, 0, 0)
}
