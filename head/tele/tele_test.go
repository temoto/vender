package tele

import (
	"context"
	"math/rand"
	"testing"

	proto "github.com/golang/protobuf/proto"
	"github.com/temoto/vender/helpers"
	"github.com/temoto/vender/log2"
)

func NewTestContext(t testing.TB, logLevel log2.Level) context.Context {
	broker := NewMqttMock()
	ctx := context.Background()
	ctx = context.WithValue(ctx, log2.ContextKey, log2.NewTest(t, logLevel))
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

func TestCommand(t *testing.T) {
	t.Parallel()
	tele := new(Tele)
	conf := Config{
		Enabled:    true,
		MqttBroker: "mock",
		Id:         "vmtest",
	}
	ctx := NewTestContext(t, log2.LDebug)
	broker := GetMqttMock(ctx)
	tele.Init(ctx, conf)
	outCmd := Command{
		Id:   rand.Uint32(),
		Task: Command_Report,
	}
	b, err := proto.Marshal(&outCmd)
	if err != nil {
		t.Fatal(err)
	}
	broker.TestPublish(t, "vmtest/r/c", b)
	cmd := <-tele.CommandChan()
	helpers.AssertEqual(t, cmd.Id, outCmd.Id)
	helpers.AssertEqual(t, uint32(cmd.Task), uint32(outCmd.Task))
}
