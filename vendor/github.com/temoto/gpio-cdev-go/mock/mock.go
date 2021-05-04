// Use to stub gpio in your tests.
// This package only contains code that could be simply generated from gpio types.
package gpio_mock

import (
	"time"

	"github.com/stretchr/testify/mock"
	"github.com/temoto/gpio-cdev-go"
)

type MockChip struct{ mock.Mock }

func (m *MockChip) Close() error { return m.Called().Error(0) }

func (m *MockChip) Info() gpio.ChipInfo { return m.Called().Get(0).(gpio.ChipInfo) }

func (m *MockChip) LineInfo(line uint32) (gpio.LineInfo, error) {
	returns := m.Called(line)
	return returns.Get(0).(gpio.LineInfo), returns.Error(1)
}

func (m *MockChip) OpenLines(flag gpio.RequestFlag, consumerLabel string, lines ...uint32) (gpio.Lineser, error) {
	args := []interface{}{flag, consumerLabel}
	for _, x := range lines {
		args = append(args, x)
	}
	returns := m.Called(args...)
	return returns.Get(0).(gpio.Lineser), returns.Error(1)
}

func (m *MockChip) GetLineEvent(line uint32, flag gpio.RequestFlag, events gpio.EventFlag, consumerLabel string) (gpio.Eventer, error) {
	returns := m.Called(line, flag, events, consumerLabel)
	return returns.Get(0).(gpio.Eventer), returns.Error(1)
}

type MockLines struct{ mock.Mock }

func (m *MockLines) Close() error { return m.Called().Error(0) }

func (m *MockLines) Flush() error { return m.Called().Error(0) }

func (m *MockLines) LineOffsets() []uint32 { return m.Called().Get(0).([]uint32) }

func (m *MockLines) Read() (gpio.HandleData, error) {
	returns := m.Called()
	return returns.Get(0).(gpio.HandleData), returns.Error(1)
}

func (m *MockLines) SetBulk(bs ...byte) {
	args := make([]interface{}, len(bs))
	for i, x := range bs {
		args[i] = x
	}
	m.Called(args...)
}

func (m *MockLines) SetFunc(line uint32) gpio.LineSetFunc {
	return m.Called(line).Get(0).(gpio.LineSetFunc)
}

type MockEvent struct{ mock.Mock }

func (m *MockEvent) Close() error { return m.Called().Error(0) }

func (m *MockEvent) Read() (byte, error) {
	returns := m.Called()
	return returns.Get(0).(byte), returns.Error(1)
}

func (m *MockEvent) Wait(timeout time.Duration) (gpio.EventData, error) {
	returns := m.Called(timeout)
	return returns.Get(0).(gpio.EventData), returns.Error(1)
}

// compile-time interface check
var _ gpio.Chiper = &MockChip{}
