package engine

import (
	"context"

	"github.com/temoto/errors"
	"github.com/temoto/vender/helpers"
)

// Sequence executor. Specialized version of Tree for performance.
// Error in one action aborts whole group.
// Build graph with NewSeq().Append()
type Seq struct {
	name  string
	_b    [8]Doer
	items []Doer
}

func NewSeq(name string) *Seq {
	self := &Seq{name: name}
	self.items = self._b[:0]
	return self
}

func (self *Seq) Append(d Doer) *Seq {
	self.items = append(self.items, d)
	return self
}

func (self *Seq) Validate() error {
	errs := make([]error, 0, len(self.items))

	for _, d := range self.items {
		if err := d.Validate(); err != nil {
			err = errors.Annotatef(err, "node=%s validate", d.String())
			errs = append(errs, err)
		}
	}

	return helpers.FoldErrors(errs)
}

func (self *Seq) Do(ctx context.Context) error {
	for _, d := range self.items {
		if err := d.Do(ctx); err != nil {
			return err
		}
	}
	return nil
}

func (self *Seq) String() string {
	return self.name
}
