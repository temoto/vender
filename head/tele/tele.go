package tele

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io/ioutil"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	proto "github.com/golang/protobuf/proto"
	"github.com/juju/errors"
	"github.com/temoto/vender/helpers"
	"github.com/temoto/vender/log2"
)

const (
	defaultConnectTimeout = 10 * time.Minute
	defaultStateInterval  = 5 * time.Minute
)

type Tele struct {
	Log       *log2.Log
	m         mqtt.Client
	mopt      *mqtt.ClientOptions
	stopCh    chan struct{}
	lastState State
	stateCh   chan State
	cmdCh     chan *Command
	tmCh      chan *Telemetry

	stateInterval time.Duration

	topicPrefix    string
	topicState     string
	topicTelemetry string
	topicCommand   string
}

type State byte

const (
	StateInvalid State = iota
	StateBoot
	StateWork
	StateDisconnected
	StateProblem
)

type Config struct {
	ConnectTimeoutSec int    `hcl:"connect_timeout_sec"`
	Enabled           bool   `hcl:"enable"`
	Id                string `hcl:"id"`
	LogDebug          bool   `hcl:"log_debug"`
	MqttBroker        string `hcl:"mqtt_broker"`
	MqttKeepaliveSec  int    `hcl:"keepalive_sec"`
	MqttLogDebug      bool   `hcl:"mqtt_log_debug"`
	MqttPassword      string `hcl:"mqtt_password"` // secret
	StateIntervalSec  int    `hcl:"state_interval_sec"`
	TlsCaFile         string `hcl:"tls_ca_file"`
	TlsPsk            string `hcl:"tls_psk"` // secret
}

func (self *Tele) Init(ctx context.Context, tc Config) {
	// by default disabled
	if !tc.Enabled {
		return
	}

	self.stopCh = make(chan struct{})
	self.stateCh = make(chan State)
	self.cmdCh = make(chan *Command, 2)
	log := log2.ContextValueLogger(ctx)
	self.Log = log.Clone(log2.LInfo)
	if tc.LogDebug {
		self.Log.SetLevel(log2.LDebug)
	}
	mqttLog := self.Log.Clone(log2.LDebug)
	// TODO wrap with level filter and prefix "tele.mqtt critical/error/warn/debug"
	mqtt.CRITICAL = mqttLog
	mqtt.ERROR = mqttLog
	mqtt.WARN = mqttLog
	if tc.MqttLogDebug {
		mqtt.DEBUG = mqttLog
	}

	self.topicPrefix = tc.Id
	self.topicState = fmt.Sprintf("%s/w/s1", self.topicPrefix)
	self.topicTelemetry = fmt.Sprintf("%s/w/t1", self.topicPrefix)
	self.topicCommand = fmt.Sprintf("%s/r/c", self.topicPrefix)

	willPayload := []byte{byte(StateDisconnected)}
	credFun := func() (string, string) { return tc.Id, tc.MqttPassword }
	connectTimeout := intSecondDefault(tc.ConnectTimeoutSec, defaultConnectTimeout)
	keepaliveTimeout := connectTimeout / 2
	networkTimeout := keepaliveTimeout / 4
	if networkTimeout < 1*time.Second {
		networkTimeout = 1 * time.Second
	}
	self.stateInterval = intSecondDefault(tc.StateIntervalSec, defaultStateInterval)

	defaultHandler := func(_ mqtt.Client, msg mqtt.Message) {
		self.Log.Errorf("unexpected mqtt message: %v", msg)
	}

	tlsconf := new(tls.Config)
	if tc.TlsCaFile != "" {
		tlsconf.RootCAs = x509.NewCertPool()
		cabytes, err := ioutil.ReadFile(tc.TlsCaFile)
		if err != nil {
			panic(err)
		}
		tlsconf.RootCAs.AppendCertsFromPEM(cabytes)
	}
	if tc.TlsPsk != "" {
		copy(tlsconf.SessionTicketKey[:], helpers.MustHex(tc.TlsPsk))
	}
	self.mopt = mqtt.NewClientOptions().
		AddBroker(tc.MqttBroker).
		SetAutoReconnect(true).
		SetBinaryWill(self.topicState, willPayload, 1, true).
		SetCleanSession(false).
		SetClientID(tc.Id).
		SetConnectTimeout(connectTimeout).
		SetCredentialsProvider(credFun).
		SetDefaultPublishHandler(defaultHandler).
		SetKeepAlive(keepaliveTimeout).
		SetMaxReconnectInterval(connectTimeout).
		SetMessageChannelDepth(1).
		SetOrderMatters(false).
		SetPingTimeout(networkTimeout).
		SetTLSConfig(tlsconf).
		SetWriteTimeout(networkTimeout)
	if tc.MqttBroker == "mock" {
		mock := GetMqttMock(ctx)
		mock.MockNew(self.mopt)
		self.m = mock
	} else {
		self.m = mqtt.NewClient(self.mopt)
	}

	if t := self.m.Connect(); self.tokenWait(t, "connect") != nil {
		return
	}

	t := self.m.Subscribe(self.topicCommand, 1, func(_ mqtt.Client, msg mqtt.Message) {
		self.Log.Debugf("well command")
		c := new(Command)
		err := proto.Unmarshal(msg.Payload(), c)
		if err != nil {
			self.Log.Errorf("command parse err=%v", err)
			return
		}
		self.cmdCh <- c
		msg.Ack()
	})
	if self.tokenWait(t, "subscribe:"+self.topicCommand) != nil {
		return
	}

	self.lastState = StateBoot
	self.sendState(self.lastState)
	go self.worker()
}

