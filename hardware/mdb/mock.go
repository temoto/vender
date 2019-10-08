// Public API to easy create MDB stubs for test code.

package mdb

import (
	"bytes"
	"context"
	"encoding/hex"
	"fmt"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/juju/errors"
	"github.com/temoto/vender/log2"
)

const MockTimeout = 5 * time.Second

type MockR [2]string

func (self MockR) String() string {
	return fmt.Sprintf("expect=%s response=%s", self[0], self[1])
}

type MockUart struct {
	t  testing.TB
	mu sync.Mutex
	m  map[string]string
	q  chan MockR
}

func NewMockUart(t testing.TB) *MockUart {
	self := &MockUart{
		t: t,
		q: make(chan MockR),
	}
	return self
}

func (self *MockUart) Open(path string) error { return nil }
func (self *MockUart) Close() error {
	self.mu.Lock()
	defer self.mu.Unlock()
	select {
	case _, ok := <-self.q:
		err := errors.Errorf("mdb-mock: Close() with non-empty queue")
		if !ok {
			err = errors.Errorf("code error mdb-mock already closed")
		}
		// panic(err)
		// self.t.Log(err)
		self.t.Fatal(err)
		return err
	default:
		close(self.q)
		return nil
	}
}

func (self *MockUart) Break(d, sleep time.Duration) error {
	runtime.Gosched()
	return nil
}

func (self *MockUart) Tx(request, response []byte) (n int, err error) {
	self.t.Helper()
	self.mu.Lock()
	defer self.mu.Unlock()
	if self.m != nil {
		if len(self.m) == 0 {
			err = errors.Errorf("mdb-mock: map ended, received=%x", request)
			self.t.Error(err)
			return 0, err
		}
		return self.txMap(request, response)
	}
	return self.txQueue(request, response)
}

// ExpectMap() in random order
func (self *MockUart) txMap(request, response []byte) (int, error) {
	requestHex := hex.EncodeToString(request)
	responseHex, found := self.m[requestHex]
	if !found {
		// must not call self.t.Error() here
		return 0, ErrTimeout
	}
	delete(self.m, requestHex)
	rp := MustPacketFromHex(responseHex, true)
	n := copy(response, rp.Bytes())
	return n, nil
}

// Expect() requests in defined order
func (self *MockUart) txQueue(request, response []byte) (n int, err error) {
	var rr MockR
	var ok bool
	select {
	case rr, ok = <-self.q:
		if !ok {
			err = errors.Errorf("mdb-mock: queue ended, received=%x", request)
			self.t.Error(err)
			return 0, err
		}
	case <-time.After(MockTimeout):
		err = errors.Errorf("mdb-mock: queue timeout, received=%x", request)
		self.t.Error(err)
		return 0, err
	}
	expect := MustPacketFromHex(rr[0], true)

	if !bytes.Equal(request, expect.Bytes()) {
		err = errors.Errorf("mdb-mock: request expected=%x actual=%x", expect.Bytes(), request)
		self.t.Error(err)
		return 0, err
	}

	// TODO support testing errors
	// if rr.Rerr != nil {
	// 	self.t.Logf("mdb-mock: Tx returns error=%v", rr.Rerr)
	// 	return 0, rr.Rerr
	// }

	rp := MustPacketFromHex(rr[1], true)
	n = copy(response, rp.Bytes())
	return n, err
}

// usage:
// m, mock:= NewTestMdber(t)
// defer mock.Close()
// go use_mdb(m)
// mock.Expect(...)
// go use_mdb(m)
// mock.Expect(...)
// wait use_mdb() to finish to catch all possible errors
func (self *MockUart) Expect(rrs []MockR) {
	self.t.Helper()

	for _, rr := range rrs {
		select {
		case self.q <- rr:
		case <-time.After(MockTimeout):
			err := errors.Errorf("mdb-mock: background processing is too slow, timeout sending into mock queue rr=%s", rr)
			self.t.Fatal(err)
		}
	}
}

func (self *MockUart) ExpectMap(rrs map[string]string) {
	self.t.Helper()
	self.mu.Lock()
	if rrs == nil {
		self.m = nil
	} else {
		self.m = make(map[string]string)
		for k := range rrs {
			self.m[k] = rrs[k]
		}
	}
	self.mu.Unlock()
}

func NewMockBus(t testing.TB) (*Bus, *MockUart) {
	mock := NewMockUart(t)
	b := NewBus(mock, log2.NewTest(t, log2.LDebug), func(e error) {
		t.Logf("bus.Error: %v", e)
	})
	return b, mock
}

const MockContextKey = "test/mdb-mock"

// sorry for this ugly convolution
// working around import cycle on a time budget
func MockFromContext(ctx context.Context) *MockUart {
	return ctx.Value(MockContextKey).(*MockUart)
}
