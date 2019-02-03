package mega

import (
	"encoding/hex"
	"fmt"
	"math/rand"
	"strings"
	"testing"
	"time"
)

func TestParseResponse(t *testing.T) {
	t.Parallel()
	type Case struct {
		name      string
		input     string
		expect    string
		expectErr string
	}
	cases := []Case{
		{"response-empty", "", "", "response empty not valid"},
		{"response-empty-valid-length", "00", "", ""},
		{"response-length-max", "f1", "", "response=f1 claims length=241 > MAX=80 not valid"},
		{"response-under-length", "01", "", "response=01 claims length=1 > buffer=0 not valid"},
		{"packet-short", "0101", "", "packet=01 claims length=1 < min=4 not valid"},
		{"packet-under-length", "0104", "", "packet=04 claims length=4 > buffer=1 not valid"},
		{"packet-corrupted", "04040000ff", "", "packet=040000ff crc=ff actual=86 not valid"},
		{"packet-invalid-header", "0404000086", "", "packet=04000086 header=00 not valid"},
		{"ok", "0404000115", "00:01", ""},
		{"ok-and-garbage", "0404d0019cffffff", "d0:01", ""},
		{"ok-and-short", "060485018c04ff", "85:01", "packet=04ff claims length=4 > buffer=2 not valid"},
		{"debug-beebee", "07070004beebeefe", "00:04beebee", ""},
		{"mdb-success", "04043e0821", "3e:08", ""},
		{"mdb-success-and-twi", "09041508cd0500063077", "15:08,00:0630", ""},
	}
	rand.New(rand.NewSource(time.Now().UnixNano())).Shuffle(len(cases), func(i int, j int) { cases[i], cases[j] = cases[j], cases[i] })
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			// t.Parallel()
			input, err := hex.DecodeString(c.input)
			if err != nil {
				t.Fatalf("invalid input=%s err='%v'", c.input, err)
			}
			var ps []string
			err = ParseResponse(input, func(p Packet) {
				ps = append(ps, fmt.Sprintf("%02x:%s", p.Id, p.SimpleHex()))
			})
			errString := ""
			if err != nil {
				errString = err.Error()
			}
			if errString != c.expectErr {
				t.Errorf("input=%s expected err='%v' actual='%v'", c.input, c.expectErr, err)
			}
			psjoined := strings.Join(ps, ",")
			if psjoined != c.expect {
				t.Errorf("input=%s expected packets='%s' actual='%s'", c.input, c.expect, psjoined)
			}
		})
	}
}
