package native

import (
	"strings"
	"testing"

	"github.com/boru-lang/boru/eng/go"
)

// `behave` exposed four capability slots — compare, canon, nodify,
// unify — while the kernel could dispatch more. A boru type therefore
// could say how it ORDERS, RENDERS, PROJECTS and UNIFIES, but not
// whether it is EMPTY, when two of its values are the SAME, or how big
// it is. `size` was reachable only from Go; truthiness and deep
// equality were reachable from neither.
//
// These tests pin the three slots that close that gap, in both
// directions: an installed body owns the answer, and the untouched
// builtin keeps the kernel rule.

// bcRun runs src on a fresh registry and returns the whole stack.
func bcRun(t *testing.T, src string) []Value {
	t.Helper()
	out, err := w9Run(t, w9Reg(t), src)
	if err != nil {
		t.Fatalf("run %q: %v", src, err)
	}
	return out
}

func bcTop(t *testing.T, src string) Value {
	t.Helper()
	out := bcRun(t, src)
	if len(out) == 0 {
		t.Fatalf("run %q: empty stack", src)
	}
	return out[len(out)-1]
}

func bcBool(t *testing.T, src string) bool {
	t.Helper()
	v := bcTop(t, src)
	b, err := eng.AsBoolean(v)
	if err != nil {
		t.Fatalf("run %q: not a boolean: %v (%s)", src, err, v.String())
	}
	return b
}

// TestBehaveTruthySlot: a type overrides what it means in a boolean
// position, and the type it refines does not change.
func TestBehaveTruthySlot(t *testing.T) {
	// Level says "truthy iff above 5", overriding Integer's magnitude
	// rule in BOTH directions — 3 becomes falsy, and 0 would become
	// truthy if it were above the threshold (see the second program).
	src := `
def Level (refine Integer)
behave truthy/q (fn [[Level] [Boolean] [a gt 5]])
def lo:Level 3
def hi:Level 9
[(if lo [true] [false]) (if hi [true] [false]) (if 3 [true] [false])]`
	got := bcTop(t, src)
	elems, err := eng.AsMutableList(got)
	if err != nil || len(elems) != 3 {
		t.Fatalf("expected a 3-element list, got %s (%v)", got.String(), err)
	}
	lo, _ := eng.AsBoolean(elems[0])
	hi, _ := eng.AsBoolean(elems[1])
	plain, _ := eng.AsBoolean(elems[2])
	if lo {
		t.Error("Level 3 must be falsy — the installed body said so")
	}
	if !hi {
		t.Error("Level 9 must be truthy")
	}
	if !plain {
		t.Error("a plain Integer 3 must keep the magnitude rule — the override is per-type")
	}

	// Inverting the kernel rule the other way: zero truthy.
	if !bcBool(t, `
def Always (refine Integer)
behave truthy/q (fn [[Always] [Boolean] [true]])
def z:Always 0
if z [true] [false]`) {
		t.Error("a body returning true must win over the zero-is-falsy rule")
	}
}

// TestBehaveSizeSlot: `size` was a Go-only capability; the slot makes it
// reachable from boru.
func TestBehaveSizeSlot(t *testing.T) {
	v := bcTop(t, `
def Level (refine Integer)
behave size/q (fn [[Level] [Integer] [42]])
def x:Level 3
size x`)
	n, err := eng.AsInteger(v)
	if err != nil || n != 42 {
		t.Errorf("size = %s (%v), want 42 from the installed body", v.String(), err)
	}
	// The builtin rule is untouched: a plain Integer sizes to its floored
	// magnitude.
	v = bcTop(t, "size 3")
	if n, _ := eng.AsInteger(v); n != 3 {
		t.Errorf("plain size 3 = %s, want 3", v.String())
	}
}

