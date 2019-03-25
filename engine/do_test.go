package engine

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/juju/errors"
	"github.com/temoto/vender/helpers"
	"github.com/temoto/vender/log2"
)

func TestTransactionConcurrent(t *testing.T) {
	t.Parallel()
	tx := NewTransaction("tx1")
	do1 := &Sleep{10 * time.Millisecond}
	do2 := &Sleep{50 * time.Millisecond}
	n11 := NewNode(do1, &tx.Root)
	n12 := NewNode(do1, &tx.Root)
	n13 := NewNode(do1, &tx.Root)
	n21 := NewNode(do2, n11)
	n22 := NewNode(do2, n12, n13)
	n23 := NewNode(do2, n11, n13)
	n3 := NewNode(&mockdo{name: "check"}, n21, n22, n23)
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

func TestTransactionWide(t *testing.T) {
	t.Parallel()
	tx := NewTransaction("wide")
	do1 := &mockdo{}
	do2 := &mockdo{}
	n11 := NewNode(do1, &tx.Root)
	n12 := NewNode(do1, &tx.Root)
	n13 := NewNode(do1, &tx.Root)
	n14 := NewNode(do1, &tx.Root)
	n15 := NewNode(do1, &tx.Root)
	n21 := NewNode(do2, n11)
	n22 := NewNode(do2, n11, n12)
	n23 := NewNode(do2, n12, n13)
	n24 := NewNode(do2, n13, n14)
	n25 := NewNode(do2, n11, n13, n15)
	n3 := NewNode(&mockdo{name: "check"}, n21, n22, n23, n24, n25)
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

func TestTransactionFail(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	ctx = context.WithValue(ctx, log2.ContextKey, log2.NewTest(t, log2.LDebug))
	tx := NewTransaction("fail")
	doErr := &Func{F: func(ctx context.Context) error {
		return errors.Errorf("intentional-error")
	}}
	doCheck := &mockdo{name: "check"}
	tx.Root.Append(doErr).Append(doCheck)
	// dots := tx.Root.Dot("UD")
	// t.Logf("%s", dots)
	err := tx.Do(ctx)
	if err == nil {
		t.Fatalf("tx.Do() unexpected err=nil")
	}
	if !strings.Contains(err.Error(), "intentional-error") {
		t.Fatalf("expected tx.Do() error, err=%v", err)
	}
	helpers.AssertEqual(t, doCheck.called, int32(0))
}

func TestTransactionRestart(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	ctx = context.WithValue(ctx, log2.ContextKey, log2.NewTest(t, log2.LDebug))
	tx := NewTransaction("restart")
	doErr := &Func{F: func(ctx context.Context) error {
		return errors.Errorf("intentional-error")
	}}
	doCheck := &mockdo{name: "check"}
	tx.Root.Append(&Nothing{"success"}).Append(doErr).Append(doCheck)

	check := func() {
		err := tx.Do(ctx)
		if err == nil {
			t.Fatalf("tx.Do() unexpected err=nil")
		}
		if !strings.Contains(err.Error(), "intentional-error") {
			t.Fatalf("expected tx.Do() error, err=%v", err)
		}
		helpers.AssertEqual(t, doCheck.called, int32(0))
	}

	check()
	check()
}
