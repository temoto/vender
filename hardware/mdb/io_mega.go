package mdb

import (
	"time"

	"github.com/juju/errors"
	mega "github.com/temoto/vender/hardware/mega-client"
)

var (
	ErrTimeout = errors.NewErr("MDB timeout")
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
	case mega.RESPONSE_MDB_SUCCESS:
		return nil
	case mega.RESPONSE_MDB_BUSY:
		err := errors.NewErr("MDB busy TODO=autoretry")
		err.SetLocation(2)
		return &err
	case mega.RESPONSE_MDB_TIMEOUT:
		err := ErrTimeout
		err.SetLocation(2)
		return &err
	default:
		err := errors.NewErr("mega response=%s", p.String())
		err.SetLocation(2)
		return &err
	}
}

func (self *megaUart) Break(d time.Duration) error {
	p, err := self.c.DoMdbBusReset(d)
	if err != nil {
		return err
	}
	return responseError(&p)
}

func (self *megaUart) Tx(request, response []byte) (int, error) {
	p, err := self.c.DoMdbTxSimple(request)
	if err != nil {
		return 0, err
	}
	err = responseError(&p)
	n := 0
	if err == nil {
		n = copy(response, p.Data())
	}
	return n, err
}
