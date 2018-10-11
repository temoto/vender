package bill

import (
	"context"
	"math/rand"
	"testing"
	"time"

	"github.com/temoto/vender/currency"
	"github.com/temoto/vender/hardware/mdb"
	"github.com/temoto/vender/hardware/money"
)

type _PR = money.PollResult
type _PI = money.PollItem

func testMake(t testing.TB, replyFunc mdb.TestReplyFunc) *BillValidator {
	mdber, reqCh, respCh := mdb.NewTestMDBChan(t)
	go func() {
		defer close(respCh)
		// InitSequence, SETUP
		// TODO fill real response
		mdb.TestChanTx(t, reqCh, respCh, "31", "011810000a0000c8001fff01050a32640000000000000000000000")

		// InitSequence, EXPANSION IDENTIFICATION
		// TODO fill real response
		mdb.TestChanTx(t, reqCh, respCh, "3700", "49435430303030303030303030303056372d5255523530303030300120")

		// InitSequence, STACKER
		mdb.TestChanTx(t, reqCh, respCh, "36", "000b")

		// InitSequence, BILL TYPE
		mdb.TestChanTx(t, reqCh, respCh, "34ffff0000", "")

		if replyFunc != nil {
			replyFunc(t, reqCh, respCh)
		}
	}()
	bv := &BillValidator{mdb: mdber}
	err := bv.Init(context.Background(), mdber)
	if err != nil {
		t.Fatalf("bv.Init err=%v", err)
	}
	bv.billTypeCredit[0] = currency.Nominal(5)
	bv.billTypeCredit[1] = currency.Nominal(10)
	bv.billTypeCredit[2] = currency.Nominal(20)
	return bv
}

func checkPoll(t *testing.T, input string, expected _PR) {
	reply := func(t testing.TB, reqCh <-chan *mdb.Packet, respCh chan<- *mdb.Packet) {
		mdb.TestChanTx(t, reqCh, respCh, "33", input)
	}
	bv := testMake(t, reply)
	actual := bv.CommandPoll()
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
	rand.New(rand.NewSource(time.Now().UnixNano())).Shuffle(len(cases), func(i int, j int) { cases[i], cases[j] = cases[j], cases[i] })
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			checkPoll(t, c.input, c.expect)
		})
	}
}
