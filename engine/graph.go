package engine

import (
	"fmt"
	"sort"
	"strings"
	"unicode"

	"github.com/temoto/vender/helpers/msync"
)

type Graph struct {
	name   string
	idMap  map[string]*Node
	root   *Node
	rootId string
}

type Node struct {
	Doer
	parents  []*Node
	children []*Node
	done     msync.Signal // exec
	callers  int32        // debug
}

// not used yet
// func (self *Node) Clear() {
// 	// help garbage collector
// 	// TODO is it redundant?
// 	for i := range self.parents {
// 		self.parents[i] = nil
// 	}
// 	self.parents = nil
// 	for i, n := range self.children {
// 		n.Clear()
// 		self.children[i] = nil // help GC
// 	}
// 	self.children = nil
// }

func newNode(d Doer, after ...*Node) *Node {
	node := &Node{
		Doer:    d,
		parents: make([]*Node, 0, 8),
	}
	for _, parent := range after {
		parent.AppendNode(node)
	}
	return node
}
func (self *Node) Append(d Doer, after ...*Node) *Node {
	node, ok := d.(*Node)
	if !ok {
		node = newNode(d)
	}
	return self.AppendNode(node, after...)
}
func (self *Node) AppendNode(node *Node, after ...*Node) *Node {
	self.children = append(self.children, node)
	node.parents = append(node.parents, self)
	node.parents = append(node.parents, after...)
	for _, parent := range after {
		parent.children = append(parent.children, node)
	}
	return node
}

type edge struct {
	from *Node
	to   *Node
}

func (self *Node) Walk(fun func(*Node) bool) {
	if fun(self) {
		return
	}
	for _, child := range self.children {
		child.Walk(fun)
	}
}

func (self *Node) Collect(pout *[]*Node) {
	if self == nil {
		return
	}

	visited := make(map[*Node]struct{})
	self.Walk(func(node *Node) bool {
		if _, ok := visited[node]; !ok {
			visited[node] = struct{}{}
			*pout = append(*pout, node)
		}
		return false
	})
}

func (self *Node) Dot(rankdir string) string {
	if rankdir == "" {
		rankdir = "LR"
	}
	result := fmt.Sprintf(`digraph %[1]s {
labelloc=top;
label=%[1]s;
rankdir=%s;
node [shape=plaintext];
`, self.DotString(), rankdir)

	items := make([]*Node, 0, (8+len(self.children))*2)
	self.Collect(&items)

	lines := make([]string, 0, len(items))
	visited := make(map[edge]uint, len(items)*2)
	for _, n := range self.children {
		visited[edge{nil, n}] = 1
		dotNode(&lines, n, 1, visited)
	}
	dotRanks(&lines, items, visited)

	sort.Strings(lines)
	result += strings.Join(lines, "") + "}\n"
	return result
}

var _dot_id = &unicode.RangeTable{
	R16: []unicode.Range16{
		{0x0030, 0x0039, 1}, // 0-9
		{0x0041, 0x005a, 1}, // A-Z
		{0x005f, 0x005f, 1}, // _
		{0x0061, 0x007a, 1}, // a-z
	},
	LatinOffset: 3,
}

func (self *Node) DotString() string {
	ns := self.String()
	for _, r := range ns {
		if !unicode.Is(_dot_id, r) {
			ns = `"` + ns + `"`
			return ns
		}
	}
	return ns
}

func dotNode(pout *[]string, node *Node, level uint, visited map[edge]uint) {
	for _, child := range node.children {
		if _, ok := visited[edge{node, child}]; !ok {
			visited[edge{node, child}] = level + 1
			*pout = append(*pout, fmt.Sprintf("%s -> %s [label=\"\"];\n", node.DotString(), child.DotString()))
		}
		dotNode(pout, child, level+1, visited)
	}
}

func dotRanks(pout *[]string, items []*Node, visited map[edge]uint) {
	levels := make(map[string]uint, len(items))
	for e, l := range visited {
		key := e.to.DotString()
		if exl, ok := levels[key]; !ok || l < exl {
			levels[key] = l
		}
	}
	ranked := make(map[uint][]string, len(levels))
	for n, l := range levels {
		ranked[l] = append(ranked[l], n)
	}
	for _, ns := range ranked {
		sort.Strings(ns)
		*pout = append(*pout, fmt.Sprintf("{ rank=same; %s }\n", strings.Join(ns, ", ")))
	}
}
