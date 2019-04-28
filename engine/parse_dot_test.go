package engine

import (
	"fmt"
	"strings"
	"testing"

	"github.com/temoto/vender/helpers"
)

func TestParseDot(t *testing.T) {
	t.Parallel()
	type Case struct {
		name      string
		input     string
		expectFun func(*Scenario) string
	}
	cases := []Case{
		Case{"invalid", "completely+bananas", func(s *Scenario) string { return "Error in S0:" }},
		Case{"not-directed", "graph {}", func(s *Scenario) string { return "graph must be directed" }},
		Case{"subgraph-reuses-name", "digraph { n; subgraph n {} }", func(s *Scenario) string { return "node=n already exists" }},
		Case{"cluster-reuses-name", "digraph { n; subgraph cluster_n {} }", func(s *Scenario) string { return "node=n already exists" }},
		Case{"empty-with-name", "digraph gname {}", func(s *Scenario) string { s.name = "gname"; return "" }},
		Case{"graph-comment", `digraph { comment="/hey/" }`, func(s *Scenario) string { return "" }},
		Case{"node-comment-invalid-value", `digraph { boot[comment="something"]; }`, func(s *Scenario) string { return "not valid" }},
		Case{"node-comment-invalid-part", `digraph { boot[comment="/v1/gar=bage"]; }`, func(s *Scenario) string { return "not valid" }},
		Case{"node-v1-r", `digraph { boot[comment="/v1/r=foo"]; }`, func(s *Scenario) string {
			testerr(t, s.Add(&sNode{id: "boot", kind: NodeDoer}))
			s.SetRun("boot", "foo")
			return ""
		}},
		Case{"node-v1-all-attrs", `digraph { boot[comment="/v1/v=val:se=subent:r=run"]; }`, func(s *Scenario) string {
			testerr(t, s.Add(&sNode{id: "boot", kind: NodeDoer}))
			s.AddEnter("", "subent")
			s.SetValidate("boot", "val")
			s.SetRun("boot", "run")
			return ""
		}},
		Case{"subgraph", `digraph { subgraph sg { x -> y; } }`, func(s *Scenario) string {
			testerr(t, s.Add(&sNode{id: "sg", kind: NodeBlock}))
			testerr(t, s.Add(&sNode{id: "x", kind: NodeDoer, parent: "sg"}))
			testerr(t, s.Add(&sNode{id: "y", kind: NodeDoer, parent: "sg"}))
			testerr(t, s.Edge("x", "y"))
			return ""
		}},
		Case{"complex", `digraph complex {
boot[comment="/v1/v=val"];
boot -> s1x; boot -> s2a;
s1y -> end; s2b -> end;
subgraph cluster_s1 {
  s1x[comment="/v1/sl=s1x_leaving_s1"];
  s1x -> s1y;
};
subgraph cluster_s2 {
  s2a -> s2b;
};
}`, func(s *Scenario) string {
			s.name = "complex"
			testerr(t, s.Add(&sNode{id: "boot", kind: NodeDoer}))
			testerr(t, s.Add(&sNode{id: "s1", kind: NodeBlock}))
			testerr(t, s.Add(&sNode{id: "s2", kind: NodeBlock}))
			testerr(t, s.Add(&sNode{id: "end", kind: NodeDoer}))
			testerr(t, s.Add(&sNode{id: "s1x", kind: NodeDoer, parent: "s1"}))
			testerr(t, s.Add(&sNode{id: "s1y", kind: NodeDoer, parent: "s1"}))
			testerr(t, s.Add(&sNode{id: "s2a", kind: NodeDoer, parent: "s2"}))
			testerr(t, s.Add(&sNode{id: "s2b", kind: NodeDoer, parent: "s2"}))
			testerr(t, s.Edge("boot", "s1x"))
			testerr(t, s.Edge("boot", "s2a"))
			testerr(t, s.Edge("s1x", "s1y"))
			testerr(t, s.Edge("s2a", "s2b"))
			testerr(t, s.Edge("s1y", "end"))
			testerr(t, s.Edge("s2b", "end"))
			s.SetValidate("boot", "val")
			s.AddLeave("s1", "s1x_leaving_s1")
			return ""
		}},
	}
	helpers.RandUnix().Shuffle(len(cases), func(i int, j int) { cases[i], cases[j] = cases[j], cases[i] })
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			expect := NewScenario("")
			expectErrContain := c.expectFun(expect)
			s, err := ParseDot([]byte(c.input))
			if err != nil {
				if expectErrContain != "" && strings.Contains(err.Error(), expectErrContain) {
					return
				}
				t.Fatalf("unexpected err=%v", err)
			} else if expectErrContain != "" {
				t.Fatalf("error expected=*%v* actual=%v", expectErrContain, err)
			}
			if s.name != expect.name {
				t.Errorf("scenario.name expected='%s' actual='%s'", expect.name, s.name)
			}

			assertFunsEqual(t, s, expect)

			if len(s.idMap) != len(expect.idMap) {
				t.Errorf("idMap len expected=%d actual=%d", len(expect.idMap), len(s.idMap))
			}
			// collect unique node ids from both expect and actual scenarios
			allIds := newStringSet()
			for id := range expect.idMap {
				allIds.Add(id)
			}
			for id := range s.idMap {
				allIds.Add(id)
			}

			for id := range allIds {
				assertNodeEqual(t, s, expect, id)
			}

			for id := range s.idMap {
				as := strings.Join(s.parents[id], ",")
				es := strings.Join(expect.parents[id], ",")
				if as != es {
					t.Errorf("parents[%s] expected=%s actual=%s", id, es, as)
				}
			}
		})
	}
}

func testerr(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Error(err)
	}
}

func assertNodeEqual(t testing.TB, sactual, sexpect *Scenario, name string) {
	nactual := sactual.Get(name)
	nexpect := sexpect.Get(name)
	if nactual == nil && nexpect == nil {
		t.Fatalf("code error both nactual=nexpect=nil")
	}
	if (nactual == nil && nexpect != nil) || (nactual != nil && nexpect == nil) {
		t.Fatalf("name='%s' node actual=%v expect=%v", name, nactual, nexpect)
	}
	ans := nactual.String()
	ens := nexpect.String()
	// t.Logf("debug %s", ans)
	if ans != ens {
		t.Errorf("node expected='%s' actual='%s'", ens, ans)
	}
}

func assertFunsEqual(t testing.TB, sactual, sexpect *Scenario) {
	allKeys := newStringSet()
	for key := range sexpect.funMap {
		allKeys.Add(key)
	}
	for key := range sactual.funMap {
		allKeys.Add(key)
	}
	for key := range allKeys {
		a := fmt.Sprintf("%#v", sactual.Funs(key))
		e := fmt.Sprintf("%#v", sexpect.Funs(key))
		if a != e {
			t.Errorf("funs[%s] actual=%s expected=%s", key, a, e)
		}
	}
}
