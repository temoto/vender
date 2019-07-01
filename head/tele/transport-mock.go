package tele

import (
	"context"
	"testing"
	"time"

	tele_config "github.com/temoto/vender/head/tele/config"
	"github.com/temoto/vender/log2"
)

type transportMock struct {
	t              testing.TB
	onCommand      func([]byte) bool
	networkTimeout time.Duration
	outBuffer      int
	outTelemetry   chan []byte
	outState       chan []byte
	outResponse    chan []byte
}

func (self *transportMock) Init(ctx context.Context, log *log2.Log, teleConfig tele_config.Config, onCommand func([]byte) bool, willPayload []byte) error {
	self.onCommand = func(payload []byte) bool {
		self.t.Logf("mock command=%x", payload)
		return onCommand(payload)
	}
	if self.networkTimeout == 0 {
		self.networkTimeout = defaultNetworkTimeout
	}
	self.outTelemetry = make(chan []byte, self.outBuffer)
	self.outState = make(chan []byte, self.outBuffer)
	self.outResponse = make(chan []byte, self.outBuffer)
	return nil
}

func (self *transportMock) SendTelemetry(payload []byte) bool {
	select {
	case self.outTelemetry <- payload:
		self.t.Logf("mock delivered telemetry=%x", payload)
	case <-time.After(self.networkTimeout):
		self.t.Logf("mock network timeout")
		return false
	}
	return true
}

func (self *transportMock) SendState(payload []byte) bool {
	select {
	case self.outState <- payload:
		self.t.Logf("mock delivered state=%x", payload)
	case <-time.After(self.networkTimeout):
		self.t.Logf("mock network timeout")
		return false
	}
	return true
}

func (self *transportMock) SendCommandResponse(topicSuffix string, payload []byte) bool {
	select {
	case self.outResponse <- payload:
		self.t.Logf("mock delivered topic=%s response=%x", topicSuffix, payload)
	case <-time.After(self.networkTimeout):
		self.t.Logf("mock network timeout")
		return false
	}
	return true
}
