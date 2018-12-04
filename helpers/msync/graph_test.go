package msync

import (
	"testing"

	"github.com/temoto/vender/helpers"
)

func TestDot(t *testing.T) {
	tx := NewTransaction("check recipe")
	nenumdev := NewNode(&DoFunc{Name: "recipe.EnumDevices"}, &tx.Root)
	NewNode(&DoFunc{Name: "check conveyor"}, nenumdev).Append(&mockdo{name: "MDB da"})
	ncheckcup := nenumdev.Append(&DoFunc{Name: "check cup"})
	ncheckcup.Append(&mockdo{name: "MDB e3"})
	ncheckcup.Append(&DoFunc{Name: "cup stock > 1?"})
	dots := tx.Root.Dot("UD")
	expect := `digraph "check recipe" {
labelloc=top;
label="check recipe";
rankdir=UD;
node [shape=plaintext];
"Func=check conveyor" -> "MDB da" [label=""];
"Func=check cup" -> "Func=cup stock > 1?" [label=""];
"Func=check cup" -> "MDB e3" [label=""];
"Func=recipe.EnumDevices" -> "Func=check conveyor" [label=""];
"Func=recipe.EnumDevices" -> "Func=check cup" [label=""];
{ rank=same; "Func=check conveyor", "Func=check cup" }
{ rank=same; "Func=cup stock > 1?", "MDB da", "MDB e3" }
{ rank=same; "Func=recipe.EnumDevices" }
}
`
	helpers.AssertEqual(t, dots, expect)
}
