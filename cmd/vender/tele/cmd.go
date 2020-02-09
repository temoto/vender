package tele

import (
	"context"
	"encoding/hex"

	"github.com/c-bata/go-prompt"
	"github.com/golang/protobuf/proto"
	"github.com/juju/errors"
	"github.com/temoto/spq"
	"github.com/temoto/vender/cmd/vender/subcmd"
	"github.com/temoto/vender/hardware"
	"github.com/temoto/vender/helpers/cli"
	"github.com/temoto/vender/internal/state"
	tele_api "github.com/temoto/vender/tele"
)

const modName = "tele"

var Mod = subcmd.Mod{Name: modName, Main: Main}

func Main(ctx context.Context, config *state.Config) error {
	g := state.GetGlobal(ctx)
	synthConfig := &state.Config{
		Tele: config.Tele,
	}
	synthConfig.Hardware.XXX_Devices = nil
	synthConfig.Tele.Enabled = true
	synthConfig.Tele.PersistPath = spq.OnlyForTesting
	synthConfig.Tele.LogDebug = true
	g.MustInit(ctx, synthConfig)

	if err := hardware.Enum(ctx); err != nil {
		err = errors.Annotate(err, "hardware enum")
		return err
	}

	g.Log.Debugf("tele init complete, running")
	// for g.Alive.IsRunning() {
	// 	g.Log.Debugf("before telesys")
	// 	telesys.Error(fmt.Errorf("tele tick"))
	// 	time.Sleep(5 * time.Second)
	// 	// time.Sleep(99 * time.Millisecond)
	// }

	cli.MainLoop(modName, newExecutor(ctx), newCompleter(ctx))
	return nil
}

func newCompleter(ctx context.Context) func(d prompt.Document) []prompt.Suggest {
	// suggests := []prompt.Suggest{}
	return func(d prompt.Document) []prompt.Suggest {
		// return prompt.FilterFuzzy(suggests, d.GetWordBeforeCursor(), true)
		return nil
	}
}

func newExecutor(ctx context.Context) func(string) {
	g := state.GetGlobal(ctx)
	return func(line string) {
		// mosquitto_sub wrongly strips leading zero in hex format
		if len(line)%2 == 1 {
			line = "0" + line
		}
		b, err := hex.DecodeString(line)
		if err != nil {
			g.Log.Errorf("hex.Decode err=%v", err)
		}

		var tm tele_api.Telemetry
		if err := proto.Unmarshal(b, &tm); err != nil {
			g.Log.Errorf("proto.Unmarshal err=%v", err)
		}
		g.Log.Info(proto.MarshalTextString(&tm))
	}
}
