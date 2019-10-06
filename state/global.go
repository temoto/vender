package state

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/juju/errors"
	"github.com/temoto/alive"
	"github.com/temoto/vender/engine"
	"github.com/temoto/vender/engine/inventory"
	tele_api "github.com/temoto/vender/head/tele/api"
	"github.com/temoto/vender/helpers"
	"github.com/temoto/vender/log2"
)

type Global struct {
	Alive     *alive.Alive
	Config    *Config
	Engine    *engine.Engine
	Hardware  hardware // hardware.go
	Inventory *inventory.Inventory
	Log       *log2.Log
	Tele      tele_api.Teler

	XXX_money atomic.Value // *money.MoneySystem crutch to import cycle
	XXX_ui    atomic.Value // *ui.UI crutch to import cycle

	_copy_guard sync.Mutex //nolint:U1000
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
	g.Config.Tele.GetInventory = func() interface{} {
		pb := &tele_api.Inventory{Stocks: make([]*tele_api.Inventory_StockItem, 0, 16)}
		g.Inventory.Iter(func(s *inventory.Stock) {
			if s.Enabled() {
				si := &tele_api.Inventory_StockItem{
					// TODO Code: 0,
					Name:  s.Name,
					Value: int32(s.Value()),
				}
				pb.Stocks = append(pb.Stocks, si)
			}
		})
		return pb
	}
	if g.Config.Tele.PersistPath == "" {
		g.Config.Tele.PersistPath = filepath.Join(g.Config.Persist.Root, "tele")
	}
	if err := g.Tele.Init(ctx, g.Log, g.Config.Tele); err != nil {
		g.Tele = tele_api.Noop{}
		return errors.Annotate(err, "tele init")
	}

	if g.Config.Money.Scale == 0 {
		g.Config.Money.Scale = 1
		g.Log.Errorf("config: money.scale is not set")
	} else if g.Config.Money.Scale < 0 {
		return errors.NotValidf("config: money.scale < 0")
	}
	g.Config.Money.CreditMax *= g.Config.Money.Scale
	g.Config.Money.ChangeOverCompensate *= g.Config.Money.Scale

	const initTasks = 3
	wg := sync.WaitGroup{}
	wg.Add(initTasks)
	errch := make(chan error, initTasks)

	go wrapErrChan(&wg, errch, g.initInput)
	go wrapErrChan(&wg, errch, func() error { return g.initInventory(ctx) }) // storage read
	go wrapErrChan(&wg, errch, g.initEngine)
	// TODO init money system, load money state from storage

	wg.Wait()
	close(errch)

	errs := make([]error, 0, initTasks)
	// TODO engine.try-resolve-all-lazy after all other inits finished

	for e := range errch {
		errs = append(errs, e)
	}
	return helpers.FoldErrors(errs)
}

func (g *Global) MustInit(ctx context.Context, cfg *Config) {
	err := g.Init(ctx, cfg)
	if err != nil {
		g.Fatal(err)
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

func (g *Global) Fatal(err error, args ...interface{}) {
	if err != nil {
		g.Error(err, args...)
		g.Alive.Stop()
		select {
		case <-g.Alive.WaitChan():
		case <-time.After(5 * time.Second):
		}
		os.Exit(1)
	}
}

func (g *Global) initEngine() error {
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

	return helpers.FoldErrors(errs)
}

func (g *Global) initInventory(ctx context.Context) error {
	// TODO ctx should be enough
	if err := g.Inventory.Init(ctx, &g.Config.Engine.Inventory, g.Engine); err != nil {
		return err
	}
	err := g.Inventory.Persist.Init("inventory", g.Inventory, g.Config.Persist.Root, g.Config.Engine.Inventory.Persist, g.Log)
	if err == nil {
		err = g.Inventory.Persist.Load()
	}
	return errors.Annotate(err, "initInventory")
}

func wrapErrChan(wg *sync.WaitGroup, ch chan<- error, fun func() error) {
	defer wg.Done()
	if err := fun(); err != nil {
		ch <- err
	}
}

type once struct {
	sync.Mutex
	called bool
	err    error
}

func (o *once) lockedDo(f func() error) {
	if o.called {
		return
	}
	o.called = true
	o.err = f()
}
