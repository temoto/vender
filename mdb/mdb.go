package mdb

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"sync"
	"syscall"
	"time"
	"unsafe"
)

type MDB struct {
	Debug bool

	recvBuf     []byte
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
	// begin critical path
	if err = self.set9(false); err != nil {
		return
	}
	if _, err = self.w.Write(PacketNul1.b[:1]); err != nil {
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

func (self *MDB) locked_recv(dst *Packet) error {
	var err error
	var b, chkin, chkout byte
	var part []byte
	self.recvBuf = self.recvBuf[:0]

	// begin critical path
	if err = self.set9(false); err != nil {
		return err
	}
recvLoop:
	for {
		// self.locked_recvWait(1, time.Millisecond)
		if part, err = self.r.ReadSlice(0xff); err != nil {
			return err
		}
		n := len(part)
		if n > 1 {
			self.recvBuf = append(self.recvBuf, part[:n-1]...)
		}
		if b, err = self.r.ReadByte(); err != nil {
			return err
		}
		switch b {
		case 0x00:
			if chkin, err = self.r.ReadByte(); err != nil {
				return err
			}
			break recvLoop
		case 0xff:
			self.recvBuf = append(self.recvBuf, b)
		default:
			err = fmt.Errorf("recv unknown sequence ff %x", b)
			return err
		}
		if len(self.recvBuf) > PacketMaxLength {
			err = errors.New("recv self.recvBuf overflow")
			return err
		}
	}
	// end critical path

	for _, b = range self.recvBuf {
		chkout += b
	}
	// if self.Debug {
	// 	PacketFromBytes(self.recvBuf).Logf("debug: MDB.recv %s")
	// }
	if chkin != chkout {
		if self.Debug {
			log.Printf("debug: MDB.recv InvalidChecksum frompacket=%x actual=%x", chkin, chkout)
		}
		return InvalidChecksum{Received: chkin, Actual: chkout}
	}
	dst.write(self.recvBuf)
	return nil
}

func (self *MDB) Tx(request, response *Packet) error {
	if response.readonly {
		return ErrPacketReadonly
	}
	if request.Len() == 0 {
		return nil
	}
	var err error

	self.lk.Lock()
	defer self.lk.Unlock()
	// TODO
	// self.f.SetDeadline(time.Now().Add(time.Second))
	// defer self.f.SetDeadline(time.Time{})

	if err = self.locked_send(request.Bytes()); err != nil {
		return err
	}
	// ack must arrive <5ms after recv
	// begin critical path
	if err = self.locked_recv(response); err != nil {
		return err
	}
	if response.l > 0 {
		err = self.locked_sendAck()
	}
	// end critical path

	if self.Debug {
		acks := ""
		if response.l > 0 {
			acks = "\n> (01) 00 (ACK)"
		}
		log.Printf("debug: MDB.Tx (multi-line)\n> (%02d) %s\n< (%02d) %s%s\nerr=%v",
			request.l, request.Format(), response.l, response.Format(), acks, err)
	}
	return err
}
