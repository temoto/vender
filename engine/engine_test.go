package engine

import (
	"context"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/pkg/errors"
	"github.com/temoto/vender/log2"
)

func TestEngineExecute(t *testing.T) {
	t.Parallel()
	// log.SetFlags(log.Flags() | log.Lmicroseconds)
	type Case struct {
		name  string
		input string
		check func(t testing.TB, engine *Engine, scenario *Scenario, done chan<- struct{}) error
	}
	cases := []Case{
		Case{"simple", `
digraph simple {
boot[comment="/v1/v=val"];
n1[comment="/v1/r=sleep"];
n2[comment="/v1/v=val:r=finish"]
boot -> n1 -> n2;
}`, func(t testing.TB, engine *Engine, scenario *Scenario, done chan<- struct{}) error {
			valCount := int32(0)
			engine.actions["val"] = Func0{F: func() error { atomic.AddInt32(&valCount, 1); return nil }}
			engine.actions["sleep"] = Func0{F: func() error {
				vc := atomic.LoadInt32(&valCount)
				if vc != 2 {
					return errors.Errorf("valCount before n1.run expected=%d actual=%d", 2, vc)
				}
				time.Sleep(100 * time.Millisecond)
				return nil
			}}
			engine.actions["finish"] = Func0{F: func() error { done <- struct{}{}; return nil }}

			ctx := context.Background()
			return engine.Execute(ctx, scenario)
		}},
	}

	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			ctx := context.Background()
			ctx = context.WithValue(ctx, log2.ContextKey, log2.NewTest(t, log2.LDebug))
			e := NewEngine(ctx)
			scenario, err := ParseDot([]byte(strings.TrimSpace(c.input)))
			if err != nil {
				t.Fatalf("ParseDot err=%v", err)
			}
			done := make(chan struct{}, 1)
			err = c.check(t, e, scenario, done)
			if err != nil {
				t.Fatal(err)
			}
			select {
			case <-done:
			case <-time.After(2 * time.Second):
				t.Fatalf("timeout")
			}
		})
	}
}
