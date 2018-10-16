package mdb

import (
	"context"
	"fmt"
	"log"
	"runtime/debug"
	"sync"
	"time"

	"github.com/juju/errors"
	"github.com/temoto/vender/helpers"
)

type Uarter interface {
	Break(d time.Duration) error
	Close() error
	Open(path string, baud int) error
	Tx(request, response []byte) (int, error)
}

type Mdber interface {
	BreakCustom(keep, sleep int) error
	Tx(request, response *Packet) error
	TxRetry(request, response *Packet) error
	SetLog(logf helpers.LogFunc) helpers.LogFunc
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
	log     helpers.LogFunc
	recvBuf []byte
	io      Uarter
	lk      sync.Mutex
}

type InvalidChecksum struct {
	Received byte
	Actual   byte
}

func (self InvalidChecksum) Error() string {
	return fmt.Sprintf("Invalid checksum received=%x actual=%x", self.Received, self.Actual)
}

type FeatureNotSupported string

func (self FeatureNotSupported) Error() string { return string(self) }

func checksum(bs []byte) byte {
	var chk byte
	for _, b := range bs {
		chk += b
	}
	return chk
}

func NewMDB(u Uarter, path string, baud int) (*mdb, error) {
	self := &mdb{
		io:      u,
		log:     helpers.Discardf,
		recvBuf: make([]byte, 0, PacketMaxLength),
	}
	if baud == 0 {
		baud = 9600
	}
	err := self.io.Open(path, baud)
	return self, errors.Trace(err)
}

func (self *mdb) SetLog(logf helpers.LogFunc) (previous helpers.LogFunc) {
	previous, self.log = logf, self.log
	return previous
}

func (self *mdb) BreakCustom(keep, sleep int) error {
	self.log("debug: mdb.BreakCustom keep=%d sleep=%d", keep, sleep)
	err := self.io.Break(time.Duration(keep) * time.Millisecond)
	if err == nil {
		time.Sleep(time.Duration(sleep) * time.Millisecond)
	}
	return errors.Trace(err)
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
	var n int
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

	rbs := request.Bytes()
	// rbs = append(rbs, checksum(rbs))
	n, err = self.io.Tx(rbs, response.b[:])
	response.l = n

	acks := ""
	if response.l > 0 {
		acks = "\n> (01) 00 (ACK)"
	}
	self.log("debug: mdb.Tx (multi-line)\n> (%02d) %s\n< (%02d) %s%s\nerr=%v",
		request.l, request.Format(), response.l, response.Format(), acks, err)
	return errors.Trace(err)
}

func (self *mdb) TxRetry(request, response *Packet) error {
	const retries = 5
	delay := 100 * time.Millisecond
	var err error
	for i := 1; i <= retries; i++ {
		err = self.Tx(request, response)
		if errors.IsTimeout(err) {
			log.Printf("mdb request=%s err=%v timeout, retry in %v", request.Format(), err, delay)
			delay *= 2
			continue
		}
		break
	}
	return errors.Trace(err)
}
