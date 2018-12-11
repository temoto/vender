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
	mega "github.com/temoto/vender/hardware/mega-client"
	"github.com/temoto/vender/helpers"
)

type Config struct {
	Hardware struct {
		IodinPath string `hcl:"iodin_path"`
		Mega      struct {
			I2CBus  int `hcl:"i2c_bus"`
			I2CAddr int `hcl:"i2c_addr"`
			Pin     int `hcl:"pin"`
		}
		Mdb struct {
			Log        bool `hcl:"log_enable"`
			Uarter     mdb.Uarter
			UartDevice string `hcl:"uart_device"`
			UartDriver string `hcl:"uart_driver"`
		}
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

func ContextWithConfig(ctx context.Context, config *Config) context.Context {
	return context.WithValue(ctx, "config", config)
}

func ReadConfig(r io.Reader) (*Config, error) {
	b, err := ioutil.ReadAll(r)
	if err != nil {
		return nil, err
	}
	c := new(Config)
	err = hcl.Unmarshal(b, c)

	switch c.Hardware.Mdb.UartDriver {
	case "", "file":
		c.Hardware.Mdb.Uarter = mdb.NewFileUart()
	case "mega":
		mega, err := mega.NewClient(byte(c.Hardware.Mega.I2CBus), byte(c.Hardware.Mega.I2CAddr), uint(c.Hardware.Mega.Pin))
		if err != nil {
			return nil, errors.Annotatef(err, "config: mdb.uart_driver=%s mega=%#v", c.Hardware.Mdb.UartDriver, c.Hardware.Mega)
		}
		c.Hardware.Mdb.Uarter = mdb.NewMegaUart(mega)
	case "iodin":
		iodin, err := iodin.NewClient(c.Hardware.IodinPath)
		if err != nil {
			return nil, errors.Annotatef(err, "config: mdb.uart_driver=%s iodin_path=%s", c.Hardware.Mdb.UartDriver, c.Hardware.IodinPath)
		}
		c.Hardware.Mdb.Uarter = mdb.NewIodinUart(iodin)
	default:
		return nil, fmt.Errorf("config: unknown mdb.uart_driver=\"%s\" valid: file, fast", c.Hardware.Mdb.UartDriver)
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
