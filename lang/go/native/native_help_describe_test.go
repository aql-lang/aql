package native

import (
	"strings"
	"testing"

	"github.com/aql-lang/aql/lang/go/native/help"
)

// fnDefWithProvenance builds a module-export-style FnDef value carrying
// Module/Export/Doc provenance and a single 2-arg forward-eligible sig,
// mirroring what a module maker + stampExportProvenance produce.
func fnDefWithProvenance() Value {
	return NewFnDef(FnDefInfo{
		Name:   "indices",
		Module: "aql:array-util",
		Export: "ArrayUtil",
		Doc:    "Position of each needle in the haystack, -1 if absent.",
		Signatures: []FnSig{{
			Params:     []FnParam{{Type: TList}, {Type: TList}},
			Returns:    []*Type{TList},
			Body:       []Value{NewWord("indices")},
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
	if info.Module != "aql:array-util" {
		t.Errorf("Module = %q, want aql:array-util", info.Module)
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
// binding `import` installs (a ModuleExport in the def stack), and asserts
// the negative cases return nil rather than mis-resolving.
func TestBuildQualifiedFuncInfo(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatalf("DefaultRegistry: %v", err)
	}
	fields := NewOrderedMap()
	fields.Set("indices", fnDefWithProvenance())
	r.Defs.Push("ArrayUtil", NewModuleExport("ArrayUtil", fields, Value{}))

	info := BuildQualifiedFuncInfo(r, "ArrayUtil.indices")
	if info == nil {
		t.Fatal("BuildQualifiedFuncInfo returned nil for an imported export")
	}
	if info.Module != "aql:array-util" || info.Name != "ArrayUtil.indices" {
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
	if !strings.Contains(out, "Module: aql:array-util") {
		t.Errorf("output missing Module provenance line:\n%s", out)
	}
	if !strings.Contains(out, "forward") {
		t.Errorf("expected forward precedence:\n%s", out)
	}
}
