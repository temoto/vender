package mdb

import (
	"time"

	"github.com/juju/errors"
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

type FeatureNotSupported string

func (self FeatureNotSupported) Error() string { return string(self) }

type Bus struct {
	Error func(error)
	Log   *log2.Log
	u     Uarter
}

func NewBus(u Uarter, log *log2.Log, errfun func(error)) *Bus {
	return &Bus{
		Error: errfun,
		Log:   log,
		u:     u,
	}
}

func (b *Bus) ResetDefault() error {
	return errors.Trace(b.Reset(DefaultBusResetKeep, DefaultBusResetSleep))
}

func (b *Bus) Reset(keep, sleep time.Duration) error {
	b.Log.Debugf("mdb.bus.Reset keep=%v sleep=%v", keep, sleep)
	return errors.Trace(b.u.Break(keep, sleep))
}

func (b *Bus) Tx(request Packet, response *Packet) error {
	if response == nil {
		panic("code error mdb.Tx() response=nil")
	}
	if response.readonly {
		return ErrPacketReadonly
	}
	if request.l == 0 {
		return nil
	}

	rbs := request.Bytes()
	n, err := b.u.Tx(rbs, response.b[:])
	response.l = n

	if err != nil {
		return errors.Annotatef(err, "mdb.Tx send=%x recv=%x", request.Bytes(), response.Bytes())
	}
	// explicit level check to save costly .Format()
	if b.Log.Enabled(log2.LDebug) {
		b.Log.Debugf("mdb.Tx (%02d) %s -> (%02d) %s",
			request.Len(), request.Format(), response.Len(), response.Format())
	}
	return nil
}
