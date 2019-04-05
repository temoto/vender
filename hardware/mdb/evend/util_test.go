package evend

import (
	"context"
	"testing"

	"github.com/temoto/vender/hardware/mdb"
	"github.com/temoto/vender/head/state"
	"github.com/temoto/vender/log2"
)

func testMake(t testing.TB, initFunc, replyFunc mdb.TestReplyFunc) context.Context {
	ctx := state.NewTestContext(t, "money { scale=100 }", log2.LDebug)

	mdber, reqCh, respCh := mdb.NewTestMDBChan(t, ctx)
	config := state.GetConfig(ctx)
	config.Global().Hardware.Mdb.Mdber = mdber
	if _, err := config.Mdber(); err != nil {
		t.Fatal(err)
	}

	go func() {
		defer close(respCh)
		initFunc(t, reqCh, respCh)
		if replyFunc != nil {
			replyFunc(t, reqCh, respCh)
		}
	}()

	return ctx
}
