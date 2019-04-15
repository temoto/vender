package engine

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/juju/errors"
	"github.com/temoto/vender/helpers"
	"github.com/temoto/vender/log2"
)

func TestTreeConcurrent(t *testing.T) {
	t.Parallel()

	tx := NewTree("tx1")
	do1 := &Sleep{10 * time.Millisecond}
	do2 := &Sleep{50 * time.Millisecond}
	n11 := newNode(do1, &tx.Root)
	n12 := newNode(do1, &tx.Root)
	n13 := newNode(do1, &tx.Root)
	n21 := newNode(do2, n11)
	n22 := newNode(do2, n12, n13)
	n23 := newNode(do2, n11, n13)
	n3 := newNode(&mockdo{name: "check"}, n21, n22, n23)
	// dots := tx.Root.Dot("UD")
	// t.Logf("%s", dots)
	tbegin := time.Now()
	ctx := context.Background()
	ctx = context.WithValue(ctx, log2.ContextKey, log2.NewTest(t, log2.LDebug))
	err := tx.Do(ctx)
	duration := time.Since(tbegin)
	if err != nil {
		t.Fatal(err)
	}
	helpers.AssertEqual(t, n3.Doer.(*mockdo).called, int32(1))
	// expect duration about do1+do2 but not much more
	if duration < do2.Duration {
		t.Errorf("total duration too low: %v", duration)
	}
	if duration > do2.Duration*2 {
		t.Errorf("total duration too much: %v", duration)
	}
}

func TestTreeWide(t *testing.T) {
	t.Parallel()

	tx := NewTree("wide")
	do1 := &mockdo{}
	do2 := &mockdo{}
	n11 := newNode(do1, &tx.Root)
	n12 := newNode(do1, &tx.Root)
	n13 := newNode(do1, &tx.Root)
	n14 := newNode(do1, &tx.Root)
	n15 := newNode(do1, &tx.Root)
	n21 := newNode(do2, n11)
	n22 := newNode(do2, n11, n12)
	n23 := newNode(do2, n12, n13)
	n24 := newNode(do2, n13, n14)
	n25 := newNode(do2, n11, n13, n15)
	n3 := newNode(&mockdo{name: "check"}, n21, n22, n23, n24, n25)
	// dots := tx.Root.Dot("UD")
	// t.Logf("%s", dots)
	ctx := context.Background()
	ctx = context.WithValue(ctx, log2.ContextKey, log2.NewTest(t, log2.LDebug))
	err := tx.Do(ctx)
	if err != nil {
		t.Fatal(err)
	}
	helpers.AssertEqual(t, do1.called, int32(5))
	helpers.AssertEqual(t, do2.called, int32(5))
	helpers.AssertEqual(t, n3.Doer.(*mockdo).called, int32(1))
}

func TestTreeFail(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	ctx = context.WithValue(ctx, log2.ContextKey, log2.NewTest(t, log2.LDebug))
	tx := NewTree("fail")
	doErr := &Func{F: func(ctx context.Context) error {
		return errors.Errorf("intentional-error")
	}}
	DoCheck := &mockdo{name: "check"}
	tx.Root.Append(doErr).Append(DoCheck)
	// dots := tx.Root.Dot("UD")
	// t.Logf("%s", dots)
	err := tx.Do(ctx)
	if err == nil {
		t.Fatalf("tx.Do() unexpected err=nil")
	}
	if !strings.Contains(err.Error(), "intentional-error") {
		t.Fatalf("expected tx.Do() error, err=%v", err)
	}
	helpers.AssertEqual(t, DoCheck.called, int32(0))
}

func TestTreeRestart(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	ctx = context.WithValue(ctx, log2.ContextKey, log2.NewTest(t, log2.LDebug))
	tx := NewTree("restart")
	doErr := &Func{F: func(ctx context.Context) error {
		return errors.Errorf("intentional-error")
	}}
	DoCheck := &mockdo{name: "check"}
	tx.Root.Append(&Nothing{"success"}).Append(doErr).Append(DoCheck)

	check := func() {
		err := tx.Do(ctx)
		if err == nil {
			t.Fatalf("tx.Do() unexpected err=nil")
		}
		if !strings.Contains(err.Error(), "intentional-error") {
			t.Fatalf("expected tx.Do() error, err=%v", err)
		}
		helpers.AssertEqual(t, DoCheck.called, int32(0))
	}

	check()
	check()
}

// compile-time test
var _ = ArgApplier(new(Tree))
var _ = ArgApplier(new(FuncArg))

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
	tx := NewTree("complex")
	tx.Root.Append(Nothing{"prepare"}).Append(action).Append(Nothing{"cleanup"})
	var applied Doer = tx.Apply(42)
	if err := tx.Validate(); err != nil {
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
			case "tree":
				t := NewTree(fmt.Sprintf("%s-%d", kind, length))
				tail := &t.Root
				for i := 1; i <= length; i++ {
					tail = tail.Append(Func{Name: "stub-action", F: op})
				}
				tx = t
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

	b.Run("tree-3", mkbench("tree", 3))
	b.Run("tree-5", mkbench("tree", 5))
	b.Run("seq-3", mkbench("seq", 3))
	b.Run("seq-5", mkbench("seq", 5))
}
