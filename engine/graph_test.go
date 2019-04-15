package engine

import (
	"sort"
	"testing"

	"github.com/temoto/vender/helpers"
)

func TestNodeCollect(t *testing.T) {
	t.Parallel()
	type Case struct {
		name   string
		input  func() *Node
		expect []string
	}
	cases := []Case{
		Case{"empty", func() *Node { return nil }, nil},
		Case{"1", func() *Node { return newNode(&mockdo{name: "1"}) }, []string{"1"}},
		Case{"triangle", func() *Node {
			root := newNode(&mockdo{name: "1"})
			root.Append(&mockdo{name: "2-left"})
			root.Append(&mockdo{name: "3-right"})
			return root
		}, []string{"1", "2-left", "3-right"}},
		Case{"diamond", func() *Node {
			nbegin := newNode(&mockdo{name: "1-begin"})
			nleft := newNode(&mockdo{name: "2-left"}, nbegin)
			nright := newNode(&mockdo{name: "3-right"}, nbegin)
			newNode(&mockdo{name: "4-end"}, nleft, nright)
			return nbegin
		}, []string{"1-begin", "2-left", "3-right", "4-end"}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			node := c.input()
			ns := make([]*Node, 0, len(c.expect))
			node.Collect(&ns)
			t.Logf("collected: %v", ns)
			helpers.AssertEqual(t, len(ns), len(c.expect))
			ss := make([]string, len(c.expect))
			for i, n := range ns {
				ss[i] = n.String()
			}
			sort.Strings(ss)
			for i := range ss {
				helpers.AssertEqual(t, ss[i], c.expect[i])
			}
		})
	}
}

func TestDot(t *testing.T) {
	t.Parallel()
	tx := NewTree("check recipe")
	nenumdev := newNode(&Func{Name: "recipe.EnumDevices"}, &tx.Root)
	ncheckConv := newNode(&Func{Name: "check conveyor"})
	ncheckConv.Append(&mockdo{name: "MDB da"})
	nenumdev.Append(ncheckConv)
	ncheckCup := nenumdev.Append(&Func{Name: "check cup"})
	ncheckCup.Append(&mockdo{name: "MDB e3"})
	ncheckCup.Append(&Func{Name: "cup stock > 1?"})
	dots := tx.Root.Dot("UD")
	t.Logf("result:\n%s", dots)
	expect := `digraph "check recipe" {
labelloc=top;
label="check recipe";
rankdir=UD;
node [shape=plaintext];
"check conveyor" -> "MDB da" [label=""];
"check cup" -> "MDB e3" [label=""];
"check cup" -> "cup stock > 1?" [label=""];
"recipe.EnumDevices" -> "check conveyor" [label=""];
"recipe.EnumDevices" -> "check cup" [label=""];
{ rank=same; "MDB da", "MDB e3", "cup stock > 1?" }
{ rank=same; "check conveyor", "check cup" }
{ rank=same; "recipe.EnumDevices" }
}
`
	helpers.AssertEqual(t, dots, expect)
}
