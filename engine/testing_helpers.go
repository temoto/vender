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

func DoCheckError(t testing.TB, d Doer, ctx context.Context) error {
	t.Helper()
	if err := d.Do(ctx); err != nil {
		t.Errorf("d=%s err=%v", d.String(), err)
		return err
	}
	return nil
}
func DoCheckFatal(t testing.TB, d Doer, ctx context.Context) {
	t.Helper()
	if err := d.Do(ctx); err != nil {
		t.Fatalf("d=%s err=%v", d.String(), err)
	}
}
