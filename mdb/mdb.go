package main

import (
	"bufio"
	"encoding/hex"
	"errors"
	"fmt"
	"golang.org/x/sys/unix"
	"log"
	"os"
	"sync"
	"syscall"
	"unsafe"
)

const (
	cTCSETS2 = 0x402C542B
	cBOTHER  = 0x1000
	cNCCS    = 19
	cCMSPAR  = 0x40000000
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
	lk  sync.Mutex
	f   *os.File
	r   *bufio.Reader
	t2  termios2
	bin []byte
}

func NewMDB(path string, baud int, vmin byte) (*MDB, error) {
	if baud != 9600 {
		return nil, errors.New("Not implemented support for baud rate other than 9600")
	}
	f, err := os.OpenFile("/dev/ttyAMA0", syscall.O_RDWR|syscall.O_NOCTTY, 0600)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err != nil {
			f.Close()
		}
	}()
	self := &MDB{
		f: f,
		t2: termios2{
			c_iflag: unix.IGNBRK | unix.INPCK | unix.PARMRK,
			//c_cflag:  syscall.CLOCAL | syscall.CREAD | syscall.PARENB | cCMSPAR,
			c_cflag:  0x400009bd | unix.PARENB,
			c_ispeed: speed_t(unix.B9600),
			c_ospeed: speed_t(unix.B9600),
		},
		bin: make([]byte, 37),
		r:   bufio.NewReader(f),
	}
	self.t2.c_cc[syscall.VMIN] = cc_t(vmin)
	if err = self.tcsets2(); err != nil {
		return nil, err
	}
	return self, nil
}

func (self *MDB) Break() error {
	r, _, errno := syscall.Syscall(
		syscall.SYS_IOCTL,
		uintptr(self.f.Fd()),
		uintptr(unix.TCSBRKP),
		uintptr(3))
	if errno != 0 {
		return os.NewSyscallError("SYS_IOCTL", errno)
	}
	if r != 0 {
		return errors.New("unknown error from SYS_IOCTL")
	}
	return nil
}

func (self *MDB) tcsets2() error {
	r, _, errno := syscall.Syscall(
		syscall.SYS_IOCTL,
		uintptr(self.f.Fd()),
		uintptr(cTCSETS2),
		uintptr(unsafe.Pointer(&self.t2)))
	if errno != 0 {
		return os.NewSyscallError("SYS_IOCTL", errno)
	}
	if r != 0 {
		return errors.New("unknown error from SYS_IOCTL")
	}
	return nil
}

func (self *MDB) set9(b bool) error {
	if b {
		self.t2.c_cflag |= syscall.PARODD
	} else {
		self.t2.c_cflag &= ^tcflag_t(syscall.PARODD)
	}
	return self.tcsets2()
}

func (self *MDB) Send(b []byte) error {
	if len(b) == 0 {
		return nil
	}
	self.lk.Lock()
	defer self.lk.Unlock()
	var chk byte
	for _, x := range b {
		chk += x
	}
	b = append(b, chk)
	log.Printf("send: %x", b)

	var err error
	// begin critical path
	if err = self.set9(true); err != nil {
		return err
	}
	if _, err = self.f.Write(b[:1]); err != nil {
		return err
	}
	if err = self.set9(false); err != nil {
		return err
	}
	if _, err = self.f.Write(b[1:]); err != nil {
		return err
	}
	// end critical path

	return nil
}

func (self *MDB) Recv(out []byte) (err error) {
	defer func() {
		if err != nil {
			log.Printf("recv error: %s", err)
		}
	}()
	if cap(out) < 37 {
		err = errors.New("Recv out buffer must have capacity >= 37")
		return
	}
	var b, chkin, chkout byte
	var part []byte
	nmax := cap(out)
	out = out[:0]

	self.lk.Lock()
	defer self.lk.Unlock()

	// begin critical path
	if err = self.set9(false); err != nil {
		return
	}
recvLoop:
	for {
		if part, err = self.r.ReadSlice(0xff); err != nil {
			return
		}
        log.Printf("recv part: %x",part)
		n := len(part)
		if n > 1 {
			out = append(out, part[:n-1]...)
		}
		if b, err = self.r.ReadByte(); err != nil {
			return
		}
		switch b {
		case 0x00:
			if chkin, err = self.r.ReadByte(); err != nil {
				return
			}
			break recvLoop
		case 0xff:
			return fmt.Errorf("recv TODO does ff ff encodes 'ff' from uart?")
			// out=append(out,b)
		default:
			err = fmt.Errorf("recv unknown sequence ff %x", b)
			return
		}
		if len(out) > nmax {
			err = errors.New("recv out overflow")
			return
		}
	}
	// end critical path

	for _, b = range out {
		chkout += b
	}
	log.Printf("recv: %x chkin=%x chkout=%x", out, chkin, chkout)
	return nil
}

func main() {
	mdb, err := NewMDB("/dev/ttyAMA0", 9600, 1)
	if err != nil {
		panic(err)
	}

	if len(os.Args) == 1 {
		log.Fatalf("usage: %s hex", os.Args[0])
	}
	bout, err := hex.DecodeString(os.Args[1])
	if err != nil {
		log.Fatal(err)
	}
	mdb.Send(bout)
	bin := make([]byte, 0, 37)
	mdb.Recv(bin)
	mdb.f.Close()
}
