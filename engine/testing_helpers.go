package engine

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func (e *Engine) TestDo(t testing.TB, ctx context.Context, name string) {
	t.Helper()
	t.Logf("Engine.TestDo name=%s", name)
	d := e.Resolve(name)
	require.NotNil(t, d, "Doer")
	require.NoError(t, d.Do(ctx), d.String())
}
