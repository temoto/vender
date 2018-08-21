package bill

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

//go:generate stringer -type=PollItemStatus
type PollItemStatus byte

const (
	StatusInfo PollItemStatus = iota
	StatusError
	StatusFatal
	StatusBusy
	StatusDisabled
	StatusWasReset
	StatusRejected
	StatusCredit
	StatusEscrow
)

type PollItem struct {
	Status   PollItemStatus
	Error    error
	Nominal  currency.Nominal
	Attempts uint8
}

func (self *PollItem) String() string {
	return fmt.Sprintf("%s n=%s atmps=%d err=%s",
		self.Status.String(), currency.Amount(self.Nominal).Format100I(), self.Attempts, self.Error)
}
