package mdb

import (
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
	cTCSETSW2 = 0x402c542c
	cTCSETSF2 = 0x402c542d
)

type ErrTimeoutT string

type Timeouter interface {
	Timeout() bool
}

func (e ErrTimeoutT) Error() string { return string(e) }
func (ErrTimeoutT) Timeout() bool   { return true }

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

type fdReader struct {
	fd      uintptr
	timeout time.Duration
}

func (self fdReader) Read(p []byte) (n int, err error) {
	err = io_wait_read(self.fd, 1, self.timeout)
	if err != nil {
		return 0, err
	}
	// TODO bench optimist read, then io_wait if needed
	return syscall.Read(int(self.fd), p)
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

func io_wait_read(fd uintptr, min int, wait time.Duration) error {
	var err error
	var out int
	tbegin := time.Now()
	tfinal := tbegin.Add(wait)
	for {
		err = ioctl(fd, uintptr(cFIONREAD), uintptr(unsafe.Pointer(&out)))
		if err != nil {
			return err
		}
		if out >= min {
			return nil
		}
		time.Sleep(wait / 16)
		if time.Now().After(tfinal) {
			return ErrTimeoutT("io_wait_read timeout")
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
	// must use ioctl with drain - cTCSETSW2?
	// but it makes 9bit switch very slow
	err := io_tcsets2(fd, t2)
	return err
}

func io_tcsets2(fd uintptr, t2 *termios2) error {
	return ioctl(fd, uintptr(cTCSETS2), uintptr(unsafe.Pointer(t2)))
}

// flush output
func io_tcsetsw2(fd uintptr, t2 *termios2) error {
	return ioctl(fd, uintptr(cTCSETSW2), uintptr(unsafe.Pointer(t2)))
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
