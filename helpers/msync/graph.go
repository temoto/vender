package msync

import (
	"fmt"
	"sort"
	"strings"
)

type Node struct {
	Doer
	parents  []*Node
	children []*Node
	done     Signal // exec
	callers  int32  // debug
}

func (self *Node) Clear() {
	// help garbage collector
	// TODO is it redundant?
	for i := range self.parents {
		self.parents[i] = nil
	}
	self.parents = nil
	for i, n := range self.children {
		n.Clear()
		self.children[i] = nil // help GC
	}
	self.children = nil
}

func NewNode(d Doer, after ...*Node) *Node {
	node := &Node{Doer: d, parents: after, done: NewSignal()}
	for _, parent := range after {
		parent.children = append(parent.children, node)
	}
	return node
}
func (self *Node) Append(d Doer, after ...*Node) *Node {
	return NewNode(d, append(after, self)...)
}

func (self *Node) count() int {
	// FIXME do it without collect() gathering a whole array of nodes
	xs := make([]*Node, 0, 32)
	self.collect(&xs)
	return len(xs)
}

type edge struct {
	from *Node
	to   *Node
}

func (self *Node) collect(pout *[]*Node) {
	*pout = append(*pout, self.children...)
	for _, n := range self.children {
		n.collect(pout)
	}
}

func (self *Node) Dot(rankdir string) string {
	if rankdir == "" {
		rankdir = "LR"
	}
	result := fmt.Sprintf(`digraph "%[1]s" {
labelloc=top;
label="%[1]s";
rankdir=%s;
node [shape=plaintext];
`, self.String(), rankdir)

	items := make([]*Node, 0, (8+len(self.children))*2)
	self.collect(&items)

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

func mapStrings(ns []*Node) []string {
	result := make([]string, len(ns))
	for i, n := range ns {
		result[i] = fmt.Sprintf(`"%s"`, n.String())
	}
	return result
}

func dotNode(pout *[]string, node *Node, level uint, visited map[edge]uint) {
	for _, child := range node.children {
		if _, ok := visited[edge{node, child}]; !ok {
			visited[edge{node, child}] = level + 1
			*pout = append(*pout, fmt.Sprintf("\"%s\" -> \"%s\" [label=\"\"];\n", node.String(), child.String()))
		}
		dotNode(pout, child, level+1, visited)
	}
}

func dotRanks(pout *[]string, items []*Node, visited map[edge]uint) {
	levels := make(map[*Node]uint, len(items))
	for e, l := range visited {
		if exl, ok := levels[e.to]; !ok || l < exl {
			levels[e.to] = l
		}
	}
	ranked := make(map[uint][]*Node, len(levels))
	for n, l := range levels {
		ranked[l] = append(ranked[l], n)
	}
	for _, nodes := range ranked {
		ns := mapStrings(nodes)
		sort.Slice(ns, func(i int, j int) bool { return ns[i] < ns[j] })
		*pout = append(*pout, fmt.Sprintf("{ rank=same; %s }\n", strings.Join(ns, ", ")))
	}
}
