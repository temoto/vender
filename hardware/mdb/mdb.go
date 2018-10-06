package mdb

import (
	"context"
	"errors"
	"fmt"
	"log"
	"runtime/debug"
	"sync"
	"time"
)

type Mdber interface {
	BreakCustom(keep, sleep int) error
	Tx(request, response *Packet) error
	TxDebug(request *Packet, debug bool) (response *Packet)
	SetDebug(bool) bool
}

// Context[key] -> Mdber or panic
func ContextValueMdber(ctx context.Context, key interface{}) Mdber {
	v := ctx.Value(key)
	if v == nil {
		panic(fmt.Errorf("context['%v'] is nil", key))
	}
	if cfg, ok := v.(Mdber); ok {
		return cfg
	}
	panic(fmt.Errorf("context['%v'] expected type Mdber", key))
}

type mdb struct {
	debug   bool
	recvBuf []byte
	io      Uarter
	lk      sync.Mutex
}

type InvalidChecksum struct {
	Received byte
	Actual   byte
}

func (self InvalidChecksum) Error() string {
	return "Invalid checksum"
}

func NewMDB(u Uarter, path string, baud int) (*mdb, error) {
	self := &mdb{
		io:      u,
		recvBuf: make([]byte, 0, PacketMaxLength),
	}
	err := self.io.Open(path, baud)
	return self, err
}

func (self *mdb) SetDebug(d bool) (previous bool) {
	previous, self.debug = d, self.debug
	return previous
}

func (self *mdb) BreakCustom(keep, sleep int) (err error) {
	if self.debug {
		log.Printf("debug: mdb.BreakCustom keep=%d sleep=%d", keep, sleep)
	}
	err = self.io.Break(time.Duration(keep) * time.Millisecond)
	if err == nil {
		time.Sleep(time.Duration(sleep) * time.Millisecond)
	}
	return err
}

func (self *mdb) locked_send(b []byte) (err error) {
	if len(b) == 0 {
		return nil
	}

	var chk byte
	for _, x := range b {
		chk += x
	}
	b = append(b, chk)
	if self.debug {
		log.Printf("debug: mdb.send  out='%x'", b)
	}

	return io_write(self.io, b, true)
}

func (self *mdb) locked_sendAck() (err error) {
	return io_write(self.io, PacketNul1.b[:1], false)
}

func (self *mdb) locked_recv(dst *Packet) error {
	var err error
	var b, chkin, chkout byte
	var part []byte
	self.recvBuf = self.recvBuf[:0]

	// begin critical path
	if err = self.io.ResetRead(); err != nil {
		return err
	}
recvLoop:
	for {
		if part, err = self.io.ReadSlice(0xff); err != nil {
			return err
		}
		n := len(part)
		if n > 1 {
			self.recvBuf = append(self.recvBuf, part[:n-1]...)
		}
		if b, err = self.io.ReadByte(); err != nil {
			return err
		}
		switch b {
		case 0x00:
			if chkin, err = self.io.ReadByte(); err != nil {
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
	// 	PacketFromBytes(self.recvBuf).Logf("debug: mdb.recv %s")
	// }
	if chkin != chkout {
		if self.debug {
			log.Printf("debug: mdb.recv InvalidChecksum frompacket=%x actual=%x", chkin, chkout)
		}
		return InvalidChecksum{Received: chkin, Actual: chkout}
	}
	dst.write(self.recvBuf)
	return nil
}

func (self *mdb) Tx(request, response *Packet) error {
	if request == nil || response == nil {
		panic("mdb.Tx() both request and response must be not nil")
	}
	if response.readonly {
		return ErrPacketReadonly
	}
	if request.Len() == 0 {
		return nil
	}
	var err error

	self.lk.Lock()
	defer self.lk.Unlock()
	saveGCPercent := debug.SetGCPercent(-1)
	defer debug.SetGCPercent(saveGCPercent)
	// FIXME crutch to avoid slow set9 with drain
	time.Sleep(10 * time.Millisecond)
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

	if self.debug {
		acks := ""
		if response.l > 0 {
			acks = "\n> (01) 00 (ACK)"
		}
		log.Printf("debug: mdb.Tx (multi-line)\n> (%02d) %s\n< (%02d) %s%s\nerr=%v",
			request.l, request.Format(), response.l, response.Format(), acks, err)
	}
	return err
}

func (self *mdb) TxDebug(request *Packet, debug bool) *Packet {
	response := new(Packet)
	saveDebug := self.debug
	self.SetDebug(debug)
	err := self.Tx(request, response)
	self.SetDebug(saveDebug)
	if err != nil {
		log.Printf("mdb request=%s err=%v", request.Format(), err)
		return nil
	}
	return response
}
