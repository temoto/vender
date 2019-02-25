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
	"unicode"
)

func hexSpecial(input string) string {
	result := ""
	for _, r := range input {
		if unicode.In(r, unicode.Digit, unicode.Letter, unicode.Punct, unicode.Space) {
			result += string(r)
		} else {
			result += fmt.Sprintf("{%02x}", r)
		}
	}
	return result
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

func TestLog2(t *testing.T) {
	t.Parallel()
	buf := bytes.NewBuffer(nil)
	l := NewWriter(buf, LAll)
	l.SetFlags(log.Lshortfile)

	type Case struct {
		name   string
		fun    func(format string, args ...interface{}) (string, int)
		expect string
	}
	cases := []Case{
		Case{"debug", func(format string, args ...interface{}) (string, int) {
			l.Debugf(format, args...)
			return callerShort(1)
		}, "debug: check\n"},
		Case{"info", func(format string, args ...interface{}) (string, int) {
			l.Infof(format, args...)
			return callerShort(1)
		}, "check\n"},
		Case{"error", func(format string, args ...interface{}) (string, int) {
			l.Errorf(format, args...)
			return callerShort(1)
		}, "error: check\n"},
	}
	for _, c := range cases {
		c := c
		buf.Reset()
		file, line := c.fun("check")
		c.expect = fmt.Sprintf("%s:%d: %s", file, line-1, c.expect)
		s := buf.String()
		if s != c.expect {
			t.Errorf("debug expected='%s' actual='%s'", hexSpecial(c.expect), hexSpecial(s))
		}
	}
}

func benchCapture(call func(Func)) string {
	s := ""
	call(func(format string, args ...interface{}) {
		s = fmt.Sprintf(format+"\n", args...)
	})
	return s
}

func BenchmarkLog2(b *testing.B) {
	call := func(f Func) { f("example log with arg1=%s and arg2=%d", "example-arg", 12345678) }
	const expect string = "example log with arg1=example-arg and arg2=12345678\n"

	prepareStd := func(w io.Writer) Func { return log.New(w, "", 0).Printf }
	prepareMe := func(w io.Writer) Func { l := NewWriter(w, LInfo); l.SetFlags(0); return l.Infof }
	prepareMeSkipLevel := func(w io.Writer) Func { l := NewWriter(w, LError); l.SetFlags(0); return l.Infof }

	type Case struct {
		name    string
		prepare func(w io.Writer) Func
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
				expectTotal := len(result) * b.N
				buf.Grow(expectTotal)
				if result != expect {
					b.Fatalf("expected='%s' actual='%s'", hexSpecial(expect), hexSpecial(result))
				}
				buf.Reset()

				b.SetBytes(int64(len(result)))
				b.ResetTimer()
				for i := 1; i <= b.N; i++ {
					call(fun)
				}
				b.StopTimer()

				if b.N == 1 && dest == "buffer" && !strings.Contains(c.name, "skip") {
					written := string(buf.Bytes())
					total := len(written)
					if total != expectTotal {
						b.Logf("expect='%s' buf='%s'", hexSpecial(result), hexSpecial(written))
						b.Errorf("len(buf) expected=%d actual=%d", expectTotal, total)
					}
				}
			})
		}
	}
}
