package engine

import (
	"context"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"
)

func BenchmarkExec(b *testing.B) {
	type execFun = func(ctx context.Context, d Doer) error
	mkbench := func(ctx context.Context, e *Engine, fun execFun) func(*testing.B) {
		return func(b *testing.B) {
			e.RegisterNewFunc("noop", func(context.Context) error { runtime.Gosched(); return nil })
			require.NoError(b, e.RegisterParse("inner2", "noop noop"))
			require.NoError(b, e.RegisterParse("inner3", "noop noop noop"))
			require.NoError(b, e.RegisterParse("complex", "inner3 noop inner2"))
			d := e.Resolve("complex")

			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if err := fun(ctx, d); err != nil {
					b.Fatal(err)
				}
			}
		}
	}

	{
		ctx, e := newTestContext(b)
		b.Run("raw", mkbench(ctx, e, e.Exec))
	}
	{
		ctx, e := newTestContext(b)
		b.Run("validate", mkbench(ctx, e, e.ValidateExec))
	}
}
