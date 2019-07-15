package mega

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/temoto/alive"
	"github.com/temoto/errors"
	gpio "github.com/temoto/gpio-cdev-go"
	"github.com/temoto/vender/log2"
	"periph.io/x/periph/conn/physic"
	"periph.io/x/periph/conn/spi"
	"periph.io/x/periph/conn/spi/spireg"
	"periph.io/x/periph/host"
)

const modName string = "mega-client"
const DefaultTimeout = 200 * time.Millisecond
const DefaultSpiSpeed = 200 * physic.KiloHertz
const busyDelay = 500 * time.Microsecond

var (
	ErrCriticalTimeout = errors.Timeoutf("CRITICAL mega response timeout")
	ErrResponseEmpty   = errors.New("mega response empty")
	ErrRequestBusy     = errors.New("mega request busy")
)

type Client struct { //nolint:maligned
	Log      *log2.Log
	TwiChan  chan uint16
	pinev    *gpio.LineEventHandle
	spiPort  spi.Port
	spiConn  spi.Conn
	refcount int32
	alive    *alive.Alive
	stat     Stat
	notify   chan struct{}
	txch     chan *tx
}

type Config struct {
	SpiBus        string
	SpiSpeed      string
	NotifyPinChip string
	NotifyPinName string
}

type Stat struct {
	Request   uint32
	Error     uint32
	TwiListen uint32
	Reset     uint32
}

type tx struct {
	send *Frame
	recv *Frame
	wait time.Duration
	err  error
	done chan struct{}
}

func NewClient(config *Config, log *log2.Log) (*Client, error) {
	notifyPinLine, err := strconv.ParseUint(config.NotifyPinName, 10, 16)
	if err != nil {
		return nil, errors.Annotate(err, "notify pin must be number TODO implement name lookup")
	}

	if _, err = host.Init(); err != nil {
		return nil, errors.Annotate(err, "periph/init")
	}

	spiPort, err := spireg.Open(config.SpiBus)
	if err != nil {
		return nil, errors.Annotate(err, "SPI Open")
	}
	spiSpeed := DefaultSpiSpeed
	if config.SpiSpeed != "" {
		if err := spiSpeed.Set(config.SpiSpeed); err != nil {
			return nil, errors.Annotate(err, "SPI speed parse")
		}
	}
	spiMode := spi.Mode(0)
	spiConn, err := spiPort.Connect(spiSpeed, spiMode, 8)
	if err != nil {
		spiPort.Close()
		return nil, errors.Annotate(err, "SPI Connect")
	}

	pinChip, err := gpio.Open(config.NotifyPinChip, "vender-mega")
	if err != nil {
		spiPort.Close()
		return nil, errors.Annotatef(err, "notify pin open chip=%s", config.NotifyPinChip)
	}

	self := &Client{
		Log:     log,
		TwiChan: make(chan uint16, TWI_LISTEN_MAX_LENGTH/2),
		alive:   alive.NewAlive(),
		notify:  make(chan struct{}),
		spiConn: spiConn,
		spiPort: spiPort,
		txch:    make(chan *tx),
	}

	self.pinev, err = pinChip.GetLineEvent(uint32(notifyPinLine), 0,
		gpio.GPIOEVENT_REQUEST_RISING_EDGE, "vender-mega")
	if err != nil {
		self.Log.Error(errors.Annotate(err, "gpio.EventLoop"))
		self.alive.Stop()
	}

	self.alive.Add(2)
	go self.ioLoop()
	go self.notifyLoop()

	// FIXME
	// if err = self.expectReset(); err != nil { return nil, err }
	var resetf Frame
	self.Tx(nil, &resetf, 0)
	return self, nil
}

func (self *Client) Close() error {
	close(self.TwiChan)
	self.alive.Stop()
	self.alive.Wait()
	return errors.NotImplementedf("")
}

func (self *Client) IncRef(debug string) {
	self.Log.Debugf("%s incref by %s", modName, debug)
	atomic.AddInt32(&self.refcount, 1)
}
func (self *Client) DecRef(debug string) error {
	self.Log.Debugf("%s decref by %s", modName, debug)
	new := atomic.AddInt32(&self.refcount, -1)
	switch {
	case new > 0:
		return nil
	case new == 0:
		return self.Close()
	}
	panic(fmt.Sprintf("code error %s decref<0 debug=%s", modName, debug))
}

func (self *Client) DoStatus() (Frame, error) {
	return self.DoTimeout(COMMAND_STATUS, nil, DefaultTimeout)
}

func (self *Client) DoMdbBusReset(d time.Duration) (Frame, error) {
	buf := [2]byte{}
	binary.BigEndian.PutUint16(buf[:], uint16(d/time.Millisecond))
	return self.DoTimeout(COMMAND_MDB_BUS_RESET, buf[:], d+DefaultTimeout)
}

func (self *Client) DoMdbTxSimple(data []byte) (Frame, error) {
	return self.DoTimeout(COMMAND_MDB_TRANSACTION_SIMPLE, data, DefaultTimeout)
}

