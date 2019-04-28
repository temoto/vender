package engine

import (
	"context"
	"testing"
)

func TestDo(t testing.TB, ctx context.Context, name string) {
	t.Helper()
	e := GetEngine(ctx)
	d := e.Resolve(name)
	DoCheckError(t, d, ctx)
}

func DoCheckError(t testing.TB, d Doer, ctx context.Context) {
	t.Helper()
	err := d.Do(ctx)
	if err != nil {
		t.Errorf("d=%s err=%v", d.String(), err)
	}
}
func DoCheckFatal(t testing.TB, d Doer, ctx context.Context) {
	t.Helper()
	if err := d.Do(ctx); err != nil {
		t.Fatalf("d=%s err=%v", d.String(), err)
	}
}
