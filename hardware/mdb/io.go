package mdb

import (
	"bufio"
	"bytes"
	"errors"
	"io"
	"log"
	"os"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/unix"
)

const (
	cBOTHER   = 0x1000
	cCMSPAR   = 0x40000000
	cFIONREAD = 0x541b
	cNCCS     = 19
	cTCSBRKP  = 0x5425
	cTCSETS2  = 0x402c542b
	cTCSETSF2 = 0x402c542d
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

func ioctl(fd uintptr, op, arg uintptr) (err error) {
	r, _, errno := syscall.Syscall(syscall.SYS_IOCTL, fd, op, arg)
	if errno != 0 {
		err = os.NewSyscallError("SYS_IOCTL", errno)
	} else if r != 0 {
		err = errors.New("unknown error from SYS_IOCTL")
	}
	if err != nil {
		log.Printf("debug: mdb.ioctl op=%x arg=%x err=%s", op, arg, err)
	}
	return err
}

// store[:b] already consumed, ignore
// store[b:l] ready to consume
// store[l:] space for reads
func io_read_slice(store []byte, l int, r io.Reader, delim byte) (int, []byte, error) {
	for {
		if l > 0 {
			if di := bytes.IndexByte(store[:l], delim); di >= 0 {
				result := store[:di+1]
				return l, result, nil
			}
		}
		n, err := r.Read(store[l:])
		if err != nil {
			return l, nil, err
		}
		l += n
	}
}

func io_wait_read(fd uintptr, min int, wait time.Duration) (ok bool, err error) {
	var out int
	tbegin := time.Now()
	tfinal := tbegin.Add(wait)
	for {
		err = ioctl(fd, uintptr(cFIONREAD), uintptr(unsafe.Pointer(&out)))
		if err != nil {
			return false, err
		}
		if out >= min {
			return true, nil
		}
		time.Sleep(wait / 20)
		if time.Now().After(tfinal) {
			return false, nil
		}
	}
}

func io_reset_termios(fd uintptr, t2 *termios2, baud int, vmin byte) error {
	if baud != 9600 {
		return errors.New("Not implemented support for baud rate other than 9600")
	}
	*t2 = termios2{
		c_iflag:  unix.IGNBRK | unix.INPCK | unix.PARMRK,
		c_cflag:  cCMSPAR | syscall.CLOCAL | syscall.CREAD | unix.CSTART | syscall.CS8 | unix.PARENB | unix.PARMRK | unix.IGNPAR,
		c_ispeed: speed_t(unix.B9600),
		c_ospeed: speed_t(unix.B9600),
	}
	t2.c_cc[syscall.VMIN] = cc_t(vmin)
	return io_tcsetsf2(fd, t2)
}

func io_set9(fd uintptr, t2 *termios2, b bool) error {
	last_parodd := (t2.c_cflag & syscall.PARODD) == syscall.PARODD
	if b == last_parodd {
		return nil
	}
	if b {
		t2.c_cflag |= syscall.PARODD
	} else {
		t2.c_cflag &= ^tcflag_t(syscall.PARODD)
	}
	err := io_tcsets2(fd, t2)
	return err
}

func io_tcsets2(fd uintptr, t2 *termios2) error {
	return ioctl(fd, uintptr(cTCSETS2), uintptr(unsafe.Pointer(t2)))
}

// flush input and output
func io_tcsetsf2(fd uintptr, t2 *termios2) error {
	return ioctl(fd, uintptr(cTCSETSF2), uintptr(unsafe.Pointer(t2)))
}

func io_write(u Uarter, p []byte, start9 bool) (err error) {
	if len(p) == 0 {
		return nil
	}
	if err = u.set9(start9); err != nil {
		return err
	}
	if _, err = u.write(p[:1]); err != nil {
		return err
	}
	if len(p) > 1 {
		if err = u.set9(false); err != nil {
			return err
		}
		if _, err = u.write(p[1:]); err != nil {
			return err
		}
	}
	return nil
}

type Uarter interface {
	Open(path string, baud int) error
	Break(d time.Duration) error
	ResetRead() error
	ReadSlice(delim byte) ([]byte, error)
	ReadByte() (byte, error)
	Close() error

	set9(bool) error
	write(p []byte) (int, error)
}

// Uarter with standard os.File
type fileUart struct {
	f  *os.File
	r  *bufio.Reader
	t2 termios2
}

func NewFileUart() *fileUart { return &fileUart{} }

func (self *fileUart) set9(b bool) error { return io_set9(self.f.Fd(), &self.t2, b) }

func (self *fileUart) write(p []byte) (int, error) { return self.f.Write(p) }

func (self *fileUart) Break(d time.Duration) (err error) {
	ms := int(d / time.Millisecond)
	if err = self.ResetRead(); err != nil {
		return err
	}
	return ioctl(self.f.Fd(), uintptr(cTCSBRKP), uintptr(ms/100))
}

func (self *fileUart) Close() error { return self.f.Close() }

func (self *fileUart) Open(path string, baud int) (err error) {
	if self.f != nil {
		self.f.Close()
	}
	self.f, err = os.OpenFile(path, syscall.O_RDWR|syscall.O_NOCTTY, 0600)
	if err != nil {
		return err
	}
	self.r = bufio.NewReader(self.f)
	err = io_reset_termios(self.f.Fd(), &self.t2, baud, 1)
	if err != nil {
		self.f.Close()
		self.f = nil
		self.r = nil
		return err
	}
	return nil
}

func (self *fileUart) ReadByte() (byte, error) { return self.r.ReadByte() }

func (self *fileUart) ReadSlice(delim byte) ([]byte, error) { return self.r.ReadSlice(delim) }

func (self *fileUart) ResetRead() (err error) {
	self.r.Reset(self.f)
	if err = self.set9(false); err != nil {
		return err
	}
	return nil
}

// WIP Uarter with low-level syscalls
type fastUart struct {
	fd  int
	buf [PacketMaxLength]byte // read storage array
	br  []byte                // ready to read, starts at buf[bri:]
	bri int                   // buf[:bri] was consumed
	t2  termios2
}

type fdReader int

func (fd fdReader) Read(p []byte) (int, error) {
	// io_wait_read(self.fd, 1, wait)
	return syscall.Read(int(fd), p)
}

func NewFastUart() *fastUart { return &fastUart{fd: -1} }

func (self *fastUart) set9(b bool) error { return io_set9(uintptr(self.fd), &self.t2, b) }

func (self *fastUart) write(p []byte) (int, error) { return syscall.Write(self.fd, p) }

func (self *fastUart) Break(d time.Duration) (err error) {
	ms := int(d / time.Millisecond)
	if err = self.ResetRead(); err != nil {
		return err
	}
	return ioctl(uintptr(self.fd), uintptr(cTCSBRKP), uintptr(ms/100))
}

func (self *fastUart) Close() error {
	self.ResetRead()
	err := syscall.Close(self.fd)
	self.fd = -1
	return err
}

func (self *fastUart) Open(path string, baud int) (err error) {
	if self.fd < 0 {
		if err = self.Close(); err != nil {
			return err
		}
	}

	perm := uint32(0600)
	const O_DIRECT = 0x4000
	flag := syscall.O_RDWR | syscall.O_CLOEXEC | syscall.O_NOCTTY
	// if linux
	flag |= O_DIRECT
	// TODO
	// flag |= syscall.O_NONBLOCK
	self.fd, err = syscall.Open(path, syscall.O_RDWR|syscall.O_CLOEXEC|syscall.O_NOCTTY|O_DIRECT, perm)
	if err != nil {
		return err
	}
	syscall.CloseOnExec(self.fd)

	// TODO try vmin=0 + waitRecv
	return io_reset_termios(uintptr(self.fd), &self.t2, baud, 1)
}

func (self *fastUart) ReadByte() (b byte, err error) {
	if len(self.br) < 1 {
		n, err := fdReader(self.fd).Read(self.buf[self.bri:])
		if err != nil {
			return 0, err
		}
		self.br = self.buf[self.bri : self.bri+n]
	}
	b = self.br[0]
	self.br = self.br[1:]
	self.bri++
	return b, nil
}

func (self *fastUart) ReadSlice(delim byte) ([]byte, error) {
	l1 := len(self.br)
	l2, result, err := io_read_slice(self.buf[self.bri:], l1, fdReader(self.fd), delim)
	if err != nil {
		return nil, err
	}
	self.br = self.buf[self.bri : self.bri+l2]
	return result, nil
}

func (self *fastUart) ResetRead() error {
	self.br = nil
	self.bri = 0
	if err := self.set9(false); err != nil {
		return err
	}
	return nil
}

// Mock Uarter for tests
type nullUart struct {
	src io.Reader
	r   *bufio.Reader
	w   io.Writer
}

func NewNullUart(r io.Reader, w io.Writer) *nullUart {
	return &nullUart{
		src: r,
		r:   bufio.NewReader(r),
		w:   w,
	}
}

func (self *nullUart) set9(b bool) error { return nil }

func (self *nullUart) read(p []byte) (int, error) { return self.r.Read(p) }

func (self *nullUart) write(p []byte) (int, error) { return self.w.Write(p) }

func (self *nullUart) Break(d time.Duration) (err error) {
	self.ResetRead()
	time.Sleep(d)
	return nil
}

func (self *nullUart) Close() error {
	self.src = nil
	self.r = nil
	self.w = nil
	return nil
}

func (self *nullUart) Open(path string, baud int) (err error) { return nil }

func (self *nullUart) ReadByte() (byte, error) { return self.r.ReadByte() }

func (self *nullUart) ReadSlice(delim byte) ([]byte, error) { return self.r.ReadSlice(delim) }

func (self *nullUart) ResetRead() error {
	self.r.Reset(self.src)
	return nil
}
