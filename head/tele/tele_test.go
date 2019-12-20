package tele_test

import (
	"context"
	"encoding/hex"
	"fmt"
	"math/rand"
	"testing"
	"time"

	proto "github.com/golang/protobuf/proto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/temoto/spq"
	"github.com/temoto/vender/engine"
	"github.com/temoto/vender/hardware"
	"github.com/temoto/vender/hardware/lcd"
	"github.com/temoto/vender/head/money"
	"github.com/temoto/vender/head/tele"
	tele_api "github.com/temoto/vender/head/tele/api"
	tele_config "github.com/temoto/vender/head/tele/config"
	"github.com/temoto/vender/head/ui"
	"github.com/temoto/vender/helpers"
	"github.com/temoto/vender/state"
	state_new "github.com/temoto/vender/state/new"
)

type tenv struct { //nolint:maligned
	version string
	config  string
	trans   *transportMock
	cmd     *tele_api.Command // abstraction leak, local for TestCommand

	cfg  *tele_config.Config
	ctx  context.Context
	flag bool
	tele tele_api.Teler
	vmid int32
}

func TestCommand(t *testing.T) {
	// FIXME ugly `mqtt.CRITICAL/ERROR/WARN/DEBUG` global variables
	// t.Parallel()
	cases := []struct {
		name   string
		config string
		cmd    tele_api.Command
		before func(testing.TB, *tenv)
		check  func(testing.TB, *tenv)
	}{
		{name: "report",
			config: `engine { inventory {
	tele_add_name = true
	stock "paper" {}
  stock "rock" {}
}}`,
			cmd: tele_api.Command{
				Id:         rand.Uint32(),
				Task:       &tele_api.Command_Report{Report: &tele_api.Command_ArgReport{}},
				ReplyTopic: "t",
			},
			before: func(t testing.TB, env *tenv) {
				require.NoError(t, hardware.Enum(env.ctx))
				g := state.GetGlobal(env.ctx)
				moneysys := &money.MoneySystem{}
				require.NoError(t, moneysys.Start(env.ctx))
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
				assert.Equal(t, env.vmid, tm.VmId)
				require.NotNil(t, tm.Inventory)
				assert.Equal(t, `stocks:<value:3 name:"paper" valuef:3.14 > stocks:<value:42 name:"rock" valuef:42 > `, proto.CompactTextString(tm.Inventory))
				// TODO
				t.Logf("cashbox=%#v", tm.MoneyCashbox)
				t.Logf("change=%#v", tm.MoneyChange)
			}},
		{name: "exec",
			cmd: tele_api.Command{
				Id:         rand.Uint32(),
				Task:       &tele_api.Command_Exec{Exec: &tele_api.Command_ArgExec{Scenario: "action_stub"}},
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
		{name: "set-inventory",
			config: `engine { inventory {
		tele_add_name = true
		stock "paper" {}
		stock "rock" {}
	}}`,
			cmd: tele_api.Command{
				Id: rand.Uint32(),
				Task: &tele_api.Command_SetInventory{SetInventory: &tele_api.Command_ArgSetInventory{
					New: &tele_api.Inventory{Stocks: []*tele_api.Inventory_StockItem{
						&tele_api.Inventory_StockItem{Name: "paper", Valuef: 3.14},
					}},
				}},
				ReplyTopic: "t",
			},
			check: func(t testing.TB, env *tenv) {
				b := <-env.trans.outResponse
				var r tele_api.Response
				require.NoError(t, proto.Unmarshal(b, &r))
				assert.Equal(t, env.cmd.Id, r.CommandId)
				assert.Equal(t, "", r.Error)
				g := state.GetGlobal(env.ctx)
				paperStock, err := g.Inventory.Get("paper")
				require.NoError(t, err)
				assert.Equal(t, float32(3.14), paperStock.Value())
			}},
		{name: "stop",
			config: `engine { menu { item "1" { price=0 scenario="" } } }
			ui { front { reset_sec=5 } }`,
			cmd: tele_api.Command{
				Id:         rand.Uint32(),
				Task:       &tele_api.Command_Stop{Stop: &tele_api.Command_ArgStop{}},
				ReplyTopic: "t",
			},
			before: func(t testing.TB, env *tenv) {
				g := state.GetGlobal(env.ctx)
				g.Config.Tele.FIXME_stopDelaySec = 1
				g.Hardware.HD44780.Display = lcd.NewMockTextDisplay(&lcd.TextDisplayConfig{Width: 16})
				ms := money.MoneySystem{}
				require.NoError(t, ms.Start(env.ctx))
				uix := &ui.UI{}
				uix.XXX_testSetState(ui.StateFrontBegin)
				require.NoError(t, uix.Init(env.ctx))
				go uix.Loop(env.ctx)
			},
			check: func(t testing.TB, env *tenv) {
				b := <-env.trans.outResponse
				var r tele_api.Response
				require.NoError(t, proto.Unmarshal(b, &r))
				assert.Equal(t, env.cmd.Id, r.CommandId)
				assert.Equal(t, "", r.Error)
				g := state.GetGlobal(env.ctx)
				// assert g.Stop() is called before test timeout
				g.Alive.Wait()
			}},
	}
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			rand := helpers.RandUnix()
			env := &tenv{
				config: c.config,
				vmid:   -rand.Int31(),
			}
			testSetup(t, env)
			env.cmd = &c.cmd
			defer env.tele.Close()

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

func TestApi(t *testing.T) {
	cases := []struct {
		name   string
		config string
		setup  func(testing.TB, *tenv)
		check  func(testing.TB, *tenv)
	}{
		{name: "error",
			config: ``,
			check: func(t testing.TB, env *tenv) {
				e := fmt.Errorf("ohi")
				env.tele.Error(e)
				b := <-env.trans.outTelemetry
				var tm tele_api.Telemetry
				require.NoError(t, proto.Unmarshal(b, &tm))
				require.NotNil(t, tm.Error)
				assert.Equal(t, env.vmid, tm.VmId)
				assert.InDelta(t, time.Now().Unix(), tm.Time/1e9, 10)
				assert.Equal(t, e.Error(), tm.Error.Message)
				assert.Equal(t, env.version, tm.BuildVersion)
			}},
		{name: "state",
			config: ``,
			check: func(t testing.TB, env *tenv) {
				// check marshaling of all defined states in random order
				states := make([]tele_api.State, 0, len(tele_api.State_name))
				for k := range tele_api.State_name {
					s := tele_api.State(k)
					states = append(states, s)
				}
				// but since we'd only send duplicate state after tele.stateInterval,
				// ensure first item is not current (boot)
				if states[0] == tele_api.State_Boot {
					states[0], states[1] = states[1], states[0]
				}
				// consume State_Boot before checking others
				assert.Equal(t, "01", hex.EncodeToString(<-env.trans.outState))
				for _, s := range states {
					env.tele.State(s)
					assert.Equal(t, fmt.Sprintf("%02x", int32(s)), hex.EncodeToString(<-env.trans.outState))
				}
			}},
		{name: "state-queue",
			config: ``,
			check: func(t testing.TB, env *tenv) {
				env.tele.State(tele_api.State_Nominal)
				env.tele.State(tele_api.State_Problem)
				env.tele.State(tele_api.State_Lock)
				assert.Equal(t, "01", hex.EncodeToString(<-env.trans.outState))
				assert.Equal(t, "02", hex.EncodeToString(<-env.trans.outState))
				assert.Equal(t, "04", hex.EncodeToString(<-env.trans.outState))
				assert.Equal(t, "06", hex.EncodeToString(<-env.trans.outState))
			}},
		{name: "report-service", config: ``,
			check: func(t testing.TB, env *tenv) {
				g := state.GetGlobal(env.ctx)
				moneysys := &money.MoneySystem{}
				require.NoError(t, moneysys.Start(env.ctx))
				g.XXX_money.Store(moneysys)

				env.tele.Report(env.ctx, true)
				payload := <-env.trans.outTelemetry
				var tm tele_api.Telemetry
				require.NoError(t, proto.Unmarshal(payload, &tm))
				assert.Nil(t, tm.Error)
				assert.Equal(t, env.vmid, tm.VmId)
				assert.True(t, tm.AtService)
				assert.NotNil(t, tm.Inventory)
				assert.Equal(t, env.version, tm.BuildVersion)
			}},
		{name: "disabled", config: ``,
			setup: func(t testing.TB, env *tenv) {
				env.cfg = &tele_config.Config{
					Enabled:     false,
					LogDebug:    true,
					PersistPath: spq.OnlyForTesting,
				}
				testSetup(t, env)
				env.tele.State(tele_api.State_Nominal)
			}},
		// TODO Teler.StatModify
		// TODO Teler.Transaction
	}
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			rand := helpers.RandUnix()
			env := &tenv{
				config:  c.config,
				version: randString(rand, 65),
				vmid:    -rand.Int31(),
			}
			if c.setup == nil {
				c.setup = testSetup
			}
			c.setup(t, env)
			defer env.tele.Close()
			if c.check != nil {
				c.check(t, env)
			}
		})
	}
}

func randString(r *rand.Rand, maxLength uint) string {
	// based on testing/quick.Value() case String
	numChars := r.Intn(int(maxLength))
	codePoints := make([]rune, numChars)
	for i := 0; i < numChars; i++ {
		cp := r.Intn(0x10ffff)  // generate all unicode
		if r.Int31n(100) < 90 { // but more often than not,
			cp &= 0xff // prefer ascii for readability
		}
		codePoints[i] = rune(cp)
	}
	return string(codePoints)
}

func testSetup(t testing.TB, env *tenv) {
	if env.trans == nil {
		env.trans = &transportMock{t: t, outBuffer: 32}
	}
	env.tele = tele.NewWithTransporter(env.trans)

	ctx, g := state_new.NewTestContext(t, env.version, env.config)
	env.ctx = ctx
	g.Tele = env.tele
	if env.cfg == nil {
		env.cfg = &g.Config.Tele
		env.cfg.Enabled = true
		env.cfg.LogDebug = true
		env.cfg.PersistPath = spq.OnlyForTesting
		env.cfg.VmId = int(env.vmid)
	}
	require.NoError(t, env.tele.Init(env.ctx, g.Log, *env.cfg))
}
