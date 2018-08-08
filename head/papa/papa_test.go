package papa

import (
	"context"
	"testing"

	"github.com/temoto/vender/head/state"
)

func TestPapaStart(t *testing.T) {
	ctx := context.Background()
	ctx = context.WithValue(ctx, "config", &state.Config{})
	onStart(ctx)
}
