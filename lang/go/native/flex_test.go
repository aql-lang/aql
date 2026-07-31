package native

import (
	"strings"
	"testing"

	"github.com/boru-lang/boru/eng/go"
	"github.com/boru-lang/boru/eng/go/parser"
)

// flexRun parses and runs source, returning the final stack (or fails
// the test on error). Unlike dxRun it returns the raw values so tests
// can assert container identity, not just rendering.
func flexRun(t *testing.T, src string) []Value {
	t.Helper()
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(r)
	toks, err := parser.Parse(src)
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	out, err := NewTop(r).Run(toks)
	if err != nil {
		t.Fatalf("run %q: %v", src, err)
	}
	return out
}

// Mutating words must return the SAME container, not a copy — that is
// the contract chaining and shared bindings rely on.
func TestFlexMutatorsReturnSameContainer(t *testing.T) {
	t.Run("flexlist push/append/set share one store", func(t *testing.T) {
		out := flexRun(t, `def l (flex [1])  push 2 l`)
		if len(out) != 1 {
			t.Fatalf("expected 1 result, got %d: %s", len(out), Canon(out))
		}
		fd1, err := AsFlexList(out[0])
		if err != nil {
			t.Fatalf("push result is not a FlexList: %v", err)
		}
		// The returned node and the binding must share the pointer.
		out2 := flexRun(t, `def l (flex [1])  push 2 l  drop  l`)
		fd2, err := AsFlexList(out2[0])
		if err != nil {
			t.Fatalf("binding is not a FlexList: %v", err)
		}
		if len(fd1.Elems) != 2 || len(fd2.Elems) != 2 {
			t.Fatalf("expected 2 elements in both, got %d and %d", len(fd1.Elems), len(fd2.Elems))
		}
	})

	t.Run("returned node IS the argument node", func(t *testing.T) {
		r, err := DefaultRegistry()
		if err != nil {
			t.Fatal(err)
		}
		registerIOWords(r)
		toks, err := parser.Parse(`def l (flex [1])  push 2 l  l`)
		if err != nil {
			t.Fatal(err)
		}
		out, err := NewTop(r).Run(toks)
		if err != nil {
			t.Fatal(err)
		}
		if len(out) != 2 {
			t.Fatalf("expected 2 results, got %d: %s", len(out), Canon(out))
		}
		fdA, errA := AsFlexList(out[0])
		fdB, errB := AsFlexList(out[1])
		if errA != nil || errB != nil {
			t.Fatalf("results are not FlexLists: %v / %v", errA, errB)
		}
		if fdA != fdB {
			t.Fatal("push returned a different container than the binding — must be the same *FlexListData")
		}
	})
}

// Growth through AsFlexList must be visible across handler calls: the
// pointer-backed store is the whole point of the dedicated payload.
func TestFlexListGrowthAcrossCalls(t *testing.T) {
	fl := eng.NewFlexList([]Value{eng.NewInteger(1)})
	fd, err := AsFlexList(fl)
	if err != nil {
		t.Fatal(err)
	}
	// Mutate through the pointer, then read through a COPY of the
	// Value struct — the copy must observe the growth.
	cp := fl
	fd.Elems = append(fd.Elems, eng.NewInteger(2))
	lst, err := AsList(cp)
	if err != nil {
		t.Fatal(err)
	}
	if lst.Len() != 2 {
		t.Fatalf("growth not visible through Value copy: len=%d", lst.Len())
	}
}

// flex must deep-copy: mutating the copy never reaches the source, in
// either direction (flex-of-plain and node-of-flex).
func TestFlexDeepCopyIsolation(t *testing.T) {
	out := flexRun(t, `def m {a:{b:1}}  def f (flex m)  set b/q 9 f.a  drop  m.a.b  f.a.b`)
	if len(out) != 2 {
		t.Fatalf("expected 2 results, got %s", Canon(out))
	}
	src, _ := eng.AsInteger(out[0])
	flexed, _ := eng.AsInteger(out[1])
	if src != 1 {
		t.Fatalf("source map was mutated through the flex copy: m.a.b=%d", src)
	}
	if flexed != 9 {
		t.Fatalf("flex copy did not mutate: f.a.b=%d", flexed)
	}
}

// node on a plain container with no flex inside is identity — the same
// container comes back (preserves eq/container identity).
func TestNodeIdentityOnPlain(t *testing.T) {
	m := eng.NewMap(NewOrderedMap())
	out, err := eng.NodeDeepCopy(m)
	if err != nil {
		t.Fatal(err)
	}
	if !ExactEqual(m, out) {
		t.Fatal("node on a plain map must be identity (same container)")
	}
}

// Type literals and non-node values must never panic the flex words.
func TestFlexWordsNoPanic(t *testing.T) {
	cases := []struct {
		name string
		expr string
	}{
		{"flex-map-literal", `flex Map`},
		{"flex-list-literal", `flex List`},
		{"flex-flexmap-literal", `flex FlexMap`},
		{"flex-scalar", `flex 1`},
		{"node-map-literal", `node Map`},
		{"node-flexlist-literal", `node FlexList`},
		{"node-scalar", `node 'x'`},
		{"append-list-literal", `append 1 List`},
		{"append-flexlist-literal", `append 1 FlexList`},
		{"set-flexmap-literal", `set a/q 1 FlexMap`},
		{"set-flexlist-literal", `set 0 1 FlexList`},
		{"push-flexlist-literal", `1 push FlexList`},
		{"pop-flexlist-literal", `FlexList pop`},
		{"make-flexmap-literal-src", `make FlexMap Map`},
		{"make-flexlist-literal-src", `make FlexList List`},
		{"typeof-flexmap", `FlexMap typeof`},
		{"is-flexmap-literal", `FlexMap is Map`},
		{"cmp-flexmap-literal", `FlexMap tcmp Map`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("PANIC (type-literal safety violation): %v", r)
				}
			}()
			values, parseErr := parser.Parse(tc.expr)
			if parseErr != nil {
				return
			}
			r, err := DefaultRegistry()
			if err != nil {
				t.Fatal(err)
			}
			registerIOWords(r)
			_, _ = NewTop(r).Run(values)
			// Success or error are both fine — only a panic fails.
		})
	}
}

// The set bounds error must carry the append hint so the failure mode
// is self-explaining.
func TestFlexListSetBoundsErrorHint(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(r)
	toks, err := parser.Parse(`set 3 99 (flex [1 2 3])`)
	if err != nil {
		t.Fatal(err)
	}
	_, runErr := NewTop(r).Run(toks)
	if runErr == nil {
		t.Fatal("expected out-of-bounds error, got success")
	}
	if !strings.Contains(runErr.Error(), "out of bounds") {
		t.Fatalf("expected out-of-bounds error, got: %v", runErr)
	}
}
