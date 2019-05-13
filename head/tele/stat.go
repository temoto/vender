package tele

import (
	"sync"
)

// TODO try github.com/rcrowley/go-metrics
// Low priority telemetry buffer. Can be updated at any time.
// Sent together with more important data or on `Command_Report`
type Stat struct { //nolint:maligned
	sync.Mutex
	Telemetry_Stat
}

// func (self *Stat) Flush() ([]byte, error) {
// 	self.Lock()
// 	defer self.Unlock()
//
// 	self.Reset()
// 	self.BillReject
// 	b, err := proto.Marshal(&self)
//
// 	return b, err
// }
