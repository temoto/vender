package tele

import (
	"context"
	"fmt"
	"testing"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/juju/errors"
)

type MqttMock struct {
	Opt  *mqtt.ClientOptions
	Pub  chan MockMsg
	subs []MockSub
}
type MockSub struct {
	Pattern string
	Qos     byte
	Handler mqtt.MessageHandler
}

func NewMqttMock() *MqttMock {
	return &MqttMock{
		Pub:  make(chan MockMsg, 32),
		subs: make([]MockSub, 0, 16),
	}
}

func (self *MqttMock) MockNew(opt *mqtt.ClientOptions) {
	self.Opt = opt
}

func (self *MqttMock) TestPublish(t testing.TB, topic string, payload []byte) {
	for _, sub := range self.subs {
		// TODO pattern-match
		if topic == sub.Pattern {
			msg := MockMsg{T: topic, P: payload}
			if sub.Qos > 0 {
				msg.acked = make(chan struct{})
			}
			sub.Handler(self, msg)
			if sub.Qos > 0 {
				select {
				case <-msg.acked:
				default:
					t.Errorf("message='%s' handled without Ack()", string(payload))
					return
				}
			}
			return
		}
	}
	t.Errorf("not subscribed for topic=%s", topic)
}

func (self *MqttMock) Disconnect(uint)        {}
func (self *MqttMock) IsConnected() bool      { return true }
func (self *MqttMock) IsConnectionOpen() bool { return true }

func (self *MqttMock) Connect() mqtt.Token { return mockToken{nil} }

func (self *MqttMock) Publish(topic string, qos byte, retain bool, payload interface{}) mqtt.Token {
	return mockToken{nil}
}

func (self *MqttMock) Subscribe(pattern string, qos byte, handler mqtt.MessageHandler) mqtt.Token {
	self.subs = append(self.subs, MockSub{pattern, qos, handler})
	return mockToken{nil}
}

func (self *MqttMock) AddRoute(string, mqtt.MessageHandler) { panic("not implemented") }

func (self *MqttMock) OptionsReader() mqtt.ClientOptionsReader {
	panic("not implemented")
	// return mqtt.ClientOptionsReader{}
}

func (self *MqttMock) SubscribeMultiple(map[string]byte, mqtt.MessageHandler) mqtt.Token {
	panic("not implemented")
}
func (self *MqttMock) Unsubscribe(...string) mqtt.Token { panic("not implemented") }

type mockToken struct{ error }

func (tok mockToken) Error() error { return tok.error }
func (tok mockToken) Wait() bool   { return !errors.IsTimeout(tok.error) }
func (tok mockToken) WaitTimeout(time.Duration) bool {
	panic("not implemented")
	// return !errors.IsTimeout(tok.error)
}

type MockMsg struct {
	T     string
	P     []byte
	acked chan struct{}
}

func (msg MockMsg) Ack() {
	if msg.acked != nil {
		close(msg.acked)
	}
}

func (msg MockMsg) Duplicate() bool   { return false }
func (msg MockMsg) MessageID() uint16 { return 0 }
func (msg MockMsg) Payload() []byte   { return msg.P }
func (msg MockMsg) Qos() byte         { return 0 }
func (msg MockMsg) Retained() bool    { return false }
func (msg MockMsg) Topic() string     { return msg.T }

const mqttMockContextKey = "tele/mqtt-mock"

func ContextWithMqttMock(ctx context.Context, c mqtt.Client) context.Context {
	return context.WithValue(ctx, mqttMockContextKey, c)
}
func GetMqttMock(ctx context.Context) *MqttMock {
	const key = mqttMockContextKey
	v := ctx.Value(key)
	if v == nil {
		panic(fmt.Sprintf("context['%s'] is nil", key))
	}
	if x, ok := v.(*MqttMock); ok {
		return x
	}
	panic(fmt.Sprintf("context['%s'] unexpected=%#v", key, v))
}
