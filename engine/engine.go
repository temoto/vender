package engine

import (
	"context"
	"fmt"
	"sync"

	"github.com/juju/errors"
	"github.com/temoto/vender/helpers"
	"github.com/temoto/vender/log2"
)

const ContextKey = "run/engine"

var EngineStop = errors.New("")

type Engine struct {
	Log     *log2.Log
	lk      sync.Mutex
	actions map[string]Doer
}

// Context[key] -> *Engine or panic
func ContextValueEngine(ctx context.Context, key interface{}) *Engine {
	v := ctx.Value(key)
	if v == nil {
		panic(fmt.Errorf("context['%v'] is nil", key))
	}
	if cfg, ok := v.(*Engine); ok {
		return cfg
	}
	panic(fmt.Errorf("context['%v'] expected type *Engine", key))
}

func NewEngine(ctx context.Context) *Engine {
	log := log2.ContextValueLogger(ctx, log2.ContextKey)
	self := &Engine{
		Log:     log,
		actions: make(map[string]Doer, 64),
	}
	return self
}

func (self *Engine) Register(action string, d Doer) {
	self.lk.Lock()
	self.actions[action] = d
	self.lk.Unlock()
}
func (self *Engine) Resolve(action string) Doer {
	d, ok := self.actions[action]
	if !ok {
		self.Log.Errorf("engine.Resolve action=%s not found", action)
	}
	return d
}

func (self *Engine) Execute(ctx context.Context, scenario *Scenario) error {
	resolve := func(key string) bool {
		if key == "" {
			return true
		}
		_, ok := self.actions[key]
		return ok
	}
	err := scenario.Validate(ctx, resolve)
	if err != nil {
		return errors.Trace(err)
	}

	if err = self.validate(ctx, scenario); err != nil {
		return errors.Trace(err)
	}

	tx, err := scenario.ToTransaction(ctx, func(action, nodeName string) Doer {
		return self.actions[action]
	})
	if err != nil {
		return errors.Trace(err)
	}
	return tx.Do(ctx)
}

// execute scenario validate actions
func (self *Engine) validate(ctx context.Context, scenario *Scenario) error {
	errs := make([]error, 0, len(scenario.idMap))

	for id, f := range scenario.funMap {
		if f.validate == "" {
			continue
		}
		if v, ok := self.actions[f.validate]; ok {
			err := v.Do(ctx)
			if err != nil {
				errs = append(errs, errors.Annotatef(err, "node=%s", id))
			}
		}
	}

	return helpers.FoldErrors(errs)
}
