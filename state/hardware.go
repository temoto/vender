package state

import (
	"fmt"
	"time"

	"github.com/juju/errors"
	"github.com/temoto/iodin/client/go-iodin"
	"github.com/temoto/vender/hardware/input"
	"github.com/temoto/vender/hardware/lcd"
	"github.com/temoto/vender/hardware/mdb"
	"github.com/temoto/vender/hardware/mega-client"
	"github.com/temoto/vender/log2"
)

func (g *Global) Iodin() (*iodin.Client, error) {
	if x := g.Hardware.iodin.Load(); x != nil {
		return x.(*iodin.Client), nil
	}

	g.lk.Lock()
	defer g.lk.Unlock()
	if x := g.Hardware.iodin.Load(); x != nil {
		return x.(*iodin.Client), nil
	}

	cfg := &g.Config.Hardware
	client, err := iodin.NewClient(cfg.IodinPath)
	if err != nil {
		return nil, errors.Annotatef(err, "config: iodin_path=%s", cfg.IodinPath)
	}
	g.Hardware.iodin.Store(client)

	return client, nil
}

func (g *Global) Mdber() (*mdb.Mdb, error) {
	var err error

	g.initMdberOnce.Do(func() {
		defer recoverFatal(g.Log) // fix sync.Once silent panic

		// This may only be already set by NewTestContext()
		// TODO assert it's test runner?
		if g.Hardware.Mdb.Mdber != nil {
			return
		}

		switch g.Config.Hardware.Mdb.UartDriver {
		case "file":
			g.Hardware.Mdb.Uarter = mdb.NewFileUart(g.Log)
		case "mega":
			var mc *mega.Client
			mc, err = g.Mega()
			if err != nil {
				err = errors.Annotate(err, "Mdber() driver=mega")
				return
			}
			g.Hardware.Mdb.Uarter = mdb.NewMegaUart(mc)
		case "iodin":
			var iodin *iodin.Client
			iodin, err = g.Iodin()
			if err != nil {
				err = errors.Annotate(err, "Mdber() driver=iodin")
				return
			}
			g.Hardware.Mdb.Uarter = mdb.NewIodinUart(iodin)
		default:
			err = fmt.Errorf("config: unknown mdb.uart_driver=\"%s\" valid: file, mega, iodin", g.Config.Hardware.Mdb.UartDriver)
			return
		}
		mdbLog := g.Log.Clone(log2.LInfo)
		if g.Config.Hardware.Mdb.LogDebug {
			mdbLog.SetLevel(log2.LDebug)
		}

		var mdber *mdb.Mdb
		mdber, err = mdb.NewMDB(g.Hardware.Mdb.Uarter, g.Config.Hardware.Mdb.UartDevice, mdbLog)
		if err != nil {
			err = errors.Annotatef(err, "config: mdb=%v", g.Config.Hardware.Mdb)
			return
		}
		g.Hardware.Mdb.Mdber = mdber
	})

	return g.Hardware.Mdb.Mdber, err
}

func (g *Global) Mega() (*mega.Client, error) {
	var err error
	g.initMegaOnce.Do(func() {
		defer recoverFatal(g.Log) // fix sync.Once silent panic
		devConfig := &g.Config.Hardware.Mega
		megaConfig := &mega.Config{
			SpiBus:        devConfig.Spi,
			NotifyPinChip: devConfig.PinChip,
			NotifyPinName: devConfig.Pin,
		}
		log := g.Log.Clone(log2.LInfo)
		if devConfig.LogDebug {
			log.SetLevel(log2.LDebug)
		}
		var client *mega.Client
		client, err = mega.NewClient(megaConfig, log)
		if err != nil {
			err = errors.Annotatef(err, "mega config=%#v", megaConfig)
		}
		g.Hardware.mega.Store(client)
	})
	x := g.Hardware.mega.Load()
	return x.(*mega.Client), err
}

func (g *Global) MustDisplay() *lcd.TextDisplay {
	d, err := g.Display()
	if err != nil {
		g.Log.Fatal(err)
	}
	if d == nil {
		g.Log.Fatal("display is not available")
	}
	return d
}

func (g *Global) Display() (*lcd.TextDisplay, error) {
	var err error
	g.initDisplayOnce.Do(func() {
		defer recoverFatal(g.Log) // fix sync.Once silent panic
		devConfig := &g.Config.Hardware.HD44780
		if !devConfig.Enable {
			g.Log.Infof("display hardware disabled")
			return
		}

		dev := new(lcd.LCD)
		if err := dev.Init(devConfig.PinChip, devConfig.Pinmap, devConfig.Page1); err != nil {
			err = errors.Annotatef(err, "lcd.Init config=%#v", devConfig)
			return
		}
		ctrl := lcd.ControlOn
		if devConfig.ControlBlink {
			ctrl |= lcd.ControlBlink
		}
		if devConfig.ControlCursor {
			ctrl |= lcd.ControlUnderscore
		}
		dev.SetControl(ctrl)
		g.Hardware.HD44780.Device = dev

		displayConfig := &lcd.TextDisplayConfig{
			Width:       uint16(devConfig.Width),
			Codepage:    devConfig.Codepage,
			ScrollDelay: time.Duration(devConfig.ScrollDelay) * time.Millisecond,
		}
		var d *lcd.TextDisplay
		d, err = lcd.NewTextDisplay(displayConfig)
		if err != nil {
			err = errors.Annotatef(err, "lcd.NewTextDisplay config=%#v", displayConfig)
			return
		}
		d.SetDevice(g.Hardware.HD44780.Device)
		go d.Run()
		g.Hardware.HD44780.Display.Store(d)
	})
	if x := g.Hardware.HD44780.Display.Load(); x != nil {
		return x.(*lcd.TextDisplay), err
	}
	return nil, err
}

func (g *Global) initInput() {
	g.initInputOnce.Do(func() {
		defer recoverFatal(g.Log) // fix sync.Once silent panic
		g.Hardware.Input = input.NewDispatch(g.Log, g.Alive.StopChan())

		// support more input sources here
		sources := make([]input.Source, 0, 4)

		if src, err := g.initInputEvendKeyboard(); err != nil {
			g.Log.Error(errors.ErrorStack(err))
		} else if src != nil {
			sources = append(sources, src)
		}

		if !g.Config.Hardware.Input.DevInputEvent.Enable {
			g.Log.Infof("input=%s disabled", input.DevInputEventTag)
		} else {
			src, err := input.NewDevInputEventSource(g.Config.Hardware.Input.DevInputEvent.Device)
			err = errors.Annotatef(err, "input=%s", input.DevInputEventTag)
			if err != nil {
				g.Log.Error(errors.ErrorStack(err))
			} else if src != nil {
				sources = append(sources, src)
			}
		}

		go g.Hardware.Input.Run(sources)
	})
}

func (g *Global) initInputEvendKeyboard() (input.Source, error) {
	const tag = input.EvendKeyboardSourceTag
	if !g.Config.Hardware.Input.EvendKeyboard.Enable {
		g.Log.Infof("input=%s disabled", tag)
		return nil, nil
	}

	mc, err := g.Mega()
	if err != nil {
		err = errors.Annotatef(err, "input=%s", tag)
		err = errors.Annotatef(err, "config: keyboard needs mega")
		return nil, err
	}
	ekb, err := input.NewEvendKeyboard(mc)
	if err != nil {
		err = errors.Annotatef(err, "input=%s", tag)
		err = errors.Annotatef(err, "config: %#v", g.Config.Hardware.Input)
		return nil, err
	}
	return ekb, nil
}
