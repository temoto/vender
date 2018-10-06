package mdb

import (
	"log"
	"syscall"
	"time"
)

// WIP Uarter with low-level syscalls
type fastUart struct {
	fd     int
	reader fdReader
	buf    [PacketMaxLength]byte // read storage array
	br     []byte                // ready to read, starts at buf[bri:]
	bri    int                   // buf[:bri] was consumed
	t2     termios2
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
	self.reader = fdReader{fd: uintptr(self.fd), timeout: 20 * time.Millisecond}
	return io_reset_termios(uintptr(self.fd), &self.t2, baud, 1)
}

func (self *fastUart) ReadByte() (b byte, err error) {
	if len(self.br) < 1 {
		n, err := self.reader.Read(self.buf[self.bri:])
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
	l2, result, err := io_read_slice(self.buf[self.bri:], l1, self.reader, delim)
	if err != nil {
		return nil, err
	}
	self.br = self.buf[self.bri : self.bri+l2]
	log.Printf("fast read %x", self.br)
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
