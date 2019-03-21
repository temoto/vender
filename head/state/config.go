package state

import (
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hashicorp/hcl"
	"github.com/juju/errors"
	iodin "github.com/temoto/iodin/client/go-iodin"
	"github.com/temoto/vender/engine"
	"github.com/temoto/vender/hardware/mdb"
	mega "github.com/temoto/vender/hardware/mega-client"
	"github.com/temoto/vender/log2"
)

type Config struct {
	Hardware struct {
		IodinPath string `hcl:"iodin_path"`
		// TODO KeyboardListenAddr int
		Mega struct {
			Spi string `hcl:"spi"`
			Pin string `hcl:"pin"`
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

func ReadConfig(r io.Reader, log *log2.Log) (*Config, error) {
	b, err := ioutil.ReadAll(r)
	if err != nil {
		return nil, err
	}
	c := new(Config)
	err = hcl.Unmarshal(b, c)

	switch c.Hardware.Mdb.UartDriver {
	case "", "file":
		c.Hardware.Mdb.Uarter = mdb.NewFileUart(log)
	case "mega":
		mega, err := mega.NewClient(c.Hardware.Mega.Spi, c.Hardware.Mega.Pin, log)
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

func ReadConfigFile(path string, log *log2.Log) (*Config, error) {
	if pathAbs, err := filepath.Abs(path); err != nil {
		log.Errorf("filepath.Abs(%s) error=%v", path, err)
	} else {
		path = pathAbs
	}
	log.Debugf("reading config file %s", path)

	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return ReadConfig(f, log)
}

func MustReadConfig(r io.Reader, log *log2.Log) *Config {
	c, err := ReadConfig(r, log)
	if err != nil {
		log.Fatal(err)
	}
	return c
}

func MustReadConfigFile(path string, log *log2.Log) *Config {
	c, err := ReadConfigFile(path, log)
	if err != nil {
		log.Fatal(err)
	}
	return c
}

func NewTestContext(t testing.TB, config string, logLevel log2.Level) context.Context {
	ctx := context.Background()
	log := log2.NewTest(t, logLevel)
	log.SetFlags(log2.LTestFlags)
	ctx = context.WithValue(ctx, log2.ContextKey, log)
	ctx = ContextWithConfig(ctx, MustReadConfig(strings.NewReader(config), log))
	ctx = context.WithValue(ctx, engine.ContextKey, engine.NewEngine(ctx))
	return ctx
}
