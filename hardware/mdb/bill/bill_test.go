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
	if a.Ready != b.Ready {
		t.Errorf("PoolResult.Ready a=%t b=%t", a.Ready, b.Ready)
	}
	if a.Delay != b.Delay {
		t.Errorf("PoolResult.Delay a=%v b=%v", a.Delay, b.Delay)
	}
	if a.Error != b.Error {
		t.Errorf("PoolResult.Error a=%s b=%s", a.Error, b.Error)
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
			ias = fmt.Sprintf("%#v", ia)
		}
		if i < len(b.Items) {
			ib = &b.Items[i]
			ibs = fmt.Sprintf("%#v", ib)
		}
		if *ia != *ib {
			t.Errorf("PoolResult.Items[%d] a=%s b=%s", i, ias, ibs)
		}
	}
}

func checkPoll(t *testing.T, input string, expected PollResult) {
	inp := mdb.PacketFromHex(input)
	r := bytes.NewReader(inp.Wire(true))
	w := bytes.NewBuffer(nil)
	m, err := mdb.NewMDB(mdb.NewNullUart(r, w), "", 9600)
	if err != nil {
		t.Fatal(err)
	}
	bv := &BillValidator{mdb: m}
	bv.Init(context.WithValue(context.Background(), "run/mdber", m))
	bv.billTypeCredit[0] = currency.Nominal(5)
	bv.billTypeCredit[1] = currency.Nominal(10)
	bv.billTypeCredit[2] = currency.Nominal(20)
	actual := bv.Poll()
	writeExpect := mdb.PacketFromHex("33").Wire(false)
	if len(input) > 0 {
		writeExpect = append(writeExpect, 0)
	}
	if !bytes.Equal(w.Bytes(), writeExpect) {
		t.Fatalf("Poll() must write packet, found=%x expected=%x", w.Bytes(), writeExpect)
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
		Case{"empty", "", PollResult{
			Ready: true, Delay: delayNext},
		},
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
