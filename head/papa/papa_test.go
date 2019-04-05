package papa

import (
	"testing"

	"github.com/temoto/vender/head/state"
	"github.com/temoto/vender/log2"
)

func TestPapaStart(t *testing.T) {
	ctx := state.NewTestContext(t, "money { scale=100 }", log2.LDebug)
	sys := PapaSystem{}
	if err := sys.Start(ctx); err != nil {
		t.Fatal(err)
	}
}
