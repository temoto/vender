package coin

import (
	"context"
	"fmt"
	"math/rand"
	"testing"
	"time"

	"github.com/juju/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/temoto/vender/currency"
	"github.com/temoto/vender/hardware/mdb"
	"github.com/temoto/vender/hardware/money"
	"github.com/temoto/vender/log2"
	"github.com/temoto/vender/state"
	state_new "github.com/temoto/vender/state/new"
)

type _PI = money.PollItem

const testScalingFactor currency.Nominal = 100
const testConfig = "money { scale=100 change_over_compensate=10 }"

func mockInitRs() []mdb.MockR {
	setupResponse := fmt.Sprintf("021643%02x0200170102050a0a1900000000000000000000", testScalingFactor)
	return []mdb.MockR{
		// initer, RESET
		{"08", ""},
		// initer, POLL
		{"0b", "0b"},
		// initer, SETUP
		{"09", setupResponse},

		// initer, EXPANSION IDENTIFICATION
		{"0f00", "434f47303030303030303030303030463030313230303120202020029000000003"},

		// initer, FEATURE ENABLE
		{"0f0100000002", ""},

		// initer, DIAG STATUS
		{"0f05", "01000600"},

		// initer, TUBE STATUS
		{"0a", "0000110008"},
	}
}

func mockContext(t testing.TB, rs []mdb.MockR) context.Context {
	ctx, _ := state_new.NewTestContext(t, testConfig)
	mock := mdb.MockFromContext(ctx)
	go func() {
		mock.Expect(mockInitRs())
		mock.Expect(rs)
	}()
	return ctx
}

func newDevice(t testing.TB, ctx context.Context) *CoinAcceptor {
	ca := &CoinAcceptor{}
	ca.dispenseTimeout = 1
	err := ca.Init(ctx)
	require.NoError(t, err)
	return ca
}

func checkPoll(t testing.TB, input string, expected []_PI) {
	ctx := mockContext(t, []mdb.MockR{{"0b", input}})
	ca := newDevice(t, ctx)
	defer mdb.MockFromContext(ctx).Close()
	// ca.AcceptMax(ctx, 1000)
	response := mdb.Packet{}
	err := ca.Device.TxKnown(ca.Device.PacketPoll, &response)
	require.NoError(t, err, "POLL")
	assert.True(t, ca.Device.State().Online())
	pis := make([]_PI, 0, len(input)/2)
	poll := ca.pollFun(func(pi money.PollItem) bool { pis = append(pis, pi); return false })
	_, err = poll(response)
	require.NoError(t, err)
	assert.Equal(t, expected, pis)
}

func TestCoinOffline(t *testing.T) {
	t.Parallel()

	ctx, _ := state_new.NewTestContext(t, testConfig)
	mock := mdb.MockFromContext(ctx)
	mock.ExpectMap(map[string]string{"": ""})
	defer mock.Close()

	ca := new(CoinAcceptor)
	err := ca.Init(ctx)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "mdb.coin is offline")
	assert.Equal(t, errors.Cause(err), mdb.ErrOffline)
	assert.Equal(t, mdb.DeviceOffline, ca.Device.State())
}

