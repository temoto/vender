package mega

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/juju/errors"
	"github.com/temoto/alive"
	"github.com/temoto/vender/log2"
	"periph.io/x/periph/conn/gpio"
	"periph.io/x/periph/conn/gpio/gpioreg"
	"periph.io/x/periph/conn/physic"
	"periph.io/x/periph/conn/spi"
	"periph.io/x/periph/conn/spi/spireg"
	"periph.io/x/periph/host"
)

const modName string = "mega-client"
const DefaultTimeout = 100 * time.Millisecond

var ErrResponseEmpty = errors.New("mega response empty")

type Client struct {
	Log      *log2.Log
	TwiChan  chan uint16
	txlk     sync.Mutex
	spiPort  spi.Port
	spiConn  spi.Conn
	pin      gpio.PinIO
	refcount int32
	alive    *alive.Alive
	stat     Stat
}
type Stat struct {
	Request   uint32
	Error     uint32
	TwiListen uint32
	Reset     uint32
}

func NewClient(bus string, notifyPinName string, log *log2.Log) (*Client, error) {
	if _, err := host.Init(); err != nil {
		return nil, errors.Annotate(err, "periph/init")
	}

	spiPort, err := spireg.Open(bus)
	if err != nil {
		return nil, errors.Annotate(err, "SPI Open")
	}
	spiSpeed := 400 * physic.KiloHertz
	spiMode := spi.Mode(0)
	spiConn, err := spiPort.Connect(spiSpeed, spiMode, 8)
	if err != nil {
		spiPort.Close()
		return nil, errors.Annotate(err, "SPI Connect")
	}

	pin := gpioreg.ByName(notifyPinName)
	if pin == nil {
		spiPort.Close()
		return nil, errors.Annotate(err, "notify pin open")
	}
	err = pin.In(gpio.PullDown, gpio.RisingEdge)
	if err != nil {
		spiPort.Close()
		return nil, errors.Annotate(err, "notify pin setup")
	}

	self := &Client{
		Log:     log,
		spiPort: spiPort,
		spiConn: spiConn,
		pin:     pin,
		alive:   alive.NewAlive(),
		TwiChan: make(chan uint16, TWI_LISTEN_MAX_LENGTH/2),
	}

	// try to read RESET
	f := new(Frame)
	err = self.readParse(f)
	switch err {
	case ErrResponseEmpty:
		// mega was reset at other time and RESET was read
	case nil:
		// TODO header==RESET -> OK, other -> log error
		if f.ResponseKind() != RESPONSE_RESET {
			self.Log.Errorf("first frame not reset: %s", f.ResponseString())
		}
	default:
		self.Close()
		return nil, err
	}

	go self.pinLoop()

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

	var err error
	if send != nil {
		err = self.write(send)
		if err != nil {
			atomic.AddUint32(&self.stat.Error, 1)
			return err
		}
	}

	// Tx() invoked to send only
	if recv == nil {
		return nil
	}

	var timeoutErr error

wait:
	if self.pin.Read() == gpio.High {
		goto read
	}
	if timeout > 0 {
		edge := self.pin.WaitForEdge(timeout)
		if edge || (self.pin.Read() == gpio.High) {
			goto read
		}
		timeoutErr = errors.Timeoutf("mega-client Tx() send=%x response timeout=%s", send.Bytes(), timeout)
	}

read:
	err = self.readParse(recv)
	switch err {
	case ErrResponseEmpty:
		// wait again
	case nil:
		switch recv.ResponseKind() {
		case RESPONSE_TWI_LISTEN, RESPONSE_RESET:
			goto wait
		}
		return nil
	default:
		return err
	}
	if timeout == 0 {
		return err
	}
	if timeoutErr != nil {
		return timeoutErr
	}
	goto wait
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
			tmp := Frame{}
			err = self.readParse(&tmp)
			if err != nil {
				self.Log.Errorf("stray read err=%v", err)
				return err
			}
			self.Log.Debugf("stray read frame=%s", tmp.ResponseString())
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

func (self *Client) pinLoop() {
	const edgeTimeout = 1 * time.Second
	const expectLevel = gpio.High
	var err error
	var pinFrame Frame
	for {
		edge := self.pin.WaitForEdge(edgeTimeout)
		if !self.alive.IsRunning() {
			break
		}
		if !(edge || (self.pin.Read() == expectLevel)) {
			continue
		}

		// re-read pin with lock to skip reads intended for concurrent Tx()
		self.txlk.Lock()
		shouldRead := self.pin.Read() == expectLevel
		if shouldRead {
			err = self.readParse(&pinFrame)
		}
		self.txlk.Unlock()
		if !shouldRead {
			break
		}
		switch err {
		case ErrResponseEmpty:
			// pinLoop expects "side" frames (twi,reset) only and they are hidden by readParse()
		case nil:
			self.Log.Errorf("%s pinLoop() CRITICAL stray packet=%s debug=%x", modName, pinFrame.ResponseString(), pinFrame.Bytes())
		default:
			self.Log.Errorf("%s pinLoop() read err=%v", modName, err)
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

func (self *Client) readParse(f *Frame) error {
	remoteLength, err := self.poll()
	if remoteLength == 0 {
		return ErrResponseEmpty
	}

	var buf [BUFFER_SIZE + totalOverheads]byte
	bs := buf[:remoteLength+totalOverheads]
	bs[0] = ProtocolVersion
	// self.Log.Debugf("%s read out=%x", modName, bs)
	err = self.spiConn.Tx(bs, bs)
	// self.Log.Debugf("%s read -in=%x err=%v", modName, bs, err)
	if err != nil {
		return err
	}

	err = self.parse(bs, f)
	if err != nil {
		return err
	}
	err = self.ack(f)
	if err != nil {
		return err
	}
	switch f.ResponseKind() {
	case RESPONSE_TWI_LISTEN, RESPONSE_RESET:
		// consume asynchronous frames
		return ErrResponseEmpty
	}
	return nil
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

	if _, _, err = parsePadding(buf[:], true); err != nil {
		return err
	}

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
