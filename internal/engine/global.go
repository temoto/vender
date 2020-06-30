package engine

import (
	"context"
	"fmt"
)

const ContextKey = "run/engine"

func GetGlobal(ctx context.Context) *Engine {
	v := ctx.Value(ContextKey)
	if v == nil {
		panic(fmt.Sprintf("context['%s'] is nil", ContextKey))
	}
	if g, ok := v.(*Engine); ok {
		return g
	}
	panic(fmt.Sprintf("context['%s'] expected type *Engine actual=%#v", ContextKey, v))
}
