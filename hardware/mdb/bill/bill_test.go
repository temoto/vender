package bill

import (
	"bytes"
	"context"
	"fmt"
	"testing"

	"github.com/temoto/vender/currency"
	"github.com/temoto/vender/hardware/mdb"
)

// TODO generate this code
func (a *PollResult) testEqual(t *testing.T, b *PollResult) {
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

func checkPoll(t *testing.T, input string, expected PollResult) {
	inp := mdb.PacketFromHex(input)
	m, mockRead, w := mdb.NewTestMDB(t)
	bv := &BillValidator{mdb: m}
	bv.Init(context.Background(), m)
	w.Reset()
	mockRead(inp.Wire(true))
	bv.billTypeCredit[0] = currency.Nominal(5)
	bv.billTypeCredit[1] = currency.Nominal(10)
	bv.billTypeCredit[2] = currency.Nominal(20)
	actual := bv.CommandPoll()
	writeExpect := mdb.PacketFromHex("33").Wire(false)
	if len(input) > 0 {
		writeExpect = append(writeExpect, 0)
	}
	if !bytes.Equal(w.Bytes(), writeExpect) {
		t.Fatalf("CommandPoll() must send packet, found=%x expected=%x", w.Bytes(), writeExpect)
	}
	actual.testEqual(t, &expected)
}

func TestBillPoll(t *testing.T) {
	t.Parallel()
	type Case struct {
		name   string
		input  string
		expect PollResult
	}
	cases := []Case{
		Case{"empty", "", PollResult{Delay: delayNext}},
		Case{"disabled", "09", PollResult{
			Delay: delayNext,
			Items: []PollItem{PollItem{Status: StatusDisabled}},
		}},
		Case{"reset,disabled", "0609", PollResult{
			Delay: delayNext,
			Items: []PollItem{
				PollItem{Status: StatusWasReset},
				PollItem{Status: StatusDisabled},
			},
		}},
		Case{"escrow", "9209", PollResult{
			Delay: delayNext,
			Items: []PollItem{
				PollItem{Status: StatusEscrow, Nominal: 20},
				PollItem{Status: StatusDisabled},
			},
		}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) { checkPoll(t, c.input, c.expect) })
	}
}
