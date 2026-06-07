package modules

import (
	"sort"
	"testing"

	"github.com/aql-lang/aql/eng/go/parser"
	"github.com/aql-lang/aql/lang/go/native"
)

// TestModuleExportDocs asserts that every native-module FUNCTION export has a
// one-line Doc (sourced from the moduleDocs table in docs.go and applied by
// stampExportProvenance). It also pins the provenance: each function export
// gets its Module (import id) and Export (namespace) stamped. This keeps the
// doc table from falling behind the export set — a new export without a doc
// fails here, the describe-DX analogue of the ADR-003 spec-coverage guard.
//
// Type exports (capitalised names like Decision.Cond) are not functions and
// are intentionally not required to carry a Doc.
func TestModuleExportDocs(t *testing.T) {
	reg, err := native.DefaultRegistry()
	if err != nil {
		t.Fatalf("DefaultRegistry: %v", err)
	}
	reg.SetParseFunc(parser.Parse) // AQL modules parse their source on import

	var missing []string
	names := Names()
	sort.Strings(names)
	for _, name := range names {
		desc, derr := Resolve(name, reg)
		if derr != nil {
			t.Fatalf("resolve module %q: %v", name, derr)
		}
		nss := make([]string, 0, len(desc.Exports))
		for ns := range desc.Exports {
			nss = append(nss, ns)
		}
		sort.Strings(nss)
		for _, ns := range nss {
			om := desc.Exports[ns]
			for _, key := range om.Keys() {
				v, _ := om.Get(key)
				fn, ok := native.FnDefFromValue(v)
				if !ok {
					continue // type export or non-function value
				}
				qualified := ns + "." + key
				if fn.Module != desc.Ref {
					t.Errorf("%s: Module = %q, want %q", qualified, fn.Module, desc.Ref)
				}
				if fn.Export != ns {
					t.Errorf("%s: Export = %q, want %q", qualified, fn.Export, ns)
				}
				if fn.Doc == "" {
					missing = append(missing, qualified)
				}
			}
		}
	}

	if len(missing) > 0 {
		sort.Strings(missing)
		t.Fatalf("%d native-module function export(s) have no Doc summary. "+
			"Add an entry per word to moduleDocs in lang/go/modules/docs.go:\n  %s",
			len(missing), join(missing))
	}
}

func join(s []string) string {
	out := ""
	for i, v := range s {
		if i > 0 {
			out += "\n  "
		}
		out += v
	}
	return out
}
