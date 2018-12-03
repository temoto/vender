package mdb

import (
	"bufio"
	"io"
	"log"
	"os"
	"runtime/debug"
	"syscall"
	"time"
	"unsafe"

	"github.com/juju/errors"
	"golang.org/x/sys/unix"
)

type fileUart struct {
	f  *os.File // set nil in test
	fd uintptr
	r  io.Reader // override in test
	w  io.Writer // override in test
	br *bufio.Reader
	t2 termios2
}

func NewFileUart() *fileUart {
	return &fileUart{
		br: bufio.NewReader(nil),
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
	// log.Printf("debug: mdb.write9 p=%x start9=%t", p, start9)
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

func (self *fileUart) Break(d time.Duration) (err error) {
	ms := int(d / time.Millisecond)
	if err = self.resetRead(); err != nil {
		return errors.Annotate(err, "fileUart.Break")
	}
	return ioctl(self.fd, uintptr(cTCSBRKP), uintptr(ms/100))
}

func (self *fileUart) Close() error {
	self.f = nil
	self.r = nil
	self.w = nil
	return self.f.Close()
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
		log.Printf("get serial fail err=%v", err)
	} else {
		ser.flags |= cASYNC_LOW_LATENCY
		err = ioctl(self.fd, uintptr(cTIOCSSERIAL), uintptr(unsafe.Pointer(&ser)))
		if err != nil {
			log.Printf("set serial fail err=%v", err)
		}
	}
	return nil
}

func (self *fileUart) Tx(request, response []byte) (n int, err error) {
	if len(request) == 0 {
		return 0, errors.New("Tx request empty")
	}

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
		// log.Printf("debug: mdb.fileUart.Tx InvalidChecksum frompacket=%x actual=%x", chkin, chkcomp)
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
		if part, err = src.ReadSlice(0xff); err != nil {
			return n, errors.Trace(err)
		}
		// log.Printf("bufferReadPacket readFF=%x", part)
		pl := len(part)
		// TODO check n+pl overflow
		n += copy(dst[n:], part[:pl-1])
		// log.Printf("bufferReadPacket append %02d dst=%x", pl-1, dst[:n])
		if b, err = src.ReadByte(); err != nil {
			return n, errors.Trace(err)
		}
		// log.Printf("bufferReadPacket readByte=%02x", b)
		switch b {
		case 0x00:
			if b, err = src.ReadByte(); err != nil {
				return n, errors.Trace(err)
			}
			// log.Printf("bufferReadPacket seq=ff00 chk=%02x", b)
			dst[n] = b
			n++
			// log.Printf("bufferReadPacket dst=%x next=copy,return", dst[:n])
			return n, nil
		case 0xff:
			dst[n] = b
			n++
			// log.Printf("bufferReadPacket seq=ffff dst=%x", dst[:n])
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
	cBOTHER            = 0x1000
	cCMSPAR            = 0x40000000
	cFIONREAD          = 0x541b
	cNCCS              = 19
	cTCSBRKP           = 0x5425
	cTCSETS2           = 0x402c542b
	cTCSETSW2          = 0x402c542c // flush output TODO verify
	cTCSETSF2          = 0x402c542d // flush both input,output TODO verify
	cASYNC_LOW_LATENCY = 1 << 13
	cTIOCGSERIAL       = 0x541E
	cTIOCSSERIAL       = 0x541F
)

type cc_t byte
type speed_t uint32
type tcflag_t uint32
type termios2 struct {
	c_iflag  tcflag_t    // input mode flags
	c_oflag  tcflag_t    // output mode flags
	c_cflag  tcflag_t    // control mode flags
	c_lflag  tcflag_t    // local mode flags
	c_line   cc_t        // line discipline
	c_cc     [cNCCS]cc_t // control characters
	c_ispeed speed_t     // input speed
	c_ospeed speed_t     // output speed
}
type serial_info struct {
	_type          int32
	line           int32
	port           uint32
	irq            int32
	flags          int32
	xmit_fifo_size int32
	_pad           [200]byte
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
	return syscall.Read(int(self.fd), p)
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
	if err != nil {
		// log.Printf("debug: mdb.ioctl op=%x arg=%x err=%s", op, arg, err)
	}
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
