package engine

import (
	"context"
	"fmt"

	"github.com/temoto/errors"
)

var ErrArgNotApplied = errors.Errorf("Argument is not applied")
var ErrArgOverwrite = errors.Errorf("Argument already applied")

func ArgApply(d Doer, a Arg) Doer {
	return ForceLazy(d).(ArgApplier).Apply(a)
}

type Arg int32 // maybe interface{}
type ArgApplier interface {
	Apply(a Arg) Doer
	Applied() bool
}

func (Fail) Applied() bool    { return true }
func (f Fail) Apply(Arg) Doer { return f }

type FuncArg struct {
	Name string
	F    func(context.Context, Arg) error
	V    ValidateFunc
	arg  Arg
	set  bool
}

func (self FuncArg) Validate() error {
	if !self.set {
		return ErrArgNotApplied
	}
	return useValidator(self.V)
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
	// Copied already because (self FuncArg) not pointer receiver
	// this is redundant line to make copy clear for reading.
	copied := self
	copied.arg = a
	copied.set = true
	return copied
}
func (self FuncArg) Applied( /*TODO arg name?*/ ) bool { return self.set }

// Apply makes copy of Seq, applying `arg` to exactly one (first) placeholder.
// TODO possible roadmap:
// - move Seq.(ArgApplier) to generic ArgApply() using Iter/Mapper implemented by Seq/Tree
func (seq *Seq) Apply(arg Arg) Doer {
	result := &Seq{
		name:  seq.name,
		items: make([]Doer, len(seq.items)),
	}

	found := -1
	for i, child := range seq.items {
		result.items[i] = child

		forced := ForceLazy(child)
		if x, ok := forced.(ArgApplier); ok && !x.Applied() {
			if found == -1 {
				found = i
				result.items[i] = x.Apply(arg)
			} else {
				panic(fmt.Sprintf("code error Seq.Apply: multiple arg placeholders in %s at %#v", seq.String(), forced))
			}
		}
	}
	if found == -1 {
		panic(fmt.Sprintf("code error Seq.Apply: no arg placeholders in %s", seq.String()))
	}

	return result
}
func (seq *Seq) Applied( /*TODO arg name?*/ ) bool {
	result := true
	for _, d := range seq.items {
		if x, ok := d.(ArgApplier); ok && !x.Applied() {
			return false
		}
	}
	return result
}

func (self *RestartError) Apply(a Arg) Doer {
	d := ForceLazy(self.Doer)
	if x, ok := d.(ArgApplier); ok {
		if !x.Applied() { // success path
			return &RestartError{
				Doer:  x.Apply(a),
				Check: self.Check,
				Reset: self.Reset,
			}
		} else {
			return Fail{E: ErrArgOverwrite}
		}
	} else {
		panic(fmt.Sprintf("code error RestartError.Apply: no arg placeholders in %s", self.String()))
	}
}
func (self *RestartError) Applied( /*TODO arg name?*/ ) bool {
	if x, ok := self.Doer.(ArgApplier); ok && !x.Applied() {
		return false
	}
	return true
}

type IgnoreArg struct{ Doer }

func (self IgnoreArg) Apply(Arg) Doer { return self.Doer }
func (IgnoreArg) Applied() bool       { return false }

// compile-time interface checks
var _ ArgApplier = &RestartError{}
var _ ArgApplier = &Seq{}
var _ ArgApplier = FuncArg{}
var _ ArgApplier = IgnoreArg{}
