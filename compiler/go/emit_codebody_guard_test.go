package compiler

import (
	"strings"
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// TestRecordCodeBodyClosureReadArms pins the §9f guard: a code BODY
// argument that READS a name dyn-bound to a compiled closure refuses,
// because the body's native re-runs its tokens through the interpreter
// and a ClosurePayload is invokable only through the VM's re-entrant
// runner. The scope is the READ, not the bind — §9b's factory family
// applies exactly such closures from compiled code and must stay
// compiling, which a blanket refusal at the bind would have cost.
func TestRecordCodeBodyClosureReadArms(t *testing.T) {
	es := NewEmitState()
	body := core.NewList([]core.Value{core.NewWord("h"), core.NewInteger(1)})

	// No noted closures: nothing to refuse.
	if es.recordCodeBodyClosureRead([]core.Value{body}) {
		t.Error("an empty closure set must not refuse")
	}

	es.dynBoundClosures = map[string]bool{"h": true}

	// A non-list argument is not a body.
	if es.recordCodeBodyClosureRead([]core.Value{core.NewInteger(3)}) {
		t.Error("a non-list argument must not refuse")
	}
	// A list carrier carries no readable tokens.
	if es.recordCodeBodyClosureRead([]core.Value{core.NewCarrier(core.TList)}) {
		t.Error("a list carrier must not refuse")
	}
	// A body naming an UNRELATED word is untouched.
	other := core.NewList([]core.Value{core.NewWord("g"), core.NewInteger(1)})
	if es.recordCodeBodyClosureRead([]core.Value{other}) {
		t.Error("a body reading an unrelated name must not refuse")
	}
	// The shape itself: the body reads the dyn-bound closure.
	if !es.recordCodeBodyClosureRead([]core.Value{other, body}) {
		t.Fatal("a body reading a dyn-bound compiled closure must refuse")
	}
	if es.Compilable || !strings.Contains(es.Reason, "compiled closure") {
		t.Errorf("the refusal must mark the program (compilable=%v reason=%q)", es.Compilable, es.Reason)
	}
}
