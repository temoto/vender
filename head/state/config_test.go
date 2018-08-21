package state

import (
	"strings"
	"testing"
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
			func(c *Config) bool { return c.Mdb.UartBaudrate == 9600 }},
		Case{"mdb",
			"mdb { uart_device = \"/dev/shmoo\" }",
			func(c *Config) bool { return c.Mdb.UartDevice == "/dev/shmoo" },
		},
	}
	mkCheck := func(c Case) func(*testing.T) {
		return func(t *testing.T) {
			r := strings.NewReader(c.input)
			cfg, err := ReadConfig(r)
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
