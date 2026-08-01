package native

import (
	"strings"
	"testing"

	"github.com/boru-lang/boru/lang/go/native/help"
)

// fnDefWithProvenance builds a module-export-style FnDef value carrying
// Module/Export/Doc provenance and a single 2-arg forward-eligible sig,
// mirroring what a module maker + stampExportProvenance produce.
func fnDefWithProvenance() Value {
	return NewFunction(FnDefInfo{
		Name:   "indices",
		Module: "boru:array-util",
		Export: "ArrayUtil",
		Doc:    "Position of each needle in the haystack, -1 if absent.",
		Signatures: []FnSig{{
			Params:     []FnParam{{Type: TList}, {Type: TList}},
			Returns:    []*Type{TList},
			Impl:       Boru([]Value{NewWord("indices")}),
			BarrierPos: -1,
		}},
	})
}

// TestFnDefFuncInfoProvenance pins that FnDefFuncInfo carries the module
// origin and doc onto the help.FuncInfo, treats the -1 barrier sentinel as
// forward-eligible, and uses the signature's authored Returns.
func TestFnDefFuncInfoProvenance(t *testing.T) {
	fn, _ := FnDefFromValue(fnDefWithProvenance())
	info := FnDefFuncInfo("ArrayUtil.indices", fn)

	if info.Name != "ArrayUtil.indices" {
		t.Errorf("Name = %q, want ArrayUtil.indices", info.Name)
	}
	if info.Module != "boru:array-util" {
		t.Errorf("Module = %q, want boru:array-util", info.Module)
	}
	if !strings.HasPrefix(info.Doc, "Position of each needle") {
		t.Errorf("Doc = %q, want the indices summary", info.Doc)
	}
	if !info.ForwardArgs {
		t.Error("ForwardArgs = false; BarrierPos -1 should read as forward")
	}
	if len(info.Sigs) != 1 || len(info.Sigs[0].Args) != 2 {
		t.Fatalf("Sigs = %+v, want one 2-arg sig", info.Sigs)
	}
	if got := info.Sigs[0].Returns; len(got) != 1 || !strings.HasSuffix(got[0], "List") {
		t.Errorf("Returns = %v, want a List return", got)
	}
}

// TestBuildQualifiedFuncInfo resolves a dotted name against the namespace
// binding `import` installs (a facet-carrying namespace Map in the def stack), and asserts
// the negative cases return nil rather than mis-resolving.
func TestBuildQualifiedFuncInfo(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatalf("DefaultRegistry: %v", err)
	}
	fields := NewOrderedMap()
	fields.Set("indices", fnDefWithProvenance())
	r.Defs.Push("ArrayUtil", NewModuleNamespace("ArrayUtil", fields, Value{}))

	info := BuildQualifiedFuncInfo(r, "ArrayUtil.indices")
	if info == nil {
		t.Fatal("BuildQualifiedFuncInfo returned nil for an imported export")
	}
	if info.Module != "boru:array-util" || info.Name != "ArrayUtil.indices" {
		t.Errorf("got Name=%q Module=%q", info.Name, info.Module)
	}

	// Negatives: bare name, unbound namespace, unknown word, no dot.
	for _, bad := range []string{"indices", "Nope.indices", "ArrayUtil.nope", "ArrayUtil", ".indices", "ArrayUtil."} {
		if got := BuildQualifiedFuncInfo(r, bad); got != nil {
			t.Errorf("BuildQualifiedFuncInfo(%q) = %+v, want nil", bad, got)
		}
	}
}

// TestFormatDynamicModuleHeader pins the rendered describe output for a
// module export: the doc as the header summary and a Module: provenance line.
func TestFormatDynamicModuleHeader(t *testing.T) {
	fn, _ := FnDefFromValue(fnDefWithProvenance())
	out := help.FormatDynamic(*FnDefFuncInfo("ArrayUtil.indices", fn))

	if !strings.Contains(out, "ArrayUtil.indices — Position of each needle") {
		t.Errorf("header missing doc summary:\n%s", out)
	}
	if !strings.Contains(out, "Module: boru:array-util") {
		t.Errorf("output missing Module provenance line:\n%s", out)
	}
	if !strings.Contains(out, "forward") {
		t.Errorf("expected forward precedence:\n%s", out)
	}
}

// TestBuildFuncInfoKeywordSlots pins that BuildFuncInfo carries the
// keyword-slot render overrides for def's synthesized constructor
// forms — position 1 of the `def name fn [...]` form is `fn/q`, while
// the name-capture /q slot at position 0 (no pattern) stays bare.
func TestBuildFuncInfoKeywordSlots(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	info := BuildFuncInfo(r, "def")
	if info == nil {
		t.Fatal("BuildFuncInfo(def) = nil")
	}
	var sawFnForm, sawGenChain bool
	for _, s := range info.Sigs {
		// The plain fn form: [Atom fn/q List] — pos 1 is the `fn` keyword.
		if len(s.Args) == 3 && s.Keywords[1] == "fn/q" {
			sawFnForm = true
			if _, ok := s.Keywords[0]; ok {
				t.Error("position 0 (name capture, no pattern) must NOT be a keyword slot")
			}
		}
		// A gen chain: [Atom gen/q List <ctor>/q ...] — pos 1 is `gen`.
		if len(s.Args) >= 4 && s.Keywords[1] == "gen/q" && s.Keywords[3] != "" {
			sawGenChain = true
		}
	}
	if !sawFnForm {
		t.Error("no `def name fn [...]` form rendered position 1 as fn/q")
	}
	if !sawGenChain {
		t.Error("no gen-chain form rendered gen/q + a ctor keyword")
	}
}

// TestSigKeywordSlotsHelper unit-tests the shared populate helper: a /q
// slot with an Atom pattern is a keyword; a /q slot with no pattern and
// a non-/q slot are not.
func TestSigKeywordSlotsHelper(t *testing.T) {
	kwAtom := NewAtom("fn")
	sig := Signature{
		Args:      []*Type{TAtom, TAtom, TList},
		QuoteArgs: map[int]bool{0: true, 1: true},
		Patterns:  map[int]Value{1: kwAtom},
	}
	kw := sigKeywordSlots(sig)
	if kw[1] != "fn/q" {
		t.Errorf("position 1 = %q, want fn/q", kw[1])
	}
	if _, ok := kw[0]; ok {
		t.Error("position 0 (/q, no pattern) must not be a keyword slot")
	}
	// A sig with no keyword slots yields nil.
	plain := Signature{Args: []*Type{TInteger}}
	if sigKeywordSlots(plain) != nil {
		t.Error("a plain sig must yield nil keyword slots")
	}
	// Defensive: a /q slot whose pattern conforms to Atom but is a BARE
	// atom type literal (no payload) is skipped, not rendered.
	bareLit := Signature{
		Args:      []*Type{TAtom},
		QuoteArgs: map[int]bool{0: true},
		Patterns:  map[int]Value{0: NewTypeLiteral(TAtom)},
	}
	if sigKeywordSlots(bareLit) != nil {
		t.Error("a bare-atom-literal pattern must not render as a keyword")
	}
}
