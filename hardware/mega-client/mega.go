package mega

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"strconv"
	"sync"
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
const DefaultTimeout = 100 * time.Millisecond

var ErrResponseEmpty = errors.New("mega response empty")
var ErrRequestBusy = errors.New("mega request busy")

type Client struct {
	Log      *log2.Log
	TwiChan  chan uint16
	txlk     sync.Mutex
	readCh   chan FrameError
	spiPort  spi.Port
	spiConn  spi.Conn
	refcount int32
	alive    *alive.Alive
	stat     Stat

	xxx_expect_reset uint32
}
type Stat struct {
	Request   uint32
	Error     uint32
	TwiListen uint32
	Reset     uint32
}

type FrameError struct {
	f Frame
	e error
}

func NewClient(bus string, notifyPinChip, notifyPinName string, log *log2.Log) (*Client, error) {
	notifyPinLine, err := strconv.ParseUint(notifyPinName, 10, 16)
	if err != nil {
		return nil, errors.Annotate(err, "notify pin must be number TODO implement name lookup")
	}

	if _, err = host.Init(); err != nil {
		return nil, errors.Annotate(err, "periph/init")
	}

	spiPort, err := spireg.Open(bus)
	if err != nil {
		return nil, errors.Annotate(err, "SPI Open")
	}
	spiSpeed := 200 * physic.KiloHertz
	spiMode := spi.Mode(0)
	spiConn, err := spiPort.Connect(spiSpeed, spiMode, 8)
	if err != nil {
		spiPort.Close()
		return nil, errors.Annotate(err, "SPI Connect")
	}

	pinChip, err := gpio.Open(notifyPinChip, "vender-mega")
	if err != nil {
		spiPort.Close()
		return nil, errors.Annotatef(err, "notify pin open chip=%s", notifyPinChip)
	}

	self := &Client{
		Log:     log,
		spiPort: spiPort,
		spiConn: spiConn,
		readCh:  make(chan FrameError),
		alive:   alive.NewAlive(),
		TwiChan: make(chan uint16, TWI_LISTEN_MAX_LENGTH/2),
	}

	pinev, err := pinChip.GetLineEvent(uint32(notifyPinLine), 0,
		gpio.GPIOEVENT_REQUEST_RISING_EDGE, "vender-mega")
	if err != nil {
		self.Log.Error(errors.Annotate(err, "gpio.EventLoop"))
		self.alive.Stop()
	}

	go self.readLoop(pinev)

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
	self.txlk.Lock()
	defer self.txlk.Unlock()

	if send != nil {
		err := self.write(send)
		if err != nil {
			atomic.AddUint32(&self.stat.Error, 1)
			return err
		}
	}

	// Tx() invoked to send only
	if recv == nil {
		return nil
	}

	// Duplicate select here is valid, or at least, following naive single is not correct.
	// select { case <-readCh | case <-timeout }
	// For timeout=0 case, per Go spec, select would pick any case.
	// What we actually want with timeout=0 is strong preference to readCh, if available.
	if timeout == 0 {
		select {
		case fe := <-self.readCh:
			if fe.e != nil {
				return fe.e
			}
			*recv = fe.f
			return nil
		default:
			return ErrResponseEmpty
		}
	} else {
		tmr := time.NewTimer(timeout)
		defer tmr.Stop()
		select {
		case fe := <-self.readCh:
			if fe.e != nil {
				return fe.e
			}
			*recv = fe.f
			return nil
		case <-tmr.C:
			return errors.Timeoutf("mega-client Tx() send=%x response timeout=%s", send.Bytes(), timeout)
		}
	}
}

// bring mega to known state: expect or force RESET
func (self *Client) expectReset() error {
	try := func(cmd *Frame, timeout time.Duration) (bool, error) {
		var response Frame
		err := self.Tx(cmd, &response, timeout)
		self.Log.Debugf("%s expectReset response=%v err=%v", modName, response.ResponseString(), err)
		switch err {
		case ErrResponseEmpty:
			// mega was reset at other time and RESET has been consumed
			return false, nil
		case nil:
			if response.ResponseKind() == RESPONSE_RESET {
				return true, nil // success path
			}
			self.Log.Errorf("%s expectReset response=%s", modName, response.ResponseString())
			return false, nil
		default:
			self.Close()
			err = errors.Annotatef(err, "mega init RESET read error")
			return false, err
		}
	}

	// Sorry for ugly hack, I couldn't think of better way.
	atomic.StoreUint32(&self.xxx_expect_reset, 1)
	defer atomic.StoreUint32(&self.xxx_expect_reset, 0)

	ok, err := try(nil, 0)
	if err != nil {
		err = errors.Annotate(err, "expect reset")
		return err
	} else if !ok {
		cmd := NewCommand(COMMAND_RESET, 0xff)
		ok, err = try(&cmd, DefaultTimeout*3)
		if err != nil {
			err = errors.Annotate(err, "expect reset")
			return err
		} else if !ok {
			err = errors.Errorf("CRITICAL hardware problem: no reset response even after command")
			return err
		}
	}
	return nil
}

