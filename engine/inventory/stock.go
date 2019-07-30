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
	"github.com/temoto/vender/helpers/atomic_float"
)

const tuneKeyFormat = "run/inventory-%s-tune"

type Stock struct { //nolint:maligned
	Name      string
	enabled   uint32 // atomic
	check     bool
	hwRate    float32 // TODO table // FIXME concurrency
	spendRate float32
	min       float32
	value     atomic_float.F32
	tuneKey   string

	_unused_copy_guard sync.Mutex //nolint:U1000
}

func NewStock(c engine_config.Stock, e *engine.Engine) (*Stock, error) {
	if c.Name == "" {
		return nil, errors.Errorf("stock name=(empty) is invalid")
	}
	if c.HwRate == 0 {
		c.HwRate = 1
	}
	if c.SpendRate < 0 {
		return nil, errors.Errorf("stock name=%s invalid spend_rate=%f", c.Name, c.SpendRate)
	}
	if c.SpendRate == 0 {
		c.SpendRate = 1
	}
	// log.Printf("stock=%s hwRate=%f spendRate=%f", c.Name, c.HwRate, c.SpendRate)

	s := &Stock{
		Name:      c.Name,
		check:     c.Check,
		enabled:   1,
		hwRate:    c.HwRate,
		spendRate: c.SpendRate,
		min:       c.Min,
		tuneKey:   fmt.Sprintf(tuneKeyFormat, c.Name),
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

func (s *Stock) Enabled() bool { return atomic.LoadUint32(&s.enabled) == 1 }

func (s *Stock) Value() float32     { return s.value.Load() }
func (s *Stock) Set(new float32)    { s.value.Store(new) }
func (s *Stock) Has(v float32) bool { return s.value.Load()-v >= s.min }
func (s *Stock) String() string {
	return fmt.Sprintf("source(name=%s value=%f)", s.Name, s.Value())
}

func (s *Stock) Wrap(a engine.ArgApplier) engine.Doer {
	d := &custom{
		stock:  s,
		before: a,
	}
	return d
}

func (s *Stock) TranslateHw(arg engine.Arg) float32    { return translate(int32(arg), s.hwRate) }
func (s *Stock) TranslateSpend(arg engine.Arg) float32 { return translate(int32(arg), s.spendRate) }

// signature match engine.Func0.F
func (s *Stock) spend1() error {
	s.spendValue(s.TranslateSpend(1))
	return nil
}

// signature match engine.FuncArg.F
func (s *Stock) spendArg(ctx context.Context, arg engine.Arg) error {
	s.spendValue(s.TranslateSpend(arg))
	return nil
}

func (s *Stock) spendValue(v float32) {
	if s.Enabled() {
		s.value.Add(-v)
		// log.Printf("stock=%s value=%f", s.Name, s.Value())
	}
}

type custom struct {
	stock  *Stock
	before engine.ArgApplier
	after  engine.Doer
	arg    engine.Arg
	spend  float32
}

func (c *custom) Applied() bool { return c.after != nil }
func (c *custom) Apply(arg engine.Arg) engine.Doer {
	hwArg := engine.Arg(c.stock.TranslateHw(arg))
	applied := &custom{
		stock:  c.stock,
		before: c.before,
		after:  c.before.Apply(hwArg),
		arg:    arg,
		spend:  c.stock.TranslateSpend(arg),
	}
	return applied
}

func (c *custom) Validate() error {
	if err := c.after.Validate(); err != nil {
		return errors.Annotatef(err, "stock=%s", c.stock.Name)
	}
	if !c.stock.Enabled() {
		return nil
	}
	if !c.stock.check {
		return nil
	}
	if c.stock.Has(c.spend) {
		return nil
	}
	return ErrStockLow
}

func (c *custom) Do(ctx context.Context) error {
	if tunedCtx, tuneRate, ok := takeTuneRate(ctx, c.stock.tuneKey); ok {
		tunedArg := engine.Arg(math.Round(float64(c.arg) * float64(tuneRate)))
		return c.Apply(tunedArg).Do(tunedCtx)
	}

	// log.Printf("stock=%s value=%f arg=%v spending=%f", c.stock.Name, c.stock.Value(), c.arg, c.spend)
	// TODO remove this redundant check when sure that Validate() is called in all proper places
	if c.stock.check && !c.stock.Has(c.spend) {
		return errors.Errorf("stock=%s check fail", c.stock.Name)
	}

	if err := c.after.Validate(); err != nil {
		return errors.Annotatef(err, "stock=%s", c.stock.Name)
	}
	err := c.after.Do(ctx)
	if err != nil {
		return err
	}
	c.stock.spendValue(c.spend)
	return nil
}

func (c *custom) String() string {
	return fmt.Sprintf("stock.%s(%d)", c.stock.Name, c.arg)
}

func takeTuneRate(ctx context.Context, key string) (context.Context, float32, bool) {
	v := ctx.Value(key)
	if v == nil { // either no tuning or masked to avoid Do() recursion
		return ctx, 0, false
	}
	if tuneRate, ok := v.(float32); ok { // tuning found for the first time
		ctx = context.WithValue(ctx, key, nil)
		return ctx, tuneRate, true
	}
	panic(fmt.Sprintf("code error takeTuneRate(key=%s) found invalid value=%#v", key, v))
}

func translate(arg int32, rate float32) float32 {
	if arg == 0 {
		return 0
	}
	result := float32(math.Round(float64(arg) * float64(rate)))
	if result == 0 {
		return 1
	}
	return result
}
