package inventory

import (
	"context"
	fmt "fmt"
	"testing"
	"testing/quick"

	"github.com/stretchr/testify/assert"
	"github.com/temoto/errors"
	"github.com/temoto/vender/engine"
	engine_config "github.com/temoto/vender/engine/config"
	"github.com/temoto/vender/helpers"
	"github.com/temoto/vender/log2"
)

func TestStockErrors(t *testing.T) {
	t.Parallel()

	rand := helpers.RandUnix()
	try := func(c engine_config.Stock) string {
		ctx := context.Background()
		log := log2.NewTest(t, log2.LDebug)
		e := engine.NewEngine(log)
		e.Register("fail", engine.Func0{F: func() error { return errors.New("expected error") }})
		s, err := NewStock(c, e)
		if err != nil {
			return err.Error()
		}
		initial := rand.Float32() * 100
		s.Set(initial)
		if c.RegisterAdd != "" {
			d := e.Resolve(fmt.Sprintf("add.%s(1)", c.Name))
			err = d.Validate()
			if err != nil {
				return err.Error()
			}
			err = d.Do(ctx)
			if err != nil {
				return err.Error()
			}
		}
		// Stock must not decrease on errors
		assert.Equal(t, initial, s.Value())
		return ""
	}

	assert.Equal(t, "stock name=(empty) is invalid", try(engine_config.Stock{}))
	assert.Contains(t, try(engine_config.Stock{Name: "a", RegisterAdd: "foo"}), "no arg placeholders in add.a")
	assert.Equal(t, ErrStockLow.Error(), try(engine_config.Stock{Name: "b", Check: true, SpendRate: 100, RegisterAdd: "ignore(?)"}))
	assert.Contains(t, try(engine_config.Stock{Name: "d", RegisterAdd: "ignore(?) foobar"}), "foobar not resolved")
	assert.Contains(t, try(engine_config.Stock{Name: "e", RegisterAdd: "ignore(?) fail"}), "expected error")
}

func TestSpendValue(t *testing.T) {
	t.Parallel()

	log := log2.NewTest(t, log2.LDebug)
	rand := helpers.RandUnix()
	f := func(i1, i2, i3 int32) bool {
		e := engine.NewEngine(log)
		c := engine_config.Stock{
			Name:      "quicktea",
			SpendRate: float32(i2%1000) + (float32(i2%10000) / 10000), // FIXME test wider range of spend_rate
		}
		arg := engine.Arg(uint32(i3) % 100) // FIXME test wider range of arg
		s, err := NewStock(c, e)
		if c.SpendRate == 0 {
			c.SpendRate = 1
		}
		if c.SpendRate < 0 {
			return assert.Error(t, err)
		}
		if !assert.NoError(t, err) {
			return false
		}
		spent := s.TranslateSpend(arg)
		initial := rand.Float32() * (1 << 20)
		s.Set(initial)
		err = s.spendArg(context.Background(), arg)
		if !assert.NoError(t, err) {
			return false
		}
		final := s.Value()
		return assert.Equal(t, final-initial, -spent, "spend_rate=%g initial=%g final=%g arg=%d spent=%g", c.SpendRate, initial, final, arg, spent)
	}
	assert.NoError(t, quick.Check(f, &quick.Config{MaxCount: 10000}))
}
