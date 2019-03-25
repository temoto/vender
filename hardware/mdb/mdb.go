package mdb

import (
	"fmt"
	"sync"
	"time"

	"github.com/juju/errors"
	"github.com/temoto/vender/log2"
)

var (
	ErrNak     = errors.Errorf("MDB NAK")
	ErrBusy    = errors.Errorf("MDB busy")
	ErrTimeout = errors.Errorf("MDB timeout")
)

type Uarter interface {
	Break(d time.Duration) error
	Close() error
	Open(options string) error
	Tx(request, response []byte) (int, error)
}

type Mdb struct {
	Log *log2.Log
	io  Uarter
	lk  sync.Mutex
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

func NewMDB(u Uarter, options string, log *log2.Log) (*Mdb, error) {
	self := &Mdb{
		Log: log,
		io:  u,
	}
	err := self.io.Open(options)
	if err != nil {
		return nil, errors.Annotatef(err, "Mdb.NewMDB Uarter=%s Open(%s)", u, options)
	}
	return self, nil
}

func (self *Mdb) BreakCustom(keep, sleep time.Duration) error {
	self.Log.Debugf("mdb.BreakCustom keep=%v sleep=%v", keep, sleep)
	self.lk.Lock()
	err := self.io.Break(keep)
	if err == nil {
		time.Sleep(sleep)
	}
	self.lk.Unlock()
	return errors.Trace(err)
}

func (self *Mdb) Tx(request Packet, response *Packet) error {
	if self == nil {
		panic(fmt.Sprintf("code error mdb=nil request=%x", request.Bytes()))
	}
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

	if err != nil {
		return errors.Annotatef(err, "Tx send=%s recv=%s", request.Format(), response.Format())
	}
	if self.Log.Enabled(log2.LDebug) {
		self.Log.Debugf("mdb.Tx (%02d) %s -> (%02d) %s",
			request.l, request.Format(), response.l, response.Format())
	}
	return nil
}
