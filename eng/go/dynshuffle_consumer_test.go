package eng

import "testing"

// TestDynShuffleConsumerAtEdges pins dynShuffleConsumerAt's negative
// branches directly (the positive path — a PLAIN `drop` after a
// mixed-arity mutator over a dynamic receiver — is exercised end-to-end
// by lang/go/test/stamp_setdrop_test.go):
//
//   - an out-of-range index is not a consumer (the execMatch call site
//     bounds-checks before calling, but the helper must stay total);
//   - a non-word token is not a consumer;
//   - a shuffle NAME with no registered binding (a bare kernel registry)
//     or with a non-single/non-all-Any signature must not qualify — the
//     word's dispatch shape is then not the trusted stack-only one.
func TestDynShuffleConsumerAtEdges(t *testing.T) {
	r, err := NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	e := New(r)
	e.Tape = NewTape([]Value{NewWord("drop"), NewInteger(5)}, stackHeadroom)

	if e.dynShuffleConsumerAt(-1) {
		t.Fatal("negative index qualified as a shuffle consumer")
	}
	if e.dynShuffleConsumerAt(e.Tape.Len()) {
		t.Fatal("out-of-range index qualified as a shuffle consumer")
	}
	if e.dynShuffleConsumerAt(1) {
		t.Fatal("a literal token qualified as a shuffle consumer")
	}
	// `drop` is a shuffle NAME, but this bare kernel registry has no
	// binding for it — Lookup returns nil, so it must not qualify.
	if e.dynShuffleConsumerAt(0) {
		t.Fatal("an unregistered shuffle name qualified as a consumer")
	}

	// A registered `drop` whose single signature is NOT all-Any (an
	// Integer-typed shadow): the dispatch shape is type-directed, so it
	// must not qualify either.
	r.upsertFnDef("drop", Signature{Args: []*Type{TInteger}, Impl: Go(func(a []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) { return nil, nil })})
	if e.dynShuffleConsumerAt(0) {
		t.Fatal("a non-all-Any drop signature qualified as a consumer")
	}

	// The genuine shape: single all-Any signature — qualifies.
	r2, err := NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	r2.upsertFnDef("drop", Signature{Args: []*Type{TAny}, Impl: Go(func(a []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) { return nil, nil })})
	e2 := New(r2)
	e2.Tape = NewTape([]Value{NewWord("drop")}, stackHeadroom)
	if !e2.dynShuffleConsumerAt(0) {
		t.Fatal("a plain single all-Any drop did not qualify")
	}
	// A MODIFIED word never qualifies (the /s form changes collection).
	e2.Tape = NewTape([]Value{NewWordModified("drop", -1, true, false)}, stackHeadroom)
	if e2.dynShuffleConsumerAt(0) {
		t.Fatal("a modified shuffle word qualified as a consumer")
	}
}
