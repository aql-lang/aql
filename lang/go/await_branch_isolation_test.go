package lang

import (
	"fmt"
	"strings"
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
// / FlexList / Store / Table / class-instance Value holds a POINTER to
// shared state. Nothing deep-copied the payload, so an in-place mutation
// was visible to every sibling branch and to the parent. The Go-level
// comment was accurate; the user-facing prose generalised from it.
//
// This is undefined behaviour, not merely a surprise: OrderedMap.Set is an
// unsynchronised map assign plus a slice append, and `make test-race` runs
// this whole package under the detector.
//
// The resolution is REFUSAL, not a silent deep copy: sharing a stateful
// container across concurrent branches is rejected at the boundary, the
// same answer `send` gives at a process boundary. Cloning would make these
// programs work silently, but it hides the sharing the author wrote and
// pays a per-branch copy for every container in scope. Refusing states the
// rule once, at the point where it is violated.

const awaitPrelude = "import \"aql:time-util\"\n"

// TestAwaitRefusesSharedMutableContainer covers every way a branch reaches
// a stateful container, in BOTH engine modes. The compiled shape matters
// on its own: a branch body arrives as a synthetic fn-value carrier rather
// than a token list, and an earlier version of this check walked only the
// list — leaving the boundary unguarded on the default path, which is the
// only path most programs take.
func TestAwaitRefusesSharedMutableContainer(t *testing.T) {
	cases := []struct{ name, src, want string }{
		// Named directly in the branch body.
		{"FlexMap by def", awaitPrelude + `def m (make FlexMap {})
TimeUtil.await [[m set a 1] [m set b 2]] end`, "m"},
		// Reached through a lambda. `m` is module-level, so nothing is
		// CAPTURED — the reference is dynamic and lives only in the
		// lambda's body, which is why the walk has to follow into it.
		// The refusal names `m`, not `w1`: the container is what the
		// author has to change, and the lambda is only how it was reached.
		{"FlexMap through a lambda", awaitPrelude + `def m (make FlexMap {})
def w1 ([] => [m set a 1])
def w2 ([] => [m set b 2])
TimeUtil.await [[w1] [w2]] end`, "m"},
		// Nested inside an immutable Map: the container is still shared,
		// so the enclosing binding is refused.
		{"FlexMap nested in a plain Map", awaitPrelude + `def box (make FlexMap {})
def holder {nested: box}
TimeUtil.await [[(holder dot nested) set a 1] [(holder dot nested) set b 2]] end`, "holder"},
		// FlexList is the same hazard with a different payload.
		{"FlexList by def", awaitPrelude + `def l (flex [0])
TimeUtil.await [[l set 0 1] [l set 0 2]] end`, "l"},
	}
	for _, c := range cases {
		for _, mode := range []string{"compiled", "interpreted"} {
			t.Run(c.name+"/"+mode, func(t *testing.T) {
				a, _ := New()
				var err error
				if mode == "compiled" {
					_, err = a.Run(c.src)
				} else {
					_, err = a.RunInterp(c.src)
				}
				if err == nil {
					t.Fatalf("sharing a stateful container across branches must "+
						"be refused, got no error for:\n%s", c.src)
				}
				msg := fmt.Sprint(err)
				if !strings.Contains(msg, "not_sendable") {
					t.Errorf("want a not_sendable refusal, got: %s", msg)
				}
				if !strings.Contains(msg, "`"+c.want+"`") {
					t.Errorf("the refusal must NAME the offending binding (%q) so "+
						"the author knows which one to change, got: %s", c.want, msg)
				}
			})
		}
	}
}

// TestAwaitAcceptsImmutableAndScalarSharing is the negative half, and it
// carries most of the weight: a refusal rule that over-fires is worse than
// the bug, because it breaks working programs. An immutable Map's `set`
// returns a copy, so it was never the hazard and must stay legal.
func TestAwaitAcceptsImmutableAndScalarSharing(t *testing.T) {
	cases := []struct{ name, src, want string }{
		{"immutable Map", awaitPrelude + `def m {}
TimeUtil.await [[m set a 1] [m set b 2]] end
m`, "[[{a:1} {b:2}] {}]"},
		{"scalars and strings", awaitPrelude + `def n 40
def s "x"
TimeUtil.await [[n add 1] [n add 2] [s]] end`, "[[41 42 'x']]"},
		{"plain List", awaitPrelude + `def xs [1 2 3]
TimeUtil.await [[xs size] [xs size]] end`, "[[3 3]]"},
		// A RECURSIVE fn must not send the reachability walk into a loop.
		{"recursive fn reference", awaitPrelude +
			`def fact fn [[n:Integer] [Integer] [if (n lte 1) [1] [n mul (fact (n sub 1))]]]
TimeUtil.await [[fact 5] [fact 6]] end`, "[[120 720]]"},
		// A container built INSIDE a branch is private to it by
		// construction — the documented way to use one concurrently.
		{"FlexMap built inside each branch", awaitPrelude +
			`TimeUtil.await [[def a (make FlexMap {}) a set k 1] [def b (make FlexMap {}) b set k 2]] end`,
			"[[{k:1} {k:2}]]"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			a, _ := New()
			got, err := a.Run(c.src)
			if err != nil {
				t.Fatalf("must remain legal: %v", err)
			}
			if fmt.Sprint(got) != c.want {
				t.Errorf("got %v, want %s", got, c.want)
			}
			b, _ := New()
			want, _ := b.RunInterp(c.src)
			if fmt.Sprint(got) != fmt.Sprint(want) {
				t.Errorf("compiled %v != interpreter %v", got, want)
			}
		})
	}
}