func TestCoinNoDiag(t *testing.T) {
	t.Parallel()

	ctx, _ := state_new.NewTestContext(t, testConfig)
	mock := mdb.MockFromContext(ctx)
	mock.ExpectMap(map[string]string{
		"08": "",                                               // initer, RESET
		"0b": "0b",                                             // initer, POLL
		"09": "021643640200170102050a0a1900000000000000000000", // initer, SETUP
		"0a": "0000110008",                                     // initer, TUBE STATUS
		"":   "",
	})
	defer mock.Close()

	ca := new(CoinAcceptor)
	err := ca.Init(ctx)
	require.NoError(t, err)
	assert.True(t, ca.Device.State().Online())
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
		// TODO Case{"reset", "0b", []_PI{{Status: money.StatusWasReset}}},
		Case{"reset", "0b", []_PI{}},
		// TODO Case{"slugs", "21", []_PI{_PI{Status: money.StatusInfo, Error: ErrSlugs, DataCount: 1}}},
		Case{"slugs", "21", []_PI{}},
		Case{"deposited-cashbox", "4109", []_PI{{Status: money.StatusCredit, DataNominal: 2 * testScalingFactor, DataCount: 1, DataCashbox: true}}},
		Case{"return-request", "01", []_PI{{Status: money.StatusReturnRequest}}},
		Case{"deposited-tube", "521e", []_PI{{Status: money.StatusCredit, DataNominal: 5 * testScalingFactor, DataCount: 1}}},
		Case{"deposited-reject", "7300", []_PI{{Status: money.StatusRejected, DataNominal: 10 * testScalingFactor, DataCount: 1}}},
		Case{"dispensed", "9251", []_PI{{Status: money.StatusDispensed, DataNominal: 5 * testScalingFactor, DataCount: 1}}},
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

func TestCoinPayout(t *testing.T) {
	t.Parallel()

	rs := []mdb.MockR{
		{"0f0207", ""},
		{"0f04", "00"},
		{"0f04", ""},
		{"0f03", "07000000"},
	}
	ctx := mockContext(t, rs)
	defer mdb.MockFromContext(ctx).Close()
	ca := newDevice(t, ctx)

	dispensed := new(currency.NominalGroup)
	dispensed.SetValid(ca.SupportedNominals())
	err := ca.NewPayout(7*currency.Amount(ca.scalingFactor), dispensed).Do(ctx)
	require.NoError(t, err)
	assert.Equal(t, "1:7,total:7", dispensed.String())
}

func TestCoinAccept(t *testing.T) {
	t.Parallel()

	ctx := mockContext(t, []mdb.MockR{{"0c001fffff", ""}})
	defer mdb.MockFromContext(ctx).Close()
	ca := newDevice(t, ctx)

	err := ca.AcceptMax(1000).Do(ctx)
	require.NoError(t, err)
}

func TestCoinDispenseSmart(t *testing.T) {
	t.Parallel()

	// type Case struct {
	// 	tubes  currency.NominalGroup
	// 	input  currency.Amount
	// 	over   bool
	// 	expect currency.NominalGroup
	// }
	// cases := []Case{
	// }
	rs := []mdb.MockR{
		{"0a", "00000003"},
		{"0f0201", ""},
		{"0f04", ""},
		{"0f03", "00"},
		{"0f0201", ""},
		{"0f04", ""},
		{"0f03", "00"},
		{"0f0202", ""},
		{"0f04", ""},
		{"0f03", "0001"},
	}
	ctx := mockContext(t, rs)
	defer mdb.MockFromContext(ctx).Close()
	ca := newDevice(t, ctx)
	ca.dispenseSmart = true

	dispensed := new(currency.NominalGroup)
	err := ca.NewDispenseSmart(1*currency.Amount(ca.scalingFactor), true, dispensed).Do(ctx)
	require.NoError(t, err)
	assert.Equal(t, "2:1,total:2", dispensed.String())
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
func checkDiag(t testing.TB, input string, expected DiagResult) {
	ctx := mockContext(t, []mdb.MockR{{"0f05", input}})
	defer mdb.MockFromContext(ctx).Close()
	ca := newDevice(t, ctx)
	dr := new(DiagResult)
	err := ca.CommandExpansionSendDiagStatus(dr)
	require.NoError(t, err, "CommandExpansionSendDiagStatus()")

	msg := fmt.Sprintf("checkDiag input=%s dr=(%d)%s expect=(%d)%s", input, len(*dr), dr.Error(), len(expected), expected.Error())
	require.Equal(t, len(expected), len(*dr), msg)
	for i, ds := range *dr {
		assert.Equal(t, expected[i], ds, msg)
	}
}

func BenchmarkCoinPoll(b *testing.B) {
	type Case struct {
		name  string
		input string
	}
	cases := []Case{
		{"empty", ""},
		{"reset", "0b"},
		{"deposited-tube", "521e"},
	}
	for _, c := range cases {
		c := c
		b.Run(c.name, func(b *testing.B) {
			b.ReportAllocs()
			rs := make([]mdb.MockR, 0, b.N)
			for i := 1; i <= b.N; i++ {
				rs = append(rs, mdb.MockR{"0b", c.input})
			}
			ctx := mockContext(b, rs)

			g := state.GetGlobal(ctx)
			g.Log.SetLevel(log2.LError)
			// g.Hardware.Mdb.Mdber.Log.SetLevel(log2.LError)

			defer mdb.MockFromContext(ctx).Close()
			ca := newDevice(b, ctx)
			parse := ca.pollFun(func(money.PollItem) bool { return false })
			b.SetBytes(int64(len(c.input) / 2))
			b.ResetTimer()
			for i := 1; i <= b.N; i++ {
				response := mdb.Packet{}
				if err := ca.Device.TxKnown(ca.Device.PacketPoll, &response); err != nil {
					b.Fatal(err)
				}
				if _, err := parse(response); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
