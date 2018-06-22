package mdb

import (
	"bufio"
	"errors"
	"fmt"
	"golang.org/x/sys/unix"
	"log"
	"os"
	"sync"
	"syscall"
	"time"
	"unsafe"
)

const (
	cBOTHER  = 0x1000
	cCMSPAR  = 0x40000000
	cNCCS    = 19
	cTCSETS2 = 0x402C542B
	cTCSBRKP = 0x5425
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
type MDB struct {
	Debug bool

	bin    []byte
	f      *os.File
	lk     sync.Mutex
	r      *bufio.Reader
	skipIO bool // for tests and benchmark
	t2     termios2
}

var (
	InvalidChecksum = errors.New("Invalid checksum")
)

func (self *MDB) Open(path string, baud int, vmin byte) (err error) {
	if self.f != nil {
		self.f.Close()
	}
	if baud != 9600 {
		return errors.New("Not implemented support for baud rate other than 9600")
	}
	self.f, err = os.OpenFile(path, syscall.O_RDWR|syscall.O_NOCTTY, 0600)
	if err != nil {
		return err
	}
	self.t2 = termios2{
		c_iflag: unix.IGNBRK | unix.INPCK | unix.PARMRK,
		//c_cflag:  syscall.CLOCAL | syscall.CREAD | syscall.PARENB | cCMSPAR,
		c_cflag:  0x400009bd | unix.PARENB,
		c_ispeed: speed_t(unix.B9600),
		c_ospeed: speed_t(unix.B9600),
	}
	self.bin = make([]byte, 37)
	self.r = bufio.NewReader(self.f)
	self.t2.c_cc[syscall.VMIN] = cc_t(vmin)
	if err = self.tcsets2(); err != nil {
		self.f.Close()
		return err
	}
	return nil
}

func (self *MDB) ioctl(op, arg uintptr) (err error) {
	if self.skipIO {
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
	return self.ioctl(uintptr(cTCSETS2), uintptr(unsafe.Pointer(&self.t2)))
}

func (self *MDB) set9(b bool) error {
	if b {
		self.t2.c_cflag |= syscall.PARODD
	} else {
		self.t2.c_cflag &= ^tcflag_t(syscall.PARODD)
	}
	return self.tcsets2()
}

func (self *MDB) BreakCustom(keep, sleep int) (err error) {
	if self.Debug {
		log.Printf("debug: MDB.BreakCustom keep=%d sleep=%d", keep, sleep)
	}
	err = self.ioctl(uintptr(cTCSBRKP), uintptr(int(keep/100)))
	if err == nil {
		time.Sleep(time.Duration(sleep) * time.Millisecond)
	}
	return err
}

func (self *MDB) locked_send(b []byte) (err error) {
	if len(b) == 0 {
		return nil
	}

	var chk byte
	for _, x := range b {
		chk += x
	}
	b = append(b, chk)
	if self.Debug {
		log.Printf("debug: MDB.send  out='%x'", b)
	}

	// begin critical path
	if err = self.set9(true); err != nil {
		return
	}
	if _, err = self.f.Write(b[:1]); err != nil {
		return
	}
	if err = self.set9(false); err != nil {
		return
	}
	if _, err = self.f.Write(b[1:]); err != nil {
		return
	}
	// end critical path

	return nil
}

func (self *MDB) locked_sendByte(out byte) error {
	b := [2]byte{out, 0}
	return self.locked_send(b[:1])
}

func (self *MDB) locked_recv() ([]byte, error) {
	var err error
	var b, chkin, chkout byte
	var part []byte
	nmax := cap(self.bin)
	self.bin = self.bin[:0]
	// begin critical path
	if err = self.set9(false); err != nil {
		return nil, err
	}
recvLoop:
	for {
		if part, err = self.r.ReadSlice(0xff); err != nil {
			return nil, err
		}
		n := len(part)
		if n > 1 {
			self.bin = append(self.bin, part[:n-1]...)
		}
		if b, err = self.r.ReadByte(); err != nil {
			return nil, err
		}
		switch b {
		case 0x00:
			if chkin, err = self.r.ReadByte(); err != nil {
				return nil, err
			}
			break recvLoop
		case 0xff:
			self.bin = append(self.bin, b)
		default:
			err = fmt.Errorf("recv unknown sequence ff %x", b)
			return nil, err
		}
		if len(self.bin) > nmax {
			err = errors.New("recv self.bin overflow")
			return nil, err
		}
	}
	// end critical path

	for _, b = range self.bin {
		chkout += b
	}
	if self.Debug {
		log.Printf("debug: MDB.recv  bin='%x' chkin=%x chkout=%x", self.bin, chkin, chkout)
		log.Printf("debug: MDB.recv offset 1 2 3 4 5 6 7 8 910 1 2 3 4 5 6 7 8 920 1 2 3 4 5 6 7 8 930 1 2 3 4 5 6 7 8 940'"[:len(self.bin)*2+22])
	}
	if chkin != chkout {
		return nil, InvalidChecksum
	}
	return self.bin, nil
}

func (self *MDB) Tx(bsend, brecv []byte) error {
	if len(bsend) == 0 {
		return nil
	}
	var err error
	var b []byte

	self.lk.Lock()
	defer self.lk.Unlock()

	if err = self.locked_send(bsend); err != nil {
		return err
	}
	if b, err = self.locked_recv(); err != nil {
		return err
	}
	// copy(brecv, b)
	brecv = append(brecv, b...)
	if self.Debug {
		log.Printf("debug: MDB.Tx  brecv='%x' len=%d", brecv, len(brecv))
	}
	if len(brecv) > 0 {
		err = self.locked_sendByte(0x00)
	}
	return err
}
