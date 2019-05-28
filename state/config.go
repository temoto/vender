package state

import (
	"context"
	"fmt"
	"path/filepath"
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
	evend_config "github.com/temoto/vender/hardware/mdb/evend/config"
	mega "github.com/temoto/vender/hardware/mega-client"
	"github.com/temoto/vender/head/tele"
	"github.com/temoto/vender/helpers"
	"github.com/temoto/vender/log2"
)

type Config struct {
	g Global

	// includeSeen contains absolute paths to prevent include loops
	includeSeen map[string]struct{}
	// Include is only used for Unmarshal
	Include []ConfigSource `hcl:"include"`

	Hardware struct {
		Evend   evend_config.Config `hcl:"evend"`
		HD44780 struct {            //nolint:maligned
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

	Engine struct {
		Aliases []Alias `hcl:"alias"`
		Menu    struct {
			MsgIntro        string      `hcl:"msg_intro"`
			ResetTimeoutSec int         `hcl:"reset_sec"`
			Items           []*MenuItem `hcl:"item"`
		}
	}

	Money struct {
		Scale                int `hcl:"scale"`
		CreditMax            int `hcl:"credit_max"`
		ChangeOverCompensate int `hcl:"change_over_compensate"`
	}
	Tele tele.Config
}

type ConfigSource struct {
	Name     string `hcl:"name,key"`
	Optional bool   `hcl:"optional"`
}

type Alias struct {
	Name     string `hcl:"name,key"`
	Scenario string `hcl:"scenario"`

	Doer engine.Doer `hcl:"-"`
}

type MenuItem struct {
	Code      string `hcl:"code,key"`
	Name      string `hcl:"name"`
	XXX_Price int    `hcl:"price"` // use scaled `Price`, this is for decoding config only
	Scenario  string `hcl:"scenario"`

	Price currency.Amount `hcl:"-"`
	Doer  engine.Doer     `hcl:"-"`
}

func (self *MenuItem) String() string { return fmt.Sprintf("menu.%s %s", self.Code, self.Name) }

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

// TODO move to Global? but it needs c.Hardware
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
	errs := make([]error, 0)
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
		err := errors.NotValidf("config: money.scale is not set")
		errs = append(errs, err)
	} else if c.Money.Scale < 0 {
		err := errors.NotValidf("config: money.scale < 0")
		errs = append(errs, err)
	}
	c.Money.CreditMax *= c.Money.Scale
	c.Money.ChangeOverCompensate *= c.Money.Scale

	// log := log2.ContextValueLogger(ctx)
	e := engine.GetEngine(ctx)
	for _, x := range c.Engine.Menu.Items {
		var err error
		x.Price = c.ScaleI(x.XXX_Price)
		x.Doer, err = e.ParseText(x.Name, x.Scenario)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		// log.Debugf("config.engine.menu %s pxxx=%d ps=%d", x.String(), x.XXX_Price, x.Price)
		e.Register("menu."+x.Code, x.Doer)
	}
	for _, x := range c.Engine.Aliases {
		var err error
		x.Doer, err = e.ParseText(x.Name, x.Scenario)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		// log.Debugf("config.engine.alias name=%s scenario=%s", x.Name, x.Scenario)
		e.Register(x.Name, x.Doer)
	}

	return helpers.FoldErrors(errs)
}
func (c *Config) MustInit(ctx context.Context) {
	log := log2.ContextValueLogger(ctx)
	err := c.Init(ctx)
	if err != nil {
		log.Fatal(errors.ErrorStack(err))
	}
}

func (c *Config) read(fs FullReader, source ConfigSource, errs *[]error) {
	norm := fs.Normalize(source.Name)
	if _, ok := c.includeSeen[norm]; ok {
		c.g.Log.Fatalf("config duplicate source=%s", source.Name)
	} else {
		c.g.Log.Debugf("config reading source='%s' path=%s", source.Name, norm)
	}
	c.includeSeen[source.Name] = struct{}{}
	c.includeSeen[norm] = struct{}{}

	bs, err := fs.ReadAll(norm)
	if bs == nil && err == nil {
		if !source.Optional {
			err = errors.NotFoundf("config required name=%s path=%s", source.Name, norm)
			*errs = append(*errs, err)
			return
		}
	}
	if err != nil {
		*errs = append(*errs, errors.Annotatef(err, "config source=%s", source.Name))
		return
	}

	err = hcl.Unmarshal(bs, c)
	if err != nil {
		err = errors.Annotatef(err, "config unmarshal source=%s content='%s'", source.Name, string(bs))
		*errs = append(*errs, err)
		return
	}

	var includes []ConfigSource
	includes, c.Include = c.Include, nil
	for _, include := range includes {
		includeNorm := fs.Normalize(include.Name)
		if _, ok := c.includeSeen[includeNorm]; ok {
			err = errors.Errorf("config include loop: from=%s include=%s", source.Name, include.Name)
			*errs = append(*errs, err)
			continue
		}
		c.read(fs, include, errs)
	}
}

func ReadConfig(ctx context.Context, fs FullReader, names ...string) (*Config, error) {
	log := log2.ContextValueLogger(ctx)
	if len(names) == 0 {
		log.Fatal("code error [Must]ReadConfig() without names")
	}

	if osfs, ok := fs.(*OsFullReader); ok {
		dir, name := filepath.Split(names[0])
		osfs.SetBase(dir)
		names[0] = name
	}
	c := &Config{
		includeSeen: make(map[string]struct{}),
	}
	c.g.Log = log
	errs := make([]error, 0, 8)
	for _, name := range names {
		c.read(fs, ConfigSource{Name: name}, &errs)
	}
	return c, helpers.FoldErrors(errs)
}

func MustReadConfig(ctx context.Context, fs FullReader, names ...string) *Config {
	log := log2.ContextValueLogger(ctx)
	c, err := ReadConfig(ctx, fs, names...)
	if err != nil {
		log.Fatal(errors.ErrorStack(err))
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
	fs := NewMockFullReader(map[string]string{
		"test-inline": confString,
	})

	ctx := context.Background()
	log := log2.NewTest(t, log2.LDebug)
	log.SetFlags(log2.LTestFlags)
	ctx = context.WithValue(ctx, log2.ContextKey, log)
	ctx = context.WithValue(ctx, engine.ContextKey, engine.NewEngine(ctx))
	config := MustReadConfig(ctx, fs, "test-inline")
	config.MustInit(ctx)
	ctx = ContextWithConfig(ctx, config)

	mdber, mdbMock := mdb.NewTestMdber(t)
	config.Global().Hardware.Mdb.Mdber = mdber
	if _, err := config.Mdber(); err != nil {
		t.Fatal(err)
	}
	ctx = context.WithValue(ctx, mdb.MockContextKey, mdbMock)

	return ctx
}
