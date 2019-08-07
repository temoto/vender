package mdb

import (
	"sync"
	"time"

	"github.com/juju/errors"
	"github.com/temoto/vender/hardware/mega-client"
)

const (
	DelayErr = 10 * time.Millisecond
)

type megaUart struct {
	c  *mega.Client
	lk sync.Mutex
}

func NewMegaUart(client *mega.Client) Uarter {
	return &megaUart{c: client}
}
func (self *megaUart) Open(_ string) error {
	self.c.IncRef("mdb-uart")
	return nil
	// _, err := self.c.DoTimeout(mega.COMMAND_STATUS, nil, 5*time.Second)
	// return err
}
func (self *megaUart) Close() error {
	return self.c.DecRef("mdb-uart")
}

func responseError(r mega.Mdb_result_t, arg byte) error {
	switch r {
	case mega.MDB_RESULT_SUCCESS:
		return nil
	case mega.MDB_RESULT_BUSY:
		// err := errors.NewErr("MDB busy state=%s", mega.Mdb_state_t(p.Fields.MdbError).String())
		return ErrBusy
	case mega.MDB_RESULT_TIMEOUT:
		return ErrTimeout
	case mega.MDB_RESULT_NAK:
		return ErrNak
	default:
		err := errors.NewErr("mega MDB error result=%s arg=%02x", r.String(), arg)
		err.SetLocation(2)
		return &err
	}
}

func (self *megaUart) Break(d, sleep time.Duration) error {
	self.lk.Lock()
	defer self.lk.Unlock()

	var f mega.Frame
	var err error
	for retry := 1; retry <= 3; retry++ {
		f, err = self.c.DoMdbBusReset(d)
		switch errors.Cause(err) {
		case nil: // success path
			err = responseError(f.Fields.MdbResult, f.Fields.MdbError)
			if err == nil {
				time.Sleep(sleep)
				return nil
			}
			time.Sleep(DelayErr)

		case mega.ErrCriticalProtocol:
			self.c.Log.Fatal(errors.ErrorStack(err))

		default:
			return err
		}
	}
	return err
}

func (self *megaUart) Tx(request, response []byte) (int, error) {
	self.lk.Lock()
	defer self.lk.Unlock()

	var f mega.Frame
	var err error
	for retry := 1; retry <= 3; retry++ {
		// FIXME should not be here, but fixes MDB busy/uart_unexpected race
		time.Sleep(1 * time.Millisecond)

		f, err = self.c.DoMdbTxSimple(request)
		self.c.Log.Debugf("mdb.mega.txsimple request=%x f=%s err=%v", request, f.ResponseString(), err)
		switch errors.Cause(err) {
		case nil: // success path
			err = responseError(f.Fields.MdbResult, f.Fields.MdbError)
			if err == nil {
				n := copy(response, f.Fields.MdbData)
				return n, nil
			}
			time.Sleep(DelayErr)

		case mega.ErrCriticalProtocol:
			self.c.Log.Fatal(errors.ErrorStack(err))

		default:
			return 0, err
		}
	}
	return 0, err
}
