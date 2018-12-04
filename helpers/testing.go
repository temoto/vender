package helpers

import (
	"io/ioutil"
	"log"
	"math/rand"
	"testing"
	"time"

	"github.com/juju/errors"
)

type FatalFunc func(...interface{})

type Fataler interface {
	Fatal(...interface{})
}

// Compatible: log.Printf, testing.TB.Logf, Discardf
type LogFunc func(format string, args ...interface{})

func Discardf(format string, args ...interface{}) {}

func LogDiscard() {
	log.SetOutput(ioutil.Discard)
}

type TestLogWriter struct{ testing.TB }

func (self TestLogWriter) Write(p []byte) (int, error) {
	self.Helper()
	if len(p) == 0 {
		return 0, nil
	}
	self.Logf(string(p))
	return len(p), nil
}

func LogToTest(t testing.TB) {
	log.SetFlags(log.Ltime | log.Lmicroseconds | log.Lshortfile)
	log.SetOutput(TestLogWriter{t})
}

func RandUnix() *rand.Rand {
	return rand.New(rand.NewSource(time.Now().UnixNano()))
}

func AssertEqual(t testing.TB, a, b interface{}) {
	t.Helper()
	switch a.(type) {
	case bool:
		if a.(bool) == b.(bool) {
			return
		}
	case int:
		if a.(int) == b.(int) {
			return
		}
	case uint:
		if a.(uint) == b.(uint) {
			return
		}
	case int32:
		if a.(int32) == b.(int32) {
			return
		}
	case uint32:
		if a.(uint32) == b.(uint32) {
			return
		}
	case string:
		if a.(string) == b.(string) {
			return
		}
	default:
		panic(errors.Errorf("code error AssertEqual does not support type of %#v", a))
	}
	t.Fatalf("assert equal fail\na=%#v\nb=%#v", a, b)
}
