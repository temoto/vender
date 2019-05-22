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
		check     func(testing.TB, *Config)
		expectErr string
	}
	cases := []Case{
		{"empty", "", nil, "money.scale is not set"},
		{"mdb",
			`hardware { mdb { uart_device = "/dev/shmoo" } } money { scale = 1 }`,
			func(t testing.TB, c *Config) {
				helpers.AssertEqual(t, c.Hardware.Mdb.UartDevice, "/dev/shmoo")
			},
			"",
		},
		{"menu-items",
			`
menu {
	item "1" { name = "first" price = 13 scenario = "sleep(1s)" }
	item "2" { name = "second" price = 5 }
}
money { scale = 10 }`,
			func(t testing.TB, c *Config) {
				ok := len(c.Menu.Items) == 2 &&
					c.Menu.Items[0].Name == "first" &&
					c.Menu.Items[0].Doer.String() == c.Menu.Items[0].Name &&
					c.Menu.Items[0].Price == 130 &&
					c.Menu.Items[1].Name == "second" &&
					c.Menu.Items[1].Doer.String() == c.Menu.Items[1].Name &&
					c.Menu.Items[1].Price == 50
				if !ok {
					t.Logf("menu items:")
					for _, item := range c.Menu.Items {
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
			func(t testing.TB, c *Config) {
				helpers.AssertEqual(t, c.Money.Scale, 7)
			}, ""},
		{"include-overwrites", `
money { scale = 1 }
include "money-scale-7" {}`,
			func(t testing.TB, c *Config) {
				helpers.AssertEqual(t, c.Money.Scale, 7)
			}, ""},
	}
	mkCheck := func(c Case) func(*testing.T) {
		return func(t *testing.T) {
			log := log2.NewTest(t, log2.LDebug)
			ctx := context.Background()
			ctx = context.WithValue(ctx, log2.ContextKey, log)
			ctx = context.WithValue(ctx, engine.ContextKey, engine.NewEngine(ctx))
			fs := NewMockFullReader(map[string]string{
				"test-inline":   c.input,
				"empty":         "",
				"money-scale-7": "money{scale=7}",
			})
			cfg, err := ReadConfig(ctx, fs, "test-inline")
			if err == nil {
				err = cfg.Init(ctx)
			}
			if c.expectErr == "" {
				if err != nil {
					t.Fatalf("error expected=nil actual='%v'", errors.ErrorStack(err))
				}
				if c.check != nil {
					c.check(t, cfg)
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
	t.Logf("this test needs OS open|read|stat access to file `../vender.hcl`")
	log := log2.NewTest(t, log2.LDebug)
	ctx := context.Background()
	ctx = context.WithValue(ctx, log2.ContextKey, log)
	ctx = context.WithValue(ctx, engine.ContextKey, engine.NewEngine(ctx))
	_, err := ReadConfig(ctx, NewOsFullReader(), "../vender.hcl")
	if err != nil {
		t.Error(err)
	}
}
