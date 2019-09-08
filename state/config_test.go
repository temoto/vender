package state

import (
	"context"
	"strings"
	"testing"

	"github.com/juju/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/temoto/alive"
	"github.com/temoto/vender/engine"
	"github.com/temoto/vender/engine/inventory"
	tele_api "github.com/temoto/vender/head/tele/api"
	"github.com/temoto/vender/helpers"
	"github.com/temoto/vender/log2"
)

func TestReadConfig(t *testing.T) {
	t.Parallel()

	type Case struct {
		name      string
		input     string
		check     func(testing.TB, context.Context)
		expectErr string
	}
	cases := []Case{
		{"empty", "", func(t testing.TB, ctx context.Context) {
			// c := GetConfig(ctx)
			// TODO check defaults
		}, ""},

		{"mdb",
			`hardware { mdb { uart_device = "/dev/shmoo" } } money { scale = 1 }`,
			func(t testing.TB, ctx context.Context) {
				g := GetGlobal(ctx)
				assert.Equal(t, "/dev/shmoo", g.Config.Hardware.Mdb.UartDevice)
			},
			"",
		},

		{"alias", `
engine {
	alias "simple(?)" { scenario = "mock1 mock2(?)" }
	alias "complex" { scenario = "simple(1) simple(2)" }
}`,
			func(t testing.TB, ctx context.Context) {
				e := GetGlobal(ctx).Engine
				mock1calls, mock2calls, mockArg := 0, 0, 0
				e.Register("mock1", engine.Func0{F: func() error { mock1calls++; return nil }})
				e.Register("mock2(?)", engine.FuncArg{F: func(_ context.Context, arg engine.Arg) error {
					mock2calls++
					mockArg += int(arg)
					return nil
				}})
				err := e.Resolve("complex").Do(ctx)
				if err != nil {
					t.Error(err)
				}
				assert.Equal(t, 2, mock1calls)
				assert.Equal(t, 2, mock2calls)
				assert.Equal(t, 3, mockArg)
			},
			"",
		},

		{"menu-items",
			`
engine { menu {
	item "1" { name = "first" price = 13 scenario = "sleep(1s)" }
	item "2" { name = "second" price = 5 }
} }
money { scale = 10 }`,
			func(t testing.TB, ctx context.Context) {
				g := GetGlobal(ctx)
				items := g.Config.Engine.Menu.Items
				ok := len(items) == 2 &&
					items[0].Name == "first" &&
					items[0].Doer.String() == items[0].Name &&
					items[0].Price == 130 &&
					items[1].Name == "second" &&
					items[1].Doer.String() == items[1].Name &&
					items[1].Price == 50
				if !ok {
					t.Logf("menu items:")
					for _, item := range items {
						t.Logf("- %#v", item)
					}
					t.Fail()
				}
			},
			"",
		},

		{"include-normalize", `
money { scale = 1 }
include "./empty" {}`,
			nil, ""},

		{"include-optional", `
include "money-scale-7" {}
include "non-exist" { optional = true }`,
			func(t testing.TB, ctx context.Context) {
				g := GetGlobal(ctx)
				assert.Equal(t, 7, g.Config.Money.Scale)
			}, ""},

		{"include-overwrites", `
money { scale = 1 }
include "money-scale-7" {}`,
			func(t testing.TB, ctx context.Context) {
				g := GetGlobal(ctx)
				assert.Equal(t, 7, g.Config.Money.Scale)
			}, ""},

		{"inventory-simple", `
engine { inventory {
	stock "espresso" { spend_rate=9 }
}}`,
			func(t testing.TB, ctx context.Context) {
				g := GetGlobal(ctx)
				stock, err := g.Inventory.Get("espresso")
				assert.NoError(t, err)
				initial := helpers.RandUnix().Float32() * (1 << 20)
				stock.Set(initial)
				g.Engine.TestDo(t, ctx, "stock.espresso.spend1")
				g.Engine.TestDo(t, ctx, "stock.espresso.spend(3)")
				assert.Equal(t, float32(initial-4*9), stock.Value())
			}, ""},

		{"inventory-register", `
engine { inventory {
	stock "tea" { check=true hw_rate=0.5 register_add="tea.drop(?)" spend_rate=3 }
}}`,
			func(t testing.TB, ctx context.Context) {
				g := GetGlobal(ctx)
				stock, err := g.Inventory.Get("tea")
				stock.Set(13)
				require.NoError(t, err)
				hwarg := int32(0)
				g.Engine.Register("tea.drop(?)", engine.FuncArg{
					F: func(ctx context.Context, arg engine.Arg) error {
						hwarg = int32(arg)
						return nil
					}})
				g.Engine.TestDo(t, ctx, "add.tea(4)")
				assert.Equal(t, int32(2), hwarg)
				assert.Equal(t, float32(13-4*3), stock.Value())
			}, ""},

		{"error-syntax", `hello`, nil, "key 'hello' expected start of object"},
		{"error-include-loop", `include "include-loop" {}`, nil, "config include loop: from=include-loop include=include-loop"},
	}
	mkCheck := func(c Case) func(*testing.T) {
		return func(t *testing.T) {
			// log := log2.NewStderr(log2.LDebug) // helps with panics
			log := log2.NewTest(t, log2.LDebug)

			// XXX FIXME code duplicate from NewContext but stupid import cycle
			// ctx, g := NewContext(log)
			g := &Global{
				Alive:     alive.NewAlive(),
				Engine:    engine.NewEngine(log),
				Inventory: new(inventory.Inventory),
				Log:       log,
				Tele:      tele_api.NewStub(),
			}
			ctx := context.Background()
			ctx = context.WithValue(ctx, log2.ContextKey, log)
			ctx = context.WithValue(ctx, ContextKey, g)

			fs := NewMockFullReader(map[string]string{
				"test-inline":   c.input,
				"empty":         "",
				"money-scale-7": "money{scale=7}",
				"error-syntax":  "hello",
				"include-loop":  `include "include-loop" {}`,
			})
			cfg, err := ReadConfig(log, fs, "test-inline")
			if err == nil {
				err = g.Init(ctx, cfg)
			}
			if c.expectErr == "" {
				if err != nil {
					t.Fatalf("error expected=nil actual='%v'", errors.ErrorStack(err))
				}
				if c.check != nil {
					c.check(t, ctx)
				}
			} else {
				if !strings.Contains(err.Error(), c.expectErr) {
					t.Fatalf("error expected='%s' actual='%v'", c.expectErr, err)
				}
			}
		}
	}
	for _, c := range cases {
		t.Run(c.name, mkCheck(c))
	}
}

func TestFunctionalBundled(t *testing.T) {
	// not Parallel
	t.Logf("this test needs OS open|read|stat access to file `../vender.hcl`")

	log := log2.NewTest(t, log2.LDebug)
	MustReadConfig(log, NewOsFullReader(), "../vender.hcl")
}
