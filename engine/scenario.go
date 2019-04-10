package engine

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/juju/errors"
	"github.com/temoto/vender/helpers"
)

type sNode struct {
	id     string
	kind   NodeKind
	parent string
	edges  []string
}

// TODO FIXME stringer
func (self *sNode) Tag() string {
	switch self.kind {
	case NodeInvalid:
		panic(fmt.Sprintf("code error node=%s kind=invalid", self.id))
	case NodeDoer:
		return "NodeDo"
	case NodeBlock:
		return "Block"
	case NodeSpecial:
		return "Special"
	default:
		panic(fmt.Sprintf("code error node=%s kind=%d unknown value", self.id, self.kind))
	}
}
func (self *sNode) String() string {
	return fmt.Sprintf("%s(id=%s/%s edges=%s)",
		self.Tag(), self.parent, self.id, strings.Join(self.edges, ","))
}

type nodeFuns struct {
	validate string // once just before executing whole scenario
	run      string
	enter    []string // each time block is entered
	leave    []string // each time block is left
}

func (self *nodeFuns) Empty() bool {
	if self == nil {
		return true
	}
	return len(self.validate) == 0 && len(self.run) == 0 && len(self.enter) == 0 && len(self.leave) == 0
}
func (self *nodeFuns) check(resolve func(key string) bool) error {
	if self.Empty() {
		return nil
	}
	errs := make([]error, 0)
	if !resolve(self.validate) {
		errs = append(errs, errors.NotFoundf("validate=%s", self.validate))
	}
	if !resolve(self.run) {
		errs = append(errs, errors.NotFoundf("run=%s", self.run))
	}
	for i, x := range self.enter {
		if !resolve(x) {
			errs = append(errs, errors.NotFoundf("enter[%d]=%s", i, x))
		}
	}
	for i, x := range self.leave {
		if !resolve(x) {
			errs = append(errs, errors.NotFoundf("leave[%d]=%s", i, x))
		}
	}
	return helpers.FoldErrors(errs)
}

type Scenario struct {
	name    string
	rootId  string
	idMap   map[string]*sNode
	funMap  map[string]*nodeFuns
	parents map[string][]string
}

func NewScenario(name string) *Scenario {
	s := &Scenario{
		name:    name,
		idMap:   make(map[string]*sNode),
		funMap:  make(map[string]*nodeFuns),
		parents: make(map[string][]string),
	}
	s.idMap[""] = &sNode{kind: NodeBlock} // implicit root node
	return s
}

func (self *Scenario) Add(node *sNode) error {
	if _, ok := self.idMap[node.id]; ok {
		return errors.AlreadyExistsf("node=%s", node.id)
	}
	self.idMap[node.id] = node
	if self.rootId == "" {
		self.rootId = node.id
	}
	return nil
}

func (self *Scenario) Edge(src, dst string) error {
	srcNode := self.Get(src)
	srcNode.edges = append(srcNode.edges, dst)
	self.parents[dst] = append(self.parents[dst], src)
	return nil
}

func (self *Scenario) Funs(id string) *nodeFuns {
	fs, ok := self.funMap[id]
	if !ok {
		fs = new(nodeFuns)
		self.funMap[id] = fs
	}
	return fs
}

func (self *Scenario) SetValidate(id string, action string) {
	fs := self.Funs(id)
	fs.validate = action
}
func (self *Scenario) SetRun(id string, action string) {
	fs := self.Funs(id)
	fs.run = action
}
func (self *Scenario) AddEnter(id string, action string) {
	fs := self.Funs(id)
	fs.enter = append(fs.enter, action)
}
func (self *Scenario) AddLeave(id string, action string) {
	fs := self.Funs(id)
	fs.leave = append(fs.leave, action)
}

func (self *Scenario) Get(name string) *sNode {
	return self.idMap[name]
}

