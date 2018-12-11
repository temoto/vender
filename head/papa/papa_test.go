package papa

import (
	"context"
	"testing"

	"github.com/temoto/vender/head/state"
)

func TestPapaStart(t *testing.T) {
	ctx := context.Background()
	ctx = state.ContextWithConfig(ctx, &state.Config{})
	sys := PapaSystem{}
	if err := sys.Start(ctx); err != nil {
		t.Fatal(err)
	}
}
