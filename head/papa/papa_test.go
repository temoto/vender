package papa

import (
	"context"
	"strings"
	"testing"

	"github.com/temoto/vender/head/state"
	"github.com/temoto/vender/log2"
)

func TestPapaStart(t *testing.T) {
	ctx := context.Background()
	log := log2.NewTest(t, log2.LDebug)
	ctx = context.WithValue(ctx, log2.ContextKey, log)
	ctx = state.ContextWithConfig(ctx, state.MustReadConfig(strings.NewReader(""), log))
	sys := PapaSystem{}
	if err := sys.Start(ctx); err != nil {
		t.Fatal(err)
	}
}
