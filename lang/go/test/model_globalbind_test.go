package test

import (
	"fmt"
	"strings"
	"testing"

	lang "github.com/boru-lang/boru/lang/go"
)

// The 2026-07-15 flip composite's root cause, pinned at the module level: a
// compiled request `def h (Model.new …)` kept the check pass's Model CARRIER
// as the persistent binding, so the NEXT request — the watch test's
// `Model.stop mdl`, in either engine — raised model_bad_handle. OpBindGlobal
// writes the runtime Extension handle back into the kept slot. (The
// language-level classes are pinned in lang/go/bytecode_globalbind_test.go;
// these two pin the handle-bearing module shapes that surfaced the bug.)
func TestModelHandleDefPersistsAcrossRequests(t *testing.T) {
	a, err := lang.New()
	if err != nil {
		t.Fatal(err)
	}
	src := modelImp + `def h (Model.new {src:'a: 1'}) (Model.run h) get 'ok'`
	vals, ran, reason, rerr := a.RunAutoValues(src)
	if rerr != nil || !ran {
		t.Fatalf("compiled request: ran=%v reason=%q err=%v", ran, reason, rerr)
	}
	if len(vals) == 0 || vals[len(vals)-1].String() != "true" {
		t.Fatalf("run ok: %v", vals)
	}
	// Request 2, interpreter: the handle must be the live Extension.
	out, rerr := a.RunInterp(`(Model.model h) get 'a'`)
	if rerr != nil {
		t.Fatalf("next-request Model.model: %v", rerr)
	}
	if fmt.Sprint(out) != "[1]" {
		t.Errorf("next-request model read = %v, want [1]", out)
	}
	// And Model.stop — the exact word the watch composite failed on — must
	// see a Model, not a carrier (a no-watch handle stops as a no-op).
	if _, rerr := a.RunInterp(`Model.stop h`); rerr != nil {
		t.Errorf("next-request Model.stop: %v", rerr)
	}
}

// A service handle stored by a compiled request keeps accepting handlers on
// the next request (pre-fix: "add: expected a Service, got Service" — the
// carrier's rendering makes the old failure mode legible).
func TestServiceHandleDefPersistsAcrossRequests(t *testing.T) {
	a, err := lang.New()
	if err != nil {
		t.Fatal(err)
	}
	if _, ran, reason, rerr := a.RunAutoValues(`def svc (service {})`); rerr != nil || !ran {
		t.Fatalf("compiled request: ran=%v reason=%q err=%v", ran, reason, rerr)
	}
	out, rerr := a.RunInterp(`add {cmd:"X"} ([req:Map state:Any] => [ 42 ]) svc (call {cmd:"X"} svc)`)
	if rerr != nil {
		t.Fatalf("next-request service use: %v", rerr)
	}
	if !strings.Contains(fmt.Sprint(out), "42") {
		t.Errorf("next-request service call = %v, want 42", out)
	}
}
