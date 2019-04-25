package state

import (
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hashicorp/hcl"
	"github.com/juju/errors"
	"github.com/temoto/alive"
	iodin "github.com/temoto/iodin/client/go-iodin"
	"github.com/temoto/vender/currency"
	"github.com/temoto/vender/engine"
	"github.com/temoto/vender/engine/inventory"
	keyboard "github.com/temoto/vender/hardware/evend-keyboard"
	"github.com/temoto/vender/hardware/lcd"
	"github.com/temoto/vender/hardware/mdb"
	mega "github.com/temoto/vender/hardware/mega-client"
	"github.com/temoto/vender/head/tele"
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
			LogDebug   bool   `hcl:"log_debug"`
			UartDevice string `hcl:"uart_device"`
			UartDriver string `hcl:"uart_driver"` // file|mega|iodin
		}
		Mega struct {
			Spi string `hcl:"spi"`
			Pin string `hcl:"pin"`
		}
	}
	Money struct {
		Scale                int `hcl:"scale"`
		CreditMax            int `hcl:"credit_max"`
		ChangeOverCompensate int `hcl:"change_over_compensate"`
	}
	Tele tele.Config
}

type Global struct {
	lk sync.Mutex

	Alive    *alive.Alive
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
		Mega atomic.Value // *mega.Client
	}

	Inventory inventory.Inventory

	Log *log2.Log

	Tele tele.Tele
}

const ContextKey = "config"

// Context["config"] -> *Config or panic
func GetConfig(ctx context.Context) *Config {
	v := ctx.Value(ContextKey)
	if v == nil {
		panic(fmt.Sprintf("context['%s'] is nil", ContextKey))
	}
	if cfg, ok := v.(*Config); ok {
		return cfg
	}
	panic(fmt.Sprintf("context['%s'] expected type *Config actual=%#v", ContextKey, v))
}
func GetGlobal(ctx context.Context) *Global { return &GetConfig(ctx).g }
func ContextWithConfig(ctx context.Context, config *Config) context.Context {
	return context.WithValue(ctx, ContextKey, config)
}

func (c *Config) Global() *Global { return &c.g }

func (c *Config) Mdber() (*mdb.Mdb, error) {
	if c.g.Hardware.Mdb.Mdber != nil {
		return c.g.Hardware.Mdb.Mdber, nil
	}

	c.g.lk.Lock()
	defer c.g.lk.Unlock()

	switch c.Hardware.Mdb.UartDriver {
	case "", "file":
		c.g.Hardware.Mdb.Uarter = mdb.NewFileUart(c.g.Log)
	case "mega":
		m, err := c.g.Mega(c)
		if err != nil {
			return nil, err
		}
		c.g.Hardware.Mdb.Uarter = mdb.NewMegaUart(m)
	case "iodin":
		iodin, err := iodin.NewClient(c.Hardware.IodinPath)
		if err != nil {
			return nil, errors.Annotatef(err, "config: mdb.uart_driver=%s iodin_path=%s", c.Hardware.Mdb.UartDriver, c.Hardware.IodinPath)
		}
		c.g.Hardware.Mdb.Uarter = mdb.NewIodinUart(iodin)
	default:
		return nil, fmt.Errorf("config: unknown mdb.uart_driver=\"%s\" valid: file, fast", c.Hardware.Mdb.UartDriver)
	}
	mdbLog := c.g.Log.Clone(log2.LInfo)
	if c.Hardware.Mdb.LogDebug {
		mdbLog.SetLevel(log2.LDebug)
	}
	m, err := mdb.NewMDB(c.g.Hardware.Mdb.Uarter, c.Hardware.Mdb.UartDevice, mdbLog)
	if err != nil {
		return nil, errors.Annotatef(err, "config: mdb=%v", c.Hardware.Mdb)
	}
	c.g.Hardware.Mdb.Mdber = m

	return c.g.Hardware.Mdb.Mdber, nil
}

func (c *Config) ScaleI(i int) currency.Amount {
	return currency.Amount(i) * currency.Amount(c.Money.Scale)
}
func (c *Config) ScaleU(u uint32) currency.Amount          { return currency.Amount(u * uint32(c.Money.Scale)) }
func (c *Config) ScaleA(a currency.Amount) currency.Amount { return a * currency.Amount(c.Money.Scale) }

