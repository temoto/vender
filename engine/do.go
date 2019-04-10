package engine

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/juju/errors"
	"github.com/temoto/vender/helpers"
	"github.com/temoto/vender/helpers/msync"
	"github.com/temoto/vender/log2"
)

type Doer interface {
	Validate() error
	Do(context.Context) error
	String() string // for logs
}

// Graph executor.
// Error in one action aborts whole group.
// Build graph with NewTree().Root.Append()
type Tree struct {
	Root Node
}

func NewTree(name string) *Tree {
	return &Tree{
		Root: Node{Doer: Nothing{name}},
	}
}

type Nothing struct{ Name string }

func (self Nothing) Do(ctx context.Context) error { return nil }
func (self Nothing) Validate() error              { return nil }
func (self Nothing) String() string               { return self.Name }

func (self *Tree) Validate() error {
	errs := make([]error, 0, 8)

	visited := make(map[*Node]struct{})
	walk(&self.Root, func(node *Node) bool {
		if _, ok := visited[node]; !ok {
			visited[node] = struct{}{}
			d := node.Doer
			if err := d.Validate(); err != nil {
				err = errors.Annotatef(err, "node=%s validate", d.String())
				errs = append(errs, err)
			}
		}
		return false
	})

	if len(errs) != 0 {
		return helpers.FoldErrors(errs)
	}
	return nil
}

func (self *Tree) Do(ctx context.Context) error {
	items := make([]*Node, 0, 32)
	self.Root.Collect(&items)

	state := execState{
		errch:  make(chan error, len(items)),
		failed: 0,
	}

	self.Root.done = msync.NewSignal()
	for _, n := range items {
		n.callers = 0
		n.done = msync.NewSignal()
	}
	state.wg.Add(1)
	walkExec(ctx, &self.Root, &state)
	state.wg.Wait()
	close(state.errch)

	errs := make([]error, 0, len(items))
	for i := 0; i < len(state.errch); i++ {
		errs = append(errs, <-state.errch)
	}
	return helpers.FoldErrors(errs)
}

func (self *Tree) String() string {
	return self.Root.Doer.String()
}

func (self *Tree) Apply(arg Arg) Doer {
	var found *Node
	visited := make(map[*Node]struct{})
	walk(&self.Root, func(node *Node) bool {
		if _, ok := visited[node]; !ok {
			visited[node] = struct{}{}
			if x, ok := node.Doer.(ArgApplier); !ok || x.Applied() {
				return false
			}
			if found == nil {
				found = node
			} else {
				panic(fmt.Sprintf("code error Tree.Apply: multiple arg placeholders in %s", self.String()))
			}
		}
		return false
	})
	if found == nil {
		panic(fmt.Sprintf("code error Tree.Apply: no arg placeholders in %s", self.String()))
	}
	found.Doer = found.Doer.(ArgApplier).Apply(arg)

	return self
}
func (self *Tree) Applied( /*TODO arg name?*/ ) bool {
	result := true
	visited := make(map[*Node]struct{})
	walk(&self.Root, func(node *Node) bool {
		if _, ok := visited[node]; !ok {
			visited[node] = struct{}{}
			if x, ok := node.Doer.(ArgApplier); ok && !x.Applied() {
				result = false
				return true
			}
		}
		return false
	})
	return result
}

type execState struct {
	errch  chan error
	failed uint32
	wg     sync.WaitGroup
}

func walkExec(ctx context.Context, node *Node, state *execState) {
	log := log2.ContextValueLogger(ctx, log2.ContextKey)

	defer state.wg.Done()
	nc := atomic.AddInt32(&node.callers, 1)
	if nc <= 0 {
		panic("code error, node.callers <= 0")
	} else if nc > 1 {
		// FIXME walk graph without duplicates
		// then this state is sign of concurrent execution
		// log.Printf("cancel dup exec %v", node)
		return
	}
	// log.Printf("walk %v", node)
	defer close(node.done)
	for _, p := range node.parents {
		<-p.done
	}
	if atomic.LoadUint32(&state.failed) == 0 {
		// TODO concurrency limit _after_ wait
		// tbegin := time.Now()
		// log.Printf("exec %#v", node)
		var err error
		if _, ok := node.Doer.(Nothing); !ok {
			log.Debugf("engine execute %s", node)
			err = node.Do(ctx)
		}
		// texec := time.Since(tbegin)
		// log texec
		if err != nil {
			atomic.AddUint32(&state.failed, 1)
			state.errch <- err
			return
		}
	}
	state.wg.Add(len(node.children))
	for _, child := range node.children {
		// log.Printf("engine walk child %v", child)
		go walkExec(ctx, child, state)
	}
}

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

func DoCheckError(t testing.TB, d Doer, ctx context.Context) error {
	t.Helper()
	if err := d.Do(ctx); err != nil {
		t.Errorf("d=%s err=%v", d.String(), err)
		return err
	}
	return nil
}
func DoCheckFatal(t testing.TB, d Doer, ctx context.Context) {
	t.Helper()
	if err := d.Do(ctx); err != nil {
		t.Fatalf("d=%s err=%v", d.String(), err)
	}
}
