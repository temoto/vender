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

func (self *Device) Tx(request Packet) (r DoResult) {
	r.E = self.Mdber.Tx(request, &r.P)
	return
}

func (self *Device) NewDoTx(request Packet) (*DoRequest, <-chan DoResult) {
	d := &DoRequest{
		dev:     self,
		request: request,
		rch:     make(chan DoResult, 1),
	}
	return d, d.rch
}
func (self *Device) NewDoTxNR(request Packet) *DoRequest {
	d := &DoRequest{dev: self, request: request}
	return d
}

func (self *Device) DebugDo(parent *msync.Node, request Packet) DoResult {
	d, rch := self.NewDoTx(request)
	parent.Append(d)
	return <-rch
}

type DoResult struct {
	P Packet
	E error
}

// Doer wrap for mbder.Tx()
type DoRequest struct {
	dev     *Device
	request Packet
	rch     chan DoResult
}

func (self *DoRequest) Do(ctx context.Context) error {
	r := self.dev.Tx(self.request)
	if self.rch != nil {
		self.rch <- r
	}
	return r.E
}
func (self *DoRequest) String() string {
	return "mdb=%s" + self.request.Format()
}
