package telenet

// Complex values are read and modified atomically, but not consistently,
// i.e. it is possible to read .Count=1 .Size=0 because Size has not updated yet.

import (
	"expvar"
	"fmt"

	"github.com/golang/protobuf/proto"
	"github.com/temoto/vender/tele"
)

type SessionStat struct {
	Conn expvar.Int
	Recv Counters
	Send Counters
}

func (ss *SessionStat) Add(other *SessionStat) {
	ss.Conn.Add(other.Conn.Value())
	ss.Recv.Add(&other.Recv)
	ss.Send.Add(&other.Send)
}

func (ss *SessionStat) AddMoveFrom(other *SessionStat) {
	tmp := other.Value()
	ss.Add(&tmp)
	other.Sub(&tmp)
}

func (ss *SessionStat) Sub(other *SessionStat) {
	ss.Conn.Add(-other.Conn.Value())
	ss.Recv.Sub(&other.Recv)
	ss.Send.Sub(&other.Send)
}

func (ss *SessionStat) Value() (r SessionStat) {
	r.Conn.Set(ss.Conn.Value())
	r.Recv.Set(ss.Recv.Value())
	r.Send.Set(ss.Send.Value())
	return
}

func (ss *SessionStat) String() string {
	return fmt.Sprintf(`{"conn":%d,"recv":%s,"send":%s}`,
		ss.Conn.Value(), ss.Recv.String(), ss.Send.String())
}

type Counters struct {
	Cmd   CountSizePair
	Tele  CountSizePair
	Total CountSizePair
}

func (c *Counters) Add(c2 *Counters) {
	c.Cmd.Add(&c2.Cmd)
	c.Tele.Add(&c2.Tele)
	c.Total.Add(&c2.Total)
}

func (c *Counters) Register(p *tele.Packet) {
	size := int64(proto.Size(p))
	c.Total.Count.Add(1)
	c.Total.Size.Add(size)
	var category *CountSizePair
	switch {
	case p.Command != nil || p.Response != nil:
		category = &c.Cmd
	case p.Telemetry != nil || p.State != tele.State_Invalid:
		category = &c.Tele
	}
	if category != nil {
		category.Count.Add(1)
		category.Size.Add(size)
	}
}

func (c *Counters) Set(new Counters) {
	c.Cmd.Set(new.Cmd.Value())
	c.Tele.Set(new.Tele.Value())
	c.Total.Set(new.Total.Value())
}

func (c *Counters) Sub(other *Counters) {
	c.Cmd.Sub(&other.Cmd)
	c.Tele.Sub(&other.Tele)
	c.Total.Sub(&other.Total)
}

func (c *Counters) Value() (r Counters) {
	r.Cmd = c.Cmd.Value()
	r.Tele = c.Tele.Value()
	r.Total = c.Total.Value()
	return
}

func (c *Counters) String() string {
	return fmt.Sprintf(`{"cmd.count":%d,"cmd.size":%d,"tele.count":%d,"tele.size":%d,"total.count":%d,"total.size":%d}`,
		c.Cmd.Count.Value(), c.Cmd.Size.Value(),
		c.Tele.Count.Value(), c.Tele.Size.Value(),
		c.Total.Count.Value(), c.Total.Size.Value())
}

type CountSizePair struct {
	Count expvar.Int
	Size  expvar.Int
}

func (csp *CountSizePair) Add(other *CountSizePair) {
	csp.Count.Add(other.Count.Value())
	csp.Size.Add(other.Size.Value())
}

func (csp *CountSizePair) Value() (r CountSizePair) {
	r.Count.Set(csp.Count.Value())
	r.Size.Set(csp.Size.Value())
	return
}

func (csp *CountSizePair) Set(new CountSizePair) {
	csp.Count.Set(new.Count.Value())
	csp.Size.Set(new.Size.Value())
}

func (csp *CountSizePair) Sub(other *CountSizePair) {
	csp.Count.Add(-other.Count.Value())
	csp.Size.Add(-other.Size.Value())
}
