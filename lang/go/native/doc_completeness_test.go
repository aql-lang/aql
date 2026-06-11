package native

import (
	"sort"
	"strings"
	"testing"

	"github.com/aql-lang/aql/lang/go/native/help"
)

// TestEveryWordHasDescribeDocs is the documentation-completeness gate for the
// `describe` system: every registered core word must carry a full help Entry
// (a non-empty Summary AND Description), so `aql describe <word>` never shows
// "<not described>". Engine-internal marker words (Word/__*) are exempt.
//
// Module words are guarded separately by modules.TestModuleExportDocs (each
// export carries a Doc), and the module catalog by
// modules.TestModuleCatalogMatchesModules — together the three keep describe
// complete across words, modules, and module words.
//
// When this fails, the named word was registered without docs: add an Entry
// (Summary + Description, ideally Examples) in lang/go/native/help/help_*.go
// and slot it into a category in help_categories.go.
func TestEveryWordHasDescribeDocs(t *testing.T) {
	reg, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}

	var missing, thin []string
	for _, name := range reg.Defs.Names() {
		if reg.Lookup(name) == nil {
			continue // a def/value binding, not a registered word
		}
		if strings.HasPrefix(name, "__") {
			continue // engine-internal marker word
		}
		e := help.Lookup(name)
		switch {
		case e == nil:
			missing = append(missing, name)
		case strings.TrimSpace(e.Summary) == "" || strings.TrimSpace(e.Description) == "":
			thin = append(thin, name)
		}
	}

	sort.Strings(missing)
	sort.Strings(thin)
	if len(missing) > 0 {
		t.Errorf("%d registered word(s) have no help Entry (describe shows <not described>):\n  %s",
			len(missing), strings.Join(missing, " "))
	}
	if len(thin) > 0 {
		t.Errorf("%d word(s) have a help Entry but an empty Summary or Description:\n  %s",
			len(thin), strings.Join(thin, " "))
	}
}
