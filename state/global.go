package state

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

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
	Alive    *alive.Alive
	Config   *Config
	Engine   *engine.Engine
	Hardware struct {
		HD44780 struct {
			Device  *lcd.LCD
			Display atomic.Value // *lcd.TextDisplay
		}
		Input *input.Dispatch
		Mdb   struct {
			Mdber  *mdb.Mdb
			Uarter mdb.Uarter
		}
		iodin atomic.Value // *iodin.Client
		mega  atomic.Value // *mega.Client
	}
	Inventory *inventory.Inventory
	Log       *log2.Log
	Tele      tele.Tele

	lk sync.Mutex

	initDisplayOnce sync.Once
	initInputOnce   sync.Once
	initMegaOnce    sync.Once
	initMdberOnce   sync.Once

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

// If `Init` fails, consider `Global` is in broken state.
func (g *Global) Init(ctx context.Context, cfg *Config) error {
	g.Config = cfg

	errs := make([]error, 0)
	g.Inventory.Init()
	g.Tele.Init(ctx, g.Log, g.Config.Tele)
	g.initInput()

	if g.Config.Money.Scale == 0 {
		g.Config.Money.Scale = 1
		g.Log.Errorf("config: money.scale is not set")
	} else if g.Config.Money.Scale < 0 {
		err := errors.NotValidf("config: money.scale < 0")
		errs = append(errs, err)
	}
	g.Config.Money.CreditMax *= g.Config.Money.Scale
	g.Config.Money.ChangeOverCompensate *= g.Config.Money.Scale

	errs = append(errs, g.initEngine()...)

	return helpers.FoldErrors(errs)
}
func (g *Global) MustInit(ctx context.Context, cfg *Config) {
	err := g.Init(ctx, cfg)
	if err != nil {
		g.Log.Fatal(errors.ErrorStack(err))
	}
}

func (g *Global) Error(err error, args ...interface{}) {
	if err != nil {
		if len(args) != 0 {
			msg := args[0].(string)
			args = args[1:]
			err = errors.Annotatef(err, msg, args...)
		}
		g.Log.Errorf(errors.ErrorStack(err))
		g.Tele.Error(err)
	}
}

func (g *Global) initEngine() []error {
	errs := make([]error, 0)

	for _, x := range g.Config.Engine.Aliases {
		var err error
		x.Doer, err = g.Engine.ParseText(x.Name, x.Scenario)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		// g.Log.Debugf("config.engine.alias name=%s scenario=%s", x.Name, x.Scenario)
		g.Engine.Register(x.Name, x.Doer)
	}

	for _, x := range g.Config.Engine.Menu.Items {
		var err error
		x.Price = g.Config.ScaleI(x.XXX_Price)
		x.Doer, err = g.Engine.ParseText(x.Name, x.Scenario)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		// g.Log.Debugf("config.engine.menu %s pxxx=%d ps=%d", x.String(), x.XXX_Price, x.Price)
		g.Engine.Register("menu."+x.Code, x.Doer)
	}

	for _, x := range g.Config.Engine.Inventory.Stocks {
		stock, err := g.Inventory.RegisterStock(x)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		switch x.Register {
		case "-": // skip

		case "": // default
			x.Register = "@add.%[1]s(?)"
			fallthrough

		default:
			name := x.Register
			if strings.Contains(name, "%[") {
				name = fmt.Sprintf(x.Register, x.Name)
				if strings.Contains(name, "(MISSING)") || strings.Contains(name, "(EXTRA") {
					errs = append(errs, errors.Errorf("invalid stock register='%s'", x.Register))
					continue
				}
			}
			g.Log.Debugf("reg=%s s=%v", name, stock)
			g.Engine.Register(name, stock)
		}
	}

	return errs
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
