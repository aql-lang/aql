package eng

import "testing"

// TestRecordPolyCallTypeNameOperand pins RecordPolyCall's type-name
// cascade: a raw un-stepped Word in the claimed window that NAMES a live
// type is stepped to its canonical literal (mirroring stepWord) and bakes
// as an OpPushType operand instead of declining the whole poly window.
// The bare-name arm (`Bytes`, `String`) is driven end-to-end by
// lang/go/test/stamp_filterlambda_test.go; the SLASHED-path arm is only
// reachable from a programmatically built claim (the parser resolves
// slashed type paths to literals at parse time — see parseWord), so it is
// pinned here directly, the same seam stepWord's twin arm uses
// (engine_seam7_run_test.go::TestS7RunBareTypePathWord).
func TestRecordPolyCallTypeNameOperand(t *testing.T) {
	newState := func() *EmitState {
		r, err := NewRegistry()
		if err != nil {
			t.Fatal(err)
		}
		es := NewEmitState()
		es.bindRegistry(r)
		return es
	}
	lastOps := func(es *EmitState) []emitOperand {
		frame := es.frames[len(es.frames)-1]
		return frame[len(frame)-1].call.ops
	}

	// Slashed type path (the ResolveTypePath arm).
	es := newState()
	out := NewDynamicCarrier(TInteger)
	if !es.RecordPolyCall("convert", []Value{NewWord("Scalar/Number/Integer")}, []Value{out}, SrcPos{}, es.reg, nil) {
		t.Fatal("a slashed type-path word operand should record")
	}
	if ops := lastOps(es); len(ops) != 1 || ops[0].kind != opType {
		t.Fatalf("slashed-path operand = %+v, want a type operand", lastOps(es))
	}

	// Bare kernel type name (the typeNames arm).
	es = newState()
	if !es.RecordPolyCall("convert", []Value{NewWord("Integer")}, []Value{out}, SrcPos{}, es.reg, nil) {
		t.Fatal("a bare type-name word operand should record")
	}
	if ops := lastOps(es); len(ops) != 1 || ops[0].kind != opType {
		t.Fatalf("bare-name operand = %+v, want a type operand", lastOps(es))
	}

	// Negatives — anything but a PLAIN word naming a live type keeps the
	// unresolvable-operand decline:
	// a MODIFIED word (the /s form changes collection semantics);
	es = newState()
	if es.RecordPolyCall("convert", []Value{NewWordModified("Integer", -1, true, false)}, []Value{out}, SrcPos{}, es.reg, nil) {
		t.Fatal("a modified type-name word must decline")
	}
	// a word that names no type at all;
	es = newState()
	if es.RecordPolyCall("convert", []Value{NewWord("no-such-name")}, []Value{out}, SrcPos{}, es.reg, nil) {
		t.Fatal("a non-type word must decline")
	}
	// a slashed path that resolves to no builtin.
	es = newState()
	if es.RecordPolyCall("convert", []Value{NewWord("No/Such/Type")}, []Value{out}, SrcPos{}, es.reg, nil) {
		t.Fatal("an unknown slashed path must decline")
	}
}
