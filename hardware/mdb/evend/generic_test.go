package evend

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/temoto/vender/hardware/mdb"
	"github.com/temoto/vender/internal/engine"
	state_new "github.com/temoto/vender/internal/state/new"
)

func TestGenericProto2Error(t *testing.T) {
	t.Parallel()

	ctx, _ := state_new.NewTestContext(t, "", ``)
	mock := mdb.MockFromContext(ctx)
	defer mock.Close()
	go mock.Expect([]mdb.MockR{
		{"40", ""},
		{"41", "00ff"},
		{"43", ""},
		{"4201", ""},
		{"43", "08"},   // POLL -> 08 error state
		{"4402", "ff"}, // error code ff
	})
	dev := &Generic{}
	dev.Init(ctx, 0x40, "abstract", proto2)
	assert.Equal(t, "evend.abstract", dev.Name())
	require.NoError(t, dev.FIXME_initIO(ctx))
	const tag = "action"
	d := engine.NewSeq(tag).
		Append(dev.NewWaitReady(tag)).
		Append(dev.NewAction(tag, 0x01)).
		Append(dev.NewWaitDone(tag, time.Second))
	ch := make(chan error)
	go func() {
		ch <- d.Do(ctx)
	}()
	select {
	case err := <-ch:
		require.Error(t, err)
		require.Equal(t, "action/wait-done/poll-loop: evend errorcode=255", err.Error())
	case <-time.After(2 * time.Second):
		t.Fatal("deadlock")
	}
}
