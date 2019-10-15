package tele

import (
	"context"
	"testing"

	proto "github.com/golang/protobuf/proto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/temoto/spq"
	"github.com/temoto/vender/engine"
	"github.com/temoto/vender/head/money"
	tele_api "github.com/temoto/vender/head/tele/api"
	tele_config "github.com/temoto/vender/head/tele/config"
	"github.com/temoto/vender/helpers"
	"github.com/temoto/vender/state"
	state_new "github.com/temoto/vender/state/new"
)

type tenv struct {
	config string
	cmd    *tele_api.Command
	tele   *Tele
	trans  *transportMock
	ctx    context.Context
	flag   bool
}

func TestCommand(t *testing.T) {
	// FIXME ugly `mqtt.CRITICAL/ERROR/WARN/DEBUG` global variables
	// t.Parallel()
	rand := helpers.RandUnix()

	type tcase struct {
		name   string
		config string
		cmd    tele_api.Command
		before func(testing.TB, *tenv)
		check  func(testing.TB, *tenv)
	}
	cases := []tcase{
		{name: "report",
			config: `engine { inventory {
	tele_add_name = true
	stock "paper" {}
  stock "rock" {}
}}`,
			cmd: tele_api.Command{
				Id:         rand.Uint32(),
				Task:       &tele_api.Command_Report{&tele_api.Command_ArgReport{}},
				ReplyTopic: "t",
			},
			before: func(t testing.TB, env *tenv) {
				g := state.GetGlobal(env.ctx)
				moneysys := &money.MoneySystem{}
				g.XXX_money.Store(moneysys)
				s1, err1 := g.Inventory.Get("paper")
				require.NoError(t, err1)
				s1.Set(3.14)
				s2, err2 := g.Inventory.Get("rock")
				require.NoError(t, err2)
				s2.Set(42)
			},
			check: func(t testing.TB, env *tenv) {
				payload := <-env.trans.outTelemetry
				var tm tele_api.Telemetry
				require.NoError(t, proto.Unmarshal(payload, &tm))
				assert.Nil(t, tm.Error)
				assert.Equal(t, env.tele.vmId, tm.VmId)
				require.NotNil(t, tm.Inventory)
				assert.Equal(t, `stocks:<value:3 name:"paper" valuef:3.14 > stocks:<value:42 name:"rock" valuef:42 > `, proto.CompactTextString(tm.Inventory))
				// TODO
				t.Logf("cashbox=%#v", tm.MoneyCashbox)
				t.Logf("change=%#v", tm.MoneyChange)
			}},
		{name: "exec",
			cmd: tele_api.Command{
				Id:         rand.Uint32(),
				Task:       &tele_api.Command_Exec{&tele_api.Command_ArgExec{Scenario: "action_stub"}},
				ReplyTopic: "t",
			},
			before: func(t testing.TB, env *tenv) {
				env.flag = false
				state.GetGlobal(env.ctx).Engine.Register("action_stub", engine.Func0{F: func() error {
					env.flag = true
					return nil
				}})
			},
			check: func(t testing.TB, env *tenv) {
				b := <-env.trans.outResponse
				var r tele_api.Response
				require.NoError(t, proto.Unmarshal(b, &r))
				assert.Equal(t, env.cmd.Id, r.CommandId)
				assert.Equal(t, "", r.Error)
				assert.True(t, env.flag)
			}},
	}
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			env := &tenv{
				config: c.config,
				cmd:    &c.cmd,
				trans:  &transportMock{t: t},
			}
			var g *state.Global
			env.tele = &Tele{transport: env.trans}
			env.ctx, g = state_new.NewTestContext(t, env.config)
			g.Tele = env.tele
			// g.Log = log2.NewStderr(log2.LDebug) // useful with panics
			defer env.tele.Close()
			vmId := -rand.Int31()
			conf := tele_config.Config{
				Enabled:     true,
				LogDebug:    true,
				PersistPath: spq.OnlyForTesting,
				VmId:        int(vmId),
			}
			err := env.tele.Init(env.ctx, g.Log, conf)
			require.NoError(t, err)
			require.Equal(t, "\x01", string(<-env.trans.outState))

			if c.before != nil {
				c.before(t, env)
			}

			b, err := proto.Marshal(&c.cmd)
			require.NoError(t, err)
			require.True(t, env.trans.onCommand(env.ctx, b))

			c.check(t, env)
		})
	}
}
