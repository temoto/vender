package mdb

import (
	"bytes"
	"encoding/hex"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/temoto/errors"
	"github.com/temoto/vender/helpers"
	"github.com/temoto/vender/log2"
)

type mockReadEffect struct {
	b     []byte
	delay time.Duration
	err   error
}

type mockReader struct {
	Log *log2.Log
	pos uint
	vs  []mockReadEffect
}

func (self *mockReader) Read(p []byte) (int, error) {
	max := uint(len(self.vs))
	for {
		if self.pos >= max {
			return 0, io.EOF
		}
		mre := self.vs[self.pos]
		self.pos++
		time.Sleep(mre.delay)
		if mre.err != nil {
			self.Log.Errorf("mr.Read ret=err mre=%+v", mre)
			return 0, mre.err
		}
		if mre.b != nil {
			n := copy(p, mre.b)
			self.Log.Debugf("mr.Read ret=%d,%x mre=%+v", n, p[:n], mre)
			return n, nil
		}
	}
}

func parseMockReader(s string) *mockReader {
	mr := new(mockReader)
	for _, es := range strings.Fields(s) {
		mre := mockReadEffect{}
		for _, token := range strings.Split(es, ",") {
			switch token[0] {
			case 'b':
				b, err := hex.DecodeString(token[1:])
				if err != nil {
					panic(err)
				}
				mre.b = b
			case 'd':
				d, err := time.ParseDuration(token[1:])
				if err != nil {
					panic(err)
				}
				mre.delay = d
				// TODO case "e" error
			case 'e':
				mre.err = errors.New(token[1:])
			default:
				panic("unknown token: " + token)
			}
		}
		mr.vs = append(mr.vs, mre)
	}
	return mr
}

func testFileUart(t testing.TB, r io.Reader, w io.Writer) *fileUart {
	u := NewFileUart(log2.NewTest(t, log2.LDebug))
	u.r = r
	u.w = w
	return u
}

func checkUarterTx(t testing.TB, u Uarter, send string, bw *bytes.Buffer, expectOk string, expectErr error) {
	request, err := hex.DecodeString(send)
	if err != nil {
		panic(errors.Errorf("code error send=%s err=%v", send, err))
	}
	buf := make([]byte, PacketMaxLength)
	n, err := u.Tx(request, buf)
	buf = buf[:n]
	if expectErr != nil {
		if err.Error() != expectErr.Error() {
			t.Fatalf("error=%v expected=%v stack:\n%s", err, expectErr, errors.ErrorStack(err))
		}
	} else {
		if err != nil {
			t.Fatal(errors.ErrorStack(err))
		}
		assert.Equal(t, expectOk, hex.EncodeToString(buf))
	}
}

func TestUarterTx(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name      string
		send      string
		wexpect   string
		rmock     string
		expectOk  string
		expectErr error
	}{
		{"eof", "30", "", "", "", io.EOF},
		{"empty", "30", "", "bff0000", "", nil},
		{"complex", "30", "", "b73ffff04 bff0076", "73ff04", nil},
		{"by 1 byte", "0b", "", "b02 b16 b41 bff b00 b59", "021641", nil},
		{"invalid chk", "0b", "", "b0bff0001", "", InvalidChecksum{Actual: 0x0b, Received: 0x01}},
		{"data without chk", "0b", "", "b0b", "", io.EOF},
		{"err mid-read", "0f", "", "b30 d50ms,eIO", "", errors.New("IO")},
	}
	helpers.RandUnix().Shuffle(len(cases), func(i int, j int) { cases[i], cases[j] = cases[j], cases[i] })
	for _, c := range cases {
		c := c
		// for u in all kinds of Uarter
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			mr := parseMockReader(c.rmock)
			mr.Log = log2.NewTest(t, log2.LDebug)
			mw := bytes.NewBuffer(nil)
			u := testFileUart(t, mr, mw)
			checkUarterTx(t, u, c.send, mw, c.expectOk, c.expectErr)
		})
	}
}

func BenchmarkFileUartTx(b *testing.B) {
	b.ReportAllocs()
	mr := parseMockReader(strings.Repeat("bff0000 ", b.N))
	mw := bytes.NewBuffer(nil)
	u := testFileUart(b, mr, mw)
	u.Log = nil
	response := [PacketMaxLength]byte{}
	b.ResetTimer()
	for i := 1; i <= b.N; i++ {
		_, err := u.Tx(PacketNul1.Bytes(), response[:])
		if err != nil {
			b.Fatal(errors.ErrorStack(err))
		}
	}
}
