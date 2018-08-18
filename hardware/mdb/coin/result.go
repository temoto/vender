package coin

import (
	"fmt"
	"time"

	"github.com/temoto/vender/currency"
)

type PollResult struct {
	Time  time.Time
	Error error
	Delay time.Duration
	Items []PollItem
	Ready bool
}

// FIXME //go:generate stringer -type=PollItemStatus -type=PollItem
type PollItemStatus byte

const (
	StatusInfo PollItemStatus = iota
	StatusError
	StatusFatal
	StatusDispensed
	StatusDeposited
	StatusEscrowRequest
	StatusWasReset
	StatusRejected
)

type PollItem struct {
	Status   PollItemStatus
	Error    error
	Nominal  currency.Nominal
	Attempts uint8
}

func (self *PollItem) String() string {
	return fmt.Sprintf("<%#v n=%s atmps=%d err=%s",
		self.Status, currency.Amount(self.Nominal).Format100I(), self.Attempts, self.Error)
}
