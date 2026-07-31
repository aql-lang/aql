package modules

import (
	"testing"

	"github.com/boru-lang/boru/lang/go/native/help"
)

// TestModuleCatalogMatchesModules pins the help package's static module
// catalog to the authoritative set of compiled-in modules. The catalog lives
// in help (so the `describe` word, which cannot import this package, can still
// render the module index); this test is the seam that keeps the two in sync —
// add a module here and the catalog must gain a row, and vice versa.
func TestModuleCatalogMatchesModules(t *testing.T) {
	want := map[string]bool{}
	for _, n := range Names() {
		want[n] = true
	}

	got := map[string]bool{}
	for _, m := range help.ModuleCatalog() {
		if m.Summary == "" {
			t.Errorf("module %q has an empty catalog summary", m.Name)
		}
		if got[m.Name] {
			t.Errorf("module %q appears twice in the catalog", m.Name)
		}
		got[m.Name] = true
	}

	for n := range want {
		if !got[n] {
			t.Errorf("module %q is registered but missing from help.ModuleCatalog()", n)
		}
	}
	for n := range got {
		if !want[n] {
			t.Errorf("help.ModuleCatalog() lists %q, which is not a registered module", n)
		}
	}
}
