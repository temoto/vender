package engine

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/temoto/errors"
	"github.com/temoto/vender/log2"
)

type Doer interface {
	Validate() error
	Do(context.Context) error
	String() string // for logs
}

type Nothing struct{ Name string }

func (self Nothing) Do(ctx context.Context) error { return nil }
func (self Nothing) Validate() error              { return nil }
func (self Nothing) String() string               { return self.Name }

type Func struct {
	Name string
	F    func(context.Context) error
	V    ValidateFunc
}

func (self Func) Validate() error              { return useValidator(self.V) }
func (self Func) Do(ctx context.Context) error { return self.F(ctx) }
func (self Func) String() string               { return self.Name }

type Func0 struct {
	Name string
	F    func() error
	V    ValidateFunc
}

func (self Func0) Validate() error              { return useValidator(self.V) }
func (self Func0) Do(ctx context.Context) error { return self.F() }
func (self Func0) String() string               { return self.Name }

type Sleep struct{ time.Duration }

func (self Sleep) Validate() error              { return nil }
func (self Sleep) Do(ctx context.Context) error { time.Sleep(self.Duration); return nil }
func (self Sleep) String() string               { return fmt.Sprintf("Sleep(%v)", self.Duration) }

type RepeatN struct {
	N uint
	D Doer
}

func (self RepeatN) Validate() error { return self.D.Validate() }
func (self RepeatN) Do(ctx context.Context) error {
	// FIXME solve import cycle, use GetGlobal(ctx).Log
	log := log2.ContextValueLogger(ctx)
	var err error
	for i := uint(1); i <= self.N && err == nil; i++ {
		log.Debugf("engine loop %d/%d", i, self.N)
		err = self.D.Do(ctx)
	}
	return err
}
func (self RepeatN) String() string { return fmt.Sprintf("RepeatN(N=%d D=%s)", self.N, self.D.String()) }

type ValidateFunc func() error

func useValidator(v ValidateFunc) error {
	if v == nil {
		return nil
	}
	return v()
}

type Fail struct{ E error }

func (self Fail) Validate() error              { return self.E }
func (self Fail) Do(ctx context.Context) error { return self.E }
func (self Fail) String() string               { return self.E.Error() }

type Lazy struct {
	Name  string
	mu    sync.Mutex
	r     func(string) Doer
	cache Doer
}

const errLazyNotResolved = "lazy action=%s not resolved"

func (self *Lazy) Resolve() Doer {
	self.mu.Lock()
	d := self.cache
	if d == nil {
		d = self.r(self.Name)
		if d != nil {
			self.cache = d
		}
	}
	self.mu.Unlock()
	return d
}

func (self *Lazy) Validate() error {
	if d := self.Resolve(); d != nil {
		return d.Validate()
	}
	return errors.Errorf(errLazyNotResolved, self.Name)
}
func (self *Lazy) Do(ctx context.Context) error {
	if d := self.Resolve(); d != nil {
		return d.Do(ctx)
	}
	return errors.Errorf(errLazyNotResolved, self.Name)
}
func (self *Lazy) String() string { return self.Name }

func ForceLazy(d Doer) Doer {
	if lazy, ok := d.(*Lazy); ok {
		return lazy.Resolve()
	}
	return d
}

type RestartError struct {
	Doer
	Check func(error) bool
	Reset Doer
}

func (self *RestartError) Validate() error { return self.Doer.Validate() }
func (self *RestartError) Do(ctx context.Context) error {
	first := self.Doer.Do(ctx)
	if first != nil {
		if self.Check(first) {
			resetErr := self.Reset.Do(ctx)
			if resetErr != nil {
				return errors.Wrap(first, resetErr)
			}
			return self.Doer.Do(ctx)
		}
	}
	return first
}
func (self *RestartError) String() string { return self.Doer.String() }
