package inventory

import (
	"context"
	"fmt"
	"math"
	"sync"
	"sync/atomic"

	"github.com/temoto/errors"
	"github.com/temoto/vender/engine"
	engine_config "github.com/temoto/vender/engine/config"
	"github.com/temoto/vender/log2"
)

var (
	ErrStockLow = errors.New("Stock is too low")
)

type Spender interface{ engine.Doer }

type Inventory struct {
	log *log2.Log
	mu  sync.RWMutex
	m   map[string]*Stock
	ms  map[string]*Source
}

func (self *Inventory) Init() {
	self.mu.Lock()
	self.m = make(map[string]*Stock, 16)
	self.ms = make(map[string]*Source, 16)
	self.mu.Unlock()
}

func (self *Inventory) EnableAll()  { self.IterStock(func(s *Stock) { s.Enable() }) }
func (self *Inventory) DisableAll() { self.IterStock(func(s *Stock) { s.Disable() }) }

// func (self *Inventory) StateLoad(b []byte) error    { return nil }
// func (self *Inventory) StateStore() ([]byte, error) { return nil, nil }

func (self *Inventory) Get(name string) (*Stock, error) {
	self.mu.RLock()
	defer self.mu.RUnlock()
	if s, ok := self.m[name]; ok {
		return s, nil
	}
	return nil, errors.New("stock=%s is not registered")
}

func (self *Inventory) GetSource(name string) (*Source, error) {
	self.mu.RLock()
	defer self.mu.RUnlock()
	if s, ok := self.ms[name]; ok {
		return s, nil
	}
	return nil, errors.New("source=%s is not registered")
}

func (self *Inventory) RegisterStock(c engine_config.StockItem) (*Stock, error) {
	if c.Name == "" {
		return nil, errors.Errorf("stock empty name is invalid")
	}
	if c.Rate == 0 {
		return nil, errors.Errorf("stock=%s rate=undefined", c.Name)
	}

	stock := &Stock{
		Name:     c.Name,
		enabled:  1,
		strategy: c.Strategy,
	}
	if len(c.Sources) == 0 {
		stock.sources = stock._sa[:1]
		src := &stock.sources[0]
		src.Name = c.Name
		src.rate = c.Rate
		src.Min = c.Min
	} else {
		stock.sources = make([]Source, len(c.Sources))
		for i, cs := range c.Sources {
			src := &stock.sources[i]
			src.Name = cs
			src.rate = c.Rate
			src.Min = c.Min
		}
	}
	if c.Disabled {
		stock.enabled = 0
	}

	self.mu.Lock()
	defer self.mu.Unlock()
	if _, ok := self.m[c.Name]; ok {
		return nil, errors.Errorf("stock=%s already registered", c.Name)
	}
	self.m[stock.Name] = stock
	for i := range stock.sources {
		src := &stock.sources[i]
		self.ms[src.Name] = src
	}
	return stock, nil
}

func (self *Inventory) RegisterSource(name string, do engine.ArgApplier) error {
	self.mu.Lock()
	defer self.mu.Unlock()

	src, ok := self.ms[name]
	if !ok {
		return errors.Errorf("source=%s is not registered", name)
	}
	if src.do != nil {
		return errors.Errorf("source=%s is already registered spender=%#v", name, src.do)
	}
	src.do = do
	return nil
}

func (self *Inventory) IterStock(fun func(s *Stock)) {
	self.mu.RLock()
	for _, s := range self.m {
		fun(s)
	}
	self.mu.RUnlock()
}

func (self *Inventory) IterSource(fun func(s *Source)) {
	self.mu.RLock()
	for _, src := range self.ms {
		fun(src)
	}
	self.mu.RUnlock()
}

type Stock struct {
	_sa     [4]Source
	sources []Source

	Name     string
	strategy string
	enabled  uint32
}

func (self *Stock) Enable()  { atomic.StoreUint32(&self.enabled, 1) }
func (self *Stock) Disable() { atomic.StoreUint32(&self.enabled, 0) }

func (self *Stock) Default() *Source { return &self.sources[0] }

func (self *Stock) Enabled() bool { return atomic.LoadUint32(&self.enabled) == 1 }
func (self *Stock) Translate(arg engine.Arg) int32 {
	return translate(arg, self.Default().rate)
}

func (self *Stock) WithSpender(s Spender) *customSpend {
	return &customSpend{stock: self, spend: s}
}

