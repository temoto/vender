package main

import (
	"github.com/temoto/vender/display"
	"log"
)

var display = &lcd.LCD{}

func displayInit(w *MultiWait, args interface{}) (err error) {
	log.Println("display-init")
	if err := display.Init(); err != nil {
		return err
	}
	display.Init4()
	return
}

func init() {
	NewAction("display-init", displayInit).RegisterGlobal()
}
