package native

import (
	"strings"
	"testing"
)

// Seam-6 coverage for native_patrun.go (design/TEST-SEAMS.10.md).

func seam6c2PatternMap(k string, v Value) Value {
	om := NewOrderedMap()
	om.Set(k, v)
	return NewMap(om)
}

func TestSeam6C2RegisterPatrunTypeDuplicate(t *testing.T) {
	saved := SwapTypeInitErrs(nil)
	t.Cleanup(func() { SwapTypeInitErrs(saved) })
	if tt := registerPatrunType(); tt != nil {
		t.Fatalf("duplicate registration must return nil, got %v", tt)
	}
	err := TypeInitError()
	if err == nil || !strings.Contains(err.Error(), "already registered") {
		t.Fatalf("expected already-registered init error, got %v", err)
	}
}

func TestSeam6C2AsPatrunNonExtension(t *testing.T) {
	if _, ok := asPatrun(NewInteger(1)); ok {
		t.Fatal("integer must not unwrap as Patrun")
	}
	if _, ok := asPatrun(NewPatrun(TAny)); !ok {
		t.Fatal("NewPatrun must unwrap")
	}
}

func TestSeam6C2PatrunValTypeDefault(t *testing.T) {
	if got := patrunValType(nil); !got.Equal(TAny) {
		t.Fatalf("empty args must default to Any, got %v", got)
	}
}

func TestSeam6C2PatrunHandlerReceiverErrors(t *testing.T) {
	r := seam5Reg(t)
	notPatrun := NewInteger(9)
	pat := seam6c2PatternMap("a", NewInteger(1))

	if _, err := patrunAddHandler([]Value{pat, NewString("A"), notPatrun}, nil, nil, r); err == nil ||
		!strings.Contains(err.Error(), "add: expected a Patrun") {
		t.Fatalf("expected add receiver error, got %v", err)
	}
	if _, err := patrunFindHandler([]Value{pat, notPatrun}, nil, nil, r); err == nil ||
		!strings.Contains(err.Error(), "find: expected a Patrun") {
		t.Fatalf("expected find receiver error, got %v", err)
	}
	if _, err := patrunRemoveHandler([]Value{pat, notPatrun}, nil, nil, r); err == nil ||
		!strings.Contains(err.Error(), "remove: expected a Patrun") {
		t.Fatalf("expected remove receiver error, got %v", err)
	}
	if _, err := patrunPatternsHandler([]Value{notPatrun}, nil, nil, r); err == nil ||
		!strings.Contains(err.Error(), "patterns: expected a Patrun") {
		t.Fatalf("expected patterns receiver error, got %v", err)
	}
}

func TestSeam6C2PatrunFindOptsAndSubjectErrors(t *testing.T) {
	r := seam5Reg(t)
	pv := NewPatrun(TAny)
	subj := seam6c2PatternMap("a", NewInteger(1))

	// Bad exact option type.
	badOpts := seam6c2PatternMap("exact", NewString("yes"))
	_, err := patrunFindHandler([]Value{subj, pv, badOpts}, nil, nil, r)
	if err == nil || !strings.Contains(err.Error(), "opts.exact") {
		t.Fatalf("expected opts.exact error, got %v", err)
	}

	// Non-concrete subject.
	_, err = patrunFindHandler([]Value{NewCarrier(TMap), pv}, nil, nil, r)
	if err == nil {
		t.Fatal("expected concrete-subject error")
	}

	// Positive pair: exact:true finds only the exact rule.
	if _, err := patrunAddHandler([]Value{subj, NewString("A"), pv}, nil, nil, r); err != nil {
		t.Fatal(err)
	}
	opts := seam6c2PatternMap("exact", NewBoolean(true))
	out, err := patrunFindHandler([]Value{subj, pv, opts}, nil, nil, r)
	if err != nil || len(out) != 1 {
		t.Fatalf("expected exact find hit, got %v / %v", out, err)
	}
	if s, _ := AsString(out[0]); s != "A" {
		t.Fatalf("expected A, got %v", out[0])
	}
}

