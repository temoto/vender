package mdb

import (
	"bufio"
	"errors"
	"fmt"
	"golang.org/x/sys/unix"
	"io"
	"log"
	"os"
	"sync"
	"syscall"
	"time"
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

type MDB struct {
	Debug bool

	bin         []byte
	f           *os.File
	lk          sync.Mutex
	r           *bufio.Reader
	w           io.Writer
	skip_ioctl  bool // for tests and benchmark
	t2          termios2
	last_parodd bool // save set9 syscall
}

type InvalidChecksum struct {
	Received byte
	Actual   byte
}

func (self InvalidChecksum) Error() string {
	return "Invalid checksum"
}

func (self *MDB) Open(path string, baud int, vmin byte) (err error) {
	if self.f != nil {
		self.f.Close()
	}
	// self.f, err = os.OpenFile(path, syscall.O_RDWR|syscall.O_NOCTTY|syscall.O_NONBLOCK, 0600)
	self.f, err = os.OpenFile(path, syscall.O_RDWR|syscall.O_NOCTTY, 0600)
	if err != nil {
		return err
	}
	return self.resetTermios(baud, vmin)
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
	self.bin = make([]byte, 0, PacketMaxLength)
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
	if _, err = self.w.Write(b[:1]); err != nil {
		return
	}
	if err = self.set9(false); err != nil {
		return
	}
	if _, err = self.w.Write(b[1:]); err != nil {
		return
	}
	// end critical path

	return nil
}

func (self *MDB) locked_sendAck() (err error) {
	nullBytes1 := [1]byte{0}
	// begin critical path
	if err = self.set9(false); err != nil {
		return
	}
	// TODO: check const / stack allocation
	if _, err = self.w.Write(nullBytes1[:]); err != nil {
		return
	}
	// end critical path
	return nil
}

func (self *MDB) locked_recvWait(min int, wait time.Duration) (err error) {
	var out int
	tbegin := time.Now()
	tfinal := tbegin.Add(wait)

	for {
		err = self.ioctl(uintptr(cFIONREAD), uintptr(unsafe.Pointer(&out)))
		if err != nil {
			return err
		}
		if out >= min {
			return nil
		}
		time.Sleep(wait / 20)
		if time.Now().After(tfinal) {
			// TODO: error timeout
			break
		}
	}

	return nil
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
		// self.locked_recvWait(1, time.Millisecond)
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
	// if self.Debug {
	// 	PacketFromBytes(self.bin).Logf("debug: MDB.recv %s")
	// }
	if chkin != chkout {
		if self.Debug {
			log.Printf("debug: MDB.recv InvalidChecksum frompacket=%x actual=%x", chkin, chkout)
		}
		return nil, InvalidChecksum{Received: chkin, Actual: chkout}
	}
	return self.bin, nil
}

func (self *MDB) Tx(request, response *Packet) error {
	if response.readonly {
		return ErrPacketReadonly
	}
	if request.Len() == 0 {
		return nil
	}
	var err error
	var b []byte

	self.lk.Lock()
	defer self.lk.Unlock()
	// TODO
	// self.f.SetDeadline(time.Now().Add(time.Second))
	// defer self.f.SetDeadline(time.Time{})

	if err = self.locked_send(request.Bytes()); err != nil {
		return err
	}
	if b, err = self.locked_recv(); err != nil {
		return err
	}
	response.write(b)
	if len(b) > 0 {
		err = self.locked_sendAck()
	}
	if self.Debug {
		log.Printf("debug: MDB.Tx (%02d) b='%x'", len(b), b)
	}
	return err
}
