package mdb

import (
	"bufio"
	"io"
	"os"
	"runtime/debug"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"github.com/juju/errors"
	"github.com/temoto/vender/log2"
	"golang.org/x/sys/unix"
)

type fileUart struct {
	Log *log2.Log
	f   *os.File // set nil in test
	fd  uintptr
	r   io.Reader // override in test
	w   io.Writer // override in test
	br  *bufio.Reader
	t2  termios2
	lk  sync.Mutex
}

func NewFileUart(l *log2.Log) *fileUart {
	return &fileUart{
		Log: l,
		br:  bufio.NewReader(nil),
	}
}

func (self *fileUart) set9(b bool) error {
	last_parodd := (self.t2.c_cflag & syscall.PARODD) == syscall.PARODD
	if b == last_parodd {
		return nil
	}
	if b {
		self.t2.c_cflag |= syscall.PARODD
	} else {
		self.t2.c_cflag &= ^tcflag_t(syscall.PARODD)
	}
	if self.f == nil { // used in tests
		return nil
	}
	// must use ioctl with drain - cTCSETSW2?
	// but it makes 9bit switch very slow
	err := ioctl(self.fd, uintptr(cTCSETS2), uintptr(unsafe.Pointer(&self.t2)))
	return errors.Trace(err)
}

func (self *fileUart) write9(p []byte, start9 bool) (n int, err error) {
	// self.Log.Debugf("mdb.write9 p=%x start9=%t", p, start9)
	var n2 int
	switch len(p) {
	case 0:
		return 0, nil
	case 1:
		if err = self.set9(start9); err != nil {
			return 0, errors.Trace(err)
		}
		if n, err = self.w.Write(p[:1]); err != nil {
			return 0, errors.Trace(err)
		}
		fallthrough
	default:
		if err = self.set9(false); err != nil {
			return n, errors.Trace(err)
		}
		if n2, err = self.w.Write(p[1:]); err != nil {
			return n, errors.Trace(err)
		}
	}
	return n + n2, nil
}

func (self *fileUart) Break(d, sleep time.Duration) (err error) {
	const tag = "fileUart.Break"
	ms := int(d / time.Millisecond)
	self.lk.Lock()
	defer self.lk.Unlock()
	if err = self.resetRead(); err != nil {
		return errors.Annotate(err, tag)
	}
	if err = ioctl(self.fd, uintptr(cTCSBRKP), uintptr(ms/100)); err != nil {
		return errors.Annotate(err, tag)
	}
	time.Sleep(sleep)
	return nil
}

func (self *fileUart) Close() error {
	self.f = nil
	self.r = nil
	self.w = nil
	return errors.Trace(self.f.Close())
}

func (self *fileUart) Open(path string) (err error) {
	if self.f != nil {
		self.Close() // skip error
	}
	self.f, err = os.OpenFile(path, syscall.O_RDWR|syscall.O_NOCTTY|syscall.O_NDELAY, 0600)
	if err != nil {
		return errors.Annotate(err, "fileUart.Open:OpenFile")
	}
	self.fd = self.f.Fd()
	self.r = fdReader{fd: self.fd, timeout: 20 * time.Millisecond}
	self.br.Reset(self.r)
	self.w = self.f

	self.t2 = termios2{
		c_iflag:  unix.IGNBRK | unix.INPCK | unix.PARMRK,
		c_lflag:  0,
		c_cflag:  cCMSPAR | syscall.CLOCAL | syscall.CREAD | unix.CSTART | syscall.CS8 | unix.PARENB | unix.PARMRK | unix.IGNPAR,
		c_ispeed: speed_t(unix.B9600),
		c_ospeed: speed_t(unix.B9600),
	}
	self.t2.c_cc[syscall.VMIN] = cc_t(0)
	err = ioctl(self.fd, uintptr(cTCSETSF2), uintptr(unsafe.Pointer(&self.t2)))
	if err != nil {
		self.Close()
		return errors.Annotate(err, "fileUart.Open:ioctl")
	}
	var ser serial_info
	err = ioctl(self.fd, uintptr(cTIOCGSERIAL), uintptr(unsafe.Pointer(&ser)))
	if err != nil {
		self.Log.Errorf("get serial fail err=%v", err)
	} else {
		ser.flags |= cASYNC_LOW_LATENCY
		err = ioctl(self.fd, uintptr(cTIOCSSERIAL), uintptr(unsafe.Pointer(&ser)))
		if err != nil {
			self.Log.Errorf("set serial fail err=%v", err)
		}
	}
	return nil
}

func (self *fileUart) Tx(request, response []byte) (n int, err error) {
	if len(request) == 0 {
		return 0, errors.New("Tx request empty")
	}

	// TODO feed IO operations to loop in always running goroutine
	// that would also eliminate lock
	self.lk.Lock()
	defer self.lk.Unlock()

	saveGCPercent := debug.SetGCPercent(-1)
	defer debug.SetGCPercent(saveGCPercent)

	// FIXME crutch to avoid slow set9 with drain
	time.Sleep(20 * time.Millisecond)
	// TODO
	// self.f.SetDeadline(time.Now().Add(time.Second))
	// defer self.f.SetDeadline(time.Time{})

	chkoutb := []byte{checksum(request)}
	if _, err = self.write9(request, true); err != nil {
		return 0, errors.Trace(err)
	}
	if _, err = self.write9(chkoutb, false); err != nil {
		return 0, errors.Trace(err)
	}

	// ack must arrive <5ms after recv
	// begin critical path
	if err = self.resetRead(); err != nil {
		return 0, errors.Trace(err)
	}
	n, err = bufferReadPacket(self.br, response)
	if err != nil {
		return 0, errors.Trace(err)
	}
	chkin := response[n-1]
	n--
	chkcomp := checksum(response[:n])
	if chkin != chkcomp {
		// self.Log.Debugf("mdb.fileUart.Tx InvalidChecksum frompacket=%x actual=%x", chkin, chkcomp)
		return n, errors.Trace(InvalidChecksum{Received: chkin, Actual: chkcomp})
	}
	if n > 0 {
		_, err = self.write9(PacketNul1.b[:1], false)
	}
	// end critical path
	return n, errors.Trace(err)
}

