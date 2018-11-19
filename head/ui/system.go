package ui

import (
	"context"

	"github.com/temoto/vender/hardware/lcd"
)

type UISystem struct {
	display lcd.LCD
}

func (self *UISystem) String() string                     { return "ui" }
func (self *UISystem) Validate(ctx context.Context) error { return nil }
func (self *UISystem) Start(ctx context.Context) error {
	// TODO init keyboard
	// TODO init lcd
	self.displayInit()
	return nil
}
func (self *UISystem) Stop(ctx context.Context) error { return nil }
