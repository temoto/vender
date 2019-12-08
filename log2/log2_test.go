package log2

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLog2(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		fun  func(t testing.TB, l *Log) string
	}{
		{"caller/debug", func(t testing.TB, l *Log) string {
			l.SetFlags(log.Lshortfile)
			l.Debugf("low level var=%d", 42)
			return formatCallerShort(1) + "debug: low level var=42\n"
		}},
		{"caller/info", func(t testing.TB, l *Log) string {
			l.SetFlags(log.Lshortfile)
			l.Infof("regular state=%s", "ok")
			return formatCallerShort(1) + "regular state=ok\n"
		}},
		{"caller/error", func(t testing.TB, l *Log) string {
			l.SetFlags(log.Lshortfile)
			l.Errorf("problem")
			return formatCallerShort(1) + "error: problem\n"
		}},
		{"error-func/error", func(t testing.TB, l *Log) string {
			ech := make(chan error, 1)
			l.SetErrorFunc(func(e error) { ech <- e })
			l.SetFlags(0)
			exactError := fmt.Errorf("one particular issue")
			l.Error(exactError)
			close(ech)
			e := <-ech
			if l == nil {
				assert.Nil(t, e)
			} else {
				assert.Equal(t, exactError, e)
			}
			return "error: one particular issue\n"
		}},
		{"error-func/string", func(t testing.TB, l *Log) string {
			ech := make(chan error, 1)
			l.SetErrorFunc(func(e error) { ech <- e })
			l.SetFlags(0)
			l.Errorf("trouble var=%.1f", 3.4)
			close(ech)
			e := <-ech
			if l == nil {
				assert.Nil(t, e)
			} else {
				assert.Equal(t, "trouble var=3.4", e.Error())
			}
			return "error: trouble var=3.4\n"
		}},
	}
	for _, c := range cases {
		c := c
		t.Run(c.name+"/logger=nil", func(t *testing.T) {
			c.fun(t, nil)
		})
		t.Run(c.name, func(t *testing.T) {
			buf := bytes.NewBuffer(nil)
			l := NewWriter(buf, LAll)
			expect := c.fun(t, l)
			assert.Equal(t, expect, buf.String())
		})
	}
}

func BenchmarkLog2(b *testing.B) {
	call := func(f FmtFunc) { f("example log with arg1=%s and arg2=%d", "example-arg", 12345678) }
	const expect string = "example log with arg1=example-arg and arg2=12345678\n"

	prepareStd := func(w io.Writer) FmtFunc { return log.New(w, "", 0).Printf }
	prepareMe := func(w io.Writer) FmtFunc { l := NewWriter(w, LInfo); l.SetFlags(0); return l.Infof }
	prepareMeSkipLevel := func(w io.Writer) FmtFunc { l := NewWriter(w, LError); l.SetFlags(0); return l.Infof }

	type Case struct {
		name    string
		prepare func(w io.Writer) FmtFunc
	}
	cases := []Case{
		Case{"me-skiplevel", prepareMeSkipLevel},
		Case{"me", prepareMe},
		Case{"stdlib", prepareStd},
	}
	for _, c := range cases {
		for _, dest := range []string{"buffer", "discard"} {
			var writer io.Writer
			buf := bytes.NewBuffer(nil)
			switch dest {
			case "buffer":
				writer = buf
			case "discard":
				writer = ioutil.Discard
			default:
				panic("code error")
			}
			fun := c.prepare(writer)

			b.Run(c.name+"/"+dest, func(b *testing.B) {
				b.ReportAllocs()

				result := benchCapture(call)
				require.Equal(b, expect, result)
				expectTotal := len(result) * b.N
				buf.Grow(expectTotal)
				buf.Reset()

				b.SetBytes(int64(len(result)))
				b.ResetTimer()
				for i := 1; i <= b.N; i++ {
					call(fun)
				}
				b.StopTimer()

				if b.N == 1 && dest == "buffer" && !strings.Contains(c.name, "skip") {
					written := buf.String()
					total := len(written)
					assert.Equal(b, expectTotal, total)
					assert.Equal(b, expect, written)
				}
			})
		}
	}
}

func benchCapture(call func(FmtFunc)) string {
	s := ""
	call(func(format string, args ...interface{}) {
		s = fmt.Sprintf(format+"\n", args...)
	})
	return s
}

func callerShort(depth int) (file string, line int) {
	var ok bool
	_, file, line, ok = runtime.Caller(depth)
	if !ok {
		file = "???"
		line = 0
	}

	short := file
	for i := len(file) - 1; i > 0; i-- {
		if file[i] == '/' {
			short = file[i+1:]
			break
		}
	}
	file = short

	return
}

func formatCallerShort(depth int) string {
	file, line := callerShort(depth + 1)
	return fmt.Sprintf("%s:%d: ", file, line-1)
}
