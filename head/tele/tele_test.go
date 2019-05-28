package tele

import (
	"context"
	"fmt"
	"testing"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	proto "github.com/golang/protobuf/proto"
	tele_config "github.com/temoto/vender/head/tele/config"
	"github.com/temoto/vender/helpers"
	"github.com/temoto/vender/log2"
)

func NewTestContext(t testing.TB, log *log2.Log) context.Context {
	broker := NewMqttMock(t)
	ctx := context.Background()
	ctx = context.WithValue(ctx, log2.ContextKey, log)
	ctx = ContextWithMqttMock(ctx, broker)
	return ctx
}

// func TokenAssert(t testing.TB, tok mqtt.Token) {
// 	t.Helper()
// 	if !tok.Wait() {
// 		t.Errorf("timeout")
// 	}
// 	if err := tok.Error(); err != nil {
// 		t.Error(err)
// 	}
// }

func TestCommandReport(t *testing.T) {
	// FIXME ugly `mqtt.CRITICAL/ERROR/WARN/DEBUG` global variables
	// t.Parallel()
	rand := helpers.RandUnix()

	tele := new(Tele)
	vmId := -rand.Int31()
	topicCommand := fmt.Sprintf("vm%d/r/c", vmId)
	topicState := fmt.Sprintf("vm%d/w/1s", vmId)
	topicTelemetry := fmt.Sprintf("vm%d/w/1t", vmId)
	conf := tele_config.Config{
		Enabled:    true,
		VmId:       int(vmId),
		MqttBroker: "mock",
	}
	log := log2.NewTest(t, log2.LDebug)
	ctx := NewTestContext(t, log)
	broker := GetMqttMock(ctx)
	result := make(chan Telemetry)
	broker.Subscribe(topicState, 0, func(_ mqtt.Client, msg mqtt.Message) {})
	broker.Subscribe(topicTelemetry, 0, func(_ mqtt.Client, msg mqtt.Message) {
		srvTm := new(Telemetry)
		if err := proto.Unmarshal(msg.Payload(), srvTm); err != nil {
			t.Fatal(err)
			return
		}
		result <- *srvTm
	})

	tele.Init(ctx, log, conf)
	outCmd := Command{
		Id:   rand.Uint32(),
		Task: &Command_Report{&Command_ArgReport{}},
	}
	b, err := proto.Marshal(&outCmd)
	t.Logf("command bytes=%x", b)
	if err != nil {
		t.Fatal(err)
	}
	broker.TestPublish(t, topicCommand, b)
	tm := <-result
	if tm.Error != nil {
		t.Error(tm.Error)
	}
	helpers.AssertEqual(t, tm.VmId, vmId)
}

func TestCommandSetGiftCredit(t *testing.T) {
	// FIXME ugly `mqtt.CRITICAL/ERROR/WARN/DEBUG` global variables
	// t.Parallel()
	rand := helpers.RandUnix()

	tele := new(Tele)
	vmId := -rand.Int31()
	topicCommand := fmt.Sprintf("vm%d/r/c", vmId)
	topicState := fmt.Sprintf("vm%d/w/1s", vmId)
	conf := tele_config.Config{
		Enabled:    true,
		VmId:       int(vmId),
		MqttBroker: "mock",
	}
	log := log2.NewTest(t, log2.LDebug)
	ctx := NewTestContext(t, log)
	broker := GetMqttMock(ctx)
	broker.Subscribe(topicState, 0, func(_ mqtt.Client, msg mqtt.Message) {})

	tele.Init(ctx, log, conf)
	outAmount := uint32(rand.Int31())
	outCmd := Command{
		Id:   rand.Uint32(),
		Task: &Command_SetGiftCredit{&Command_ArgSetGiftCredit{Amount: outAmount}},
	}
	b, err := proto.Marshal(&outCmd)
	t.Logf("command bytes=%x", b)
	if err != nil {
		t.Fatal(err)
	}
	broker.TestPublish(t, topicCommand, b)

	inCmd := <-tele.CommandChan()
	if inCmd.String() != outCmd.String() {
		t.Errorf("expected=%#v actual=%#v", outCmd, inCmd)
	}
}
