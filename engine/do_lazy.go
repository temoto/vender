package engine

import (
	"context"
	"sync"

	"github.com/juju/errors"
)

func Force(d Doer) (Doer, bool, error) {
	if f, ok := d.(Forcer); ok {
		new, forced, err := f.Force()
		// log.Printf("Force d=%s -> new=%v forced=%t err=%v", d.String(), new, forced, err)
		if err != nil {
			return nil, forced, errors.Annotatef(err, "Force %s", d.String())
		}
		return new, forced, nil
	}
	// log.Printf("Force d=%s -> NA", d.String())
	return d, false, nil
}

type Forcer interface{ Force() (Doer, bool, error) }

type Lazy struct {
	Name  string
	mu    sync.Mutex
	r     func(string) (Doer, error)
	cache Doer
}

func (l *Lazy) Force() (d Doer, forced bool, err error) {
	l.mu.Lock()
	d = l.cache
	if d == nil {
		d, err = l.r(l.Name)
		if err == nil {
			// log.Printf("lazy.force store %#v", d)
			l.cache = d
		}
	}
	l.mu.Unlock()
	return
}

func (l *Lazy) Validate() error {
	d, _, err := l.Force()
	if err != nil {
		return err
	}
	return d.Validate()
}

func (l *Lazy) Do(ctx context.Context) error {
	d, _, err := l.Force()
	if err != nil {
		return err
	}
	return d.Do(ctx)
}

func (l *Lazy) String() string { return l.Name }

func (seq *Seq) Force() (Doer, bool, error) {
	result := seq.cloneEmpty()
	forcedAny := false
	for _, child := range seq.items {
		new, forced, err := Force(child)
		if err != nil {
			return nil, forced, errors.Annotatef(err, FmtErrContext, child.String())
		}
		forcedAny = forcedAny || forced
		result.Append(new)
	}
	if !forcedAny {
		seq.setItems(result.items)
		return seq, false, nil
	}
	return result, true, nil
}
