package coin

import (
	"context"
	"fmt"
	"math/rand"
	"testing"
	"time"

	"github.com/temoto/vender/currency"
	"github.com/temoto/vender/hardware/mdb"
	"github.com/temoto/vender/hardware/money"
	"github.com/temoto/vender/head/state"
	"github.com/temoto/vender/log2"
)

type _PI = money.PollItem

func mockContext(t testing.TB, replyFunc mdb.TestReplyFunc, logLevel log2.Level) context.Context {
	ctx := state.NewTestContext(t, "", logLevel)

	mdber, reqCh, respCh := mdb.NewTestMDBChan(t, ctx)
	go func() {
		defer close(respCh)
		// InitSequence, SETUP
		mdb.TestChanTx(t, reqCh, respCh, "09", "021643640200170102050a0a1900000000000000000000")

		// InitSequence, EXPANSION IDENTIFICATION
		mdb.TestChanTx(t, reqCh, respCh, "0f00", "434f47303030303030303030303030463030313230303120202020029000000003")

		// InitSequence, FEATURE ENABLE
		mdb.TestChanTx(t, reqCh, respCh, "0f0100000002", "")

		// InitSequence, DIAG STATUS
		mdb.TestChanTx(t, reqCh, respCh, "0f05", "01000600")

		// InitSequence, TUBE STATUS
		mdb.TestChanTx(t, reqCh, respCh, "0a", "0000110008")

		// InitSequence, COIN TYPE
		mdb.TestChanTx(t, reqCh, respCh, "0cffffffff", "")

		if replyFunc != nil {
			replyFunc(t, reqCh, respCh)
		}
	}()

	ctx = context.WithValue(ctx, mdb.ContextKey, mdber)
	return ctx
}

func newDevice(t testing.TB, ctx context.Context) *CoinAcceptor {
	ca := &CoinAcceptor{}
	ca.dev.DelayErr = 1
	ca.dev.DelayIdle = 1
	ca.dev.DelayNext = 1
	ca.dev.DelayReset = 1
	err := ca.Init(ctx)
	if err != nil {
		t.Fatalf("ca.Init err=%v", err)
	}
	ca.coinTypeCredit[0] = currency.Nominal(1)
	ca.coinTypeCredit[1] = currency.Nominal(2)
	ca.coinTypeCredit[2] = currency.Nominal(5)
	ca.coinTypeCredit[3] = currency.Nominal(10)
	return ca
}

func checkPoll(t testing.TB, input string, expected []_PI) {
	reply := func(t testing.TB, reqCh <-chan mdb.Packet, respCh chan<- mdb.Packet) {
		mdb.TestChanTx(t, reqCh, respCh, "0b", input)
	}
	ca := newDevice(t, mockContext(t, reply, log2.LDebug))
	r := ca.dev.DoPollSync(context.Background())
	if r.E != nil {
		t.Fatalf("POLL err=%v", r.E)
	}
	pis := make([]_PI, 0, len(input)/2)
	ca.newPoller(func(pi money.PollItem) { pis = append(pis, pi) })(r)
	money.TestPollItemsEqual(t, pis, expected)
}

func TestCoinPoll(t *testing.T) {
	t.Parallel()
	type Case struct {
		name   string
		input  string
		expect []_PI
	}
	cases := []Case{
		Case{"empty", "", []_PI{}},
		Case{"reset", "0b", []_PI{_PI{Status: money.StatusWasReset}}},
		Case{"slugs", "21", []_PI{_PI{Status: money.StatusInfo, Error: ErrSlugs, DataCount: 1}}},
		Case{"deposited-cashbox", "4109", []_PI{_PI{
			Status:      money.StatusCredit,
			DataNominal: currency.Nominal(2),
			DataCount:   1,
			DataCashbox: true,
		}}},
		Case{"deposited-tube", "521e", []_PI{_PI{Status: money.StatusCredit, DataNominal: currency.Nominal(5), DataCount: 1}}},
		Case{"deposited-reject", "7300", []_PI{_PI{Status: money.StatusRejected, DataNominal: currency.Nominal(10), DataCount: 1}}},
		Case{"dispensed", "9251", []_PI{_PI{Status: money.StatusDispensed, DataNominal: currency.Nominal(5), DataCount: 1}}},
	}
	rand.New(rand.NewSource(time.Now().UnixNano())).Shuffle(len(cases), func(i int, j int) { cases[i], cases[j] = cases[j], cases[i] })
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			checkPoll(t, c.input, c.expect)
		})
	}
}

func checkDiag(t testing.TB, input string, expected DiagResult) {
	reply := func(t testing.TB, reqCh <-chan mdb.Packet, respCh chan<- mdb.Packet) {
		mdb.TestChanTx(t, reqCh, respCh, "0f05", input)
	}
	ca := newDevice(t, mockContext(t, reply, log2.LDebug))
	dr := new(DiagResult)
	err := ca.CommandExpansionSendDiagStatus(dr)
	if err != nil {
		t.Fatalf("CommandExpansionSendDiagStatus() err=%v", err)
	}
	s := fmt.Sprintf("checkDiag input=%s dr=(%d)%s expect=(%d)%s", input, len(*dr), dr.Error(), len(expected), expected.Error())
	if len(*dr) != len(expected) {
		t.Fatal(s)
	}
	for i, ds := range *dr {
		if ds != expected[i] {
			t.Fatal(s)
		}
	}
}

func TestCoinDiag(t *testing.T) {
	t.Parallel()
	type Case struct {
		name   string
		input  string
		expect DiagResult
	}
	cases := []Case{
		Case{"empty", "", DiagResult{}},
		Case{"start", "01000600", DiagResult{DiagPoweringUp, DiagInhibited}},
		Case{"ok", "0300", DiagResult{DiagOK}},
		Case{"general-error", "1000", DiagResult{DiagGeneralError}},
		Case{"dispenser-error", "1400", DiagResult{DiagDispenserError}},
	}
	rand.New(rand.NewSource(time.Now().UnixNano())).Shuffle(len(cases), func(i int, j int) { cases[i], cases[j] = cases[j], cases[i] })
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			checkDiag(t, c.input, c.expect)
		})
	}
}

func BenchmarkCoinPoll(b *testing.B) {
	type Case struct {
		name  string
		input string
	}
	cases := []Case{
		Case{"empty", ""},
		Case{"reset", "0b"},
	}
	for _, c := range cases {
		c := c
		b.Run(c.name, func(b *testing.B) {
			b.ReportAllocs()
			reply := func(t testing.TB, reqCh <-chan mdb.Packet, respCh chan<- mdb.Packet) {
				for i := 1; i <= b.N; i++ {
					mdb.TestChanTx(t, reqCh, respCh, "0b", c.input)
				}
			}
			ca := newDevice(b, mockContext(b, reply, log2.LError))
			poller := ca.newPoller(func(money.PollItem) {})
			b.SetBytes(int64(len(c.input) / 2))
			b.ResetTimer()
			for i := 1; i <= b.N; i++ {
				poller(ca.dev.DoPollSync(context.Background()))
			}
		})
	}
}
