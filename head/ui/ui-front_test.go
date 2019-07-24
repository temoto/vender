package ui

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/temoto/vender/hardware/input"
	"github.com/temoto/vender/hardware/mdb"
	"github.com/temoto/vender/head/money"
	"github.com/temoto/vender/state"
)

func TestFrontSimple(t *testing.T) {
	t.Parallel()

	ctx, g := state.NewTestContext(t, "")
	mock := mdb.MockFromContext(ctx)
	defer mock.Close()
	mock.ExpectMap(map[string]string{
		"": "",
	})
	moneysys := new(money.MoneySystem)
	err := moneysys.Start(ctx)
	require.NoError(t, err)
	env := &tenv{ctx: ctx, g: g}
	g.Config.UI.Front.MsgStateIntro = "why hello"
	uiTestSetup(t, env, StateFrontBegin, StateFrontEnd)
	go env.ui.Loop(ctx)

	steps := []step{
		{expect: env._T("why hello", " "), inev: input.Event{Source: input.EvendKeyboardSourceTag, Key: input.EvendKeyReject}},
		{expect: "", inev: input.Event{}},
	}
	uiTestWait(t, env, steps)
}