// TestBehaveDeqSlotInstalls pins the slot's wiring. The dispatch point
// itself is covered in eng (capability_gaps_test.go) because the values
// that reach DeepEqual's terminal fall-through are not constructible
// from boru source today; what is tested here is that the slot
// validates, installs, and produces a working DeepEqualer.
func TestBehaveDeqSlotInstalls(t *testing.T) {
	r := w9Reg(t)
	if _, err := w9Run(t, r, `
def Tag (refine Ideal)
behave deq/q (fn [[Tag Tag] [Boolean] [true]])`); err != nil {
		t.Fatalf("installing the deq slot: %v", err)
	}
	ty := r.LookupTypeName("Tag")
	if ty == nil {
		t.Fatal("Tag was not installed")
	}
	de, ok := ty.Behavior().(eng.DeepEqualer)
	if !ok {
		t.Fatal("the deq slot did not produce a DeepEqualer")
	}
	eq, err := de.DeepEqualValues(NewInteger(1), NewInteger(2))
	if err != nil {
		t.Fatalf("running the installed deq body: %v", err)
	}
	if !eq {
		t.Error("the installed body returns true, so DeepEqualValues must")
	}
}

// TestBehaveNewSlotsRejectWrongShapes is the negative half — each
// validator must refuse a fn whose shape does not match the slot's
// contract, or a body would be installed that the kernel then calls
// with the wrong arity or return type.
func TestBehaveNewSlotsRejectWrongShapes(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want string
	}{
		{
			name: "truthy with two params",
			src: `def T (refine Integer)
behave truthy/q (fn [[T T] [Boolean] [true]])`,
			want: "truthy: fn must take 1 arg",
		},
		{
			name: "truthy returning a non-Boolean",
			src: `def T (refine Integer)
behave truthy/q (fn [[T] [Integer] [1]])`,
			want: "truthy: fn must return Boolean",
		},
		{
			name: "deq with one param",
			src: `def T (refine Integer)
behave deq/q (fn [[T] [Boolean] [true]])`,
			want: "deq: fn must take 2 args",
		},
		{
			name: "deq over two different types",
			src: `def T (refine Integer)
def U (refine Integer)
behave deq/q (fn [[T U] [Boolean] [true]])`,
			want: "deq: both params must be the same type",
		},
		{
			name: "deq returning a non-Boolean",
			src: `def T (refine Integer)
behave deq/q (fn [[T T] [Integer] [1]])`,
			want: "deq: fn must return Boolean",
		},
		{
			name: "size with two params",
			src: `def T (refine Integer)
behave size/q (fn [[T T] [Integer] [1]])`,
			want: "size: fn must take 1 arg",
		},
		{
			name: "size returning a non-Integer",
			src: `def T (refine Integer)
behave size/q (fn [[T] [Boolean] [true]])`,
			want: "size: fn must return Integer",
		},
	}
	for _, c := range cases {
		_, err := w9Run(t, w9Reg(t), c.src)
		if err == nil {
			t.Errorf("%s: accepted, want %q", c.name, c.want)
			continue
		}
		if !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: error %q, want it to contain %q", c.name, err.Error(), c.want)
		}
	}
}

// TestBehaveNewSlotsUntypedParamsRejected covers the remaining validator
// arm: a param with no declared type gives the slot nothing to attach
// to.
func TestBehaveNewSlotsUntypedParamsRejected(t *testing.T) {
	for _, slot := range []string{"truthy", "size", "deq"} {
		fn := "(fn [[a] [Boolean] [true]])"
		if slot == "size" {
			fn = "(fn [[a] [Integer] [1]])"
		}
		if slot == "deq" {
			fn = "(fn [[a b] [Boolean] [true]])"
		}
		if _, err := w9Run(t, w9Reg(t), "behave "+slot+"/q "+fn); err == nil {
			t.Errorf("%s: an untyped param was accepted", slot)
		}
	}
}

// TestBehaveSlotsCoexist pins that the new slots are additive with the
// existing ones — `behave` is documented as accumulating capabilities on
// one wrapper, and a second install must not drop the first.
func TestBehaveSlotsCoexist(t *testing.T) {
	v := bcTop(t, `
def Level (refine Integer)
behave truthy/q (fn [[Level] [Boolean] [a gt 5]])
behave size/q   (fn [[Level] [Integer] [99]])
behave canon/q  (fn [[Level] [String]  ["<lvl>"]])
def x:Level 3
[(if x [true] [false]) (size x) (canon x)]`)
	elems, err := eng.AsMutableList(v)
	if err != nil || len(elems) != 3 {
		t.Fatalf("expected a 3-element list, got %s (%v)", v.String(), err)
	}
	if b, _ := eng.AsBoolean(elems[0]); b {
		t.Error("the truthy slot was lost when size/canon were installed")
	}
	if n, _ := eng.AsInteger(elems[1]); n != 99 {
		t.Errorf("size = %s, want 99 — the size slot was lost", elems[1].String())
	}
	if s, _ := eng.AsString(elems[2]); !strings.Contains(s, "lvl") {
		t.Errorf("canon = %q, want the installed rendering — the canon slot was lost", s)
	}
}

