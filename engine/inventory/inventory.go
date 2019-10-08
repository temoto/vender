package inventory

import (
	"context"
	"sync"

	"github.com/juju/errors"
	"github.com/temoto/vender/engine"
	engine_config "github.com/temoto/vender/engine/config"
	"github.com/temoto/vender/helpers"
	"github.com/temoto/vender/log2"
	"github.com/temoto/vender/state/persist"
)

var (
	ErrStockLow = errors.New("Stock is too low")
)

type Inventory struct {
	persist.Persist
	log *log2.Log
	mu  sync.RWMutex
	ss  map[string]*Stock
}

func (self *Inventory) Init(ctx context.Context, c *engine_config.Inventory, engine *engine.Engine) error {
	self.log = log2.ContextValueLogger(ctx)

	self.mu.Lock()
	defer self.mu.Unlock()
	errs := make([]error, 0)
	self.ss = make(map[string]*Stock, len(c.Stocks))
	codes := make(map[uint32]string, len(c.Stocks))
	for _, stockConfig := range c.Stocks {
		if _, ok := self.ss[stockConfig.Name]; ok {
			errs = append(errs, errors.Errorf("stock=%s already registered", stockConfig.Name))
			continue
		}

		stock, err := NewStock(stockConfig, engine)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		self.ss[stock.Name] = stock
		if first, ok := codes[stock.Code]; !ok {
			codes[stock.Code] = stock.Name
		} else {
			self.log.Errorf("stock=%s duplicate code=%d first=%s", stock.Name, stock.Code, first)
		}
	}

	return helpers.FoldErrors(errs)
}

func (self *Inventory) EnableAll()  { self.Iter(func(s *Stock) { s.Enable() }) }
func (self *Inventory) DisableAll() { self.Iter(func(s *Stock) { s.Disable() }) }

func (self *Inventory) Get(name string) (*Stock, error) {
	self.mu.RLock()
	defer self.mu.RUnlock()
	if s, ok := self.ss[name]; ok {
		return s, nil
	}
	return nil, errors.Errorf("stock=%s is not registered", name)
}

func (self *Inventory) MustGet(f interface{ Fatal(...interface{}) }, name string) *Stock {
	s, err := self.Get(name)
	if err != nil {
		f.Fatal(err)
		return nil
	}
	return s
}

func (self *Inventory) Iter(fun func(s *Stock)) {
	self.mu.Lock()
	for _, stock := range self.ss {
		fun(stock)
	}
	self.mu.Unlock()
}

func (self *Inventory) WithTuning(ctx context.Context, stockName string, adj float32) (context.Context, error) {
	stock, err := self.Get(stockName)
	if err != nil {
		return ctx, errors.Annotate(err, "WithTuning")
	}
	ctx = context.WithValue(ctx, stock.tuneKey, adj)
	return ctx, nil
}
