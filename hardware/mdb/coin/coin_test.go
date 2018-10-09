package coin

import (
	"bytes"
	"context"
	"testing"

	"github.com/temoto/vender/currency"
	"github.com/temoto/vender/hardware/mdb"
	"github.com/temoto/vender/hardware/money"
)

type _PR = money.PollResult
type _PI = money.PollItem

func checkPoll(t *testing.T, input string, expected _PR) {
	inp := mdb.PacketFromHex(input)
	mdber, mockRead, w := mdb.NewTestMDB(t)
	c := &CoinAcceptor{mdb: mdber}
	c.Init(context.Background(), mdber)
	w.Reset()
	mockRead(inp.Wire(true))
	c.coinTypeCredit[0] = currency.Nominal(1)
	c.coinTypeCredit[1] = currency.Nominal(2)
	c.coinTypeCredit[2] = currency.Nominal(5)
	c.coinTypeCredit[3] = currency.Nominal(10)
	actual := c.CommandPoll()
	writeExpect := mdb.PacketFromHex("0b").Wire(false)
	if len(input) > 0 {
		writeExpect = append(writeExpect, 0)
	}
	if !bytes.Equal(w.Bytes(), writeExpect) {
		t.Fatalf("CommandPoll() must send packet, found=%x expected=%x", w.Bytes(), writeExpect)
	}
	actual.TestEqual(t, &expected)
}

func TestCoinPoll(t *testing.T) {
	t.Parallel()
	type Case struct {
		name   string
		input  string
		expect _PR
	}
	cases := []Case{
		Case{"empty", "", _PR{Delay: DelayNext}},
		Case{"reset", "0b", _PR{
			Delay: DelayNext,
			Items: []_PI{_PI{Status: money.StatusWasReset}},
		}},
		Case{"slugs", "21", _PR{
			Delay: DelayNext,
			Items: []_PI{_PI{Status: money.StatusInfo, Error: ErrSlugs, DataCount: 1}},
		}},
		Case{"deposited-cashbox", "4109", _PR{
			Delay: DelayNext,
			Items: []_PI{_PI{
				Status:      money.StatusCredit,
				DataNominal: currency.Nominal(2),
				DataCount:   1,
				DataCashbox: true,
			}},
		}},
		Case{"deposited-tube", "521e", _PR{
			Delay: DelayNext,
			Items: []_PI{_PI{Status: money.StatusCredit, DataNominal: currency.Nominal(5), DataCount: 1}},
		}},
		Case{"deposited-reject", "7300", _PR{
			Delay: DelayNext,
			Items: []_PI{_PI{Status: money.StatusRejected, DataNominal: currency.Nominal(10), DataCount: 1}},
		}},
		Case{"dispensed", "9251", _PR{
			Delay: DelayNext,
			Items: []_PI{_PI{Status: money.StatusDispensed, DataNominal: currency.Nominal(5), DataCount: 1}},
		}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) { checkPoll(t, c.input, c.expect) })
	}
}
