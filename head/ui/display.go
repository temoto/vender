package ui

import (
	"context"
	"log"

	"github.com/temoto/vender/display"
	"github.com/temoto/vender/head/state"
)

var display = &lcd.LCD{}

func displayInit() (err error) {
	log.Println("display-init")
	if err := display.Init(); err != nil {
		return err
	}
	display.Init4()
	return
}

func init() {
	state.RegisterStart(func(ctx context.Context) error {
		return displayInit()
	})
}
