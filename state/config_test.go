package state

import (
	"context"
	"strings"
	"testing"

	"github.com/juju/errors"
	"github.com/temoto/vender/engine"
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
				c := GetGlobal(ctx).Config()
				helpers.AssertEqual(t, c.Hardware.Mdb.UartDevice, "/dev/shmoo")
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
				helpers.AssertEqual(t, mock1calls, 2)
				helpers.AssertEqual(t, mock2calls, 2)
				helpers.AssertEqual(t, mockArg, 3)
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
				c := GetGlobal(ctx).Config()
				items := c.Engine.Menu.Items
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
				c := GetGlobal(ctx).Config()
				helpers.AssertEqual(t, c.Money.Scale, 7)
			}, ""},
		{"include-overwrites", `
money { scale = 1 }
include "money-scale-7" {}`,
			func(t testing.TB, ctx context.Context) {
				c := GetGlobal(ctx).Config()
				helpers.AssertEqual(t, c.Money.Scale, 7)
			}, ""},
	}
	mkCheck := func(c Case) func(*testing.T) {
		return func(t *testing.T) {
			log := log2.NewTest(t, log2.LDebug)
			ctx, g := NewContext(log)
			fs := NewMockFullReader(map[string]string{
				"test-inline":   c.input,
				"empty":         "",
				"money-scale-7": "money{scale=7}",
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
