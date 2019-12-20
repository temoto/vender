package tele_test

import (
	"context"
	"testing"
	"time"

	"github.com/temoto/vender/head/tele"
	tele_config "github.com/temoto/vender/head/tele/config"
	"github.com/temoto/vender/log2"
)

type transportMock struct {
	t              testing.TB
	onCommand      tele.CommandCallback
	networkTimeout time.Duration
	outBuffer      int
	outTelemetry   chan []byte
	outState       chan []byte
	outResponse    chan []byte
}

func (self *transportMock) Init(ctx context.Context, log *log2.Log, teleConfig tele_config.Config, onCommand tele.CommandCallback, willPayload []byte) error {
	self.onCommand = onCommand
	if self.networkTimeout == 0 {
		self.networkTimeout = tele.DefaultNetworkTimeout
	}
	self.outTelemetry = make(chan []byte, self.outBuffer)
	self.outState = make(chan []byte, self.outBuffer)
	self.outResponse = make(chan []byte, self.outBuffer)
	return nil
}

func (self *transportMock) SendTelemetry(payload []byte) bool {
	select {
	case self.outTelemetry <- copyBytes(payload):
		self.t.Logf("mock delivered telemetry=%x", payload)
	case <-time.After(self.networkTimeout):
		self.t.Logf("mock network timeout")
		return false
	}
	return true
}

func (self *transportMock) SendState(payload []byte) bool {
	select {
	case self.outState <- copyBytes(payload):
		self.t.Logf("mock delivered state=%x", payload)
	case <-time.After(self.networkTimeout):
		self.t.Logf("mock network timeout")
		return false
	}
	return true
}

func (self *transportMock) SendCommandResponse(topicSuffix string, payload []byte) bool {
	select {
	case self.outResponse <- copyBytes(payload):
		self.t.Logf("mock delivered topic=%s response=%x", topicSuffix, payload)
	case <-time.After(self.networkTimeout):
		self.t.Logf("mock network timeout")
		return false
	}
	return true
}

// split send/receive buffer identity for safe concurrent access
func copyBytes(b []byte) []byte {
	new := make([]byte, len(b))
	copy(new, b)
	return new
}
