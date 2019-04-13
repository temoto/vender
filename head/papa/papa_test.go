package papa

import (
	"testing"

	"github.com/temoto/vender/head/state"
)

func TestPapaStart(t *testing.T) {
	ctx := state.NewTestContext(t, "money { scale=100 }")
	sys := PapaSystem{}
	if err := sys.Start(ctx); err != nil {
		t.Fatal(err)
	}
}
