package mdb

import (
	"context"
	"encoding/binary"

	"github.com/temoto/vender/helpers/msync"
)

type Device struct {
	Mdber     Mdber
	Address   uint8
	Name      string
	ByteOrder binary.ByteOrder
}

type DoResult struct {
	P Packet
	E error
}

// Doer wrap for mbder.Tx()
type DoRequest struct {
	mdber   Mdber
	request *Packet
	rch     chan DoResult
}

func (self *DoRequest) Do(ctx context.Context) error {
	r := DoResult{}
	r.E = self.mdber.Tx(self.request, &r.P)
	self.rch <- r
	return r.E
}
func (self *DoRequest) String() string {
	return "mdb=%s" + self.request.Format()
}

func (self *Device) NewTxRequest(request *Packet) (*DoRequest, <-chan DoResult) {
	d := &DoRequest{
		mdber:   self.Mdber,
		request: request,
		rch:     make(chan DoResult, 1),
	}
	return d, d.rch
}

func (self *Device) DebugDo(parent *msync.Node, request *Packet) DoResult {
	d, rch := self.NewTxRequest(request)
	parent.Append(d)
	return <-rch
}
