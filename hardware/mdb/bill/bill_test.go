package bill

import (
	"context"
	"testing"

	"github.com/temoto/vender/currency"
	"github.com/temoto/vender/hardware/mdb"
	"github.com/temoto/vender/hardware/money"
	"github.com/temoto/vender/helpers"
	"github.com/temoto/vender/state"
)

type _PI = money.PollItem

const testConfig = "money { scale=100 }"
const testScalingFactor = 500 // FIXME put into SETUP response

func mockInitRs() []mdb.MockR {
	return []mdb.MockR{
		// initer, SETUP
		// FIXME put testScalingFactor here
		{"31", "011810000a0000c8001fff01050a32640000000000000000000000"},

		// initer, EXPANSION IDENTIFICATION
		// TODO fill real response
		{"3700", "49435430303030303030303030303056372d5255523530303030300120"},

		// initer, STACKER
		{"36", "000b"},
	}
}

func testMake(t testing.TB, rs []mdb.MockR) (context.Context, *BillValidator) {
	ctx, _ := state.NewTestContext(t, testConfig)

	mock := mdb.MockFromContext(ctx)
	go func() {
		mock.Expect(mockInitRs())
		mock.Expect(rs)
	}()

	bv := new(BillValidator)
	bv.dev.DelayIdle = 1
	bv.dev.DelayNext = 1
	bv.dev.DelayReset = 1

	return ctx, bv
}

func checkPoll(t *testing.T, input string, expected []_PI) {
	ctx, bv := testMake(t, []mdb.MockR{{"33", input}})
	defer mdb.MockFromContext(ctx).Close()
	err := bv.Init(ctx)
	if err != nil {
		t.Fatal(err)
	}

	pis := make([]_PI, 0, len(input)/2)
	r := bv.dev.Tx(bv.dev.PacketPoll)
	if r.E != nil {
		t.Fatalf("POLL err=%v", r.E)
	}
	poll := bv.pollFun(func(pi money.PollItem) bool {
		pis = append(pis, pi)
		return false
	})
	if _, err = poll(r.P); err != nil {
		t.Fatal(err)
	}
	money.TestPollItemsEqual(t, pis, expected)
}

func TestBillPoll(t *testing.T) {
	t.Parallel()

	type Case struct {
		name   string
		input  string
		expect []_PI
	}
	cases := []Case{
		{"empty", "", []_PI{}},
		{"disabled", "09", []_PI{
			_PI{HardwareCode: 0x09, Status: money.StatusDisabled},
		}},
		{"reset,disabled", "0609", []_PI{
			_PI{HardwareCode: 0x06, Status: money.StatusWasReset},
			_PI{HardwareCode: 0x09, Status: money.StatusDisabled},
		}},
		{"escrow", "9209", []_PI{
			_PI{HardwareCode: 0x90, Status: money.StatusEscrow, DataNominal: 20 * currency.Nominal(testScalingFactor), DataCount: 1},
			_PI{HardwareCode: 0x09, Status: money.StatusDisabled},
		}},
	}
	helpers.RandUnix().Shuffle(len(cases), func(i int, j int) { cases[i], cases[j] = cases[j], cases[i] })
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			checkPoll(t, c.input, c.expect)
		})
	}
}

func TestBillAcceptMax(t *testing.T) {
	t.Parallel()

	ctx, bv := testMake(t, []mdb.MockR{{"3400070000", ""}})
	defer mdb.MockFromContext(ctx).Close()
	if err := bv.Init(ctx); err != nil {
		t.Fatal(err)
	}
	if err := bv.AcceptMax(10000).Do(ctx); err != nil {
		t.Fatal(err)
	}
}
