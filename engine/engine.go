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

type Engine struct {
	Log     *log2.Log
	lk      sync.RWMutex
	actions map[string]Doer
}

func NewEngine(log *log2.Log) *Engine {
	self := &Engine{
		Log:     log,
		actions: make(map[string]Doer, 128),
	}
	self.actions["ignore(?)"] = FuncArg{
		Name: "ignore(?)",
		F:    func(context.Context, Arg) error { return nil }}
	return self
}

func (self *Engine) Register(action string, d Doer) {
	self.lk.Lock()
	self.actions[action] = d
	self.lk.Unlock()
}

func (self *Engine) RegisterNewFunc(name string, fun func(context.Context) error) {
	self.Register(name, Func{
		Name: name,
		F:    fun,
	})
}

func (self *Engine) RegisterNewSeq(name string, ds ...Doer) {
	tx := NewSeq(name)
	for _, d := range ds {
		tx.Append(d)
	}
	self.Register(name, tx)
}

func (self *Engine) RegisterParse(name, scenario string) error {
	d, err := self.ParseText(name, scenario)
	if err != nil {
		err = errors.Annotatef(err, "engine.RegisterParse() name=%s scenario=%s", name, scenario)
		return err
	}
	self.Register(name, d)
	return nil
}

var reActionArg = regexp.MustCompile(`^(.+)\((\d+)\)$`)

func (self *Engine) resolve(action string) Doer {
	// self.Log.Debugf(action)
	self.lk.RLock()
	defer self.lk.RUnlock()
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
	self.lk.RLock()
	r := make([]string, 0, len(self.actions))
	for k := range self.actions {
		r = append(r, k)
	}
	self.lk.RUnlock()
	return r
}

var reSleep = regexp.MustCompile(`sleep\((\d+m?s)\)`)

func (self *Engine) ResolveOrLazy(action string) (Doer, error) {
	self.lk.RLock()
	defer self.lk.RUnlock()
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

	return &Lazy{Name: action, r: self.resolve}, nil
}
func (self *Engine) MustResolveOrLazy(action string) Doer {
	d, err := self.ResolveOrLazy(action)
	if err != nil {
		return Fail{E: err}
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

func (self *Engine) ExecList(ctx context.Context, tag string, list []string) error {
	self.Log.Debugf("engine.ExecList tag=%s list=%v", tag, list)

	errs := make([]error, 0)
	for i, text := range list {
		itemTag := fmt.Sprintf("%s:%d", tag, i)
		d, err := self.ParseText(itemTag, text)
		if err == nil {
			err = d.Validate()
		}
		if err == nil {
			err = d.Do(ctx)
		}
		if err != nil {
			errs = append(errs, err)
		}
	}
	return helpers.FoldErrors(errs)
}
