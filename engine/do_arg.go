package engine

import (
	"context"
	"fmt"

	"github.com/juju/errors"
)

var ErrArgNotApplied = errors.Errorf("Argument is not applied")
var ErrArgOverwrite = errors.Errorf("Argument already applied")

type MaybeBool uint8

func ArgApply(d Doer, arg Arg) (Doer, bool, error) {
	// log.Printf("ArgApply d=%s arg=%v", d.String(), arg)
	var err error
	d, _, err = Force(d)
	if err != nil {
		return nil, false, err
	}
	if aa, ok := d.(ArgApplier); ok {
		return aa.Apply(arg)
	}
	return d, false, nil
}

type Arg int32 // maybe interface{}
type ArgApplier interface {
	Apply(a Arg) (Doer, bool, error)
}

type FuncArg struct {
	Name string
	F    func(context.Context, Arg) error
	V    ValidateFunc
	arg  Arg
	set  bool
}

func (fa FuncArg) Validate() error {
	if !fa.set {
		return errors.Annotatef(ErrArgNotApplied, FmtErrContext, fa.Name)
	}
	return useValidator(fa.V)
}
func (fa FuncArg) Do(ctx context.Context) error {
	if !fa.set {
		return errors.Annotatef(ErrArgNotApplied, FmtErrContext, fa.Name)
	}
	return fa.F(ctx, fa.arg)
}
func (fa FuncArg) String() string {
	if !fa.set {
		return fmt.Sprintf("%s:Arg?", fa.Name)
	}
	return fmt.Sprintf("%s:%v", fa.Name, fa.arg)
}

func (fa FuncArg) Apply(a Arg) (Doer, bool, error) {
	if fa.set {
		return nil, false, errors.Annotatef(ErrArgOverwrite, FmtErrContext, fa.Name)
	}
	// Copied already because (fa FuncArg) not pointer receiver
	// this is redundant line to make copy clear for reading.
	copied := fa
	copied.arg = a
	copied.set = true
	return copied, true, nil
}

// Apply makes copy of Seq, applying `arg` to exactly one (first) placeholder.
func (seq *Seq) Apply(arg Arg) (Doer, bool, error) {
	result := seq.cloneEmpty()
	found := false
	places := uint(0)
	for _, child := range seq.items {
		if found {
			result.Append(child)
			continue
		}
		new, applied, err := ArgApply(child, arg)
		// log.Printf("- %s child=%s arg=%v -> applied=%t err=%v new=%#v", seq.String(), child.String(), arg, applied, err, new)
		switch errors.Cause(err) {
		case nil: // success path
			places++
			found = applied
			result.Append(new)

		case ErrArgOverwrite:
			places++
			result.Append(child)

		case ErrArgNotApplied:
			places++
			result.Append(child)

		default:
			return nil, false, errors.Annotatef(err, FmtErrContext, seq.String())
		}
	}
	if !found && places > 0 {
		return nil, false, errors.Annotatef(ErrArgNotApplied, FmtErrContext, seq.String())
	}
	return result, true, nil
}

func (self *RestartError) Apply(arg Arg) (Doer, bool, error) {
	new, applied, err := ArgApply(self.Doer, arg)
	if err != nil {
		return nil, false, err
	}
	if !applied {
		return self, false, nil
	}
	re := &RestartError{
		Doer:  new,
		Check: self.Check,
		Reset: self.Reset,
	}
	return re, true, nil
}

type IgnoreArg struct{ Doer }

func (self IgnoreArg) Apply(Arg) (Doer, bool, error) { return self.Doer, true, nil }

// compile-time interface checks
var _ ArgApplier = &RestartError{}
var _ ArgApplier = &Seq{}
var _ ArgApplier = FuncArg{}
var _ ArgApplier = IgnoreArg{}
