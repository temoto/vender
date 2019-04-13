package inventory

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sync"
	"sync/atomic"

	"github.com/temoto/vender/engine"
)

var (
	ErrStockLow = errors.New("Stock is too low")
)

type Inventory struct {
	mu sync.Mutex
	m  map[string]*Stock
}

func (self *Inventory) Init() {
	self.mu.Lock()
	self.m = make(map[string]*Stock, 16)
	self.mu.Unlock()
}

func (self *Inventory) EnableAll() {
	self.mu.Lock()
	for _, s := range self.m {
		s.Enable()
	}
	self.mu.Unlock()
}
func (self *Inventory) DisableAll() {
	self.mu.Lock()
	for _, s := range self.m {
		s.Disable()
	}
	self.mu.Unlock()
}

func (self *Inventory) Register(name string, rate float32) *Stock {
	var s *Stock
	self.mu.Lock()
	if exist, ok := self.m[name]; ok {
		s = exist
	} else {
		s = &Stock{
			Name: name,
			rate: rate,
			min:  1,
		}
		s.Enable()
		self.m[name] = s
	}
	self.mu.Unlock()
	return s
}

type Stock struct {
	Name   string
	enable uint32
	rate   float32 // TODO table
	min    int32
	value  int32
}

func (self *Stock) Enabled() bool { return atomic.LoadUint32(&self.enable) == 1 }
func (self *Stock) Enable()       { atomic.StoreUint32(&self.enable, 1) }
func (self *Stock) Disable()      { atomic.StoreUint32(&self.enable, 0) }

func (self *Stock) Min() int32 { return atomic.LoadInt32(&self.min) }

func (self *Stock) Rate() float32     { return self.rate }
func (self *Stock) SetRate(r float32) { self.rate = r }

func (self *Stock) Value() int32 { return atomic.LoadInt32(&self.value) }
func (self *Stock) Set(v int32)  { atomic.StoreInt32(&self.value, v) }

func (self *Stock) Wrap1(d engine.Doer) engine.Doer {
	return do{d: d, s: self, v: self.TranslateArg(1)}
}
func (self *Stock) WrapArg(d engine.Doer) engine.Doer {
	return do{d: d, s: self}
}

// Human units to hardware units
func (self *Stock) TranslateArg(arg engine.Arg) int32 {
	if arg == 0 {
		return 0
	}
	result := int32(math.Round(float64(arg) * float64(self.Rate())))
	if result == 0 {
		return 1
	}
	return result
}

// Wraps `engine.Doer` with stock validation and decrement
type do struct {
	d    engine.Doer
	s    *Stock
	v    int32
	vset bool
}

func (self do) Validate() error {
	// For now arg-not-applied is critical code error
	// even when stock logic is disabled.
	if !self.vset {
		panic(engine.ErrArgNotApplied)
		// return engine.ErrArgNotApplied
	}

	if !self.s.Enabled() {
		return nil
	}
	min := self.s.Min()
	value := self.s.Value()
	if value-self.v < min {
		return ErrStockLow
	}
	return nil
}
func (self do) Do(ctx context.Context) error {
	// For now arg-not-applied is critical code error
	// even when stock logic is disabled.
	if !self.vset {
		panic(fmt.Sprintf("code error d=%s stock=%s arg not applied", self.d.String(), self.s.Name))
		// return engine.ErrArgNotApplied
	}

	if err := self.d.Do(ctx); err != nil {
		return err
	}
	if !self.s.Enabled() {
		return nil
	}

	// TODO support "actual" spending as reported by wrapped doer
	spent := self.v
	atomic.AddInt32(&self.s.value, -spent)
	// log.Printf("stock %s=%d", self.s.Name, self.s.value)
	return nil
}
func (self do) String() string { return self.d.String() + "/stock:" + self.s.Name }

func (self do) Apply(arg engine.Arg) engine.Doer {
	self.v = int32(arg)
	self.vset = true
	self.d = engine.ArgApply(self.d, arg)
	return self
}
func (self do) Applied() bool { return self.vset }
