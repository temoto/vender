package mega

import (
	"fmt"
	"math/rand"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/temoto/errors"
	gpio "github.com/temoto/gpio-cdev-go"
	gpio_mock "github.com/temoto/gpio-cdev-go/mock"
	"github.com/temoto/vender/helpers"
	"github.com/temoto/vender/log2"
)

// Helpers for testing mega package

type tenv struct {
	t          testing.TB
	rand       *rand.Rand
	log        *log2.Log
	config     Config
	notifyMock *gpio_mock.MockEvent
	spiMock    *spiMock
}

func testEnv(t *testing.T) *tenv {
	notifyMock := &gpio_mock.MockEvent{}
	notifyMock.On("Close").Return(nil)
	notifyMock.On("Read").Return(byte(0), nil)
	notifyMock.On("Wait", mock.AnythingOfType("time.Duration")).Return(gpio.EventData{}, errors.Timeoutf("")).After(100 * time.Millisecond)

	env := &tenv{
		t:          t,
		rand:       helpers.RandUnix(),
		log:        log2.NewTest(t, log2.LDebug),
		notifyMock: notifyMock,
		spiMock:    newSpiMock(t),
	}
	env.log = log2.NewStderr(log2.LDebug) // helps with panics

	env.config = Config{
		SpiBus:        testDevice,
		NotifyPinChip: testDevice,
		NotifyPinName: "1",
		testhw:        &hardware{notifier: notifyMock, spiTx: env.spiMock.Tx},
	}
	return env
}

type spiTxCall struct {
	s []byte
	r []byte
	e error
}
type spiMock struct {
	assert  *assert.Assertions
	t       testing.TB
	mu      sync.Mutex
	expects []spiTxCall
	index   int
}

func newSpiMock(t testing.TB) *spiMock {
	return &spiMock{
		expects: make([]spiTxCall, 0, 64),
		t:       t,
		assert:  assert.New(t),
	}
}

func (m *spiMock) Tx(send, recv []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.index >= len(m.expects) {
		msg := "premature end of spiMock.expects"
		m.t.Error(msg)
		panic(msg)
	}
	call := m.expects[m.index]
	m.assert.Equal(call.s, send)
	copy(recv, call.r)
	m.index++
	return call.e
}

func (m *spiMock) PushOk(sendHex, recvHex string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.expects = append(m.expects, spiTxCall{s: helpers.MustHex(sendHex), r: helpers.MustHex(recvHex)})
}

func mockGet(m *mock.Mock, method string) *mock.Call {
	for _, c := range m.ExpectedCalls {
		if c.Method == method {
			return c
		}
	}
	panic(fmt.Sprintf("call not found mock=%v method=%s", m, method))
}
