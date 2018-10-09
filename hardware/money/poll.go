package money

import (
	"fmt"
	"testing"
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
	if self.Error != nil {
		return false
	}
	return !self.HasStatus(StatusBusy)
}

func (self *PollResult) HasStatus(s PollItemStatus) bool {
	if len(self.Items) == 0 {
		return false
	}
	for _, i := range self.Items {
		if i.Status == s {
			return true
		}
	}
	return false
}

func (self *PollResult) SingleStatus() PollItemStatus {
	if len(self.Items) != 1 {
		return statusZero
	}
	return self.Items[0].Status
}

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
	Status      PollItemStatus
	Error       error
	DataNominal currency.Nominal
	DataCount   uint8
	DataCashbox bool
}

func (self *PollItem) String() string {
	return fmt.Sprintf("status=%s cashbox=%v nominal=%s count=%d err=%v",
		self.Status.String(),
		self.DataCashbox,
		currency.Amount(self.DataNominal).Format100I(),
		self.DataCount,
		self.Error,
	)
}

func (self *PollItem) Amount() currency.Amount {
	c := self.DataCount
	if c == 0 {
		c = 1
	}
	return currency.Amount(self.DataNominal) * currency.Amount(c)
}

// TODO generate this code
func (a *PollResult) TestEqual(t *testing.T, b *PollResult) {
	if a.Delay != b.Delay {
		t.Errorf("PoolResult.Delay a=%v b=%v", a.Delay, b.Delay)
	}
	if a.Error != b.Error {
		t.Errorf("PoolResult.Error a=%v b=%v", a.Error, b.Error)
	}
	if !a.Time.IsZero() && !b.Time.IsZero() && !a.Time.Equal(b.Time) {
		t.Errorf("PoolResult.Time a=%v b=%v", a.Time, b.Time)
	}
	longest := len(a.Items)
	if len(b.Items) > longest {
		longest = len(b.Items)
	}
	if len(a.Items) != len(b.Items) {
		t.Errorf("PoolResult.Items len a=%d b=%d", len(a.Items), len(b.Items))
	}
	for i := 0; i < longest; i++ {
		var ia *PollItem
		var ib *PollItem
		ias, ibs := "-", "-"
		if i < len(a.Items) {
			ia = &a.Items[i]
			ias = fmt.Sprintf("%s", ia)
		}
		if i < len(b.Items) {
			ib = &b.Items[i]
			ibs = fmt.Sprintf("%s", ib)
		}
		switch {
		case ia == nil && ib == nil: // OK
		case ia != nil && ib != nil && *ia == *ib: // OK
		case ia != ib: // one side nil
			fallthrough
		case ia != nil && ib != nil && *ia != *ib: // both not nil, different values
			t.Errorf("PoolResult.Items[%d] a=%s b=%s", i, ias, ibs)
		default:
			t.Fatalf("Code error, invalid condition check: PoolResult.Items[%d] a=%s b=%s", i, ias, ibs)
		}
	}
}
