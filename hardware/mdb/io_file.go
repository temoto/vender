package mdb

import (
	"bufio"
	"os"
	"syscall"
	"time"
	"github.com/juju/errors"
)

type fileUart struct {
	f      *os.File
	reader fdReader
	r      *bufio.Reader
	t2     termios2
}

func NewFileUart() *fileUart { return &fileUart{} }

func (self *fileUart) set9(b bool) error { return io_set9(self.f.Fd(), &self.t2, b) }

func (self *fileUart) write(p []byte) (int, error) { return self.f.Write(p) }

func (self *fileUart) Break(d time.Duration) (err error) {
	ms := int(d / time.Millisecond)
	if err = self.resetRead(); err != nil {
		return errors.Trace(err)
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
	self.reader = fdReader{fd: uintptr(self.f.Fd()), timeout: 20 * time.Millisecond}
	self.r = bufio.NewReader(self.reader)
	err = io_reset_termios(self.f.Fd(), &self.t2, baud, 0)
	if err != nil {
		self.f.Close()
		self.f = nil
		self.r = nil
		return errors.Trace(err)
	}
	return nil
}

func (self *fileUart) ReadByte() (byte, error) { return self.r.ReadByte() }

func (self *fileUart) ReadSlice(delim byte) ([]byte, error) { return self.r.ReadSlice(delim) }

func (self *fileUart) ResetRead() (err error) {
	self.r.Reset(self.reader)
	if err = self.set9(false); err != nil {
		return err
	}
	return nil
}