func (self *Client) DoTimeout(cmd Command_t, data []byte, timeout time.Duration) (Frame, error) {
	atomic.AddUint32(&self.stat.Request, 1)
	cmdFrame := NewCommand(cmd, data...)

	var response Frame
	err := self.Tx(&cmdFrame, &response, timeout)
	return response, err
}

func (self *Client) Stat() Stat {
	return self.stat
}

func (self *Client) Tx(send, recv *Frame, timeout time.Duration) error {
	done := make(chan struct{})
	tx := &tx{send: send, recv: recv, wait: timeout, done: done}
	self.txch <- tx
	<-tx.done
	return tx.err
}

func (self *Client) ioLoop() {
	defer self.alive.Done()

	bgrecv := Frame{}

	for self.alive.IsRunning() {
		select {
		case tx := <-self.txch:
			// self.Log.Debugf("ioLoop tx=%#v", tx)
			tx.err = self.ioTx(tx)
			if tx.err != nil {
				atomic.AddUint32(&self.stat.Error, 1)
			}
			close(tx.done)
			// self.Log.Debugf("ioLoop tx done err=%v", tx.err)

		case <-self.notify:
			self.alive.Add(1)
			err := self.ioReadParse(&bgrecv)
			switch err {
			case nil: // success path
				switch bgrecv.ResponseKind() {
				case RESPONSE_TWI_LISTEN:
				case RESPONSE_RESET:
					// Sorry for ugly hack, I couldn't think of better way.
					// sendToTx = atomic.LoadUint32(&self.xxx_expect_reset) == 1
				default:
					self.Log.Fatalf("%s stray packet %s", modName, bgrecv.ResponseString())
				}
			case ErrResponseEmpty:
				// XXX TODO FIXME error is still present, it only wastes time, not critical
				// self.Log.Errorf("%s FIXME tx=no notified=yes read=empty", modName)
			default:
				self.Log.Fatalf("%s stray err=%v", modName, err)
			}
		}
	}
}

// (functionally inline) track write/wait/recv chain
func (self *Client) ioTx(tx *tx) error {
	self.alive.Add(1)
	defer self.alive.Done()

	if tx.send != nil {
		err := self.ioWrite(tx.send)
		if err != nil {
			return errors.Annotatef(err, "send=%x", tx.send.Bytes())
		}
	}

	var err error
	if tx.recv != nil {
		for i := 1; i <= 3; i++ {
			notified := self.ioWait(tx.wait)
			err = self.ioReadParse(tx.recv)
			// self.Log.Debugf("iotx parsed wait=%t notified=%t err=%v recv=%v", tx.wait != 0, notified, err, tx.recv)
			if err == nil {
				// self.Log.Debugf("iotx parsed=%s", tx.recv.ResponseString())
				switch tx.recv.ResponseKind() {
				case RESPONSE_RESET, RESPONSE_TWI_LISTEN:
					// wait and read again
				default:
					// success path when response is received
					return nil
				}
			}
			if tx.wait == 0 && err == ErrResponseEmpty {
				if notified {
					// shouldn't ever happen
					self.Log.Fatalf("mega TODO iotx try=%d wait=no notified=yes read=empty", i)
				} else {
					// success path for read-only Tx() when no data is available
					return ErrResponseEmpty
				}
			}
			if tx.wait != 0 && err == ErrResponseEmpty {
				if !notified {
					// Sounds like a regular old timeout, but it's actually fatal error.
					// Need to reset mega.
					return ErrCriticalTimeout
				}
			}
			time.Sleep(busyDelay)
		}
	}
	return errors.Trace(err)
}

func (self *Client) ioWait(timeout time.Duration) bool {
	// For wait=0 case, per Go spec, select would pick any case.
	// What we actually want with wait=0 is strong preference to reading, if available.
	if timeout == 0 {
		select {
		case <-self.notify:
			return true
		default:
			return false
		}
	} else {
		tmr := time.NewTimer(timeout)
		defer tmr.Stop()
		select {
		case <-self.notify:
			return true
		case <-tmr.C:
			return false
		}
	}
}

func (self *Client) notifyLoop() {
	defer self.alive.Done()

	if value, err := self.pinev.Read(); err != nil {
		self.Log.Errorf("%s notifyLoop start Read()", modName)
	} else if value == 1 {
		self.Log.Debugf("%s notify=high on start", modName)
		self.notify <- struct{}{}
	}

	for self.alive.IsRunning() {
		edge, err := self.pinev.Wait( /*timeout*/ )
		if err != nil {
			self.Log.Errorf("%s notifyLoop Wait", modName)
			self.Close()
			return
		}
		if edge.ID == gpio.GPIOEVENT_EVENT_RISING_EDGE {
			self.notify <- struct{}{}
		}
	}
}

