package mega

import (
	"io"
	"strconv"

	"github.com/juju/errors"
	gpio "github.com/temoto/gpio-cdev-go"
	"github.com/temoto/vender/helpers"
	"periph.io/x/periph/conn/spi"
	"periph.io/x/periph/conn/spi/spireg"
	"periph.io/x/periph/host"
)

type Config struct {
	SpiBus        string
	SpiMode       int
	SpiSpeed      string
	NotifyPinChip string
	NotifyPinName string

	DontUseRawMode bool // skip ioLoop, used to bring real hardware to invalid state
	testhw         *hardware
}

type hardware struct {
	spiTx    SpiTxFunc    // used
	notifier gpio.Eventer // used

	spiPort  spi.PortCloser // only for resource cleanup
	gpioChip gpio.Chiper    // only for resource cleanup
}
type SpiTxFunc func(send, recv []byte) error

const testDevice = "\x01test" // used by tests, ignore it

// Converts strings to useful hardware talking functions.
func (h *hardware) open(c *Config) error {
	var err error
	var notifyLine uint64
	notifyLine, err = strconv.ParseUint(c.NotifyPinName, 10, 16)
	if err != nil {
		return errors.Annotate(err, "notify pin must be number TODO implement name lookup")
	}

	if c.testhw != nil {
		*h = *c.testhw
		// TODO simulate open errors
		return nil
	}

	if _, err = host.Init(); err != nil {
		return errors.Annotate(err, "periph/init")
	}

	var spiPort spi.PortCloser
	spiPort, err = spireg.Open(c.SpiBus)
	if err != nil {
		return errors.Annotatef(err, "SPI Open bus=%s", c.SpiBus)
	}
	spiSpeed := DefaultSpiSpeed
	if c.SpiSpeed != "" {
		if err = spiSpeed.Set(c.SpiSpeed); err != nil {
			return errors.Annotate(err, "SPI speed parse")
		}
	}
	var spiConn spi.Conn
	spiConn, err = spiPort.Connect(spiSpeed, spi.Mode(c.SpiMode), 8)
	if err != nil {
		return errors.Annotate(err, "SPI Connect")
	}
	h.spiTx = spiConn.Tx

	h.gpioChip, err = gpio.Open(c.NotifyPinChip, "mega")
	if err != nil {
		return errors.Annotatef(err, "notify pin open chip=%s", c.NotifyPinChip)
	}
	h.notifier, err = h.gpioChip.GetLineEvent(uint32(notifyLine), 0,
		gpio.GPIOEVENT_REQUEST_RISING_EDGE, "mega")
	if err != nil {
		return errors.Annotate(err, "gpio.GetLineEvent")
	}
	return nil
}

func (h *hardware) Close() error {
	closers := []io.Closer{
		h.spiPort,
		h.notifier,
		h.gpioChip,
	}
	errs := make([]error, len(closers))
	for i, c := range closers {
		if c != nil {
			errs[i] = c.Close()
		}
	}
	return helpers.FoldErrors(errs)
}
