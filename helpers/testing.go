package helpers

import (
	"io/ioutil"
	"log"
	"math/rand"
	"testing"
	"time"
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

