package tele

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io/ioutil"
	"net/url"
	"time"

	"github.com/256dpi/gomqtt/packet"
	"github.com/juju/errors"
	"github.com/temoto/vender/helpers"
	"github.com/temoto/vender/log2"
	tele_config "github.com/temoto/vender/tele/config"
	"github.com/temoto/vender/tele/mqtt"
)

type transportMqtt struct {
	log    *log2.Log
	m      *mqtt.Client
	stopCh chan struct{}

	topicPrefix    string
	topicState     string
	topicTelemetry string
	topicCommand   string
}

func (self *transportMqtt) Init(ctx context.Context, log *log2.Log, teleConfig tele_config.Config, onCommand CommandCallback) error {
	mqttLog := self.log.Clone(log2.LInfo)
	if teleConfig.MqttLogDebug {
		mqttLog.SetLevel(log2.LDebug)
	}

	mqttClientId := fmt.Sprintf("vm%d", teleConfig.VmId)
	self.topicPrefix = mqttClientId // coincidence
	self.topicState = fmt.Sprintf("%s/w/1s", self.topicPrefix)
	self.topicTelemetry = fmt.Sprintf("%s/w/1t", self.topicPrefix)
	self.topicCommand = fmt.Sprintf("%s/r/c", self.topicPrefix)

	networkTimeout := helpers.IntSecondDefault(teleConfig.NetworkTimeoutSec, DefaultNetworkTimeout)
	if networkTimeout < 1*time.Second {
		networkTimeout = 1 * time.Second
	}

	if _, err := url.ParseRequestURI(teleConfig.MqttBroker); err != nil {
		return errors.Annotatef(err, "tele server=%s", teleConfig.MqttBroker)
	}

	tlsconf := new(tls.Config)
	if teleConfig.TlsCaFile != "" {
		tlsconf.RootCAs = x509.NewCertPool()
		cabytes, err := ioutil.ReadFile(teleConfig.TlsCaFile)
		if err != nil {
			return errors.Annotatef(err, "TLS")
		}
		tlsconf.RootCAs.AppendCertsFromPEM(cabytes)
	}
	if teleConfig.TlsPsk != "" {
		copy(tlsconf.SessionTicketKey[:], helpers.MustHex(teleConfig.TlsPsk))
	}

	self.m = &mqtt.Client{Log: mqttLog}
	self.m.Config.BrokerURL = teleConfig.MqttBroker
	self.m.Config.TLS = tlsconf
	self.m.Config.KeepaliveSec = uint16(teleConfig.KeepaliveSec)
	self.m.Config.NetworkTimeout = networkTimeout
	self.m.Config.ClientID = mqttClientId
	self.m.Config.Username = mqttClientId
	self.m.Config.Password = teleConfig.MqttPassword
	self.m.Config.ReconnectDelay = networkTimeout * 2
	self.m.Config.OnMessage = func(msg *packet.Message) error {
		if msg.Topic != self.topicCommand {
			self.log.Errorf("tele: MQTT received message in unexpected topic=%s payload=%x", msg.Topic, msg.Payload)
			return nil
		}
		// TODO save command to spq, return nil, then try to execute
		_ = onCommand(ctx, msg.Payload)
		return nil
	}

	err := self.m.Init(nil)
	return errors.Annotatef(err, "tele init")
}

func (self *transportMqtt) Close() {
	close(self.stopCh)
	self.m.Close()
}

func (self *transportMqtt) SendState(payload []byte) bool {
	self.log.Infof("transport sendstate payload=%x", payload)
	msg := &packet.Message{Topic: self.topicState, Payload: payload, QOS: packet.QOSAtLeastOnce, Retain: true}
	err := self.m.Publish(context.Background(), msg)
	if err != nil {
		self.log.Errorf("tele: MQTT %s", err.Error())
	}
	return err == nil
}

func (self *transportMqtt) SendTelemetry(payload []byte) bool {
	msg := &packet.Message{Topic: self.topicTelemetry, Payload: payload, QOS: packet.QOSAtLeastOnce, Retain: true}
	err := self.m.Publish(context.Background(), msg)
	if err != nil {
		self.log.Errorf("tele: MQTT %s", err.Error())
	}
	return err == nil
}

func (self *transportMqtt) SendCommandResponse(topicSuffix string, payload []byte) bool {
	topic := fmt.Sprintf("%s/%s", self.topicPrefix, topicSuffix)
	self.log.Debugf("mqtt publish command response to topic=%s", topic)
	msg := &packet.Message{Topic: self.topicTelemetry, Payload: payload, QOS: packet.QOSAtLeastOnce, Retain: false}
	err := self.m.Publish(context.Background(), msg)
	if err != nil {
		self.log.Errorf("tele: MQTT %s", err.Error())
	}
	return err == nil
}
