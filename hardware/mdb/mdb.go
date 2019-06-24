package mdb

import (
	"fmt"
	"time"

	"github.com/temoto/errors"
	"github.com/temoto/vender/log2"
)

const (
	DefaultBusResetKeep  = 200 * time.Millisecond
	DefaultBusResetSleep = 500 * time.Millisecond
)

var (
	ErrNak     = errors.Errorf("MDB NAK")
	ErrBusy    = errors.Errorf("MDB busy")
	ErrTimeout = errors.Errorf("MDB timeout")
)

type Uarter interface {
	Break(d, sleep time.Duration) error
	Close() error
	Open(options string) error
	Tx(request, response []byte) (int, error)
}

type Mdb struct {
	Log *log2.Log
	io  Uarter
}

type TxFunc func(request Packet, response *Packet) error

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

func (self *Mdb) BusResetDefault() error {
	return errors.Trace(self.BusReset(DefaultBusResetKeep, DefaultBusResetSleep))
}
func (self *Mdb) BusReset(keep, sleep time.Duration) error {
	self.Log.Debugf("mdb.BusReset keep=%v sleep=%v", keep, sleep)
	return errors.Trace(self.io.Break(keep, sleep))
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
	n, err := self.io.Tx(rbs, response.b[:])
	response.l = n

	if err != nil {
		return errors.Annotatef(err, "mdb.Tx send=%s recv=%s err=%v", request.Format(), response.Format(), err)
	}
	if self.Log.Enabled(log2.LDebug) {
		self.Log.Debugf("mdb.Tx (%02d) %s -> (%02d) %s",
			request.l, request.Format(), response.l, response.Format())
	}
	return nil
}