func (self *Tele) Stop() {
	close(self.stopCh)
	for self.m.IsConnected() {
		time.Sleep(1 * time.Second)
	}
}

func (self *Tele) CommandChan() <-chan *Command { return self.cmdCh }
func (self *Tele) CommandReplyErr(c *Command, e error) {
	if c.ReplyTopic == "" {
		self.Log.Errorf("CommandReplyErr with empty reply_topic")
		return
	}
	r := Response{
		CommandId: c.Id,
		Error:     e.Error(),
	}
	b, err := proto.Marshal(&r)
	if err != nil {
		self.Log.Errorf("CommandReplyErr proto.Marshal err=%v")
		return
	}

	topic := fmt.Sprintf("%s/%s", self.topicPrefix, c.ReplyTopic)
	self.m.Publish(topic, 1, false, b)
}

func (self *Tele) isRunning() bool {
	select {
	case <-self.stopCh:
		self.m.Disconnect(uint(self.mopt.PingTimeout / time.Millisecond))
		return false
	default:
		return true
	}
}

func (self *Tele) worker() {
	for self.isRunning() {
		select {
		case self.lastState = <-self.stateCh:
		case tm := <-self.tmCh:
			self.sendTelemetry(tm)
		case <-time.After(self.stateInterval):
		}
		self.sendState(self.lastState)
	}
}

func (self *Tele) sendState(s State) {
	payload := []byte{byte(s)}
	t := self.m.Publish(self.topicState, 1, true, payload)
	self.tokenWait(t, "publish state")
}

func (self *Tele) sendTelemetry(tm *Telemetry) {
	payload, err := proto.Marshal(tm)
	if err != nil {
		self.Log.Errorf("CRITICAL telemetry Marshal tm=%v err=%v", tm, err)
		return
	}
	t := self.m.Publish(self.topicTelemetry, 1, true, payload)
	self.tokenWait(t, "publish telemetry")
}

func (self *Tele) tokenWait(t mqtt.Token, tag string) error {
	if !t.Wait() {
		err := errors.Errorf("%s timeout", tag)
		self.Log.Errorf("tele: MQTT %s", err.Error())
		return err
	}
	if err := t.Error(); err != nil {
		err = errors.Annotate(err, tag)
		self.Log.Errorf("tele: MQTT %s", err.Error())
		return err
	}
	return nil
}

func intSecondDefault(x int, def time.Duration) time.Duration {
	if x == 0 {
		return def
	}
	return time.Duration(x) * time.Second
}
