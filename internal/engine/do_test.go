package engine

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/temoto/vender/log2"
)

func TestArg(t *testing.T) {
	t.Parallel()

	worker := func(ctx context.Context, param Arg) error {
		result := ctx.Value("result").(*Arg)
		*result = param + 1
		return nil
	}
	failOnceFlag := false
	failOnce := FuncArg{F: func(ctx context.Context, param Arg) error {
		if failOnceFlag {
			return worker(ctx, param)
		}
		failOnceFlag = true
		return fmt.Errorf("this error is expected once")
	}}

	ctx := context.Background()
	ctx = context.WithValue(ctx, log2.ContextKey, log2.NewTest(t, log2.LDebug))
	var action Doer = FuncArg{Name: "worker", F: worker}

	cases := []func() Doer{
		func() Doer { return action },
		func() Doer { return NewSeq("seq").Append(action) },
		func() Doer { return NewSeq("seq-nest").Append(NewSeq("inner").Append(action)) },
		func() Doer {
			return &RestartError{
				Doer:  NewSeq("with-restart").Append(NewSeq("inner").Append(failOnce)),
				Check: func(e error) bool { return e != nil },
				Reset: Nothing{},
			}
		},
		// tx := NewTree("complex_tree")
		// tx.Root.Append(Nothing{"prepare"}).Append(action).Append(Nothing{"cleanup"})
	}
	for _, c := range cases {
		d := c()
		t.Run(d.String(), func(t *testing.T) {
			t.Logf("d=%s %#v", d.String(), d)
			arg := Arg(42)
			new, applied, err := ArgApply(d, arg)
			require.NoError(t, err)
			require.NoError(t, new.Validate())
			assert.True(t, applied)
			var result Arg
			outctx := context.WithValue(ctx, "result", &result)
			require.NoError(t, new.Do(outctx), d.String())
			assert.Equal(t, arg+1, result)
		})
	}
}

// Few actions in sequence is a common case worth optimizing.
func BenchmarkSequentialDo(b *testing.B) {
	mkbench := func(kind string, length int) func(b *testing.B) {
		return func(b *testing.B) {
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
			// 		tail = tail.Append(Func{Name: "stub-action", F: noopCtx})
			// 	}
			// 	tx = t
			case "seq":
				s := NewSeq(fmt.Sprintf("%s-%d", kind, length))
				for i := 1; i <= length; i++ {
					s.Append(Func{Name: "stub-action", F: noopCtx})
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

func noopCtx(context.Context) error { return nil }

type mockdo struct {
	name   string
	called int32
	err    error
	lk     sync.Mutex
	last   time.Time
	v      ValidateFunc
}

func (self *mockdo) Validate() error { return useValidator(self.v) }
func (self *mockdo) Do(ctx context.Context) error {
	self.lk.Lock()
	self.called += 1
	self.last = time.Now()
	self.lk.Unlock()
	return self.err
}
func (self *mockdo) String() string { return self.name }
