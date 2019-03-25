package state

import (
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/hashicorp/hcl"
	"github.com/juju/errors"
	iodin "github.com/temoto/iodin/client/go-iodin"
	"github.com/temoto/vender/engine"
	keyboard "github.com/temoto/vender/hardware/evend-keyboard"
	"github.com/temoto/vender/hardware/lcd"
	"github.com/temoto/vender/hardware/mdb"
	mega "github.com/temoto/vender/hardware/mega-client"
	"github.com/temoto/vender/log2"
)

type Config struct {
	g        Global
	Hardware struct {
		HD44780 struct {
			Enable        bool       `hcl:"enable"`
			Codepage      string     `hcl:"codepage"`
			Pinmap        lcd.PinMap `hcl:"pinmap"`
			Width         int        `hcl:"width"`
			ControlBlink  bool       `hcl:"blink"`
			ControlCursor bool       `hcl:"cursor"`
			ScrollDelay   int        `hcl:"scroll_delay"`
		}
		IodinPath string `hcl:"iodin_path"`
		Keyboard  struct {
			Enable bool `hcl:"enable"`
			// TODO ListenAddr int
		}
		Mdb struct {
			LogEnable  bool   `hcl:"log_enable"`
			UartDevice string `hcl:"uart_device"`
			UartDriver string `hcl:"uart_driver"` // file|mega|iodin
		}
		Mega struct {
			Spi string `hcl:"spi"`
			Pin string `hcl:"pin"`
		}
	}
	Papa struct {
		Address  string
		CertFile string
		Enabled  bool
	}
}

type Global struct {
	lk sync.Mutex

	Hardware struct {
		HD44780 struct {
			Device  *lcd.LCD
			Display *lcd.TextDisplay
		}
		Keyboard struct {
			Device *keyboard.Keyboard
		}
		Mdb struct {
			Mdber  *mdb.Mdb
			Uarter mdb.Uarter
		}
		Mega struct {
			Client *mega.Client
		}
	}
	Log *log2.Log
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

func (c *Config) Global() *Global {
	return &c.g
}

func (c *Config) Mega() (*mega.Client, error) {
	c.g.lk.Lock()
	defer c.g.lk.Unlock()
	err := c.requireMega()
	return c.g.Hardware.Mega.Client, err
}

func (c *Config) Mdber() (*mdb.Mdb, error) {
	c.g.lk.Lock()
	defer c.g.lk.Unlock()
	err := c.requireMdber()
	return c.g.Hardware.Mdb.Mdber, err
}

// Lazy loading starts to bite
func (c *Config) requireMega() error {
	if c.g.Hardware.Mega.Client != nil {
		return nil
	}
	client, err := mega.NewClient(c.Hardware.Mega.Spi, c.Hardware.Mega.Pin, c.g.Log)
	if err != nil {
		return errors.Annotatef(err, "config: mdb.uart_driver=%s mega=%#v", c.Hardware.Mdb.UartDriver, c.Hardware.Mega)
	}
	c.g.Hardware.Mega.Client = client
	return nil
}
func (c *Config) requireMdber() error {
	if c.g.Hardware.Mdb.Mdber != nil {
		return nil
	}

	switch c.Hardware.Mdb.UartDriver {
	case "", "file":
		c.g.Hardware.Mdb.Uarter = mdb.NewFileUart(c.g.Log)
	case "mega":
		if err := c.requireMega(); err != nil {
			return err // TODO annotate
		}
		c.g.Hardware.Mdb.Uarter = mdb.NewMegaUart(c.g.Hardware.Mega.Client)
	case "iodin":
		iodin, err := iodin.NewClient(c.Hardware.IodinPath)
		if err != nil {
			return errors.Annotatef(err, "config: mdb.uart_driver=%s iodin_path=%s", c.Hardware.Mdb.UartDriver, c.Hardware.IodinPath)
		}
		c.g.Hardware.Mdb.Uarter = mdb.NewIodinUart(iodin)
	default:
		return fmt.Errorf("config: unknown mdb.uart_driver=\"%s\" valid: file, fast", c.Hardware.Mdb.UartDriver)
	}
	m, err := mdb.NewMDB(c.g.Hardware.Mdb.Uarter, c.Hardware.Mdb.UartDevice, c.g.Log.Clone(log2.LError))
	if err != nil {
		return errors.Annotatef(err, "config: mdb=%v", c.Hardware.Mdb)
	}
	if c.Hardware.Mdb.LogEnable {
		m.Log.SetLevel(log2.LDebug)
	}
	c.g.Hardware.Mdb.Mdber = m

	return nil
}

func (c *Config) Init(log *log2.Log) error {
	c.g.Log = log
	var err error

	if c.Hardware.HD44780.Enable {
		dev := new(lcd.LCD)
		if err = dev.Init(c.Hardware.HD44780.Pinmap); err != nil {
			return errors.Annotatef(err, "config: %#v", c.Hardware)
		}
		ctrl := lcd.ControlOn
		if c.Hardware.HD44780.ControlBlink {
			ctrl |= lcd.ControlBlink
		}
		if c.Hardware.HD44780.ControlCursor {
			ctrl |= lcd.ControlUnderscore
		}
		dev.SetControl(ctrl)
		c.g.Hardware.HD44780.Device = dev

		d, err := lcd.NewTextDisplay(
			uint16(c.Hardware.HD44780.Width),
			c.Hardware.HD44780.Codepage,
			time.Duration(c.Hardware.HD44780.ScrollDelay)*time.Millisecond,
		)
		if err != nil {
			return errors.Annotatef(err, "config: %#v", c.Hardware)
		}
		d.SetDevice(c.g.Hardware.HD44780.Device)
		go d.Run()
		c.g.Hardware.HD44780.Display = d
	}

	if c.Hardware.Keyboard.Enable {
		if err = c.requireMega(); err != nil {
			return err // TODO annotate
		}
		kb, err := keyboard.NewKeyboard(c.g.Hardware.Mega.Client)
		if err != nil {
			return errors.Annotatef(err, "config: %#v", c.Hardware)
		}
		c.g.Hardware.Keyboard.Device = kb
	}

	return nil
}

func ReadConfig(r io.Reader, log *log2.Log) (*Config, error) {
	b, err := ioutil.ReadAll(r)
	if err != nil {
		return nil, err
	}
	c := new(Config)
	err = hcl.Unmarshal(b, c)
	if err != nil {
		return nil, err
	}

	if err = c.Init(log); err != nil {
		return nil, err
	}

	return c, nil
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
