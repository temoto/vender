package bill

import (
	"bytes"
	"context"
	"testing"

	"github.com/temoto/vender/currency"
	"github.com/temoto/vender/hardware/mdb"
	"github.com/temoto/vender/hardware/money"
)

func checkPoll(t *testing.T, input string, expected money.PollResult) {
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
	actual.TestEqual(t, &expected)
}

func TestBillPoll(t *testing.T) {
	t.Parallel()
	type Case struct {
		name   string
		input  string
		expect money.PollResult
	}
	cases := []Case{
		Case{"empty", "", money.PollResult{Delay: DelayNext}},
		Case{"disabled", "09", money.PollResult{
			Delay: DelayNext,
			Items: []money.PollItem{money.PollItem{Status: money.StatusDisabled}},
		}},
		Case{"reset,disabled", "0609", money.PollResult{
			Delay: DelayNext,
			Items: []money.PollItem{
				money.PollItem{Status: money.StatusWasReset},
				money.PollItem{Status: money.StatusDisabled},
			},
		}},
		Case{"escrow", "9209", money.PollResult{
			Delay: DelayNext,
			Items: []money.PollItem{
				money.PollItem{Status: money.StatusEscrow, DataNominal: 20},
				money.PollItem{Status: money.StatusDisabled},
			},
		}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) { checkPoll(t, c.input, c.expect) })
	}
}