func (self *Client) ioWrite(f *Frame) error {
	// static allocation of maximum possible size
	var bufarr [BUFFER_SIZE + totalOverheads]byte
	// slice it down to shorten SPI session time
	bs := f.Bytes()
	buf := bufarr[:len(bs)+ /*recv ioAck*/ 6+totalOverheads]
	ackExpect := []byte{0x00, 0xff, f.crc, f.crc}
	var busy bool

	for try := 1; try <= 3; try++ {
		copy(buf, bs)
		busy = false
		// self.Log.Debugf("ioWrite out=%x", buf)
		err := self.spiConn.Tx(buf, buf)
		if err != nil {
			return err
		}
		// self.Log.Debugf("ioWrite -in=%x", buf)

		var padStart int
		var errcode Errcode_t
		if padStart, errcode, err = parsePadding(buf, false); err != nil {
			return err
		}
		switch errcode {
		case 0:
		case ERROR_REQUEST_OVERWRITE:
			busy = true
			// self.Log.Debugf("mega ioWrite: input buffer is busy -> sleep,retry")
			time.Sleep(busyDelay)
			continue
		default:
			return errors.Errorf("FIXME errcode=%d buf=%x", errcode, buf)
		}
		if padStart < 6 {
			self.Log.Errorf("mega ioWrite: invalid ioAck buf=%x", buf)
			continue
		}
		ackRemote := buf[padStart-6 : padStart-2]
		if !bytes.Equal(ackRemote, ackExpect) {
			self.Log.Errorf("mega ioWrite: invalid ioAck expected=%x actual=%x", ackExpect, ackRemote)
			continue
		}

		if buf[0]&PROTOCOL_FLAG_REQUEST_BUSY != 0 {
			busy = true
			// self.Log.Debugf("mega ioWrite: input buffer is busy -> sleep,retry")
			time.Sleep(busyDelay)
			continue
		}

		break
	}
	if busy {
		return ErrRequestBusy
	}

	return nil
}

func (self *Client) ioReadParse(frame *Frame) error {
	var buf [BUFFER_SIZE + totalOverheads]byte
	buf[0] = ProtocolVersion
	lenBuf := buf[:2]
	// self.Log.Debugf("%s read length out=%x", modName, lenBuf)
	err := self.spiConn.Tx(lenBuf, lenBuf)
	remoteLength := buf[1]
	// self.Log.Debugf("%s read length -in=%x err=%v remoteLength=%d", modName, lenBuf, err, remoteLength)
	if err != nil {
		return err
	}
	if remoteLength == 0 {
		return ErrResponseEmpty
	}

	bs := buf[:remoteLength+totalOverheads]
	bs[0] = ProtocolVersion
	// self.Log.Debugf("%s read out=%x", modName, bs)
	err = self.spiConn.Tx(bs, bs)
	// self.Log.Debugf("%s read -in=%x err=%v", modName, bs, err)
	if err != nil {
		return err
	}

	err = self.parse(bs, frame)
	if err != nil {
		return err
	}
	err = self.ioAck(frame)
	return err
}

func (self *Client) ioAck(f *Frame) error {
	var buf [2 + totalOverheads]byte
	buf[0] = PROTOCOL_FLAG_REQUEST_BUSY | ProtocolVersion
	buf[1] = 2
	buf[2] = f.plen
	buf[3] = f.crc

	// self.Log.Debugf("%s ioAck out=%x", modName, buf)
	err := self.spiConn.Tx(buf[:], buf[:])
	// self.Log.Debugf("%s ioAck -in=%x err=%v", modName, buf, err)
	if err != nil {
		return err
	}

	_, _, err = parsePadding(buf[:], true)
	return err
}

func (self *Client) parse(buf []byte, f *Frame) error {
	err := f.Parse(buf)
	if err != nil {
		atomic.AddUint32(&self.stat.Error, 1)
		self.Log.Errorf("%s Parse buf=%x err=%v", modName, buf, err)
		return err
	}
	if f.plen == 0 {
		return ErrResponseEmpty
	}
	err = f.ParseFields()
	if err != nil {
		atomic.AddUint32(&self.stat.Error, 1)
		self.Log.Errorf("%s ParseFields frame=%x err=%v", modName, f.Bytes(), err)
		return err
	}

	for i := 0; i+1 < len(f.Fields.TwiData); i += 2 {
		twitem := binary.BigEndian.Uint16(f.Fields.TwiData[i : i+2])
		select {
		case self.TwiChan <- twitem:
		default:
			self.Log.Errorf("CRITICAL TwiChan is full")
			panic("code error mega TwiChan is full")
		}
	}

	switch f.ResponseKind() {
	case RESPONSE_TWI_LISTEN:
		atomic.AddUint32(&self.stat.TwiListen, 1)
	case RESPONSE_RESET:
		atomic.AddUint32(&self.stat.Reset, 1)
		if ResetFlag(f.Fields.Mcusr)&ResetFlagWatchdog != 0 {
			atomic.AddUint32(&self.stat.Error, 1)
			self.Log.Errorf("mega restarted by watchdog, info=%s", f.Fields.String())
		} else {
			self.Log.Debugf("mega normal reset, info=%s", f.Fields.String())
		}
	}

	return nil
}
