package tele

import (
	"context"
	"fmt"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/temoto/vender/helpers"
	"github.com/temoto/vender/log2"
	tele_config "github.com/temoto/vender/tele/config"
)

type transportMqtt struct {
	log       *log2.Log
	onCommand func([]byte) bool
	m         mqtt.Client
	mopt      *mqtt.ClientOptions
	// stopCh    chan struct{}

	topicPrefix    string
	topicConnect   string
	topicState     string
	topicTelemetry string
	topicCommand   string
}

func (self *transportMqtt) Init(ctx context.Context, log *log2.Log, teleConfig tele_config.Config, onCommand CommandCallback) error {
	self.log = log
	// FIXME add loglevel to config
	mqtt.ERROR = log
	mqtt.CRITICAL = log
	mqtt.WARN = log
	//	mqtt.DEBUG = log

	mqttClientId := fmt.Sprintf("vm%d", teleConfig.VmId)
	credFun := func() (string, string) {
		return mqttClientId, teleConfig.MqttPassword
	}

	self.onCommand = func(payload []byte) bool {
		return onCommand(ctx, payload)
	}
	self.topicPrefix = mqttClientId // coincidence
	self.topicConnect = fmt.Sprintf("%s/c", self.topicPrefix)
	self.topicState = fmt.Sprintf("%s/w/1s", self.topicPrefix)
	self.topicTelemetry = fmt.Sprintf("%s/w/1t", self.topicPrefix)
	self.topicCommand = fmt.Sprintf("%s/r/c", self.topicPrefix)
	keepAlive := helpers.IntSecondConfigDefault(teleConfig.KeepaliveSec, 60)
	pingTimeout := helpers.IntSecondConfigDefault(teleConfig.PingTimeoutSec, 30)
	retryInterval := helpers.IntSecondConfigDefault(teleConfig.KeepaliveSec/2, 30)
	storePath := teleConfig.StorePath
	if teleConfig.StorePath == "" {
		storePath = "/home/vmc/telemessages"
	}
	// tlsconf := new(tls.Config)
	// if teleConfig.TlsCaFile != "" {
	// 	tlsconf.RootCAs = x509.NewCertPool()
	// 	cabytes, err := ioutil.ReadFile(teleConfig.TlsCaFile)
	// 	if err != nil {
	// 		self.log.Errorf("tls not possible. certivicate file - not found")
	// 	}
	// 	tlsconf.RootCAs.AppendCertsFromPEM(cabytes)
	// }
	// if teleConfig.TlsPsk != "" {
	// 	copy(tlsconf.SessionTicketKey[:], helpers.MustHex(teleConfig.TlsPsk))
	// }
	self.mopt = mqtt.NewClientOptions().
		AddBroker(teleConfig.MqttBroker).
		SetBinaryWill(self.topicConnect, []byte{0x00}, 1, true).
		SetCleanSession(false).
		SetClientID(mqttClientId).
		SetCredentialsProvider(credFun).
		SetDefaultPublishHandler(self.messageHandler).
		SetKeepAlive(keepAlive).
		SetPingTimeout(pingTimeout).
		SetOrderMatters(false).
		// SetTLSConfig(tlsconf).
		SetResumeSubs(true).SetCleanSession(false).
		SetStore(mqtt.NewFileStore(storePath)).
		SetConnectRetryInterval(retryInterval).
		SetOnConnectHandler(self.onConnectHandler).
		SetConnectionLostHandler(self.connectLostHandler).
		SetConnectRetry(true)
	self.m = mqtt.NewClient(self.mopt)
	sConnToken := self.m.Connect()
	// if sConnToken.Wait() && sConnToken.Error() != nil {
	if sConnToken.Error() != nil {
		self.log.Errorf("token.Error\n")
	}
	return nil
}

func (self *transportMqtt) CloseTele() {
	self.log.Infof("mqtt unsubscribe")
	if token := self.m.Unsubscribe(self.topicCommand); token.Wait() && token.Error() != nil {
		self.log.Infof("mqtt unsubscribe error")
		// fmt.Println(token.Error())
		// os.Exit(1)
	}
}

func (self *transportMqtt) SendState(payload []byte) bool {
	self.log.Infof("transport sendstate payload=%x", payload)
	self.m.Publish(self.topicState, 1, false, payload)
	return true
}

func (self *transportMqtt) SendTelemetry(payload []byte) bool {
	self.m.Publish(self.topicTelemetry, 1, false, payload)
	return true
}

func (self *transportMqtt) SendCommandResponse(topicSuffix string, payload []byte) bool {
	topic := fmt.Sprintf("%s/%s", self.topicPrefix, topicSuffix)
	self.log.Infof("mqtt publish command response to topic=%s", topic)
	self.m.Publish(topic, 1, false, payload)
	return true
}

func (self *transportMqtt) messageHandler(c mqtt.Client, msg mqtt.Message) {
	payload := msg.Payload()
	self.log.Infof("mqtt income message (%x)", payload)
	self.onCommand(payload)
}

func (self *transportMqtt) connectLostHandler(c mqtt.Client, err error) {
	self.log.Infof("mqtt disconnect")
}

func (self *transportMqtt) onConnectHandler(c mqtt.Client) {
	self.log.Infof("mqtt connect")
	if token := c.Subscribe(self.topicCommand, 1, nil); token.Wait() && token.Error() != nil {
		self.log.Infof("mqtt subscribe error")
	} else {
		self.log.Infof("mqtt subscribe Ok")
		c.Publish(self.topicConnect, 1, true, []byte{0x01})
	}
}
