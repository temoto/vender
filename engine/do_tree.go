package engine

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/juju/errors"
	"github.com/temoto/vender/helpers"
	"github.com/temoto/vender/helpers/msync"
	"github.com/temoto/vender/log2"
)

// Directional graph concurrent executor.
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

func (self *Tree) Validate() error {
	errs := make([]error, 0, 8)

	visited := make(map[*Node]struct{})
	self.Root.Walk(func(node *Node) bool {
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

	state := treeExecState{
		errch:  make(chan error, len(items)),
		failed: 0,
	}

	self.Root.done = msync.NewSignal()
	for _, n := range items {
		n.callers = 0
		n.done = msync.NewSignal()
	}
	state.wg.Add(1)
	treeWalkExec(ctx, &self.Root, &state)
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
	self.Root.Walk(func(node *Node) bool {
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
	self.Root.Walk(func(node *Node) bool {
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

type treeExecState struct {
	errch  chan error
	failed uint32
	wg     sync.WaitGroup
}

func treeWalkExec(ctx context.Context, node *Node, state *treeExecState) {
	log := log2.ContextValueLogger(ctx)

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
		go treeWalkExec(ctx, child, state)
	}
}
