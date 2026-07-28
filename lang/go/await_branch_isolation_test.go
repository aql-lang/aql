package lang

import (
	"fmt"
	"testing"
)

// EXPLANATION.md — "Mutable side effects within a branch are local to that
// branch's sub-engine." HOWTO.md — "Each branch runs in a sub-engine, so
// writes to mutable objects inside one branch do not bleed into the
// others." Both were false for exactly the containers they are about.
//
// ForkConcurrent isolates execution SCOPES, and its header says so
// precisely: it clones Defs, Types, builtinWords, Contexts and Args, and
// "its later writes stay private" means writes to those scopes — `def`,
// `context set`. But cloning a binding table copies Values, and a FlexMap
// / FlexList / Store / class-instance Value holds a POINTER to shared
// state. Nothing deep-copied the payload, so an in-place mutation was
// visible to every sibling branch and to the parent. The Go-level comment
// was accurate; the user-facing prose generalised from it.
//
// This is undefined behaviour, not merely a surprise: OrderedMap.Set is an
// unsynchronised map assign plus a slice append, and `make test-race` runs
// this whole package under the detector — which is what makes these tests
// a race gate and not only a semantics gate.
//
// The immutable Map is the oracle throughout. Its `set` returns a copy, so
// it always had the documented behaviour. The fix does not invent
// semantics; it gives the mutable containers the ones the language already
// had for its non-mutable ones.

const awaitPrelude = "import \"aql:time-util\"\n"

// TestAwaitBranchesDoNotShareMutablePayloads pins the contract against the
// oracle: a mutable container must produce the SAME observable result as
// the immutable one — each branch sees only its own write, and the parent
// sees none of them.
func TestAwaitBranchesDoNotShareMutablePayloads(t *testing.T) {
	oracle := runAwait(t, awaitPrelude+`def m {}
TimeUtil.await [[m set a 1] [m set b 2]] end
m`)
	if oracle != "[[{a:1} {b:2}] {}]" {
		t.Fatalf("the immutable Map oracle itself changed — re-derive the "+
			"contract before trusting the rows below: %s", oracle)
	}
	for _, c := range []struct{ name, src string }{
		// The reported repro: a FlexMap bound by def, mutated in both branches.
		{"FlexMap bound by def", awaitPrelude + `def m (make FlexMap {})
TimeUtil.await [[m set a 1] [m set b 2]] end
m`},
		// Reached through a CAPTURED closure rather than named directly in
		// the branch — same def table, same aliasing, and the shape a real
		// program is likelier to have.
		{"FlexMap through a captured lambda", awaitPrelude + `def m (make FlexMap {})
def w1 ([] => [m set a 1])
def w2 ([] => [m set b 2])
TimeUtil.await [[w1] [w2]] end
m`},
	} {
		t.Run(c.name, func(t *testing.T) {
			if got := runAwait(t, c.src); got != oracle {
				t.Errorf("mutable container diverges from the immutable oracle\n"+
					" got: %s\nwant: %s", got, oracle)
			}
		})
	}
}

// TestAwaitBranchesShareImmutableValues is the other half of the contract:
// isolation must not mean the branches lose sight of their inputs. Reading
// a parent binding — of any kind — still works, and an immutable payload is
// still shared rather than needlessly copied.
func TestAwaitBranchesShareImmutableValues(t *testing.T) {
	got := runAwait(t, awaitPrelude+`def n 40
def s "x"
def m {k: 1}
TimeUtil.await [[n add 1] [n add 2] [s] [m dot k]] end`)
	if got != "[[41 42 'x' 1]]" {
		t.Errorf("branches must still read parent bindings: %s", got)
	}
}

// TestAwaitIsolationSurvivesMutationDepth pins that the copy is DEEP: a
// mutable container nested inside another value must not alias either.
// A shallow copy of the binding would pass the top-level test above and
// fail this one.
func TestAwaitIsolationSurvivesMutationDepth(t *testing.T) {
	got := runAwait(t, awaitPrelude+`def box (make FlexMap {})
def holder {nested: box}
TimeUtil.await [[(holder dot nested) set a 1] [(holder dot nested) set b 2]] end
box`)
	if got != "[[{a:1} {b:2}] {}]" {
		t.Errorf("a NESTED mutable container must be isolated too (a shallow "+
			"binding copy would alias it): %s", got)
	}
}

func runAwait(t *testing.T, src string) string {
	t.Helper()
	a, err := New()
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	out, err := a.Run(src)
	if err != nil {
		t.Fatalf("run %q: %v", src, err)
	}
	return fmt.Sprint(out)
}
