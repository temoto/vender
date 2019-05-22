package engine

import (
	"context"
	"fmt"
	"testing"

	"github.com/temoto/vender/helpers"
	"github.com/temoto/vender/log2"
)

// compile-time test interface is implemented
var _ = ArgApplier(new(FuncArg))
var _ = ArgApplier(new(Seq))

// var _ = ArgApplier(new(Tree))

func argnew(n string, f ArgFunc) FuncArg { return FuncArg{Name: n, F: f} }

func TestArg(t *testing.T) {
	t.Parallel()

	const expect = 42
	ok := false
	worker := func(ctx context.Context, param Arg) error {
		if param == expect {
			ok = true
		}
		return nil
	}

	ctx := context.Background()
	ctx = context.WithValue(ctx, log2.ContextKey, log2.NewTest(t, log2.LDebug))
	var action Doer = argnew("worker", worker)
	seq := NewSeq("complex_seq").Append(action)
	// tx := NewTree("complex_tree")
	// tx.Root.Append(Nothing{"prepare"}).Append(action).Append(Nothing{"cleanup"})
	var applied Doer = seq.Apply(42)
	if err := applied.Validate(); err != nil {
		t.Fatal(err)
	}
	DoCheckFatal(t, applied, ctx)
	helpers.AssertEqual(t, ok, true)
}

// Few actions in sequence is a common case worth optimizing.
func BenchmarkSequentialDo(b *testing.B) {
	mkbench := func(kind string, length int) func(b *testing.B) {
		return func(b *testing.B) {
			op := func(ctx context.Context) error { return nil }
			ctx := context.Background()
			log := log2.NewTest(b, log2.LError)
			log.SetFlags(log2.LTestFlags)
			ctx = context.WithValue(ctx, log2.ContextKey, log)

			var tx Doer
			switch kind {
			// case "tree":
			// 	t := NewTree(fmt.Sprintf("%s-%d", kind, length))
			// 	tail := &t.Root
			// 	for i := 1; i <= length; i++ {
			// 		tail = tail.Append(Func{Name: "stub-action", F: op})
			// 	}
			// 	tx = t
			case "seq":
				s := NewSeq(fmt.Sprintf("%s-%d", kind, length))
				for i := 1; i <= length; i++ {
					s.Append(Func{Name: "stub-action", F: op})
				}
				tx = s
			default:
				panic(kind)
			}

			b.ReportAllocs()
			b.ResetTimer()
			for i := 1; i <= b.N; i++ {
				if err := tx.Do(ctx); err != nil {
					b.Fatal(err)
				}
			}
		}
	}

	// b.Run("tree-3", mkbench("tree", 3))
	// b.Run("tree-5", mkbench("tree", 5))
	b.Run("seq-3", mkbench("seq", 3))
	b.Run("seq-5", mkbench("seq", 5))
}
