package state

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/juju/errors"
	"github.com/temoto/alive"
	"github.com/temoto/iodin/client/go-iodin"
	"github.com/temoto/vender/engine"
	"github.com/temoto/vender/engine/inventory"
	keyboard "github.com/temoto/vender/hardware/evend-keyboard"
	"github.com/temoto/vender/hardware/lcd"
	"github.com/temoto/vender/hardware/mdb"
	"github.com/temoto/vender/hardware/mega-client"
	"github.com/temoto/vender/head/tele"
	"github.com/temoto/vender/helpers"
	"github.com/temoto/vender/log2"
)

type Global struct {
	c  *Config
	lk sync.Mutex

	Alive    *alive.Alive
	Engine   *engine.Engine
	Hardware struct {
		HD44780 struct {
			Device  *lcd.LCD
			Display *lcd.TextDisplay
		}
		Keyboard struct {
			Device *keyboard.Keyboard
		}
		Mdb struct {
			Mdber  *mdb.Mdb
			Uarter mdb.Uarter
		}
		Iodin atomic.Value // *iodin.Client
		Mega  atomic.Value // *mega.Client
	}

	Inventory *inventory.Inventory

	Log *log2.Log

	Tele tele.Tele
}

const contextKey = "run/state-global"

func NewContext(log *log2.Log) (context.Context, *Global) {
	if log == nil {
		panic("code error state.NewContext() log=nil")
	}

	g := &Global{
		Alive:     alive.NewAlive(),
		Engine:    engine.NewEngine(log),
		Inventory: new(inventory.Inventory),
		Log:       log,
	}
	// if ctx == nil || ctx == context.TODO() {
	ctx := context.Background()
	// }
	ctx = context.WithValue(ctx, log2.ContextKey, log)
	ctx = context.WithValue(ctx, contextKey, g)

	return ctx, g
}

func GetGlobal(ctx context.Context) *Global {
	v := ctx.Value(contextKey)
	if v == nil {
		panic(fmt.Sprintf("context['%s'] is nil", contextKey))
	}
	if g, ok := v.(*Global); ok {
		return g
	}
	panic(fmt.Sprintf("context['%s'] expected type *Global actual=%#v", contextKey, v))
}

func (g *Global) Config() *Config {
	if g.c == nil {
		panic("code error global state without Init")
	}
	return g.c
}

// If `Init` fails, consider `Global` is in broken state.
func (g *Global) Init(ctx context.Context, cfg *Config) error {
	g.c = cfg

	errs := make([]error, 0)
	g.Inventory.Init()
	g.Tele.Init(ctx, g.Log, g.c.Tele)

	if g.c.Hardware.HD44780.Enable {
		dev := new(lcd.LCD)
		if err := dev.Init(g.c.Hardware.HD44780.Pinmap); err != nil {
			return errors.Annotatef(err, "config: %#v", g.c.Hardware)
		}
		ctrl := lcd.ControlOn
		if g.c.Hardware.HD44780.ControlBlink {
			ctrl |= lcd.ControlBlink
		}
		if g.c.Hardware.HD44780.ControlCursor {
			ctrl |= lcd.ControlUnderscore
		}
		dev.SetControl(ctrl)
		g.Hardware.HD44780.Device = dev

		d, err := lcd.NewTextDisplay(
			uint16(g.c.Hardware.HD44780.Width),
			g.c.Hardware.HD44780.Codepage,
			time.Duration(g.c.Hardware.HD44780.ScrollDelay)*time.Millisecond,
		)
		if err != nil {
			return errors.Annotatef(err, "config: %#v", g.c.Hardware)
		}
		d.SetDevice(g.Hardware.HD44780.Device)
		go d.Run()
		g.Hardware.HD44780.Display = d
	}

	if g.c.Hardware.Keyboard.Enable {
		m, err := g.Mega()
		if err != nil {
			return errors.Annotatef(err, "config: keyboard needs mega")
		}
		kb, err := keyboard.NewKeyboard(m)
		if err != nil {
			return errors.Annotatef(err, "config: %#v", g.c.Hardware)
		}
		g.Hardware.Keyboard.Device = kb
	}

	if g.c.Money.Scale == 0 {
		g.c.Money.Scale = 1
		g.Log.Errorf("config: money.scale is not set")
	} else if g.c.Money.Scale < 0 {
		err := errors.NotValidf("config: money.scale < 0")
		errs = append(errs, err)
	}
	g.c.Money.CreditMax *= g.c.Money.Scale
	g.c.Money.ChangeOverCompensate *= g.c.Money.Scale

	for _, x := range g.c.Engine.Menu.Items {
		var err error
		x.Price = g.c.ScaleI(x.XXX_Price)
		x.Doer, err = g.Engine.ParseText(x.Name, x.Scenario)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		// g.Log.Debugf("config.Enginengine.menu %s pxxx=%d ps=%d", x.String(), x.XXX_Price, x.Price)
		g.Engine.Register("menu."+x.Code, x.Doer)
	}
	for _, x := range g.c.Engine.Aliases {
		var err error
		x.Doer, err = g.Engine.ParseText(x.Name, x.Scenario)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		// g.Log.Debugf("config.Enginengine.alias name=%s scenario=%s", x.Name, x.Scenario)
		g.Engine.Register(x.Name, x.Doer)
	}

	return helpers.FoldErrors(errs)
}
func (g *Global) MustInit(ctx context.Context, cfg *Config) {
	err := g.Init(ctx, cfg)
	if err != nil {
		g.Log.Fatal(errors.ErrorStack(err))
	}
}

