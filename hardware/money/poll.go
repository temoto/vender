package money

import (
	"fmt"
	"testing"

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
		panic("code error")
	}
	return currency.Amount(self.DataNominal) * currency.Amount(self.DataCount)
}

// TODO generate this code
func TestPollItemsEqual(t testing.TB, as, bs []PollItem) {
	longest := len(as)
	if len(bs) > longest {
		longest = len(bs)
	}
	if len(as) != len(bs) {
		t.Errorf("PollItems len a=%d b=%d", len(as), len(bs))
	}
	for i := 0; i < longest; i++ {
		var ia *PollItem
		var ib *PollItem
		if i < len(as) {
			ia = &as[i]
		}
		if i < len(bs) {
			ib = &bs[i]
		}
		ia.TestEqual(t, ib)
	}
}
func (a *PollItem) TestEqual(t testing.TB, b *PollItem) {
	switch {
	case a == nil && b == nil: // OK
	case a != nil && b != nil && *a == *b: // OK
	case a != nil && b != nil && a.Error != b.Error:
		t.Errorf("PollItem.Error a=%v b=%v", a.Error, b.Error)
	case a != b: // one side nil
		fallthrough
	case a != nil && b != nil && *a != *b: // both not nil, different values
		t.Errorf("PollItem a=%s b=%s", a, b)
	default:
		t.Fatalf("code error, invalid condition check: PoolItem a=%s b=%s", a, b)
	}
}
