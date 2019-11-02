package engine

import (
	"context"
	"sync"

	"github.com/juju/errors"
)

func ForceLazy(d Doer) (Doer, bool) {
	if lazy, ok := d.(Forcer); ok {
		return lazy.Force()
	}
	return d, false
}

type Forcer interface{ Force() (Doer, bool) }

type Lazy struct {
	Name  string
	mu    sync.Mutex
	r     func(string) Doer
	cache Doer
}

const errLazyNotResolved = "lazy action=%s not resolved"

func (self *Lazy) Force() (d Doer, ok bool) {
	self.mu.Lock()
	d = self.cache
	if d == nil {
		d = self.r(self.Name)
		if d != nil {
			self.cache = d
			ok = true
		}
	}
	self.mu.Unlock()
	return
}

func (self *Lazy) Validate() error {
	if d, _ := self.Force(); d != nil {
		return d.Validate()
	}
	return errors.Errorf(errLazyNotResolved, self.Name)
}
func (self *Lazy) Do(ctx context.Context) error {
	if d, _ := self.Force(); d != nil {
		return d.Do(ctx)
	}
	return errors.Errorf(errLazyNotResolved, self.Name)
}
func (self *Lazy) String() string { return self.Name }

func (seq *Seq) Force() (Doer, bool) {
	result := &Seq{
		name:  seq.name,
		items: make([]Doer, len(seq.items)),
	}
	forcedAny := false

	for i, child := range seq.items {
		result.items[i] = child
		if forced, ok := ForceLazy(child); ok {
			result.items[i] = forced
			forcedAny = true
		}
	}

	if !forcedAny {
		result = seq
	}
	return result, forcedAny
}
