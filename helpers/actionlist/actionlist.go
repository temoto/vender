// Execute list of func(Context)error concurrently.
// It it, basically, a specialized parMap.
// All methods are thread-safe.
package actionlist

import (
	"context"
	"sync"

	"github.com/juju/errors"
)

type Func func(context.Context) error
type TaggedFunc struct {
	f   Func
	tag string
}

type List struct {
	lk    sync.Mutex
	items []TaggedFunc
}

func (self *List) Append(fun Func, tag string) {
	self.lk.Lock()
	self.items = append(self.items, TaggedFunc{fun, tag})
	self.lk.Unlock()
}

func (self *List) Do(ctx context.Context) []error {
	self.lk.Lock()
	defer self.lk.Unlock()

	errCh := make(chan error, len(self.items))
	for i := range self.items {
		go self.doOne(ctx, &self.items[i], errCh)
	}
	var errs []error
	for range self.items {
		if e := <-errCh; e != nil {
			errs = append(errs, e)
		}
	}
	return errs
}

func (self *List) doOne(ctx context.Context, tf *TaggedFunc, ch chan<- error) {
	if err := tf.f(ctx); err != nil {
		// errors.Annotate without call location
		tagged := errors.NewErrWithCause(err, tf.tag)
		ch <- &tagged
	} else {
		ch <- nil
	}
}
