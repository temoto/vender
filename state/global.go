package state

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/temoto/alive"
	"github.com/temoto/errors"
	"github.com/temoto/vender/engine"
	"github.com/temoto/vender/engine/inventory"
	"github.com/temoto/vender/hardware/input"
	"github.com/temoto/vender/hardware/lcd"
	"github.com/temoto/vender/hardware/mdb"
	"github.com/temoto/vender/head/tele"
	"github.com/temoto/vender/helpers"
	"github.com/temoto/vender/log2"
)

type Global struct {
	c  *Config
	lk sync.Mutex

	initInputOnce sync.Once
	initMegaOnce  sync.Once
	initMdberOnce sync.Once

	Alive    *alive.Alive
	Engine   *engine.Engine
	Hardware struct {
		HD44780 struct {
			Device  *lcd.LCD
			Display *lcd.TextDisplay
		}
		Input *input.Dispatch
		Mdb   struct {
			Mdber  *mdb.Mdb
			Uarter mdb.Uarter
		}
		Iodin atomic.Value // *iodin.Client
		Mega  atomic.Value // *mega.Client
	}

	Inventory *inventory.Inventory
	Log       *log2.Log
	Tele      tele.Tele
	XXX_money atomic.Value // *money.MoneySystem crutch to import cycle
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
	ctx := context.Background()
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
		if err := dev.Init(g.c.Hardware.HD44780.PinChip, g.c.Hardware.HD44780.Pinmap, g.c.Hardware.HD44780.Page1); err != nil {
			return errors.Annotatef(err, "lcd.Init config=%#v", g.c.Hardware.HD44780)
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

		d, err := lcd.NewTextDisplay(&lcd.TextDisplayConfig{
			Width:       uint16(g.c.Hardware.HD44780.Width),
			Codepage:    g.c.Hardware.HD44780.Codepage,
			ScrollDelay: time.Duration(g.c.Hardware.HD44780.ScrollDelay) * time.Millisecond,
		})
		if err != nil {
			return errors.Annotatef(err, "config: %#v", g.c.Hardware)
		}
		d.SetDevice(g.Hardware.HD44780.Device)
		go d.Run()
		g.Hardware.HD44780.Display = d
	}

	g.initInput()

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
		t.Fatal(errors.Trace(err))
	}
	ctx = context.WithValue(ctx, mdb.MockContextKey, mdbMock)

	return ctx, g
}

func recoverFatal(f helpers.Fataler) {
	if x := recover(); x != nil {
		f.Fatal(x)
	}
}
