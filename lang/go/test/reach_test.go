package test

import (
	"testing"

	eng "github.com/aql-lang/aql/eng/go"
	"github.com/aql-lang/aql/lang/go/native"
)

// Reach Phase D: the `reach` constructor (an inert lens value) and
// inspection via convert Map / convert List. See design/REACH.0.md §7.

func TestReachConstructorIsInert(t *testing.T) {
	// `reach` builds a Reach value that does NOT auto-evaluate.
	res, err := runNativeSteps(t, nil, []string{`reach 5 [a/q b/q]`})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if len(res) != 1 || !eng.IsReach(res[0]) {
		t.Fatalf("reach should yield an inert Reach value, got %v", res)
	}
}

func TestReachConvertMapInspects(t *testing.T) {
	// convert Map on a codequote'd reach exposes receiver + segments.
	res, err := runNativeSteps(t, nil, []string{`convert Map (codequote m.a.b)`})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	m, err := native.AsMap(res[0])
	if err != nil || m == nil {
		t.Fatalf("convert Map should yield a map, got %v", res[0])
	}
	if _, ok := m.Get("receiver"); !ok {
		t.Error("inspected reach map missing 'receiver'")
	}
	segs, ok := m.Get("segments")
	if !ok {
		t.Fatal("inspected reach map missing 'segments'")
	}
	sl, slErr := native.AsList(segs)
	if slErr != nil || sl.Len() != 2 {
		t.Errorf("expected 2 segments for m.a.b, got %v", segs)
	}
}

func TestReachConvertListKeys(t *testing.T) {
	// convert List on a reach yields its segment keys.
	res, err := runNativeSteps(t, nil, []string{`convert List (codequote m.a.b)`})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	l, err := native.AsList(res[0])
	if err != nil || l.Len() != 2 {
		t.Fatalf("expected 2 keys [a b] for m.a.b, got %v", res[0])
	}
}

// Parity guard: a parsed dot-access still evaluates eagerly (not a Reach value).
func TestParsedDotAccessStaysEager(t *testing.T) {
	res, err := runNativeSteps(t, nil, []string{`def m {a: {b: 7}}`, `m.a.b`})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if n, _ := eng.AsInteger(res[0]); n != 7 {
		t.Errorf("m.a.b = %v, want 7 (eager)", res[0])
	}
}