func TestSeam6C2PatrunFindSideTableMiss(t *testing.T) {
	r := seam5Reg(t)
	pv := NewPatrun(TAny)
	pat := seam6c2PatternMap("a", NewInteger(1))
	if _, err := patrunAddHandler([]Value{pat, NewString("A"), pv}, nil, nil, r); err != nil {
		t.Fatal(err)
	}
	// Wipe the side table but leave the trie entry: find must degrade
	// to None rather than panic.
	m, ok := asPatrun(pv)
	if !ok {
		t.Fatal("expected patrun")
	}
	for k := range m.side {
		delete(m.side, k)
	}
	out, err := patrunFindHandler([]Value{pat, pv}, nil, nil, r)
	if err != nil || len(out) != 1 || !IsNoneShape(out[0]) {
		t.Fatalf("expected None for orphaned trie entry, got %v / %v", out, err)
	}
}

func TestSeam6C2PatrunRemoveNonConcretePattern(t *testing.T) {
	r := seam5Reg(t)
	pv := NewPatrun(TAny)
	_, err := patrunRemoveHandler([]Value{NewCarrier(TMap), pv}, nil, nil, r)
	if err == nil {
		t.Fatal("expected concrete-pattern error")
	}
	// Positive pair: removing a registered pattern makes find miss.
	pat := seam6c2PatternMap("a", NewInteger(1))
	if _, err := patrunAddHandler([]Value{pat, NewString("A"), pv}, nil, nil, r); err != nil {
		t.Fatal(err)
	}
	if _, err := patrunRemoveHandler([]Value{pat, pv}, nil, nil, r); err != nil {
		t.Fatal(err)
	}
	out, err := patrunFindHandler([]Value{pat, pv}, nil, nil, r)
	if err != nil || len(out) != 1 || !IsNoneShape(out[0]) {
		t.Fatalf("expected None after remove, got %v / %v", out, err)
	}
}

func TestSeam6C2PatrunOptBoolArms(t *testing.T) {
	// Non-map opts: silently false.
	b, err := patrunOptBool(NewInteger(1), "exact")
	if err != nil || b {
		t.Fatalf("non-map opts must read false, got %v / %v", b, err)
	}
	// Missing key: false.
	b, err = patrunOptBool(NewMap(NewOrderedMap()), "exact")
	if err != nil || b {
		t.Fatalf("missing key must read false, got %v / %v", b, err)
	}
	// Non-boolean value: error.
	if _, err := patrunOptBool(seam6c2PatternMap("exact", NewString("x")), "exact"); err == nil {
		t.Fatal("expected boolean coercion error")
	}
	// Positive pair.
	b, err = patrunOptBool(seam6c2PatternMap("exact", NewBoolean(true)), "exact")
	if err != nil || !b {
		t.Fatalf("expected true, got %v / %v", b, err)
	}
}

func TestSeam6C2PatrunBehaviorEqualAndFormat(t *testing.T) {
	r := seam5Reg(t)
	b := patrunBehavior{}
	if !b.Equal(NewInteger(1), NewInteger(1)) {
		t.Fatal("identical integers must compare equal via default behavior")
	}
	if b.Equal(NewInteger(1), NewInteger(2)) {
		t.Fatal("distinct integers must not compare equal")
	}

	// A non-Patrun payload falls back to the bare rendering (the
	// basic-side Behavior shell's arm; the matcher's own formatter
	// handles the empty case below).
	if got := b.Format(NewInteger(1)); got != "Patrun()" {
		t.Fatalf("non-patrun must format Patrun(), got %q", got)
	}
	pv := NewPatrun(TAny)
	if got := b.Format(pv); got != "Patrun()" {
		t.Fatalf("empty patrun must format Patrun(), got %q", got)
	}
	if _, err := patrunAddHandler([]Value{seam6c2PatternMap("a", NewInteger(1)), NewString("A"), pv}, nil, nil, r); err != nil {
		t.Fatal(err)
	}
	if _, err := patrunAddHandler([]Value{seam6c2PatternMap("b", NewInteger(2)), NewString("B"), pv}, nil, nil, r); err != nil {
		t.Fatal(err)
	}
	got := b.Format(pv)
	if got != "Patrun(a=1 -> A; b=2 -> B)" {
		t.Fatalf("unexpected format: %q", got)
	}
}
