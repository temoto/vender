package engine

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"sync"
	"time"

	"github.com/juju/errors"
	"github.com/temoto/vender/helpers"
	"github.com/temoto/vender/log2"
)

const ContextKey = "run/engine"

type Engine struct {
	Log     *log2.Log
	lk      sync.Mutex
	actions map[string]Doer
}

// Context[key] -> *Engine or panic
func GetEngine(ctx context.Context) *Engine {
	v := ctx.Value(ContextKey)
	if v == nil {
		panic(fmt.Errorf("context['%v'] is nil", ContextKey))
	}
	if cfg, ok := v.(*Engine); ok {
		return cfg
	}
	panic(fmt.Errorf("context['%v'] expected type *Engine", ContextKey))
}

func NewEngine(ctx context.Context) *Engine {
	log := log2.ContextValueLogger(ctx)
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

func (self *Engine) RegisterNewSeq(name string, ds ...Doer) {
	tx := NewSeq(name)
	for _, d := range ds {
		tx.Append(d)
	}
	self.Register(name, tx)
}

var reActionArg = regexp.MustCompile(`^(.+)\((\d+)\)$`)

func (self *Engine) resolve(action string) Doer {
	// self.Log.Debugf(action)
	// TODO RLock?
	d, ok := self.actions[action]
	if ok {
		return d
	}

	match := reActionArg.FindStringSubmatch(action)
	if match == nil {
		return nil
	}

	normalized := match[1] + "(?)"
	d, ok = self.actions[normalized]
	if !ok {
		self.Log.Errorf("resolve action=%s normalized=%s not found", action, normalized)
		return nil
	}
	argn, _ := strconv.Atoi(match[2])
	return ArgApply(d, Arg(argn))
}

func (self *Engine) Resolve(action string) Doer {
	d := self.resolve(action)
	if d == nil {
		self.Log.Errorf("engine.Resolve action=%s not found", action)
	}
	return d
}

func (self *Engine) List() []string {
	self.lk.Lock()
	r := make([]string, 0, len(self.actions))
	for k := range self.actions {
		r = append(r, k)
	}
	self.lk.Unlock()
	return r
}

var reSleep = regexp.MustCompile(`sleep\((\d+m?s)\)`)

func (self *Engine) ResolveOrLazy(action string) (Doer, error) {
	d, ok := self.actions[action]
	if ok {
		return d, nil
	}

	if m := reSleep.FindStringSubmatch(action); len(m) == 2 {
		duration, err := time.ParseDuration(m[1])
		if err != nil {
			return nil, errors.Trace(err)
		}
		return Sleep{duration}, nil
	}

	// aliases are not subject to lazy proxy
	// FIXME
	// if strings.HasPrefix(action, "@") {
	// 	return nil, errors.Errorf("alias=%s not resolved", action)
	// }

	return &LazyResolve{Name: action, r: self.resolve}, nil
}
func (self *Engine) MustResolveOrLazy(action string) Doer {
	d, err := self.ResolveOrLazy(action)
	if err != nil {
		return Fail{err}
	}
	return d
}

var reNotSpace = regexp.MustCompile(`\S+`)

func (self *Engine) ParseText(tag, text string) (Doer, error) {
	errs := make([]error, 0)
	words := reNotSpace.FindAllString(text, -1)

	tx := NewSeq(tag)
	for _, word := range words {
		d, err := self.ResolveOrLazy(word)
		if err != nil {
			return nil, errors.Annotatef(err, "scenario=%s unparsed=%s", text, word)
		}
		tx.Append(d)
	}
	return tx, helpers.FoldErrors(errs)
}
