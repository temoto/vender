package inventory

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/temoto/vender/engine"
)

var (
	ErrEmpty = errors.New("Stock is too low")
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

func (self *Inventory) Register(name string) *Stock {
	var s *Stock
	self.mu.Lock()
	if exist, ok := self.m[name]; ok {
		s = exist
	} else {
		s = &Stock{
			Name: name,
			rate: 1.0,
			min:  1,
		}
		self.m[name] = s
	}
	self.mu.Unlock()
	return s
}

type Stock struct {
	Name  string
	rate  float32 // TODO table
	min   int32
	value int32
}

func (self *Stock) Min() int32   { return atomic.LoadInt32(&self.min) }
func (self *Stock) Value() int32 { return atomic.LoadInt32(&self.value) }
func (self *Stock) Set(v int32)  { atomic.StoreInt32(&self.value, v) }

func (self *Stock) WrapConst(d engine.Doer, arg int32) engine.Doer {
	return do{d: d, s: self, v: translateConst(arg, self.rate)}
}
func (self *Stock) Wrap1(d engine.Doer) engine.Doer {
	return self.WrapConst(d, 1)
}
func (self *Stock) WrapArg(d engine.Doer) engine.Doer {
	return do{d: d, s: self}
}

func translateConst(arg int32, rate float32) int32 {
	if arg == 0 {
		return 0
	}
	result := int32(float32(arg) * rate)
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

	if /*TODO spending validation disabled*/ false {
		return nil
	}
	min := self.s.Min()
	value := self.s.Value()
	if value-self.v < min {
		return ErrEmpty
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
	if /*TODO decrement disabled*/ false {
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