func (c *Config) Init(ctx context.Context) error {
	c.g.Log = log2.ContextValueLogger(ctx)
	c.g.Inventory.Init()
	c.g.Tele.Init(ctx, c.Tele)

	if c.Hardware.HD44780.Enable {
		dev := new(lcd.LCD)
		if err := dev.Init(c.Hardware.HD44780.Pinmap); err != nil {
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
		m, err := c.g.Mega(c)
		if err != nil {
			return errors.Annotatef(err, "config: keyboard needs mega")
		}
		kb, err := keyboard.NewKeyboard(m)
		if err != nil {
			return errors.Annotatef(err, "config: %#v", c.Hardware)
		}
		c.g.Hardware.Keyboard.Device = kb
	}

	if c.Money.Scale == 0 {
		return errors.NotValidf("config: money.scale is not set")
	} else if c.Money.Scale < 0 {
		return errors.NotValidf("config: money.scale < 0")
	}
	c.Money.CreditMax *= c.Money.Scale
	c.Money.ChangeOverCompensate *= c.Money.Scale

	return nil
}

func ReadConfig(ctx context.Context, r io.Reader) (*Config, error) {
	b, err := ioutil.ReadAll(r)
	if err != nil {
		return nil, err
	}
	c := new(Config)
	err = hcl.Unmarshal(b, c)
	if err != nil {
		return nil, err
	}

	if err = c.Init(ctx); err != nil {
		return nil, err
	}

	return c, nil
}

func ReadConfigFile(ctx context.Context, path string) (*Config, error) {
	log := log2.ContextValueLogger(ctx)
	if pathAbs, err := filepath.Abs(path); err != nil {
		log.Errorf("filepath.Abs(%s) err=%v", path, err)
	} else {
		path = pathAbs
	}
	log.Debugf("reading config file %s", path)

	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return ReadConfig(ctx, f)
}

func MustReadConfig(ctx context.Context, r io.Reader) *Config {
	c, err := ReadConfig(ctx, r)
	if err != nil {
		log := log2.ContextValueLogger(ctx)
		log.Fatal(err)
	}
	return c
}

func MustReadConfigFile(ctx context.Context, path string) *Config {
	c, err := ReadConfigFile(ctx, path)
	if err != nil {
		log.Fatal(err)
	}
	return c
}

func (g *Global) Mega(FIXME_config *Config) (*mega.Client, error) {
	if x := g.Hardware.Mega.Load(); x != nil {
		return x.(*mega.Client), nil
	}

	g.lk.Lock()
	defer g.lk.Unlock()
	if x := g.Hardware.Mega.Load(); x != nil {
		return x.(*mega.Client), nil
	}

	mcfg := &FIXME_config.Hardware.Mega
	client, err := mega.NewClient(mcfg.Spi, mcfg.Pin, g.Log)
	if err != nil {
		return nil, errors.Annotatef(err, "config: mega=%#v", mcfg)
	}
	g.Hardware.Mega.Store(client)

	return client, nil
}

func NewTestContext(t testing.TB, confString string /* logLevel log2.Level*/) context.Context {
	const defaultConfig = "money { scale=100 }"
	if confString == "" {
		confString = defaultConfig
	}

	ctx := context.Background()
	log := log2.NewTest(t, log2.LDebug)
	log.SetFlags(log2.LTestFlags)
	ctx = context.WithValue(ctx, log2.ContextKey, log)
	config := MustReadConfig(ctx, strings.NewReader(confString))
	ctx = ContextWithConfig(ctx, config)
	ctx = context.WithValue(ctx, engine.ContextKey, engine.NewEngine(ctx))

	mdber, mdbMock := mdb.NewTestMdber(t)
	config.Global().Hardware.Mdb.Mdber = mdber
	if _, err := config.Mdber(); err != nil {
		t.Fatal(err)
	}
	ctx = context.WithValue(ctx, mdb.MockContextKey, mdbMock)

	return ctx
}
