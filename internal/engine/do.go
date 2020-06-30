package engine

import (
	"context"
	"fmt"
	"time"

	"github.com/juju/errors"
	"github.com/temoto/vender/log2"
)

const FmtErrContext = "`%s`" // errors.Annotatef(err, FmtErrContext, doer.String())

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
		err = GetGlobal(ctx).ExecPart(ctx, self.D)
	}
	return err
}
func (self RepeatN) String() string {
	return fmt.Sprintf("RepeatN(N=%d D=%s)", self.N, self.D.String())
}

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

type RestartError struct {
	Doer
	Check func(error) bool
	Reset Doer
}

func (self *RestartError) Validate() error { return self.Doer.Validate() }
func (self *RestartError) Do(ctx context.Context) error {
	first := GetGlobal(ctx).ExecPart(ctx, self.Doer)
	if first != nil {
		if self.Check(first) {
			resetErr := GetGlobal(ctx).ExecPart(ctx, self.Reset)
			if resetErr != nil {
				return errors.Wrap(first, resetErr)
			}
			return GetGlobal(ctx).ExecPart(ctx, self.Doer)
		}
	}
	return first
}
func (self *RestartError) String() string { return self.Doer.String() }
