package engine

import (
	"bytes"
	"context"
	"fmt"
	"io/ioutil"
	"strings"
	"sync/atomic"
	"testing"
)

func TestScenarioWalk(t *testing.T) {
	input := `digraph complex {
boot[comment="/v1/v=val"];
boot -> s1x; boot -> s2a;
s1y -> end; s2b -> end;
subgraph cluster_s1 {
  s1x[comment="/v1/sl=s1x_leaving_s1"];
  s1x -> s1y;
};
subgraph cluster_s2 {
  s2b[comment="/v1/se=s2b_entering_s2"];
  s2a -> s2b;
};
}`
	s, err := ParseDot([]byte(input))
	if err != nil {
		t.Fatalf("ParseDot: err=%v", err)
	}
	buf := bytes.NewBuffer(nil)
	s.Walk(func(n *sNode) bool {
		buf.WriteString(fmt.Sprintf("%s %s/%s\n", n.Tag(), n.parent, n.id))
		return true
	}, func(head, tail *sNode) bool {
		buf.WriteString(fmt.Sprintf("Edge %s -> %s\n", head.id, tail.id))
		return true
	})
	actualLines := strings.Split(buf.String(), "\n")
	expect := `
Block /
NodeDo /boot
Block /s1
NodeDo s1/s1x
Edge boot -> s1x
NodeDo s1/s1y
Edge s1x -> s1y
NodeDo /end
Edge s1y -> end
Block /s2
NodeDo s2/s2a
Edge boot -> s2a
NodeDo s2/s2b
Edge s2a -> s2b
Edge s2b -> end
`
	expectLines := strings.Split(strings.TrimSpace(expect), "\n")
	for i := range expectLines {
		if i >= len(actualLines) {
			t.Fatalf("walk actual output less than expected")
		}
		el := expectLines[i]
		al := actualLines[i]
		if el != al {
			t.Errorf("line %d\nexpected: %s\nactual  : %s", i+1, el, al)
		}
	}
}

func TestScenarioToTree(t *testing.T) {
	input := `digraph simple {
begin[shape=point];
end[shape=point];
n1[comment="/v1/r=fun"];
n2a[comment="/v1/r=fun"];
n2b[comment="/v1/r=fun"];
n3[comment="/v1/r=fun"];

begin -> n1 -> n2a -> n3 -> end;
         n1 -> n2b -> n3;
}`
	s, err := ParseDot([]byte(input))
	if err != nil {
		t.Fatalf("ParseDot: err=%v", err)
	}
	ctx := context.Background()
	callCount := int32(0)
	actFun := func() error {
		atomic.AddInt32(&callCount, 1)
		return nil
	}
	tx, err := s.ToTree(ctx, func(action, name string) Doer {
		if action == "fun" {
			return Func0{Name: name, F: actFun}
		}
		panic("unknown action=" + action)
	})
	if err != nil {
		t.Fatalf("ToTree err=%v", err)
	}

	txDot := tx.Root.Dot("")
	t.Logf("result dot:\n%s", txDot)
	actualLines := strings.Split(txDot, "\n")
	expect := `
digraph simple {
labelloc=top;
label=simple;
rankdir=LR;
node [shape=plaintext];
begin -> n1 [label=""];
n1 -> n2a [label=""];
n1 -> n2b [label=""];
n2a -> n3 [label=""];
n2b -> n3 [label=""];
n3 -> end [label=""];
{ rank=same; begin }
{ rank=same; end }
{ rank=same; n1 }
{ rank=same; n2a, n2b }
{ rank=same; n3 }
}
`
	expectLines := strings.Split(strings.TrimSpace(expect), "\n")
	for i := range expectLines {
		if i >= len(actualLines) {
			t.Fatalf("walk actual output less than expected")
		}
		el := expectLines[i]
		al := actualLines[i]
		if el != al {
			t.Errorf("line %d\nexpected: %s\nactual  : %s", i+1, el, al)
		}
	}
}

func TestScenarioToTreePrototype(t *testing.T) {
	input, err := ioutil.ReadFile("../scenario/prototype.dot")
	if err != nil {
		t.Fatal(err)
	}
	s, err := ParseDot(input)
	if err != nil {
		t.Fatalf("ParseDot: err=%v", err)
	}
	ctx := context.Background()
	callCount := int32(0)
	actFun := func() error {
		atomic.AddInt32(&callCount, 1)
		return nil
	}
	tx, err := s.ToTree(ctx, func(action, name string) Doer {
		switch action {
		case "":
			return Nothing{name}
		case "fun":
			return Func0{Name: name, F: actFun}
		}
		if strings.HasPrefix(action, "mdb.evend.") {
			return Nothing{Name: name + "/" + action}
		}
		panic("unknown action=" + action)
	})
	if err != nil {
		t.Fatalf("ToTree err=%v", err)
	}
	txDot := tx.Root.Dot("")
	actualLines := strings.Split(txDot, "\n")
	expect := `
digraph g {
labelloc=top;
label=g;
rankdir=LR;
node [shape=plaintext];
"s10/mdb.evend.conveyor_move_cup" -> "s20/mdb.evend.cup_dispense" [label=""];
"s20/mdb.evend.cup_dispense" -> "s30/mdb.evend.conveyor_move_hopper" [label=""];
"s30/mdb.evend.conveyor_move_hopper" -> "s41/mdb.evend.hopper_dispense" [label=""];
"s30/mdb.evend.conveyor_move_hopper" -> "s42/mdb.evend.elevator_move_down" [label=""];
"s41/mdb.evend.hopper_dispense" -> "s50/mdb.evend.conveyor_move_elevator" [label=""];
"s42/mdb.evend.elevator_move_down" -> "s50/mdb.evend.conveyor_move_elevator" [label=""];
"s50/mdb.evend.conveyor_move_elevator" -> "s60/mdb.evend.elevator_move_conveyor" [label=""];
"s60/mdb.evend.elevator_move_conveyor" -> "s70/mdb.evend.conveyor_move_cup" [label=""];
"s70/mdb.evend.conveyor_move_cup" -> "s80/mdb.evend.elevator_move_ready" [label=""];
"s80/mdb.evend.elevator_move_ready" -> end [label=""];
begin -> "s10/mdb.evend.conveyor_move_cup" [label=""];
{ rank=same; "s10/mdb.evend.conveyor_move_cup" }
{ rank=same; "s20/mdb.evend.cup_dispense" }
{ rank=same; "s30/mdb.evend.conveyor_move_hopper" }
{ rank=same; "s41/mdb.evend.hopper_dispense", "s42/mdb.evend.elevator_move_down" }
{ rank=same; "s50/mdb.evend.conveyor_move_elevator" }
{ rank=same; "s60/mdb.evend.elevator_move_conveyor" }
{ rank=same; "s70/mdb.evend.conveyor_move_cup" }
{ rank=same; "s80/mdb.evend.elevator_move_ready" }
{ rank=same; begin }
{ rank=same; end }
}
`
	expectLines := strings.Split(strings.TrimSpace(expect), "\n")
	for i := range expectLines {
		if i >= len(actualLines) {
			t.Fatalf("walk actual output less than expected")
		}
		el := expectLines[i]
		al := actualLines[i]
		if el != al {
			t.Errorf("line %d\nexpected: %s\nactual  : %s", i+1, el, al)
		}
	}
}
