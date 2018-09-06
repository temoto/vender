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
}

func (self *PollResult) Ready() bool {
	return self.Error == nil && len(self.Items) == 0
}

//go:generate stringer -type=PollItemStatus
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
	return fmt.Sprintf("%s n=%s atmps=%d err=%v",
		self.Status.String(), currency.Amount(self.Nominal).Format100I(), self.Attempts, self.Error)
}
