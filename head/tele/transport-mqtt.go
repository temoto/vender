package tele

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io/ioutil"
	"time"

	// TODO try github.com/goiiot/libmqtt
	// TODO try github.com/256dpi/gomqtt

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/temoto/errors"
	tele_config "github.com/temoto/vender/head/tele/config"
	"github.com/temoto/vender/helpers"
	"github.com/temoto/vender/log2"
)

type transportMqtt struct {
	log       *log2.Log
	onCommand func([]byte) bool
	m         mqtt.Client
	mopt      *mqtt.ClientOptions
	stopCh    chan struct{}

	topicPrefix    string
	topicState     string
	topicTelemetry string
	topicCommand   string
}

func (self *transportMqtt) Init(ctx context.Context, log *log2.Log, teleConfig tele_config.Config, onCommand func([]byte) bool, willPayload []byte) error {
	mqttLog := self.log.Clone(log2.LDebug)
	// TODO wrap with level filter and prefix "tele.mqtt critical/error/warn/debug"
	mqtt.CRITICAL = mqttLog
	mqtt.ERROR = mqttLog
	mqtt.WARN = mqttLog
	if teleConfig.MqttLogDebug {
		mqtt.DEBUG = mqttLog
	}

	mqttClientId := fmt.Sprintf("vm%d", teleConfig.VmId)
	credFun := func() (string, string) {
		return mqttClientId, teleConfig.MqttPassword
	}

	self.topicPrefix = mqttClientId // coincidence
	self.topicState = fmt.Sprintf("%s/w/1s", self.topicPrefix)
	self.topicTelemetry = fmt.Sprintf("%s/w/1t", self.topicPrefix)
	self.topicCommand = fmt.Sprintf("%s/r/c", self.topicPrefix)

	networkTimeout := helpers.IntSecondDefault(teleConfig.NetworkTimeoutSec, defaultNetworkTimeout)
	if networkTimeout < 1*time.Second {
		networkTimeout = 1 * time.Second
	}
	connectTimeout := networkTimeout * 3
	keepaliveTimeout := helpers.IntSecondDefault(teleConfig.KeepaliveSec, networkTimeout/2)

	defaultHandler := func(_ mqtt.Client, msg mqtt.Message) {
		self.log.Errorf("unexpected mqtt message: %v", msg)
	}

	tlsconf := new(tls.Config)
	if teleConfig.TlsCaFile != "" {
		tlsconf.RootCAs = x509.NewCertPool()
		cabytes, err := ioutil.ReadFile(teleConfig.TlsCaFile)
		if err != nil {
			panic(err)
		}
		tlsconf.RootCAs.AppendCertsFromPEM(cabytes)
	}
	if teleConfig.TlsPsk != "" {
		copy(tlsconf.SessionTicketKey[:], helpers.MustHex(teleConfig.TlsPsk))
	}
	self.mopt = mqtt.NewClientOptions().
		AddBroker(teleConfig.MqttBroker).
		SetAutoReconnect(true).
		SetBinaryWill(self.topicState, willPayload, 1, true).
		SetCleanSession(false).
		SetClientID(mqttClientId).
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
	self.m = mqtt.NewClient(self.mopt)

	go self.online()
	return nil
}

func (self *transportMqtt) Close() {
	close(self.stopCh)
	for self.m.IsConnected() {
		time.Sleep(1 * time.Second)
	}
}

func (self *transportMqtt) SendState(payload []byte) bool {
	self.log.Infof("transport sendstate payload=%x", payload)
	t := self.m.Publish(self.topicState, 1, true, payload)
	err := self.tokenWait(t, "publish state")
	self.log.Infof("transport sendstate err=%v", err)
	return err == nil
}

func (self *transportMqtt) SendTelemetry(payload []byte) bool {
	t := self.m.Publish(self.topicTelemetry, 1, true, payload)
	err := self.tokenWait(t, "publish telemetry")
	return err == nil
}

func (self *transportMqtt) SendCommandResponse(topicSuffix string, payload []byte) bool {
	topic := fmt.Sprintf("%s/%s", self.topicPrefix, topicSuffix)
	t := self.m.Publish(topic, 1, false, payload)
	err := self.tokenWait(t, "publish command response")
	return err == nil
}

func (self *transportMqtt) online() {
	if self.m.IsConnected() {
		return
	}

	for self.isRunning() {
		self.log.Debugf("tele connect before")
		t := self.m.Connect()
		if self.tokenWait(t, "connect") == nil {
			break // success path
		}
		self.log.Debugf("tele connect after")
		time.Sleep(1 * time.Second)
	}

	for self.isRunning() {
		self.log.Debugf("tele sub-command before")
		t := self.m.Subscribe(self.topicCommand, 1, self.mqttSubCommand)
		if self.tokenWait(t, "subscribe:"+self.topicCommand) == nil {
			break // success path
		}
		self.log.Debugf("tele sub-command after")
		time.Sleep(1 * time.Second)
	}
}

func (self *transportMqtt) isRunning() bool {
	select {
	case <-self.stopCh:
		self.m.Disconnect(uint(self.mopt.PingTimeout / time.Millisecond))
		return false
	default:
		return true
	}
}

func (self *transportMqtt) mqttSubCommand(_ mqtt.Client, msg mqtt.Message) {
	payload := msg.Payload()
	if self.onCommand(payload) {
		msg.Ack()
	}
}

func (self *transportMqtt) tokenWait(t mqtt.Token, tag string) error {
	if !t.Wait() {
		err := errors.Errorf("%s timeout", tag)
		self.log.Errorf("tele: MQTT %s", err.Error())
		return err
	}
	if err := t.Error(); err != nil {
		err = errors.Annotate(err, tag)
		self.log.Errorf("tele: MQTT %s", err.Error())
		return err
	}
	return nil
}
