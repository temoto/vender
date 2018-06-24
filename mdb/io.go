package mdb

import (
	"bufio"
	"errors"
	"golang.org/x/sys/unix"
	"log"
	"os"
	"syscall"
	"unsafe"
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

func (self *MDB) resetTermios(baud int, vmin byte) (err error) {
	if baud != 9600 {
		return errors.New("Not implemented support for baud rate other than 9600")
	}
	self.t2 = termios2{
		c_iflag:  unix.IGNBRK | unix.INPCK | unix.PARMRK,
		c_cflag:  cCMSPAR | syscall.CLOCAL | syscall.CREAD | unix.CSTART | syscall.CS8 | unix.PARENB | unix.PARMRK | unix.IGNPAR,
		c_ispeed: speed_t(unix.B9600),
		c_ospeed: speed_t(unix.B9600),
	}
	self.recvBuf = make([]byte, 0, PacketMaxLength)
	self.r = bufio.NewReader(self.f)
	self.w = self.f
	self.t2.c_cc[syscall.VMIN] = cc_t(vmin)
	self.last_parodd = false
	if err = self.tcsetsf2(); err != nil {
		self.f.Close()
		self.f = nil
		self.r = nil
		self.w = nil
		return err
	}
	return nil
}

func (self *MDB) ioctl(op, arg uintptr) (err error) {
	if self.skip_ioctl {
		return nil
	}
	r, _, errno := syscall.Syscall(syscall.SYS_IOCTL, uintptr(self.f.Fd()), op, arg)
	if errno != 0 {
		err = os.NewSyscallError("SYS_IOCTL", errno)
	} else if r != 0 {
		err = errors.New("unknown error from SYS_IOCTL")
	}
	if err != nil && self.Debug {
		log.Printf("debug: MDB.ioctl op=%x arg=%x err=%s", op, arg, err)
	}
	return err
}

func (self *MDB) tcsets2() error {
	self.last_parodd = (self.t2.c_cflag & syscall.PARODD) == syscall.PARODD
	return self.ioctl(uintptr(cTCSETS2), uintptr(unsafe.Pointer(&self.t2)))
}

// flush input and output
func (self *MDB) tcsetsf2() error {
	self.last_parodd = (self.t2.c_cflag & syscall.PARODD) == syscall.PARODD
	return self.ioctl(uintptr(cTCSETSF2), uintptr(unsafe.Pointer(&self.t2)))
}

func (self *MDB) set9(b bool) error {
	if b == self.last_parodd {
		return nil
	}
	if b {
		self.t2.c_cflag |= syscall.PARODD
	} else {
		self.t2.c_cflag &= ^tcflag_t(syscall.PARODD)
	}
	return self.tcsets2()
}
