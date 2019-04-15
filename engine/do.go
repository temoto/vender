package engine

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/juju/errors"
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
	log := log2.ContextValueLogger(ctx, log2.ContextKey)
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

var ErrArgNotApplied = errors.Errorf("Argument is not applied")
var ErrArgOverwrite = errors.Errorf("Argument already applied")

type Arg int32 // maybe interface{}
type ArgFunc func(context.Context, Arg) error
type ArgApplier interface {
	Apply(a Arg) Doer
	Applied() bool
}
type FuncArg struct {
	Name string
	F    func(context.Context, Arg) error
	arg  Arg
	set  bool
}

func (self FuncArg) Validate() error {
	if !self.set {
		return ErrArgNotApplied
	}
	return nil
}
func (self FuncArg) Do(ctx context.Context) error {
	if !self.set {
		return ErrArgNotApplied
	}
	return self.F(ctx, self.arg)
}
func (self FuncArg) String() string {
	if !self.set {
		return fmt.Sprintf("%s:Arg?", self.Name)
	}
	return fmt.Sprintf("%s:%v", self.Name, self.arg)
}
func (self FuncArg) Apply(a Arg) Doer {
	if self.set {
		return Fail{E: ErrArgOverwrite}
	}
	self.arg = a
	self.set = true
	return self
}
func (self FuncArg) Applied() bool { return self.set }

func ArgApply(d Doer, a Arg) Doer { return d.(ArgApplier).Apply(a) }

type mockdo struct {
	name   string
	called int32
	err    error
	lk     sync.Mutex
	last   time.Time
	v      ValidateFunc
}

func (self *mockdo) Validate() error { return useValidator(self.v) }
func (self *mockdo) Do(ctx context.Context) error {
	self.lk.Lock()
	self.called += 1
	self.last = time.Now()
	self.lk.Unlock()
	return self.err
}
func (self *mockdo) String() string { return self.name }
