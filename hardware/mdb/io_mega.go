package mdb

import (
	"time"

	"github.com/juju/errors"
	mega "github.com/temoto/vender/hardware/mega-client"
)

type megaUart struct {
	c *mega.Client
}

func NewMegaUart(client *mega.Client) Uarter {
	return &megaUart{client}
}
func (self *megaUart) Open(_ string) error {
	self.c.IncRef("mdb-uart")
	_, err := self.c.DoTimeout(mega.COMMAND_STATUS, nil, 5*time.Second)
	return err
}
func (self *megaUart) Close() error {
	return self.c.DecRef("mdb-uart")
}

func (self *megaUart) Break(d time.Duration) error {
	p, err := self.c.DoMdbBusReset(d)
	if err != nil {
		return err
	}
	if mega.Response_t(p.Header) != mega.RESPONSE_MDB_SUCCESS {
		return errors.NotValidf("MdbBusReset response=%s", p)
	}
	return nil
}

func (self *megaUart) Tx(request, response []byte) (int, error) {
	p, err := self.c.DoMdbTxSimple(request)
	if err != nil {
		return 0, err
	}
	n := copy(response, p.Data())
	return n, nil
}
