package tele

import (
	"sync"
)

// Stat struct TODO try github.com/rcrowley/go-metrics
// Low priority telemetry buffer. Can be updated at any time.
// Sent together with more important data or on `Command_Report`
type Stat struct { //nolint:maligned
	sync.Mutex
	Telemetry_Stat
}

// Locked_Reset Internal for tele package. Caller must hold self.Mutex.
func (self *Stat) Locked_Reset() {
	self.Telemetry_Stat.Reset()
	self.BillRejected = make(map[uint32]uint32, 16)
	self.CoinRejected = make(map[uint32]uint32, 16)
}
