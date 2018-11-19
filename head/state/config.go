package state

import (
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"

	"github.com/hashicorp/hcl"
	"github.com/juju/errors"
	iodin "github.com/temoto/vender/hardware/iodin-client"
	"github.com/temoto/vender/hardware/mdb"
	"github.com/temoto/vender/helpers"
)

type Config struct {
	IodinPath string `hcl:"iodin_path"`
	Mdb       struct {
		Log          bool `hcl:"log_enable"`
		Uarter       mdb.Uarter
		UartDevice   string `hcl:"uart_device"`
		UartDriver   string `hcl:"uart_driver"`
		UartBaudrate int    `hcl:"uart_baudrate"`
	}
	Papa struct {
		Address  string
		CertFile string
		Enabled  bool
	}
}

// Context["config"] -> *Config or panic
func GetConfig(ctx context.Context) *Config {
	v := ctx.Value("config")
	if v == nil {
		panic("context['config'] is nil")
	}
	if cfg, ok := v.(*Config); ok {
		return cfg
	}
	panic("context['config'] expected type *Config")
}

func ReadConfig(r io.Reader) (*Config, error) {
	b, err := ioutil.ReadAll(r)
	if err != nil {
		return nil, err
	}
	c := new(Config)
	err = hcl.Unmarshal(b, c)

	switch c.Mdb.UartDriver {
	case "", "file":
		c.Mdb.Uarter = mdb.NewFileUart()
	case "iodin":
		iodin, err := iodin.NewClient(c.IodinPath)
		if err != nil {
			return nil, errors.Annotatef(err, "config: mdb.uart_driver=%s iodin_path=%s", c.Mdb.UartDriver, c.IodinPath)
		}
		c.Mdb.Uarter = mdb.NewIodinUart(iodin)
	default:
		return nil, fmt.Errorf("config: unknown mdb.uart_driver=\"%s\" valid: file, fast", c.Mdb.UartDriver)
	}

	return c, err
}

func ReadConfigFile(path string) (*Config, error) {
	if pathAbs, err := filepath.Abs(path); err != nil {
		log.Printf("filepath.Abs(%s) error=%v", path, err)
	} else {
		path = pathAbs
	}
	log.Printf("reading config file %s", path)

	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return ReadConfig(f)
}

func MustReadConfig(fatal helpers.FatalFunc, r io.Reader) *Config {
	c, err := ReadConfig(r)
	if err != nil {
		fatal(err)
	}
	return c
}

func MustReadConfigFile(fatal helpers.FatalFunc, path string) *Config {
	c, err := ReadConfigFile(path)
	if err != nil {
		fatal(err)
	}
	return c
}
