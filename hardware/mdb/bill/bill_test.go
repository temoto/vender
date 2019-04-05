package bill

import (
	"context"
	"testing"

	"github.com/temoto/vender/currency"
	"github.com/temoto/vender/hardware/mdb"
	"github.com/temoto/vender/hardware/money"
	"github.com/temoto/vender/head/state"
	"github.com/temoto/vender/helpers"
	"github.com/temoto/vender/log2"
)

type _PI = money.PollItem

const testScalingFactor = 500 // FIXME put into SETUP response

func testMake(t testing.TB, replyFunc mdb.TestReplyFunc) (context.Context, *BillValidator) {
	ctx := state.NewTestContext(t, "money { scale=100 }", log2.LDebug)

	mdber, reqCh, respCh := mdb.NewTestMDBChan(t, ctx)
	config := state.GetConfig(ctx)
	config.Global().Hardware.Mdb.Mdber = mdber
	if _, err := config.Mdber(); err != nil {
		t.Fatal(err)
	}

	go func() {
		defer close(respCh)
		// initer, SETUP
		// FIXME put testScalingFactor here
		mdb.TestChanTx(t, reqCh, respCh, "31", "011810000a0000c8001fff01050a32640000000000000000000000")

		// initer, EXPANSION IDENTIFICATION
		// TODO fill real response
		mdb.TestChanTx(t, reqCh, respCh, "3700", "49435430303030303030303030303056372d5255523530303030300120")

		// initer, STACKER
		mdb.TestChanTx(t, reqCh, respCh, "36", "000b")

		if replyFunc != nil {
			replyFunc(t, reqCh, respCh)
		}
	}()

	bv := new(BillValidator)
	bv.dev.DelayIdle = 1
	bv.dev.DelayNext = 1
	bv.dev.DelayReset = 1

	return ctx, bv
}

func checkPoll(t *testing.T, input string, expected []_PI) {
	reply := func(t testing.TB, reqCh <-chan mdb.Packet, respCh chan<- mdb.Packet) {
		mdb.TestChanTx(t, reqCh, respCh, "33", input)
	}
	ctx, bv := testMake(t, reply)
	err := bv.Init(ctx)
	if err != nil {
		t.Fatal(err)
	}

	pis := make([]_PI, 0, len(input)/2)
	r := bv.dev.Tx(bv.dev.PacketPoll)
	if r.E != nil {
		t.Fatalf("POLL err=%v", r.E)
	}
	bv.pollFun(func(pi money.PollItem) bool { pis = append(pis, pi); return false })(r.P)
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
		Case{"empty", "", []_PI{}},
		Case{"disabled", "09", []_PI{
			_PI{HardwareCode: 0x09, Status: money.StatusDisabled},
		}},
		Case{"reset,disabled", "0609", []_PI{
			_PI{HardwareCode: 0x06, Status: money.StatusWasReset},
			_PI{HardwareCode: 0x09, Status: money.StatusDisabled},
		}},
		Case{"escrow", "9209", []_PI{
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
	reply := func(t testing.TB, reqCh <-chan mdb.Packet, respCh chan<- mdb.Packet) {
		mdb.TestChanTx(t, reqCh, respCh, "3400070000", "")
	}
	ctx, bv := testMake(t, reply)
	if err := bv.Init(ctx); err != nil {
		t.Fatal(err)
	}
	if err := bv.AcceptMax(10000).Do(ctx); err != nil {
		t.Fatal(err)
	}
}

// measure allocations by real Doer graph
func BenchmarkNewIniter(b *testing.B) {
	b.ReportAllocs()
	ctx := state.NewTestContext(b, "", log2.LError)
	bv := &BillValidator{}
	bv.dev.Log = log2.ContextValueLogger(ctx, log2.ContextKey)

	b.ResetTimer()
	for i := 1; i <= b.N; i++ {
		bv.newIniter()
	}
}
