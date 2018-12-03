package mdb

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/juju/errors"
	"github.com/temoto/vender/helpers"
)

type Uarter interface {
	Break(d time.Duration) error
	Close() error
	Open(path string) error
	Tx(request, response []byte) (int, error)
}

type Mdber interface {
	BreakCustom(keep, sleep time.Duration) error
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

func NewMDB(u Uarter, path string) (*mdb, error) {
	self := &mdb{
		io:      u,
		log:     helpers.Discardf,
		recvBuf: make([]byte, 0, PacketMaxLength),
	}
	err := self.io.Open(path)
	return self, errors.Annotate(err, "NewMDB")
}

func (self *mdb) SetLog(logf helpers.LogFunc) (previous helpers.LogFunc) {
	previous, self.log = logf, self.log
	return previous
}

func (self *mdb) BreakCustom(keep, sleep time.Duration) error {
	self.log("debug: mdb.BreakCustom keep=%v sleep=%v", keep, sleep)
	err := self.io.Break(keep)
	if err == nil {
		time.Sleep(sleep)
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

	rbs := request.Bytes()
	self.lk.Lock()
	n, err := self.io.Tx(rbs, response.b[:])
	self.lk.Unlock()
	response.l = n

	self.log("debug: mdb.Tx (multi-line)\n  ...send: (%02d) %s\n  ...recv: (%02d) %s\n  ...err=%v",
		request.l, request.Format(), response.l, response.Format(), err)
	return errors.Annotatef(err, "Tx send=%s recv=%s", request.Format(), response.Format())
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
