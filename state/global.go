package state

import (
	"context"
	"fmt"
	"path/filepath"
	"sync"
	"sync/atomic"

	"github.com/juju/errors"
	"github.com/temoto/alive"
	"github.com/temoto/vender/engine"
	"github.com/temoto/vender/engine/inventory"
	"github.com/temoto/vender/hardware/input"
	"github.com/temoto/vender/hardware/lcd"
	"github.com/temoto/vender/hardware/mdb"
	tele_api "github.com/temoto/vender/head/tele/api"
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
	Tele      tele_api.Teler

	lk sync.Mutex

	initDisplayOnce sync.Once
	initInputOnce   sync.Once
	initMegaOnce    sync.Once
	initMdberOnce   sync.Once

	XXX_money atomic.Value // *money.MoneySystem crutch to import cycle
}

const ContextKey = "run/state-global"

func GetGlobal(ctx context.Context) *Global {
	v := ctx.Value(ContextKey)
	if v == nil {
		panic(fmt.Sprintf("context['%s'] is nil", ContextKey))
	}
	if g, ok := v.(*Global); ok {
		return g
	}
	panic(fmt.Sprintf("context['%s'] expected type *Global actual=%#v", ContextKey, v))
}

// If `Init` fails, consider `Global` is in broken state.
func (g *Global) Init(ctx context.Context, cfg *Config) error {
	g.Config = cfg

	if g.Config.Persist.Root == "" {
		g.Config.Persist.Root = "./tmp-vender-db"
		g.Log.Errorf("config: persist.root=empty changed=%s", g.Config.Persist.Root)
		// return errors.Errorf("config: persist.root=empty")
	}
	g.Log.Debugf("config: persist.root=%s", g.Config.Persist.Root)

	// Since tele is remote error reporting mechanism, it must be inited before anything else
	if g.Config.Tele.PersistPath == "" {
		g.Config.Tele.PersistPath = filepath.Join(g.Config.Persist.Root, "tele")
	}
	if err := g.Tele.Init(ctx, g.Log, g.Config.Tele); err != nil {
		return errors.Annotate(err, "tele init")
	}

	errs := make([]error, 0)

	// TODO ctx should be enough
	if err := g.Inventory.Init(ctx, &g.Config.Engine.Inventory, g.Engine); err != nil {
		errs = append(errs, err)
	}
	{
		err := g.Inventory.Persist.Init("inventory", g.Inventory, g.Config.Persist.Root, g.Config.Engine.Inventory.Persist, g.Log)
		if err == nil {
			err = g.Inventory.Persist.Load()
		}
		if err != nil {
			g.Error(err)
			g.Tele.State(tele_api.State_Problem)
			errs = append(errs, err)
		}
	}

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

	// TODO engine.try-resolve-all-lazy

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

	return errs
}

func recoverFatal(f helpers.Fataler) {
	if x := recover(); x != nil {
		f.Fatal(x)
	}
}
