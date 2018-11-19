package actionlist

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/juju/errors"
	"github.com/temoto/vender/helpers"
)

func TestList01(t *testing.T) {
	delay := 10 * time.Millisecond
	called := uint32(0)
	expectedErr := errors.Errorf("expected error")
	l := new(List)
	l.Append(func(ctx context.Context) error {
		atomic.AddUint32(&called, 1)
		return expectedErr
	}, t.Name()+"/fun1")
	l.Append(func(ctx context.Context) error {
		atomic.AddUint32(&called, 1)
		return nil
	}, t.Name()+"/fun2")
	slow := func(ctx context.Context) error {
		time.Sleep(delay)
		atomic.AddUint32(&called, 1)
		return nil
	}
	l.Append(slow, t.Name()+"/slow1")
	l.Append(slow, t.Name()+"/slow2")
	l.Append(slow, t.Name()+"/slow3")
	t1 := time.Now()
	errs := l.Do(context.Background())
	duration := time.Now().Sub(t1)
	helpers.AssertEqual(t, called, uint32(5))
	if duration > 2*delay {
		t.Errorf("expected total duration %v < %v", duration, 2*delay)
	}
	helpers.AssertEqual(t, len(errs), 1)
	if errors.Cause(errs[0]) != expectedErr {
		t.Fatal(helpers.FoldErrors(errs))
	}
}
