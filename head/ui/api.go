package ui

import (
	"fmt"
	"log"
)

func (self *UISystem) Logf(format string, a ...interface{}) {
	s := fmt.Sprintf(format, a...)
	log.Printf("ui: Log() %s", s)
	self.display.WriteString(s, 0, 0)
}
