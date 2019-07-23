package money

import (
	"fmt"

	"github.com/temoto/vender/currency"
)

//go:generate stringer -type=PollItemStatus -trimprefix=Status
type PollItemStatus byte

const (
	statusZero PollItemStatus = iota
	StatusInfo
	StatusError
	StatusFatal
	StatusDisabled
	StatusBusy
	StatusWasReset
	StatusCredit
	StatusRejected
	StatusEscrow
	StatusReturnRequest
	StatusDispensed
)

type PollItem struct {
	// TODO avoid time.Time for easy GC (contains pointer)
	// Time        time.Time
	Error        error
	DataNominal  currency.Nominal
	Status       PollItemStatus
	DataCount    uint8
	DataCashbox  bool
	HardwareCode byte
}

func (self *PollItem) String() string {
	return fmt.Sprintf("status=%s cashbox=%v nominal=%s count=%d hwcode=%02x err=%v",
		self.Status.String(),
		self.DataCashbox,
		currency.Amount(self.DataNominal).Format100I(),
		self.DataCount,
		self.HardwareCode,
		self.Error,
	)
}

func (self *PollItem) Amount() currency.Amount {
	if self.DataCount == 0 {
		panic("code error PollItem.DataCount=0")
	}
	return currency.Amount(self.DataNominal) * currency.Amount(self.DataCount)
}
