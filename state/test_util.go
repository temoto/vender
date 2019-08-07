package state

import (
	"context"
	"testing"

	"github.com/juju/errors"
	"github.com/temoto/vender/hardware/mdb"
	"github.com/temoto/vender/log2"
)

func NewTestContext(t testing.TB, confString string /* logLevel log2.Level*/) (context.Context, *Global) {
	fs := NewMockFullReader(map[string]string{
		"test-inline": confString,
	})

	log := log2.NewTest(t, log2.LDebug)
	// log := log2.NewStderr(log2.LDebug) // useful with panics
	log.SetFlags(log2.LTestFlags)
	ctx, g := NewContext(log)
	g.MustInit(ctx, MustReadConfig(log, fs, "test-inline"))

	mdber, mdbMock := mdb.NewTestMdber(t)
	g.Hardware.Mdb.Mdber = mdber
	if _, err := g.Mdber(); err != nil {
		t.Fatal(errors.Trace(err))
	}
	ctx = context.WithValue(ctx, mdb.MockContextKey, mdbMock)

	return ctx, g
}
