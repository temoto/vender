package state

import (
	"strings"
	"testing"

	"github.com/temoto/vender/log2"
)

func TestReadConfig(t *testing.T) {
	t.Parallel()
	type Case struct {
		name      string
		input     string
		check     func(*Config) bool
		expectErr string
	}
	cases := []Case{
		{"empty", "", nil, "money.scale is not set"},
		{"mdb",
			"hardware { mdb { uart_device = \"/dev/shmoo\" } } money { scale = 1 }",
			func(c *Config) bool { return c.Hardware.Mdb.UartDevice == "/dev/shmoo" },
			"",
		},
	}
	mkCheck := func(c Case) func(*testing.T) {
		return func(t *testing.T) {
			log := log2.NewTest(t, log2.LDebug)
			r := strings.NewReader(c.input)
			cfg, err := ReadConfig(r, log)
			if c.expectErr == "" {
				if err != nil {
					t.Fatalf("error expected=nil actual='%v'", err)
				}
				if !c.check(cfg) {
					t.Errorf("invalid cfg=%v", cfg)
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
