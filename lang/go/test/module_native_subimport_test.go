package test

import (
	"testing"

	"github.com/aql-lang/aql/eng/go/parser"
	"github.com/aql-lang/aql/lang/go/capabilities"
	"github.com/aql-lang/aql/lang/go/modules"
	"github.com/aql-lang/aql/lang/go/native"
)

// runNativeModuleSubImport builds a registry wired exactly like the
// production entry point (lang.New): the native-module Resolver is
// installed so `import "aql:math"` (and friends) resolve. Files are
// served from an in-memory FS. This is the setup the plain memfs helper
// deliberately omits — it uses a bare DefaultRegistry with no Resolver.
func runNativeModuleSubImport(t *testing.T, files map[string]string, steps []string) ([]native.Value, error) {
	t.Helper()

	reg, err := native.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	reg.SetParseFunc(parser.Parse)
	// The production wiring (lang/go/aql.go:New). Without this a file
	// imported via "./lib.aql" cannot itself import a native module.
	reg.Modules.Resolver = modules.Resolve

	mem := capabilities.NewMem()
	for path, content := range files {
		mem.Files[path] = []byte(content)
	}
	if err := reg.Capabilities.Set(native.CapMemFileOps, capabilities.FileOps(mem)); err != nil {
		t.Fatalf("set capability: %v", err)
	}

	eng := native.New(reg)
	if _, err := eng.Run([]native.Value{
		native.NewWord("context"), native.NewWord("get"), native.NewWord("__sys"),
		native.NewWord("get"), native.NewWord("fs"),
		native.NewWord("set"), native.NewWord("mem"), native.NewBoolean(true),
	}); err != nil {
		t.Fatalf("enable mem fs: %v", err)
	}

	var result []native.Value
	for _, step := range steps {
		vals, perr := parser.Parse(step)
		if perr != nil {
			return nil, perr
		}
		result, err = eng.Run(vals)
		if err != nil {
			return nil, err
		}
	}
	return result, nil
}

// TestImportedFileCanImportNativeModule is the regression test for the
// bloom-filter report: "Sub-imports cannot reach native modules. A file
// imported via ./lib.aql cannot itself import aql:math."
//
// Root cause: RunModuleBody ran the imported file in a fresh
// DefaultRegistry but never propagated parent.Modules.Resolver, so the
// child's `import "aql:math"` hit a nil Resolver and failed with
// "native module resolver not configured".
func TestImportedFileCanImportNativeModule(t *testing.T) {
	files := map[string]string{
		// lib.aql imports a native module and uses it to compute an export.
		"lib.aql": `import "aql:math"
export "Lib" { hi: (math.ceil 2.3) }`,
	}
	result, err := runNativeModuleSubImport(t, files, []string{
		`import "./lib.aql"`,
		`Lib.hi`,
	})
	if err != nil {
		t.Fatalf("imported file failed to import aql:math: %v", err)
	}
	assertResult(t, result, "3")
}

// TestNativeModuleImportTransitiveDepth pins that the propagation is
// transitive: main -> lib -> deep, where the DEEPEST file is the one
// importing the native module.
func TestNativeModuleImportTransitiveDepth(t *testing.T) {
	files := map[string]string{
		"deep.aql": `import "aql:math"
export "Deep" { r: (math.round 4.6) }`,
		"lib.aql": `import "./deep.aql"
export "Lib" { d: (Deep.r) }`,
	}
	result, err := runNativeModuleSubImport(t, files, []string{
		`import "./lib.aql"`,
		`Lib.d`,
	})
	if err != nil {
		t.Fatalf("depth-2 native import chain failed: %v", err)
	}
	assertResult(t, result, "5")
}