func (self *Client) write(f *Frame) error {
	// static allocation of maximum possible size
	var bufarr [BUFFER_SIZE + totalOverheads]byte
	// slice it down to shorten SPI session time
	bs := f.Bytes()
	buf := bufarr[:len(bs)+ /*recv ack*/ 6+totalOverheads]
	copy(buf, bs)
	ackExpect := []byte{0x00, 0xff, f.crc, f.crc}

	for try := 1; try <= 3; try++ {
		// self.Log.Debugf("write out=%x", buf)
		err := self.spiConn.Tx(buf, buf)
		if err != nil {
			return err
		}
		// self.Log.Debugf("write -in=%x", buf)

		var padStart int
		var errcode Errcode_t
		if padStart, errcode, err = parsePadding(buf, false); err != nil {
			return err
		}
		switch errcode {
		case 0:
		case ERROR_REQUEST_OVERWRITE:
			return ErrRequestBusy
		default:
			return errors.Errorf("FIXME %x", buf)
		}
		if padStart < 6 {
			self.Log.Errorf("mega write: invalid ack buf=%x", buf)
			continue
		}
		ackRemote := buf[padStart-6 : padStart-2]
		if !bytes.Equal(ackRemote, ackExpect) {
			self.Log.Errorf("mega write: invalid ack expected=%x actual=%x", ackExpect, ackRemote)
			continue
		}

		if buf[0]&PROTOCOL_FLAG_REQUEST_BUSY != 0 {
			self.Log.Debugf("mega write: input buffer is busy -> sleep,retry")
			time.Sleep(500 * time.Microsecond)
			continue
		}

		break
	}

	return nil
}

func (self *Client) readLoop(pinev *gpio.LineEventHandle) {
	// const edgeTimeout = 1 * time.Second
	for self.alive.IsRunning() {
		if pinvalue, _ := pinev.Read(); pinvalue == 0 {
			edge, err := pinev.Wait( /*edgeTimeout*/ )
			if err != nil {
				self.Log.Errorf("%s readLoop() Wait()", modName)
				break
			}
			if !self.alive.IsRunning() {
				break
			}
			if edge.ID != gpio.GPIOEVENT_EVENT_RISING_EDGE {
				continue
			}
		}

		frame, err := self.readParse()
		if err == nil && frame == nil {
			panic("code error")
		}
		switch err {
		case ErrResponseEmpty:
			// self.Log.Errorf("%s readLoop err=%v", modName, err)
		case nil:
			sendToTx := true
			if frame != nil {
				switch frame.ResponseKind() {
				case RESPONSE_TWI_LISTEN:
					sendToTx = false
				case RESPONSE_RESET:
					// Sorry for ugly hack, I couldn't think of better way.
					sendToTx = atomic.LoadUint32(&self.xxx_expect_reset) == 1
				}
			}
			if sendToTx {
				// Correct path, send response to Tx()
				self.readCh <- FrameError{f: *frame}
			}
		default:
			self.readCh <- FrameError{e: err}
		}
	}
}

func (self *Client) poll() (uint8, error) {
	var buf [2]byte
	buf[0] = ProtocolVersion

	// self.Log.Debugf("%s poll out=%x", modName, buf[:])
	err := self.spiConn.Tx(buf[:], buf[:])
	// self.Log.Debugf("%s poll -in=%x err=%v", modName, buf[:], err)
	if err != nil {
		return 0, err
	}
	return buf[1], nil
}

func (self *Client) readParse() (*Frame, error) {
	remoteLength, err := self.poll()
	if remoteLength == 0 {
		return nil, ErrResponseEmpty
	}
	if err != nil {
		return nil, err
	}

	var buf [BUFFER_SIZE + totalOverheads]byte
	bs := buf[:remoteLength+totalOverheads]
	bs[0] = ProtocolVersion
	// self.Log.Debugf("%s read out=%x", modName, bs)
	err = self.spiConn.Tx(bs, bs)
	// self.Log.Debugf("%s read -in=%x err=%v", modName, bs, err)
	if err != nil {
		return nil, err
	}

	var frame Frame
	err = self.parse(bs, &frame)
	if err != nil {
		return nil, err
	}
	err = self.ack(&frame)
	if err != nil {
		return nil, err
	}
	return &frame, nil
}

func (self *Client) ack(f *Frame) error {
	var buf [2 + totalOverheads]byte
	buf[0] = PROTOCOL_FLAG_REQUEST_BUSY | ProtocolVersion
	buf[1] = 2
	buf[2] = f.plen
	buf[3] = f.crc

	// self.Log.Debugf("%s ack out=%x", modName, buf)
	err := self.spiConn.Tx(buf[:], buf[:])
	// self.Log.Debugf("%s ack -in=%x err=%v", modName, buf, err)
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

	switch f.ResponseKind() {
	case RESPONSE_TWI_LISTEN:
		atomic.AddUint32(&self.stat.TwiListen, 1)
		for i := 0; i+1 < len(f.Fields.TwiData); i += 2 {
			twitem := binary.BigEndian.Uint16(f.Fields.TwiData[i : i+2])
			select {
			case self.TwiChan <- twitem:
			default:
				self.Log.Errorf("CRITICAL TwiChan is full")
			}
		}
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
