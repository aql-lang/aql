package native

import (
	"testing"

	"github.com/aql-lang/aql/lang/go/native/help"
)

// The category table must agree with the ENGINE registry, not merely with
// the documentation registry.
//
// help's own TestCategoryCoverage checks each listed word against
// help.Lookup — the docs. That is a real check, but it cannot see the
// engine, and the two drifted badly: 80 of 249 listed words were presented
// as builtins while every one of them raised undefined_word in a bare
// program, and two (`math-pi`, `math-e`) named no word that exists under
// any spelling — the real exports are MathUtil.pi / MathUtil.e.
//
// This guard lives in package native rather than package help because
// native imports help (describe.go, native_help.go, native_misc.go), so a
// test inside help that imported native would be an import cycle.
//
// The rule it enforces: a word in Category.Words must be callable bare,
// and a word in Category.Module must NOT be — that is precisely what makes
// it module-provided. Both directions matter. Without the second, a word
// could be demoted into a module bucket to silence the first.
func TestCategoryWordsMatchEngineRegistry(t *testing.T) {
	reg, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}

	for _, c := range help.Categories() {
		for _, w := range c.Words {
			if BuildFuncInfo(reg, w) == nil {
				t.Errorf("category %q lists %q as a CORE word, but it is not in the engine registry — "+
					"it errors as undefined_word in a bare program. Move it under Category.Module "+
					"with the module id that exports it.", c.Name, w)
			}
		}
		for id, words := range c.Module {
			for _, w := range words {
				if BuildFuncInfo(reg, w) != nil {
					t.Errorf("category %q lists %q under module %q, but it IS a core registry word — "+
						"move it back into Category.Words.", c.Name, w, id)
				}
			}
		}
	}
}

// Every module id named by the category table must be a module that really
// exists, or `describe` tells a reader to run an import that will fail.
// Keyed by module — adding aql:cli / aql:proc / aql:lang later costs one
// entry each here, rather than one per word.
func TestCategoryModuleIDsExist(t *testing.T) {
	known := map[string]bool{}
	for _, m := range help.ModuleCatalog() {
		known[m.Name] = true
	}
	for _, c := range help.Categories() {
		for _, id := range c.ModuleIDs() {
			if !known[id] {
				t.Errorf("category %q cites module %q, which is not in the module catalog "+
					"(help_render.go moduleCatalog) — `import \"aql:%s\"` would fail", c.Name, id, id)
			}
		}
	}
}