// TestUnknownBehaviorNamesEverySlot pins that the unknown-name error is
// derived from the table, not written out. The literal it replaced still
// said "compare, canon, nodify, unify" after three slots were added — a
// user asking `behave truthy/q` on a typo would have been told truthy
// does not exist.
func TestUnknownBehaviorNamesEverySlot(t *testing.T) {
	_, err := w9Run(t, w9Reg(t), `def T (refine Integer)
behave nope/q (fn [[T] [Boolean] [true]])`)
	if err == nil {
		t.Fatal("an unknown behavior name was accepted")
	}
	msg := err.Error()
	for name := range behaviors {
		if !strings.Contains(msg, name) {
			t.Errorf("the unknown-name error omits the installable slot %q: %s", name, msg)
		}
	}
	// And it must not advertise a name the table does not carry.
	if strings.Contains(msg, "nope;") {
		t.Errorf("the error echoes the bad name into the known list: %s", msg)
	}
}

// TestBehaveSizeDoesNotSwallowTheWalk pins the regression the PR #325
// review surfaced: the wrapper satisfies eng.Sizer structurally whether
// or not a size body was installed, and Sizer has no decline channel —
// so installing ANY slot must not change `size` for a type that never
// installed one. Before the fix, `behave canon` alone turned an Integer
// refine's size from the magnitude rule into 0.
func TestBehaveSizeDoesNotSwallowTheWalk(t *testing.T) {
	v := bcTop(t, `
def Level (refine Integer)
behave canon/q (fn [[Level] [String] ["L"]])
def x:Level 7
size x`)
	if n, err := eng.AsInteger(v); err != nil || n != 7 {
		t.Errorf("size = %s (%v), want 7 — a canon-only install must keep the magnitude rule", v.String(), err)
	}
}

// TestBehaveSizeCheckerDoesNotFold pins the checker half (PR #325
// review, Codex P1): sizeReturns folds a concrete container to its
// physical element count, which is only faithful when the kernel Sizer
// owns the dispatch. A `behave size` on a List refinement answers with
// its own body at runtime, so the static fold must yield to a carrier.
func TestBehaveSizeCheckerDoesNotFold(t *testing.T) {
	r := w9Reg(t)
	if _, err := w9Run(t, r, `
def Bag (refine List)
behave size/q (fn [[Bag] [Integer] [99]])`); err != nil {
		t.Fatalf("install: %v", err)
	}
	ty := r.LookupTypeName("Bag")
	if ty == nil {
		t.Fatal("Bag was not installed")
	}
	bag := w9List(NewInteger(1), NewInteger(2), NewInteger(3))
	bag.Parent = ty

	out := sizeReturns([]Value{bag}, r)
	if len(out) != 1 {
		t.Fatalf("sizeReturns = %v, want one value", out)
	}
	if IsConcrete(out[0]) {
		t.Errorf("the fold fired despite the size body: %s — runtime answers 99, "+
			"a folded 3 makes the checker model a size the runtime never reports",
			out[0].String())
	}

	// Both negatives: a plain list still folds (the fold is the point),
	// and a refinement WITHOUT a size body folds too — its Sizer owner
	// is still the kernel List node.
	plain := w9List(NewInteger(1), NewInteger(2))
	out = sizeReturns([]Value{plain}, r)
	if n, err := eng.AsInteger(out[0]); err != nil || n != 2 {
		t.Errorf("plain-list fold regressed: %v (%v)", out[0].String(), err)
	}
	if _, err := w9Run(t, r, `def Sack (refine List)`); err != nil {
		t.Fatalf("Sack: %v", err)
	}
	sack := w9List(NewInteger(1), NewInteger(2))
	sack.Parent = r.LookupTypeName("Sack")
	out = sizeReturns([]Value{sack}, r)
	if n, err := eng.AsInteger(out[0]); err != nil || n != 2 {
		t.Errorf("a body-less refinement must keep the fold: %v (%v)", out[0].String(), err)
	}
}