func (self *Stock) Validate() error          { return engine.ErrArgNotApplied }
func (self *Stock) Do(context.Context) error { return engine.ErrArgNotApplied }
func (self *Stock) String() string           { return "stock." + self.Name }

func (self *Stock) Applied() bool { return false }
func (self *Stock) Apply(arg engine.Arg) engine.Doer {
	d := &do{
		arg:   arg,
		stock: self,
		ss:    make([]Spender, len(self.sources)),
	}
	for i := range self.sources {
		src := &self.sources[i]
		if src.do == nil {
			// problem likely in config
			return engine.Fail{E: errors.Errorf("stock=%s source=%s spender not registered", self.Name, src.Name)}
		}
		// FIXME cast
		doer, ok := src.do.(engine.Doer)
		if !ok {
			// problem is definitely in code
			panic(fmt.Sprintf("code error stock=%s source=%s spender must implement Doer", self.Name, src.Name))
		}
		v := translate(arg, src.rate)
		d.ss[i] = engine.ArgApply(doer, engine.Arg(v))
	}
	return d
}

type Source struct {
	Name  string
	rate  float32 // TODO table // FIXME concurrency
	Min   int32   // FIXME private, used in valve_test
	value int32
	do    engine.ArgApplier

	_unused_copy_guard sync.Mutex //nolint:U1000
}

func (self *Source) Value() int32     { return atomic.LoadInt32(&self.value) }
func (self *Source) Set(v int32)      { atomic.StoreInt32(&self.value, v) }
func (self *Source) Has(v int32) bool { return self.Value()-v >= self.Min }
func (self *Source) String() string {
	return fmt.Sprintf("source(name=%s spender_set=%t value=%d)", self.Name, self.do != nil, self.Value())
}

// Stock.Doer with applied Arg
type do struct {
	stock *Stock
	ss    []Spender
	arg   engine.Arg
}

func (self *do) Validate() error {
	if !self.stock.Enabled() {
		return nil
	}
	// log.Printf("do %s validate arg=%d", self.stock.String(), self.arg)
	for i := range self.stock.sources {
		src := &self.stock.sources[i]
		if src.Has(int32(self.arg)) {
			// log.Printf("do %s validate arg=%d src=%s has", self.stock.String(), self.arg, src.Name)
			return nil
		}
	}
	return ErrStockLow
}

func (self *do) Do(ctx context.Context) error {
	found := false
	var source *Source
	var spender Spender
	for idx := range self.stock.sources {
		source = &self.stock.sources[idx]
		if !source.Has(int32(self.arg)) {
			// log.Printf("stock=%s discard source=%s as low", self.stock.Name, source.Name)
			continue
		}
		spender = self.ss[idx]
		if spender.Validate() != nil {
			// log.Printf("stock=%s discard spender=%s as invalid", self.stock.Name, spender.String())
			continue
		}
		found = true
		break
	}
	if !found {
		return errors.Errorf("stock=%s all sources invalid", self.stock.Name)
	}
	if err := spender.Do(ctx); err != nil {
		return err
	}
	if !self.stock.Enabled() {
		return nil
	}

	// TODO support "actual" spending
	// for now, imply atomic Do: spent all requested or nothing
	spent := int32(self.arg)

	atomic.AddInt32(&source.value, -spent)
	// log.Printf("stock %s=%d", self.stock.Name, self.stock.Value())
	return nil
}
func (self *do) String() string { return "stock:" + self.stock.Name }

// Stock.Doer with custom spender
type customSpend struct {
	stock *Stock
	spend Spender
}

func (cs customSpend) Validate() error          { return engine.ErrArgNotApplied }
func (cs customSpend) Do(context.Context) error { return engine.ErrArgNotApplied }
func (cs customSpend) String() string           { return cs.stock.String() }

func (cs customSpend) Applied() bool { return false }
func (cs customSpend) Apply(arg engine.Arg) engine.Doer {
	// TODO stock.select
	source := cs.stock.Default()
	v := translate(arg, source.rate)
	applied := engine.ArgApply(cs.spend, engine.Arg(v))
	d := &do{
		arg:   arg,
		stock: cs.stock,
		ss:    []Spender{applied},
	}
	return d
}

// Human units to hardware units
func translate(arg engine.Arg, rate float32) int32 {
	if arg == 0 {
		return 0
	}
	result := int32(math.Round(float64(arg) * float64(rate)))
	if result == 0 {
		return 1
	}
	return result
}
