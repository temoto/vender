package state

import (
	"fmt"

	"github.com/temoto/errors"
	"github.com/temoto/iodin/client/go-iodin"
	"github.com/temoto/vender/hardware/input"
	"github.com/temoto/vender/hardware/mdb"
	"github.com/temoto/vender/hardware/mega-client"
	"github.com/temoto/vender/log2"
)

func (g *Global) Iodin() (*iodin.Client, error) {
	if x := g.Hardware.Iodin.Load(); x != nil {
		return x.(*iodin.Client), nil
	}

	g.lk.Lock()
	defer g.lk.Unlock()
	if x := g.Hardware.Iodin.Load(); x != nil {
		return x.(*iodin.Client), nil
	}

	cfg := &g.Config.Hardware
	client, err := iodin.NewClient(cfg.IodinPath)
	if err != nil {
		return nil, errors.Annotatef(err, "config: iodin_path=%s", cfg.IodinPath)
	}
	g.Hardware.Iodin.Store(client)

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
	var client *mega.Client
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
		client, err = mega.NewClient(megaConfig, log)
		if err != nil {
			err = errors.Annotatef(err, "mega config=%#v", megaConfig)
		}
		g.Hardware.Mega.Store(client)
	})
	x := g.Hardware.Mega.Load()
	return x.(*mega.Client), err
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
