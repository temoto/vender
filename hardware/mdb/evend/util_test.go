package evend

import (
	"context"
	"testing"

	"github.com/temoto/vender/hardware/mdb"
	"github.com/temoto/vender/head/state"
	"github.com/temoto/vender/log2"
)

func testMake(t testing.TB, initFunc, replyFunc mdb.TestReplyFunc) context.Context {
	ctx := state.NewTestContext(t, "", log2.LDebug)

	mdber, reqCh, respCh := mdb.NewTestMDBChan(t, ctx)
	go func() {
		defer close(respCh)
		initFunc(t, reqCh, respCh)
		if replyFunc != nil {
			replyFunc(t, reqCh, respCh)
		}
	}()

	ctx = context.WithValue(ctx, mdb.ContextKey, mdber)
	return ctx
}
