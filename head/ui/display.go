package ui

import (
	"log"
)

func (self *UISystem) displayInit() (err error) {
	log.Println("display-init")
	if err := self.display.Init(); err != nil {
		return err
	}
	self.display.Init4()
	return
}
