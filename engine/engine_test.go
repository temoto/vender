package engine

import (
	"context"
	"testing"

	"github.com/juju/errors"
	"github.com/temoto/vender/log2"
)

func TestResolveLazyArg(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	ctx = context.WithValue(ctx, log2.ContextKey, log2.NewTest(t, log2.LDebug))
	e := NewEngine(ctx)
	ctx = context.WithValue(ctx, ContextKey, e)

	// lazy reference simple(?) before register
	e.RegisterNewSeq("@complex_seq(?)", e.MustResolveOrLazy("simple(?)"))
	// tx := NewTree("")
	// tx.Root.Append(e.MustResolveOrLazy("simple(?)"))
	// e.Register("@complex_tree(?)", tx)

	success := 0
	simple := FuncArg{Name: "simple", F: func(ctx context.Context, arg Arg) error {
		if arg == 42 || arg == 43 {
			success++
			return nil
		}
		return errors.Errorf("unexpected arg=%v", arg)
	}}
	e.Register("simple(?)", simple)

	TestDo(t, ctx, "@complex_seq(42)")
	TestDo(t, ctx, "@complex_seq(42)") // same arg again
	// TestDo(t, ctx, "@complex_tree(43)")
	// TestDo(t, ctx, "@complex_tree(43)") // same arg again
	// if success != 4 {
	if success != 2 {
		t.Errorf("success=%d", success)
	}
}
