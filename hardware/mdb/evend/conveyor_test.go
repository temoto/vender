package evend

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/temoto/vender/engine"
	"github.com/temoto/vender/hardware/mdb"
	"github.com/temoto/vender/head/state"
)

func testMake(t testing.TB, replyFunc mdb.TestReplyFunc) context.Context {
	mdber, reqCh, respCh := mdb.NewTestMDBChan(t)
	go func() {
		defer close(respCh)
		mdb.TestChanTx(t, reqCh, respCh, "d8", "")

		mdb.TestChanTx(t, reqCh, respCh, "d9", "011810000a0000c8001fff01050a32640000000000000000000000")

		if replyFunc != nil {
			replyFunc(t, reqCh, respCh)
		}
	}()
	ctx := context.Background()
	ctx = state.ContextWithConfig(ctx, state.MustReadConfig(t.Fatal, strings.NewReader("")))
	ctx = context.WithValue(ctx, mdb.ContextKey, mdber)
	ctx = context.WithValue(ctx, engine.ContextKey, engine.NewEngine())
	return ctx
}

func TestConveyor(t *testing.T) {
	reply := func(t testing.TB, reqCh <-chan mdb.Packet, respCh chan<- mdb.Packet) {
		mdb.TestChanTx(t, reqCh, respCh, "da010618", "")
		mdb.TestChanTx(t, reqCh, respCh, "db", "50")
		mdb.TestChanTx(t, reqCh, respCh, "db", "54")
		mdb.TestChanTx(t, reqCh, respCh, "db", "")

		mdb.TestChanTx(t, reqCh, respCh, "da010767", "")
		mdb.TestChanTx(t, reqCh, respCh, "db", "54")
		mdb.TestChanTx(t, reqCh, respCh, "db", "50")
		mdb.TestChanTx(t, reqCh, respCh, "db", "")
	}
	ctx := testMake(t, reply)
	e := engine.ContextValueEngine(ctx, engine.ContextKey)
	dc := new(DeviceConveyor)
	err := dc.Init(ctx)
	// TODO make small delay default in tests
	dc.dev.DelayNext = 1 * time.Millisecond
	if err != nil {
		t.Fatalf("Init err=%v", err)
	}
	err = e.Resolve("mdb.evend.conveyor_move_cup").Do(ctx)
	if err != nil {
		t.Fatalf("Move err=%v", err)
	}
	err = e.Resolve("mdb.evend.conveyor_move_elevator").Do(ctx)
	if err != nil {
		t.Fatalf("Move err=%v", err)
	}
}