// TestAwaitRefusalMissesCompiledFnLocalLambda pins a KNOWN LIMIT of the
// check, deliberately, so it is counted rather than believed away.
//
// The check sees what is bound in the def table when `await` dispatches.
// Compiled, some fn-body locals live in frame slots instead, so a lambda
// defined inside a fn body is simply unbound at that point and the
// indirection through it is not followed. Interpreted, it is.
//
// Both engines DO catch the same container named directly in a branch,
// and both catch the identical lambda at module level — so the gap is
// narrow. It is still an engine-mode divergence inside a check written to
// stop engine-mode divergences, which is exactly the sort of thing that
// should be visible in a test rather than in a comment nobody reads.
func TestAwaitRefusalMissesCompiledFnLocalLambda(t *testing.T) {
	const src = awaitPrelude + `def outer fn [[] [Any] [
  def m (make FlexMap {})
  def w ([] => [m set a 1])
  TimeUtil.await [[w] [w]]
]]
outer`
	a, _ := New()
	if _, err := a.Run(src); err != nil {
		t.Fatalf("LIMIT CLOSED — the compiled path now refuses a fn-local "+
			"lambda indirection (%v). Good: delete this test, and update the "+
			"LIMITS comment in native_temporal_await.go and the note in "+
			"EXPLANATION.md that both describe it as uncaught.", err)
	}
	// Interpreted, the same program IS refused — which is what makes this a
	// divergence rather than a uniform limitation.
	b, _ := New()
	if _, err := b.RunInterp(src); err == nil {
		t.Error("interpreted, this shape must still be refused; if it is not, " +
			"the walk into referenced function bodies has regressed")
	}
}

// TestAwaitRefusalAppliesToEveryMode pins the gate on all four combinator
// modes, not just the default. They are four separate entry points that
// each fork independently, so a check wired into only one of them would
// leave the other three open — the same "one seam of several" shape that
// made the compiled branch case ship inert.
func TestAwaitRefusalAppliesToEveryMode(t *testing.T) {
	for _, mode := range []string{`"all"`, `"full"`, `"first"`, `"any"`} {
		t.Run(mode, func(t *testing.T) {
			src := awaitPrelude + `def m (make FlexMap {})
TimeUtil.await {mode: ` + mode + `} [[m set a 1] [m set b 2]] end`
			a, _ := New()
			if _, err := a.Run(src); err == nil {
				t.Errorf("mode %s must refuse a shared mutable container", mode)
			} else if !strings.Contains(fmt.Sprint(err), "not_sendable") {
				t.Errorf("mode %s: want not_sendable, got %v", mode, err)
			}
		})
	}
}

// TestAwaitRefusalNamesTheContainerKind pins the message content. A
// refusal that says only "something is wrong" costs the author the
// debugging session the check was meant to save, so the kind and the
// remedy are part of the contract.
func TestAwaitRefusalNamesTheContainerKind(t *testing.T) {
	a, _ := New()
	_, err := a.Run(awaitPrelude + `def m (make FlexMap {})
TimeUtil.await [[m set a 1] [m set b 2]] end`)
	if err == nil {
		t.Fatal("want a refusal")
	}
	msg := fmt.Sprint(err)
	for _, frag := range []string{"FlexMap", "concurrent branches"} {
		if !strings.Contains(msg, frag) {
			t.Errorf("refusal should mention %q, got: %s", frag, msg)
		}
	}
}
