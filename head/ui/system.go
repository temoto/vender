package ui

import (
	"context"

	"github.com/temoto/vender/hardware/lcd"
	"github.com/temoto/vender/log2"
)

type UISystem struct {
	Log     *log2.Log
	display lcd.LCD
}

func (self *UISystem) String() string                     { return "ui" }
func (self *UISystem) Validate(ctx context.Context) error { return nil }
func (self *UISystem) Start(ctx context.Context) error {
	self.Log = log2.ContextValueLogger(ctx, log2.ContextKey)
	// TODO init keyboard
	// TODO init lcd
	self.displayInit()
	return nil
}
func (self *UISystem) Stop(ctx context.Context) error { return nil }
