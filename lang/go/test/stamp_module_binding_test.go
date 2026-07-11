package test

import (
	"strings"
	"testing"

	eng "github.com/aql-lang/aql/eng/go"
	"github.com/aql-lang/aql/eng/go/parser"
	"github.com/aql-lang/aql/lang/go/native"
)

// stampEventsFor loads src as an inline module body (module load is the
// detached-stamp trigger site) with runtime stamping armed, then returns
// the stamp events recorded for its fns.
func stampEventsFor(t *testing.T, src string) []eng.StampEvent {
	t.Helper()
	reg, err := native.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	reg.ParseFunc = parser.Parse
	reg.EnableRuntimeStamping()
	engine := native.NewTop(reg)
	vals, pErr := parser.Parse(src)
	if pErr != nil {
		t.Fatal(pErr)
	}
	if _, rErr := engine.Run(vals); rErr != nil {
		t.Fatal(rErr)
	}
	return reg.StampEvents()
}

// runStampedModule loads modSrc (the detached-stamp trigger) with runtime
// stamping armed, then runs prog in the same engine and returns its result
// stack — the end-to-end parity harness for newly stamped shapes.
func runStampedModule(t *testing.T, modSrc, prog string) []native.Value {
	t.Helper()
	reg, err := native.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	reg.ParseFunc = parser.Parse
	reg.EnableRuntimeStamping()
	engine := native.NewTop(reg)
	for _, step := range []string{modSrc, prog} {
		vals, pErr := parser.Parse(step)
		if pErr != nil {
			t.Fatal(pErr)
		}
		out, rErr := engine.Run(vals)
		if rErr != nil {
			t.Fatal(rErr)
		}
		if step == prog {
			return out
		}
	}
	return nil // unreachable: the loop returns on the prog step by construction
}

func asStringVal(v native.Value) (string, error) { return eng.AsString(v) }

func stampOutcome(evs []eng.StampEvent, name string) (string, bool) {
	for _, ev := range evs {
		if ev.Name == name {
			if ev.Stamped {
				return "", true
			}
			return ev.Reason, false
		}
	}
	return "no stamp event", false
}

// TestStampModuleBindingValueRead pins that a stored-fn unit whose body
// passes a CONCRETE module-scope value binding as a dispatch operand
// (mini-s3's `Net.recv-until conn s3-crlf {…}` shape — the delimiter is a
// module-level `def s3-crlf (convert Bytes "\r\n")`) STAMPS, with the
// binding baked as a const guarded by the ref's dep snapshot: a later
// rebind of the name degrades the ref to the interpreter rather than
// serving the stale const (the depSnap/depsFresh machinery, pinned in
// eng/go/stamp_runtime_test.go and lang/go/stamp_runtime_vm_test.go —
// storedFnDeps collects every Defs-bound body word, including value
// bindings like crlf, so the existing freshness gates cover the newly
// baked read).
func TestStampModuleBindingValueRead(t *testing.T) {
	evs := stampEventsFor(t, `
module [
  def crlf (convert Bytes "\r\n")
  def read-modbind fn [[b:Bytes] [Any] [ size (b add crlf) ]]
  export "SockT" { read-modbind: read-modbind/r }
] import
`)
	reason, ok := stampOutcome(evs, "read-modbind")
	if !ok {
		t.Fatalf("read-modbind did not stamp: %s", reason)
	}
}

// TestStampLocalDelimStillStamps pins the sibling shapes that already
// stamped before this work (body-local and param-supplied values) so the
// module-binding extension cannot regress them.
func TestStampLocalDelimStillStamps(t *testing.T) {
	evs := stampEventsFor(t, `
module [
  def read-local fn [[b:Bytes] [Any] [ def d (convert Bytes "\r\n") size (b add d) ]]
  def read-param fn [[b:Bytes d:Bytes] [Any] [ size (b add d) ]]
  export "SockT" { read-local: read-local/r read-param: read-param/r }
] import
`)
	for _, name := range []string{"read-local", "read-param"} {
		if reason, ok := stampOutcome(evs, name); !ok {
			t.Fatalf("%s did not stamp: %s", name, reason)
		}
	}
}

// TestStampModuleBindingNotMaterialisable pins the negative: a module
// binding holding a value that cannot bake as a const (a mutable flex
// map — instance identity is shared across calls) must still refuse
// rather than bake a snapshot that would diverge from the interpreter's
// live instance.
func TestStampModuleBindingNotMaterialisable(t *testing.T) {
	evs := stampEventsFor(t, `
module [
  def shared (flex {n: 1})
  def read-flex fn [[k:String] [Any] [ shared get k ]]
  export "SockT" { read-flex: read-flex/r }
] import
`)
	reason, ok := stampOutcome(evs, "read-flex")
	if ok {
		t.Fatal("read-flex stamped; a mutable module instance must not bake into a unit")
	}
	if strings.Contains(reason, "no stamp event") {
		t.Fatalf("read-flex missing stamp event: %s", reason)
	}
}
