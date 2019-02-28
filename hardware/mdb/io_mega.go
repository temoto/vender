package mdb

import (
	"time"

	"github.com/juju/errors"
	mega "github.com/temoto/vender/hardware/mega-client"
)

const (
	DelayErr = 10 * time.Millisecond
)

type megaUart struct {
	c *mega.Client
}

func NewMegaUart(client *mega.Client) Uarter {
	return &megaUart{c: client}
}
func (self *megaUart) Open(_ string) error {
	self.c.IncRef("mdb-uart")
	_, err := self.c.DoTimeout(mega.COMMAND_STATUS, nil, 5*time.Second)
	return err
}
func (self *megaUart) Close() error {
	return self.c.DecRef("mdb-uart")
}

func responseError(p *mega.Packet) error {
	switch mega.Response_t(p.Header) {
	case mega.RESPONSE_OK, mega.RESPONSE_ERROR:
		switch p.Fields.MdbResult {
		case mega.MDB_RESULT_SUCCESS:
			return nil
		case mega.MDB_RESULT_BUSY:
			// err := errors.NewErr("MDB busy state=%s", mega.Mdb_state_t(p.Fields.MdbError).String())
			err := ErrBusy
			err.SetLocation(2)
			return &err
		case mega.MDB_RESULT_TIMEOUT:
			err := ErrTimeout
			err.SetLocation(2)
			return &err
		case mega.MDB_RESULT_NAK:
			err := ErrNak
			err.SetLocation(2)
			return &err
		default:
			err := errors.NewErr("MDB error result=%s arg=%02x", p.Fields.MdbResult.String(), p.Fields.MdbError)
			err.SetLocation(2)
			return &err
		}
	}
	err := errors.NewErr("mega response=%s", p.String())
	err.SetLocation(2)
	return &err
}

func (self *megaUart) Break(d time.Duration) error {
	var p mega.Packet
	var err error
	for retry := 1; retry <= 3; retry++ {
		p, err = self.c.DoMdbBusReset(d)
		if err != nil {
			break
		}
		err = responseError(&p)
		if err == nil {
			break
		}
		time.Sleep(DelayErr)
	}
	return err
}

func (self *megaUart) Tx(request, response []byte) (int, error) {
	var p mega.Packet
	var err error
	n := 0
	for retry := 1; retry <= 3; retry++ {
		p, err = self.c.DoMdbTxSimple(request)
		if err != nil {
			break
		}
		err = responseError(&p)
		if err == nil {
			n = copy(response, p.Fields.MdbData)
			break
		}
		time.Sleep(DelayErr)
	}
	return n, err
}