func (self *Scenario) Walk(fNode func(*sNode) bool, fEdge func(*sNode, *sNode) bool) {
	visited := newStringSet()
	visited.Add("")
	if fNode != nil && !fNode(self.idMap[""]) {
		return
	}
	self.walk(visited, self.idMap[self.rootId], fNode, fEdge)
}
func (self *Scenario) walkVisitNode(visited stringSet, n *sNode, fNode func(*sNode) bool) bool {
	// log.Printf("n=%v", n)
	if visited.Add(n.id) {
		if !self.walkVisitNode(visited, self.Get(n.parent), fNode) {
			return false
		}
		if fNode != nil {
			return fNode(n)
		}
	}
	return true
}
func (self *Scenario) walk(visited stringSet, current *sNode, fNode func(*sNode) bool, fEdge func(*sNode, *sNode) bool) {
	if !self.walkVisitNode(visited, current, fNode) {
		return
	}

	for _, tailId := range current.edges {
		wasNew := !visited.Has(tailId)
		tail := self.Get(tailId)
		if !self.walkVisitNode(visited, tail, fNode) {
			return
		}
		if fEdge != nil && !fEdge(current, tail) {
			return
		}
		// cycle protection
		if wasNew {
			self.walk(visited, tail, fNode, fEdge)
		}
	}
}

func (self *Scenario) Validate(ctx context.Context, resolve func(key string) bool) error {
	errs := make([]error, 0, 32)

	self.Walk(func(node *sNode) bool {
		f := self.Funs(node.id)
		switch node.kind {
		case NodeDoer:
			if f.run == "" && f.validate == "" {
				errs = append(errs, errors.NotValidf("node=%s kind=doer no run/validate actions attached", node.id))
			}
		case NodeBlock:
			if f.run != "" {
				errs = append(errs, errors.NotValidf("node=%s kind=block can't be run", node.id))
			}
		case NodeSpecial:
			if !f.Empty() {
				errs = append(errs, errors.NotValidf("node=%s kind=special can't have actions", node.id))
			}
		default:
			errs = append(errs, errors.NotValidf("node=%s kind=%d", node.id, node.kind))
		}
		if err := f.check(resolve); err != nil {
			errs = append(errs, errors.Annotatef(err, "node=%s", node.id))
		}
		return true
	}, nil)

	return helpers.FoldErrors(errs)
}

func (self *Scenario) ToTree(ctx context.Context, resolve func(action, nodeName string) Doer) (*Tree, error) {
	m := make(map[string]*Node)
	tx := NewTree(self.name)
	// log.Printf("root s=%s tx=%s@%p", self.rootId, tx.Root.String(), &tx.Root)
	errs := make([]error, 0, len(self.idMap))

	convert := func(node *sNode) Doer {
		d := Doer(Nothing{Name: node.id})
		switch node.kind {
		case NodeDoer:
			f := self.Funs(node.id)
			if f.run != "" {
				d = resolve(f.run, node.id)
				if d == nil {
					errs = append(errs, errors.Errorf("node=%s action=%s is not registered", node.id, f.run))
				}
			}
			// default:
			// log.Printf("ToTree node=%s kind=%s skip", node.id, node.Tag())
		}
		return d
	}

	m[self.rootId] = tx.Root.Append(convert(self.Get(self.rootId)))

	self.Walk(nil, func(head, tail *sNode) bool {
		// log.Printf("ToTree edge %s -> %s", head.id, tail.id)
		headNode := m[head.id]
		if tailNode, ok := m[tail.id]; ok {
			headNode.AppendNode(tailNode)
		} else {
			tailDo := convert(tail)
			m[tail.id] = headNode.Append(tailDo)
		}
		return true
	})
	if len(errs) > 0 {
		return nil, helpers.FoldErrors(errs)
	}

	return tx, nil
}

type stringSet map[string]struct{}

func newStringSet() stringSet { return make(map[string]struct{}) }

func (self stringSet) Has(s string) bool { _, ok := self[s]; return ok }
func (self stringSet) Add(s string) bool {
	if _, ok := self[s]; ok {
		return false
	}
	self[s] = struct{}{}
	return true
}

type syncStringSet struct {
	lk sync.Mutex
	ss stringSet
}

func newSyncStringSet() *syncStringSet { return &syncStringSet{ss: newStringSet()} }

func (self *syncStringSet) Has(s string) bool {
	self.lk.Lock()
	_, ok := self.ss[s]
	self.lk.Unlock()
	return ok
}
func (self *syncStringSet) Add(s string) bool {
	self.lk.Lock()
	_, ok := self.ss[s]
	if !ok {
		self.ss[s] = struct{}{}
	}
	self.lk.Unlock()
	return !ok
}
func (self *syncStringSet) Delete(s string) bool {
	self.lk.Lock()
	_, ok := self.ss[s]
	if ok {
		delete(self.ss, s)
	}
	self.lk.Unlock()
	return ok
}
