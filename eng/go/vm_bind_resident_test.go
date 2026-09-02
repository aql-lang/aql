package eng

import (
	"testing"

	compiler "github.com/boru-lang/boru/compiler/go"
	core "github.com/boru-lang/boru/core/go"
)

// OpBindResident's VM arms, driven directly (no compilable program
// produces the op until the arm-residency bridge lands — the same
// direct-drive precedent as the fn-unit twin placement scan): the
// install arm pops the runtime value and installs it through the
// interpreter's own installer into the current registry — two
// executions stack two levels, the measured interpreter leak shape —
// and the undef arm pops the live binding, consuming no stack. Neither
// rides an unwind trail.
func TestVMBindResident(t *testing.T) {
	r, err := core.NewRegistry()
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	r.InitRootContext()

	pos := core.SrcPos{Row: 1, Col: 1}
	p := &compiler.Program{
		Code: []compiler.Instr{
			{Op: compiler.OpPushConst, Arg: 0},
			{Op: compiler.OpBindResident, Arg: 0}, // install 10 under x (pop mode)
			{Op: compiler.OpPushConst, Arg: 1},
			{Op: compiler.OpBindResident, Arg: 0}, // install 20 under x — stacks
			{Op: compiler.OpBindResident, Arg: 1}, // undef arm: pop x's top
			{Op: compiler.OpPushConst, Arg: 1},
			{Op: compiler.OpBindResident, Arg: 2}, // peek mode: y = 20, value stays
		},
		Debug:  []core.SrcPos{pos, pos, pos, pos, pos, pos, pos},
		Consts: []core.Value{core.NewInteger(10), core.NewInteger(20)},
		ResidentBinds: []compiler.ResidentBindSpec{
			{Name: "x", Twin: 0, Pop: true},
			{Name: "x", Twin: 0, Undef: true},
			{Name: "y", Twin: 0},
		},
	}
	out, err := RunProgram(p, r)
	if err != nil {
		t.Fatalf("resident program: %v", err)
	}
	// The peeked value is the program result — peek mode leaves it for
	// downstream consumers.
	if len(out) != 1 || out[0].String() != "20" {
		t.Fatalf("got %v, want the peeked [20]", out)
	}
	// Two installs then one undef: x is one level deep holding the FIRST
	// value (the undef popped the second — last element on top,
	// interpreter order).
	if d := r.Defs.Depth("x"); d != 1 {
		t.Fatalf("x depth after install/install/undef = %d, want 1", d)
	}
	if v, ok := r.Defs.Top("x"); !ok || v.String() != "10" {
		t.Fatalf("x after the undef = %v, want the first install 10", v)
	}
	if v, ok := r.Defs.Top("y"); !ok || v.String() != "20" {
		t.Fatalf("y after the peek install = %v, want 20", v)
	}
}
