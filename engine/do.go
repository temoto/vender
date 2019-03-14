package engine

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/temoto/vender/helpers"
	"github.com/temoto/vender/helpers/msync"
	"github.com/temoto/vender/log2"
)

type Doer interface {
	Do(context.Context) error
	String() string // debug
}

// Graph executor.
// Error in one action aborts whole group.
// Build graph with NewTransaction().Root.Append()
type Transaction struct {
	Root Node
}

func NewTransaction(name string) *Transaction {
	return &Transaction{
		Root: Node{Doer: Nothing{name}},
	}
}

type Nothing struct{ Name string }

func (self Nothing) Do(ctx context.Context) error { return nil }
func (self Nothing) String() string               { return self.Name }

type execState struct {
	errch  chan error
	failed uint32
	wg     sync.WaitGroup
}

func (self *Transaction) Do(ctx context.Context) error {
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

	errs := make([]error, len(state.errch))
	for i := 0; i < len(state.errch); i++ {
		errs[i] = <-state.errch
	}
	return helpers.FoldErrors(errs)
}

func (self *Transaction) String() string {
	if self == nil {
		return "nil"
	}
	return self.Root.String()
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
		// texec := time.Now().Sub(tbegin)
		// log texec
		if err != nil {
			atomic.AddUint32(&state.failed, 1)
			state.errch <- err
			return
		}
	}
	state.wg.Add(len(node.children))
	for _, child := range node.children {
		// log.Printf("walk child %v", child)
		go walkExec(ctx, child, state)
	}
}

type Func struct {
	Name string
	F    func(context.Context) error
}

func (self Func) Do(ctx context.Context) error { return self.F(ctx) }

// reflect.ValueOf()+runtime.FuncForPC().Name()
func (self Func) String() string { return "Func=" + self.Name }

type Func0 struct {
	Name string
	F    func() error
}

func (self Func0) Do(ctx context.Context) error { return self.F() }

// reflect.ValueOf()+runtime.FuncForPC().Name()
func (self Func0) String() string { return "Func=" + self.Name }

type Sleep struct{ time.Duration }

func (self Sleep) Do(ctx context.Context) error {
	time.Sleep(self.Duration)
	return nil
}
func (self Sleep) String() string { return fmt.Sprintf("Sleep(%v)", self.Duration) }

type RepeatN struct {
	N uint
	D Doer
}

func (self RepeatN) String() string { return fmt.Sprintf("RepeatN(N=%d D=%s)", self.N, self.D.String()) }
func (self RepeatN) Do(ctx context.Context) error {
	log := log2.ContextValueLogger(ctx, log2.ContextKey)
	var err error
	for i := uint(1); i <= self.N && err == nil; i++ {
		log.Debugf("engine loop %d/%d", i, self.N)
		err = self.D.Do(ctx)
	}
	return err
}

type mockdo struct {
	name   string
	called int32
	err    error
	lk     sync.Mutex
	last   time.Time
}

func (self *mockdo) Do(ctx context.Context) error {
	self.lk.Lock()
	self.called += 1
	self.last = time.Now()
	self.lk.Unlock()
	return self.err
}
func (self *mockdo) String() string { return self.name }
