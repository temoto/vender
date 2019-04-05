package mdb

import (
	"sync"
	"time"

	"github.com/juju/errors"
	iodin "github.com/temoto/iodin/client/go-iodin"
)

type iodinUart struct {
	c  *iodin.Client
	lk sync.Mutex
}

func NewIodinUart(c *iodin.Client) *iodinUart {
	c.IncRef("mdb")
	return &iodinUart{c: c}
}

func (self *iodinUart) Close() error {
	self.c.DecRef("mdb")
	self.c = nil
	return nil
}

func (self *iodinUart) Break(d, sleep time.Duration) error {
	self.lk.Lock()
	defer self.lk.Unlock()

	ms := int(d / time.Millisecond)
	var r iodin.Response
	err := self.c.Do(&iodin.Request{Command: iodin.Request_MDB_RESET, ArgUint: uint32(ms)}, &r)
	if err != nil {
		return errors.Trace(err)
	}
	time.Sleep(sleep)
	return nil
}

func (self *iodinUart) Open(path string) error {
	var r iodin.Response
	err := self.c.Do(&iodin.Request{Command: iodin.Request_MDB_OPEN, ArgBytes: []byte(path)}, &r)
	return errors.Trace(err)
}

func (self *iodinUart) Tx(request, response []byte) (n int, err error) {
	self.lk.Lock()
	defer self.lk.Unlock()

	var r iodin.Response
	err = self.c.Do(&iodin.Request{Command: iodin.Request_MDB_TX, ArgBytes: request}, &r)
	n = copy(response, r.DataBytes)
	return n, errors.Trace(err)
}
