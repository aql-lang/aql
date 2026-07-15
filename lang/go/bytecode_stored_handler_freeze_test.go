package lang

import (
	"fmt"
	"testing"
)

// TestCompiledStoredHandlerFreezeRedefine pins the fix for PR #243 comment #2: a compiled
// stored service handler FREEZES its module-level dependencies at the `add`
// (registration) source point, while the interpreter resolves those same names at
// CALL time. If a dependency is undef'd or redefined BETWEEN the `add` and the
// `call`, the frozen unit served the stale definition — a compile ≠ interpret
// MISCOMPILE (the cardinal forbidden outcome). The fix: a def/undef of a name an
// already-created stored ref reads (NotifyNameRebound) POISONS that ref, so
// Finalize leaves it unstamped (Prog nil) and InvokeCallback falls back to
// CallAQL — the interpreter, which resolves the live definition. compile ==
// interpret MUST hold, and the ref must NOT be stamped (StoredRefStampedCount 0).
func TestCompiledStoredHandlerFreezeRedefine(t *testing.T) {
	cases := []struct{ name, src, want string }{
		// A USER FN dep is undef'd then redefined between add and call. Frozen
		// helper=x+1 → 6; live helper=x+2 → 7. Fall back → 7.
		{"user fn undef+redef",
			`def helper ([x:Integer] => [x add 1])
def svc (service {})
add {} ([r:Map state:Any] => [helper 5]) svc
undef helper
def helper ([x:Integer] => [x add 2])
call {} svc`, "[7]"},
		// A bare redefinition (no undef) of a user fn dep — still diverges.
		{"user fn bare redef",
			`def helper ([x:Integer] => [x add 1])
def svc (service {})
add {} ([r:Map state:Any] => [helper 5]) svc
def helper ([x:Integer] => [x add 2])
call {} svc`, "[7]"},
		// A DATA def dep is undef'd then redefined between add and call.
		{"data def undef+redef",
			`def k 6
def svc (service {})
add {} ([r:Map state:Any] => [k]) svc
undef k
def k 11
call {} svc`, "[11]"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			a, _ := New()
			prog, reason, _, err := a.CompileCheck(c.src)
			if err != nil {
				t.Fatalf("CompileCheck error: %v", err)
			}
			if prog == nil {
				t.Fatalf("top-level program must still compile, refused: %q", reason)
			}
			// The handler ref was created at `add`, then poisoned by the later
			// def/undef of its dep — so it is recorded but NOT stamped.
			if got := prog.StoredRefCount(); got != 1 {
				t.Fatalf("StoredRefCount = %d, want 1 (the handler ref)", got)
			}
			if got := prog.StoredRefStampedCount(); got != 0 {
				t.Errorf("StoredRefStampedCount = %d, want 0 (poisoned → interpreter fallback)", got)
			}
			got, err := a.RunCompiledStrict(c.src)
			if err != nil {
				t.Fatalf("RunCompiledStrict: %v", err)
			}
			b, _ := New()
			want, _ := b.RunInterp(c.src)
			if fmt.Sprint(got) != fmt.Sprint(want) {
				t.Errorf("compiled %v != interpreter %v (MISCOMPILE)", got, want)
			}
			if fmt.Sprint(got) != c.want {
				t.Errorf("got %v, want %s", got, c.want)
			}
		})
	}
}

// TestCompiledStoredHandlerStableDepCompiles is the POSITIVE guard: a stored handler over
// a module dependency that is NEVER redefined (the shape of every real aql:net
// app handler — todo-api's live-todos, mini-redis's arg-at/kv-read) MUST still
// compile its unit and be stamped. Proves the fix is PRECISE — keyed on actual
// redefinition, not "reads a module ref" — so the apps keep their compiled
// speedup. compile == interpret, and the ref IS stamped (StoredRefStampedCount 1).
func TestCompiledStoredHandlerStableDepCompiles(t *testing.T) {
	src := `def helper ([x:Integer] => [x add 1])
def svc (service {})
add {} ([r:Map state:Any] => [helper 5]) svc
call {} svc`
	a, _ := New()
	prog, reason, _, err := a.CompileCheck(src)
	if err != nil {
		t.Fatalf("CompileCheck error: %v", err)
	}
	if prog == nil {
		t.Fatalf("must compile, refused: %q", reason)
	}
	if got := prog.StoredRefCount(); got != 1 {
		t.Fatalf("StoredRefCount = %d, want 1", got)
	}
	if got := prog.StoredRefStampedCount(); got != 1 {
		t.Errorf("StoredRefStampedCount = %d, want 1 (stable dep → compiled unit)", got)
	}
	got, err := a.RunCompiledStrict(src)
	if err != nil {
		t.Fatalf("RunCompiledStrict: %v", err)
	}
	b, _ := New()
	want, _ := b.RunInterp(src)
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("compiled %v != interpreter %v", got, want)
	}
	if fmt.Sprint(got) != "[6]" {
		t.Errorf("got %v, want [6] (frozen == live == helper 5 = 6)", got)
	}
}
