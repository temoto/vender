package tele

import (
	"context"
	"testing"

	proto "github.com/golang/protobuf/proto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/temoto/spq"
	tele_config "github.com/temoto/vender/head/tele/config"
	"github.com/temoto/vender/helpers"
	"github.com/temoto/vender/log2"
)

type tenv struct {
	cmd   *Command
	tele  *Tele
	trans *transportMock
}

func newTestTele(t testing.TB, env *tenv) {
	env.trans = &transportMock{t: t}
	env.tele = &Tele{transport: env.trans}
}

func TestCommand(t *testing.T) {
	// FIXME ugly `mqtt.CRITICAL/ERROR/WARN/DEBUG` global variables
	// t.Parallel()
	rand := helpers.RandUnix()

	type tcase struct {
		name  string
		cmd   Command
		check func(t testing.TB, env *tenv)
	}
	cases := []tcase{
		{"report",
			Command{Id: rand.Uint32(), Task: &Command_Report{&Command_ArgReport{}}},
			func(t testing.TB, env *tenv) {
				payload := <-env.trans.outTelemetry
				var tm Telemetry
				require.NoError(t, proto.Unmarshal(payload, &tm))
				assert.Nil(t, tm.Error)
				assert.Equal(t, env.tele.vmId, tm.VmId)
			}},
		{"setgiftcredit",
			Command{
				Id:   rand.Uint32(),
				Task: &Command_SetGiftCredit{&Command_ArgSetGiftCredit{Amount: uint32(rand.Int31())}},
			},
			func(t testing.TB, env *tenv) {
				inCmd := <-env.tele.CommandChan()
				assert.Equal(t, env.cmd.String(), inCmd.String())
			}},
		{"ping", Command{
			Id:         rand.Uint32(),
			Task:       &Command_Ping{&Command_ArgPing{}},
			ReplyTopic: "foobar",
		},
			func(t testing.TB, env *tenv) {
				payload := <-env.trans.outResponse
				var r Response
				require.NoError(t, proto.Unmarshal(payload, &r))
				require.Equal(t, env.cmd.Id, r.CommandId)
				assert.Equal(t, "", r.Error)
			}},
	}
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			ctx := context.Background()
			env := &tenv{cmd: &c.cmd}
			newTestTele(t, env)
			defer env.tele.Close()
			vmId := -rand.Int31()
			conf := tele_config.Config{
				Enabled:     true,
				LogDebug:    true,
				PersistPath: spq.OnlyForTesting,
				VmId:        int(vmId),
			}
			log := log2.NewTest(t, log2.LDebug)
			// log := log2.NewStderr(log2.LDebug) // useful with panics
			err := env.tele.Init(ctx, log, conf)
			require.NoError(t, err)
			require.Equal(t, "\x01", string(<-env.trans.outState))

			b, err := proto.Marshal(&c.cmd)
			require.NoError(t, err)
			require.True(t, env.trans.onCommand(b))

			c.check(t, env)
		})
	}
}
