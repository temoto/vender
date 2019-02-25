package mdb

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/juju/errors"
	"github.com/temoto/vender/log2"
)

const ContextKey = "run/mdber"

type Uarter interface {
	Break(d time.Duration) error
	Close() error
	Open(path string) error
	Tx(request, response []byte) (int, error)
}

// Context[key] -> Mdber or panic
func ContextValueMdber(ctx context.Context, key interface{}) *mdb {
	v := ctx.Value(key)
	if v == nil {
		panic(fmt.Errorf("context['%v'] is nil", key))
	}
	if m, ok := v.(*mdb); ok {
		return m
	}
	panic(fmt.Errorf("context['%v'] expected type *mdb", key))
}

type mdb struct {
	Log     *log2.Log
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

func NewMDB(u Uarter, options string, log *log2.Log) (*mdb, error) {
	self := &mdb{
		Log:     log,
		io:      u,
		recvBuf: make([]byte, 0, PacketMaxLength),
	}
	err := self.io.Open(options)
	if err != nil {
		return nil, errors.Annotatef(err, "mdb.NewMDB Uarter=%s Open(%s)", u, options)
	}
	return self, nil
}

func (self *mdb) BreakCustom(keep, sleep time.Duration) error {
	self.Log.Debugf("mdb.BreakCustom keep=%v sleep=%v", keep, sleep)
	err := self.io.Break(keep)
	if err == nil {
		time.Sleep(sleep)
	}
	return errors.Trace(err)
}

func (self *mdb) Tx(request Packet, response *Packet) error {
	if response == nil {
		panic("code error mdb.Tx() response=nil")
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

	// TODO construct arguments only when logging is enabled
	self.Log.Debugf("mdb.Tx (multi-line)\n  ...send: (%02d) %s\n  ...recv: (%02d) %s\n  ...err=%v",
		request.l, request.Format(), response.l, response.Format(), err)
	if err != nil {
		return errors.Annotatef(err, "Tx send=%s recv=%s", request.Format(), response.Format())
	}
	return nil
}