func (g *Global) Mdber() (*mdb.Mdb, error) {
	if g.Hardware.Mdb.Mdber != nil {
		return g.Hardware.Mdb.Mdber, nil
	}

	g.lk.Lock()
	defer g.lk.Unlock()

	switch g.c.Hardware.Mdb.UartDriver {
	case "", "file":
		g.Hardware.Mdb.Uarter = mdb.NewFileUart(g.Log)
	case "mega":
		m, err := g.Mega()
		if err != nil {
			return nil, err
		}
		g.Hardware.Mdb.Uarter = mdb.NewMegaUart(m)
	case "iodin":
		iodin, err := g.Iodin()
		if err != nil {
			return nil, err
		}
		g.Hardware.Mdb.Uarter = mdb.NewIodinUart(iodin)
	default:
		return nil, fmt.Errorf("config: unknown mdb.uart_driver=\"%s\" valid: file, fast", g.c.Hardware.Mdb.UartDriver)
	}
	mdbLog := g.Log.Clone(log2.LInfo)
	if g.c.Hardware.Mdb.LogDebug {
		mdbLog.SetLevel(log2.LDebug)
	}
	m, err := mdb.NewMDB(g.Hardware.Mdb.Uarter, g.c.Hardware.Mdb.UartDevice, mdbLog)
	if err != nil {
		return nil, errors.Annotatef(err, "config: mdb=%v", g.c.Hardware.Mdb)
	}
	g.Hardware.Mdb.Mdber = m

	return g.Hardware.Mdb.Mdber, nil
}

func (g *Global) Iodin() (*iodin.Client, error) {
	if x := g.Hardware.Iodin.Load(); x != nil {
		return x.(*iodin.Client), nil
	}

	g.lk.Lock()
	defer g.lk.Unlock()
	if x := g.Hardware.Iodin.Load(); x != nil {
		return x.(*iodin.Client), nil
	}

	cfg := &g.c.Hardware
	client, err := iodin.NewClient(cfg.IodinPath)
	if err != nil {
		return nil, errors.Annotatef(err, "config: iodin_path=%s", cfg.IodinPath)
	}
	g.Hardware.Iodin.Store(client)

	return client, nil
}

func (g *Global) Mega() (*mega.Client, error) {
	if x := g.Hardware.Mega.Load(); x != nil {
		return x.(*mega.Client), nil
	}

	g.lk.Lock()
	defer g.lk.Unlock()
	if x := g.Hardware.Mega.Load(); x != nil {
		return x.(*mega.Client), nil
	}

	mcfg := &g.c.Hardware.Mega
	client, err := mega.NewClient(mcfg.Spi, mcfg.Pin, g.Log)
	if err != nil {
		return nil, errors.Annotatef(err, "config: mega=%#v", mcfg)
	}
	g.Hardware.Mega.Store(client)

	return client, nil
}

func NewTestContext(t testing.TB, confString string /* logLevel log2.Level*/) (context.Context, *Global) {
	fs := NewMockFullReader(map[string]string{
		"test-inline": confString,
	})

	log := log2.NewTest(t, log2.LDebug)
	log.SetFlags(log2.LTestFlags)
	ctx, g := NewContext(log)
	g.MustInit(ctx, MustReadConfig(log, fs, "test-inline"))

	mdber, mdbMock := mdb.NewTestMdber(t)
	g.Hardware.Mdb.Mdber = mdber
	if _, err := g.Mdber(); err != nil {
		t.Fatal(err)
	}
	ctx = context.WithValue(ctx, mdb.MockContextKey, mdbMock)

	return ctx, g
}
