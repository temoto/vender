package state

import (
	"strings"
	"testing"

	"github.com/temoto/vender/log2"
)

func TestReadConfig(t *testing.T) {
	t.Parallel()
	type Case struct {
		name  string
		input string
		check func(*Config) bool
	}
	cases := []Case{
		Case{"empty", "",
			func(c *Config) bool { return !c.Hardware.Mdb.Log }},
		Case{"mdb",
			"hardware { mdb { uart_device = \"/dev/shmoo\" } }",
			func(c *Config) bool { return c.Hardware.Mdb.UartDevice == "/dev/shmoo" },
		},
	}
	mkCheck := func(c Case) func(*testing.T) {
		return func(t *testing.T) {
			log := log2.NewTest(t, log2.LDebug)
			r := strings.NewReader(c.input)
			cfg, err := ReadConfig(r, log)
			if err != nil {
				t.Fatal(err)
			}
			if !c.check(cfg) {
				t.Errorf("invalid cfg=%v", *cfg)
			}
		}
	}
	for _, c := range cases {
		t.Run(c.name, mkCheck(c))
	}
}
