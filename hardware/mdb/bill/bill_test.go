package bill

import (
	"context"
	"fmt"
	"testing"

	"github.com/juju/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/temoto/vender/currency"
	"github.com/temoto/vender/hardware/mdb"
	"github.com/temoto/vender/hardware/money"
	"github.com/temoto/vender/helpers"
	state_new "github.com/temoto/vender/state/new"
)

type _PI = money.PollItem

const testConfig = "money { scale=100 }"
const testScalingFactor currency.Nominal = 10
const devScaling currency.Nominal = 100

func mockInitRs(scaling currency.Nominal, decimal uint8) []mdb.MockR {
	return []mdb.MockR{
		// initer, RESET
		{"30", ""},
		// initer, POLL
		{"33", "0609"},
		// initer, SETUP
		{"31", fmt.Sprintf("011810%04x%02x00c8001fff01050a32640000000000000000000000", scaling, decimal)},

		// initer, EXPANSION IDENTIFICATION
		// TODO fill real response
		{"3700", "49435430303030303030303030303056372d5255523530303030300120"},

		// initer, STACKER
		{"36", "000b"},
	}
}

func testMake(t testing.TB, rs []mdb.MockR, scaling currency.Nominal, decimal uint8) (context.Context, *BillValidator) {
	ctx, _ := state_new.NewTestContext(t, testConfig)

	mock := mdb.MockFromContext(ctx)
	go func() {
		mock.Expect(mockInitRs(scaling, decimal))
		mock.Expect(rs)
	}()

	bv := new(BillValidator)
	err := bv.Init(ctx)
	require.NoError(t, err)

	return ctx, bv
}

func checkPoll(t *testing.T, input string, expected []_PI) {
	ctx, bv := testMake(t, []mdb.MockR{{"33", input}}, testScalingFactor, 0)
	defer mdb.MockFromContext(ctx).Close()

	pis := make([]_PI, 0, len(input)/2)
	r := bv.dev.Tx(bv.dev.PacketPoll)
	require.NoError(t, r.E, "POLL")
	poll := bv.pollFun(func(pi money.PollItem) bool {
		pis = append(pis, pi)
		return false
	})
	_, err := poll(r.P)
	require.NoError(t, err)
	assert.Equal(t, expected, pis)
}

func TestBillOffline(t *testing.T) {
	t.Parallel()

	ctx, _ := state_new.NewTestContext(t, testConfig)
	mock := mdb.MockFromContext(ctx)
	mock.ExpectMap(map[string]string{"": ""})
	defer mock.Close()

	bv := new(BillValidator)
	err := bv.Init(ctx)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "mdb.bill RESET: offline")
	assert.Equal(t, errors.Cause(err), mdb.ErrOffline)
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
			_PI{HardwareCode: 0x90, Status: money.StatusEscrow, DataNominal: 10 * devScaling * testScalingFactor, DataCount: 1},
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

	// FIXME explicit enable/disable escrow in config
	ctx, bv := testMake(t, []mdb.MockR{{"3400070007", ""}}, testScalingFactor, 0)
	defer mdb.MockFromContext(ctx).Close()
	err := bv.AcceptMax(10000).Do(ctx)
	require.NoError(t, err)
}

func TestBillScaling(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		scaling currency.Nominal
		decimal uint8
	}{
		{"10,0", 10, 0},
		{"100,1", 100, 1},
		{"1000,2", 1000, 2},
	}
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			ctx, bv := testMake(t, nil, c.scaling, c.decimal)
			defer mdb.MockFromContext(ctx).Close()
			ns := bv.SupportedNominals()
			assert.Equal(t, []currency.Nominal{1000, 5000, 10000, 50000, 100000}, ns)
		})
	}
}