func bufferReadPacket(src *bufio.Reader, dst []byte) (n int, err error) {
	var b byte
	var part []byte

	for {
		part, err = src.ReadSlice(0xff)
		if (err == io.EOF && len(part) == 0) || err != nil {
			return n, errors.Trace(err)
		}
		// self.Log.Debugf("bufferReadPacket readFF=%x", part)
		pl := len(part)
		// TODO check n+pl overflow
		n += copy(dst[n:], part[:pl-1])
		// self.Log.Debugf("bufferReadPacket append %02d dst=%x", pl-1, dst[:n])
		if b, err = src.ReadByte(); err != nil {
			return n, errors.Trace(err)
		}
		// self.Log.Debugf("bufferReadPacket readByte=%02x", b)
		switch b {
		case 0x00:
			if b, err = src.ReadByte(); err != nil {
				return n, errors.Trace(err)
			}
			// self.Log.Debugf("bufferReadPacket seq=ff00 chk=%02x", b)
			dst[n] = b
			n++
			// self.Log.Debugf("bufferReadPacket dst=%x next=copy,return", dst[:n])
			return n, nil
		case 0xff:
			dst[n] = b
			n++
			// self.Log.Debugf("bufferReadPacket seq=ffff dst=%x", dst[:n])
		default:
			err = errors.NotValidf("bufferReadPacket unknown sequence ff %x", b)
			return n, err
		}
	}
}

func (self *fileUart) resetRead() (err error) {
	self.br.Reset(self.r)
	if err = self.set9(false); err != nil {
		return errors.Trace(err)
	}
	return nil
}

const (
	//lint:ignore U1000 unused
	cBOTHER = 0x1000
	cCMSPAR = 0x40000000

	cNCCS              = 19
	cASYNC_LOW_LATENCY = 1 << 13

	cFIONREAD    = 0x541b
	cTCSBRKP     = 0x5425
	cTIOCGSERIAL = 0x541E
	cTIOCSSERIAL = 0x541F
	cTCSETS2     = 0x402c542b
	//lint:ignore U1000 unused
	cTCSETSW2 = 0x402c542c // flush output TODO verify
	cTCSETSF2 = 0x402c542d // flush both input,output TODO verify
)

type cc_t byte
type speed_t uint32
type tcflag_t uint32
type termios2 struct {
	c_iflag tcflag_t // input mode flags
	//lint:ignore U1000 unused
	c_oflag tcflag_t // output mode flags
	c_cflag tcflag_t // control mode flags
	c_lflag tcflag_t // local mode flags
	//lint:ignore U1000 unused
	c_line   cc_t        // line discipline
	c_cc     [cNCCS]cc_t // control characters
	c_ispeed speed_t     // input speed
	c_ospeed speed_t     // output speed
}
type serial_info struct {
	_type          int32  //lint:ignore U1000 unused
	line           int32  //lint:ignore U1000 unused
	port           uint32 //lint:ignore U1000 unused
	irq            int32  //lint:ignore U1000 unused
	flags          int32
	xmit_fifo_size int32     //lint:ignore U1000 unused
	_pad           [200]byte //lint:ignore U1000 unused
}

type fdReader struct {
	fd      uintptr
	timeout time.Duration
}

func (self fdReader) Read(p []byte) (n int, err error) {
	err = io_wait_read(self.fd, 1, self.timeout)
	if err != nil {
		return 0, errors.Trace(err)
	}
	// TODO bench optimist read, then io_wait if needed
	n, err = syscall.Read(int(self.fd), p)
	if err != nil {
		err = errors.Trace(err)
	}
	return n, err
}

func ioctl(fd uintptr, op, arg uintptr) (err error) {
	if fd+1 == 0 { // mock for test
		return nil
	}
	r, _, errno := syscall.Syscall(syscall.SYS_IOCTL, fd, op, arg)
	if errno != 0 {
		err = os.NewSyscallError("SYS_IOCTL", errno)
	} else if r != 0 {
		err = errors.New("unknown error from SYS_IOCTL")
	}
	// if err != nil {
	// 	log.Printf("mdb.ioctl op=%x arg=%x err=%s", op, arg, err)
	// }
	return errors.Annotate(err, "ioctl")
}

func io_wait_read(fd uintptr, min int, wait time.Duration) error {
	var err error
	var out int
	tbegin := time.Now()
	tfinal := tbegin.Add(wait)
	for {
		err = ioctl(fd, uintptr(cFIONREAD), uintptr(unsafe.Pointer(&out)))
		if err != nil {
			return errors.Trace(err)
		}
		if out >= min {
			return nil
		}
		time.Sleep(wait / 16)
		if time.Now().After(tfinal) {
			return errors.Timeoutf("mdb io_wait_read timeout")
		}
	}
}
