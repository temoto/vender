// Package log2 solves these issues:
// - log level filtering, e.g. show debug messages in internal tests only
// - safe concurrent change of log level
//
// Primary goal was to run parallel tests and log into t.Logf() safely,
// and TBH, would have been enough to pass around explicit stdlib *log.Logger.
// Well, log levels is just a cherry on top.
package log2

import (
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"math"
	"os"
	"sync/atomic"
	"testing"

	"github.com/juju/errors"
)

const ContextKey = "run/log"

const (
	// type specified here helped against accidentally passing flags as level
	Lmicroseconds     int = log.Lmicroseconds
	Lshortfile        int = log.Lshortfile
	LStdFlags         int = log.Ltime | Lshortfile
	LInteractiveFlags int = log.Ltime | Lshortfile | Lmicroseconds
	LServiceFlags     int = Lshortfile
	LTestFlags        int = Lshortfile | Lmicroseconds
)

func ContextValueLogger(ctx context.Context) *Log {
	const key = ContextKey
	v := ctx.Value(key)
	if v == nil {
		// return nil
		panic(fmt.Errorf("context['%v'] is nil", key))
	}
	if log, ok := v.(*Log); ok {
		return log
	}
	panic(fmt.Errorf("context['%v'] expected type *Log", key))
}

type Level int32

const (
	LError = iota
	LInfo
	LDebug
	LAll = math.MaxInt32
)

type Log struct {
	l       *log.Logger
	level   Level
	onError atomic.Value // <ErrorHandler>
	fatalf  FmtFunc
}

func NewStderr(level Level) *Log { return NewWriter(os.Stderr, level) }
func NewWriter(w io.Writer, level Level) *Log {
	if w == ioutil.Discard {
		return nil
	}
	return &Log{
		l:     log.New(w, "", LStdFlags),
		level: level,
	}
}

type ErrorFunc func(error)
type FmtFunc func(format string, args ...interface{})
type FmtFuncWriter struct{ FmtFunc }

func NewFunc(f FmtFunc, level Level) *Log { return NewWriter(FmtFuncWriter{f}, level) }
func (self FmtFuncWriter) Write(b []byte) (int, error) {
	self.FmtFunc(string(b))
	return len(b), nil
}

func NewTest(t testing.TB, level Level) *Log {
	self := NewFunc(t.Logf, level)
	self.fatalf = t.Fatalf
	return self
}

func (self *Log) Clone(level Level) *Log {
	if self == nil {
		return nil
	}
	new := NewWriter(self.l.Writer(), level)
	new.fatalf = self.fatalf
	new.storeErrorFunc(self.loadErrorFunc())
	new.SetFlags(self.l.Flags())
	return new
}

func (self *Log) SetErrorFunc(f ErrorFunc) {
	if self == nil {
		return
	}
	self.storeErrorFunc(f)
}

func (self *Log) SetLevel(l Level) {
	if self == nil {
		return
	}
	atomic.StoreInt32((*int32)(&self.level), int32(l))
}

func (self *Log) SetFlags(f int) {
	if self == nil {
		return
	}
	self.l.SetFlags(f)
}

func (self *Log) Stdlib() *log.Logger {
	if self == nil {
		return nil
	}
	return self.l
}

func (self *Log) SetOutput(w io.Writer) {
	if self == nil {
		return
	}
	self.l.SetOutput(w)
}

func (self *Log) SetPrefix(prefix string) {
	if self == nil {
		return
	}
	self.l.SetPrefix(prefix)
}

func (self *Log) Enabled(level Level) bool {
	if self == nil {
		return false
	}
	return atomic.LoadInt32((*int32)(&self.level)) >= int32(level)
}

func (self *Log) Log(level Level, s string) {
	if self.Enabled(level) {
		_ = self.l.Output(3, s)
	}
}
func (self *Log) Logf(level Level, format string, args ...interface{}) {
	if self.Enabled(level) {
		_ = self.l.Output(3, fmt.Sprintf(format, args...))
	}
}

// compatibility with eclipse.paho.mqtt
func (self *Log) Printf(format string, args ...interface{}) { self.Logf(LInfo, format, args...) }
func (self *Log) Println(args ...interface{})               { self.Log(LInfo, fmt.Sprint(args...)) }

func (self *Log) Info(args ...interface{}) {
	self.Log(LInfo, fmt.Sprint(args...))
}
func (self *Log) Infof(format string, args ...interface{}) {
	self.Logf(LInfo, format, args...)
}
func (self *Log) Debug(args ...interface{}) {
	self.Log(LDebug, "debug: "+fmt.Sprint(args...))
}
func (self *Log) Debugf(format string, args ...interface{}) {
	self.Logf(LDebug, "debug: "+format, args...)
}

func (self *Log) Error(args ...interface{}) {
	self.Log(LError, "error: "+fmt.Sprint(args...))
	if self == nil {
		return
	}
	if errfun := self.loadErrorFunc(); errfun != nil {
		var e error
		if len(args) >= 1 {
			e, _ = args[0].(error)
		}
		if e != nil {
			args = args[1:]
			if len(args) > 0 { // Log.Error(err, arg1) please don't do this
				rest := fmt.Sprint(args...)
				e = errors.Annotate(e, rest)
			}
			errfun(e)
		}
	}
}
func (self *Log) Errorf(format string, args ...interface{}) {
	self.Logf(LError, "error: "+format, args...)
	if self == nil {
		return
	}
	if errfun := self.loadErrorFunc(); errfun != nil {
		e := fmt.Errorf(fmt.Sprintf(format, args...))
		errfun(e)
	}
}

func (self *Log) Fatalf(format string, args ...interface{}) {
	if self.fatalf != nil {
		self.fatalf(format, args...)
	} else {
		self.Logf(LError, "fatal: "+format, args...)
		os.Exit(1)
	}
}
func (self *Log) Fatal(args ...interface{}) {
	s := fmt.Sprint(args...)
	if self.fatalf != nil {
		self.fatalf(s)
	} else {
		self.Logf(LError, "fatal: "+s)
		os.Exit(1)
	}
}

// workaround for atomic.Value with nil
type wrapErrorFunc struct{ ErrorFunc }

func (self *Log) loadErrorFunc() ErrorFunc {
	if x := self.onError.Load(); x != nil {
		return x.(wrapErrorFunc).ErrorFunc
	} else {
		return nil
	}
}

func (self *Log) storeErrorFunc(new ErrorFunc) {
	self.onError.Store(wrapErrorFunc{new})
}
