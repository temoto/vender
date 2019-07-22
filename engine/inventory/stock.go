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
)

type Stock struct {
	Name      string
	enabled   uint32 // atomic
	check     bool
	hwRate    float32 // TODO table // FIXME concurrency
	spendRate float32
	min       int32
	value     int32 // atomic

	_unused_copy_guard sync.Mutex //nolint:U1000
}

func NewStock(c engine_config.Stock, e *engine.Engine) (*Stock, error) {
	if c.Name == "" {
		return nil, errors.Errorf("stock name=(empty) is invalid")
	}
	if c.HwRate == 0 {
		c.HwRate = 1
	}
	if c.SpendRate == 0 {
		c.SpendRate = 1
	}

	s := &Stock{
		Name:      c.Name,
		enabled:   1,
		hwRate:    c.HwRate,
		spendRate: c.SpendRate,
		min:       c.Min,
	}

	doSpend1 := engine.Func0{
		Name: fmt.Sprintf("stock.%s.spend1", s.Name),
		F:    s.spend1,
	}
	doSpendArg := engine.FuncArg{
		Name: fmt.Sprintf("stock.%s.spend(?)", s.Name),
		F:    s.spendArg,
	}
	addName := fmt.Sprintf("add.%s(?)", s.Name)
	if c.RegisterAdd != "" {
		doAdd, err := e.ParseText(addName, c.RegisterAdd)
		if err != nil {
			return nil, errors.Annotatef(err, "stock=%s register_add", s.Name)
		}
		if doAddArg, ok := doAdd.(engine.ArgApplier); !ok {
			return nil, errors.Annotatef(err, "stock=%s register_add=%s must contain placeholder", s.Name, c.RegisterAdd)
		} else {
			e.Register(addName, s.Wrap(doAddArg))
		}
	}
	e.Register(doSpend1.Name, doSpend1)
	e.Register(doSpendArg.Name, doSpendArg)

	return s, nil
}

func (s *Stock) Enable()  { atomic.StoreUint32(&s.enabled, 1) }
func (s *Stock) Disable() { atomic.StoreUint32(&s.enabled, 0) }

func (s *Stock) Enabled() bool    { return atomic.LoadUint32(&s.enabled) == 1 }
func (s *Stock) Value() int32     { return atomic.LoadInt32(&s.value) }
func (s *Stock) Set(v int32)      { atomic.StoreInt32(&s.value, v) }
func (s *Stock) Has(v int32) bool { return s.Value()-v >= s.min }
func (s *Stock) String() string {
	return fmt.Sprintf("source(name=%s value=%d)", s.Name, s.Value())
}

func (s *Stock) Wrap(a engine.ArgApplier) engine.Doer {
	d := &custom{
		stock:  s,
		before: a,
	}
	return d
}

func (s *Stock) TranslateHw(arg engine.Arg) int32    { return translate(int32(arg), s.hwRate) }
func (s *Stock) TranslateSpend(arg engine.Arg) int32 { return translate(int32(arg), s.spendRate) }

func (s *Stock) spend1() error {
	spent := s.TranslateSpend(1)
	if s.Enabled() {
		atomic.AddInt32(&s.value, -spent)
	}
	return nil
}

func (s *Stock) spendArg(ctx context.Context, arg engine.Arg) error {
	spent := s.TranslateSpend(arg)
	if s.Enabled() {
		atomic.AddInt32(&s.value, -spent)
	}
	return nil
}

type custom struct {
	stock  *Stock
	before engine.ArgApplier
	after  engine.Doer
	arg    engine.Arg
}

func (c *custom) Applied() bool { return c.after != nil }
func (c *custom) Apply(arg engine.Arg) engine.Doer {
	hwArg := engine.Arg(c.stock.TranslateHw(arg))
	applied := &custom{
		stock: c.stock,
		after: c.before.Apply(hwArg),
		arg:   arg,
	}
	return applied
}

func (c *custom) Validate() error {
	if !c.stock.Enabled() {
		return nil
	}
	if !c.stock.check {
		return nil
	}
	spending := c.stock.TranslateSpend(c.arg)
	if c.stock.Has(spending) {
		return nil
	}
	return ErrStockLow
}

func (c *custom) Do(ctx context.Context) error {
	spending := c.stock.TranslateSpend(c.arg)
	if c.stock.check && !c.stock.Has(spending) {
		return errors.Errorf("stock=%s check fail", c.stock.Name)
	}

	if err := c.after.Validate(); err != nil {
		return errors.Annotatef(err, "stock=%s", c.stock.Name)
	}
	err := c.after.Do(ctx)
	if err != nil {
		return err
	}
	if c.stock.Enabled() {
		atomic.AddInt32(&c.stock.value, -spending)
		// log.Printf("stock %s=%d", c.stock.Name, c.stock.Value())
	}
	return nil
}

func (c *custom) String() string {
	return fmt.Sprintf("stock.%s(%d)", c.stock.Name, c.arg)
}

func translate(arg int32, rate float32) int32 {
	if arg == 0 {
		return 0
	}
	result := int32(math.Round(float64(arg) * float64(rate)))
	if result == 0 {
		return 1
	}
	return result
}
