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
	"github.com/temoto/vender/tele/mqtt"
)

func TopicCommand(vmid int32) string                 { return fmt.Sprintf("vm%d/r/c", vmid) }
func TopicResponse(vmid int32, suffix string) string { return fmt.Sprintf("vm%d/%s", vmid, suffix) }
func TopicState(vmid int32) string                   { return fmt.Sprintf("vm%d/w/1s", vmid) }
func TopicTelemetry(vmid int32) string               { return fmt.Sprintf("vm%d/w/1t", vmid) }

func (self *tele) mqttInit(ctx context.Context, onCommand CommandCallback) error {
	mqttLog := self.log.Clone(log2.LInfo)
	if self.config.MqttLogDebug {
		mqttLog.SetLevel(log2.LDebug)
	}

	mqttClientId := fmt.Sprintf("vm%d", self.config.VmId)
	self.topicState = TopicState(int32(self.config.VmId))
	self.topicTelemetry = TopicTelemetry(int32(self.config.VmId))
	self.topicCommand = TopicCommand(int32(self.config.VmId))

	networkTimeout := helpers.IntSecondDefault(self.config.NetworkTimeoutSec, DefaultNetworkTimeout)
	if networkTimeout < 1*time.Second {
		networkTimeout = 1 * time.Second
	}

	if _, err := url.ParseRequestURI(self.config.MqttBroker); err != nil {
		return errors.Annotatef(err, "tele server=%s", self.config.MqttBroker)
	}

	tlsconf := new(tls.Config)
	if self.config.TlsCaFile != "" {
		tlsconf.RootCAs = x509.NewCertPool()
		cabytes, err := ioutil.ReadFile(self.config.TlsCaFile)
		if err != nil {
			return errors.Annotatef(err, "TLS")
		}
		tlsconf.RootCAs.AppendCertsFromPEM(cabytes)
	}
	if self.config.TlsPsk != "" {
		copy(tlsconf.SessionTicketKey[:], helpers.MustHex(self.config.TlsPsk))
	}

	onMessage := func(msg *packet.Message) error {
		if msg.Topic != self.topicCommand {
			self.log.Errorf("tele: MQTT received message in unexpected topic=%s payload=%x", msg.Topic, msg.Payload)
			return nil
		}
		// TODO save command to spq, return nil, then try to execute
		_ = onCommand(ctx, msg.Payload)
		return nil
	}
	var err error
	self.mqtt, err = mqtt.NewClient(mqtt.ClientOptions{
		Log:            mqttLog,
		BrokerURL:      self.config.MqttBroker,
		TLS:            tlsconf,
		KeepaliveSec:   uint16(self.config.KeepaliveSec),
		NetworkTimeout: networkTimeout,
		ClientID:       mqttClientId,
		Username:       mqttClientId,
		Password:       self.config.MqttPassword,
		ReconnectDelay: networkTimeout / 2,
		OnMessage:      onMessage,
	})
	return errors.Annotatef(err, "tele init")
}

func (self *tele) sendState(payload []byte) bool {
	self.log.Infof("transport sendstate payload=%x", payload)
	msg := &packet.Message{Topic: self.topicState, Payload: payload, QOS: packet.QOSAtLeastOnce, Retain: true}
	err := self.mqtt.Publish(context.Background(), msg)
	if err != nil {
		err = errors.Annotate(err, "tele: sendState mqtt.Publish")
		self.log.Error(err)
	}
	return err == nil
}

func (self *tele) sendTelemetry(payload []byte) bool {
	err := self.mqtt.WaitReady(context.Background())
	if err == nil {
		msg := &packet.Message{Topic: self.topicTelemetry, Payload: payload, QOS: packet.QOSAtLeastOnce, Retain: true}
		if err = self.mqtt.Publish(context.Background(), msg); err != nil {
			err = errors.Annotate(err, "tele: sendTelemetry mqtt.Publish")
			self.log.Error(err)
		}
	}
	return err == nil
}

func (self *tele) sendCommandResponse(topicSuffix string, payload []byte) bool {
	err := self.mqtt.WaitReady(context.Background())
	if err == nil {
		topic := TopicResponse(self.vmId, topicSuffix)
		self.log.Debugf("mqtt publish command response to topic=%s", topic)
		msg := &packet.Message{Topic: topic, Payload: payload, QOS: packet.QOSAtLeastOnce, Retain: false}
		if err = self.mqtt.Publish(context.Background(), msg); err != nil {
			err = errors.Annotate(err, "tele: sendCommandResponse mqtt.Publish")
			self.log.Error(err)
		}
	}
	return err == nil
}
