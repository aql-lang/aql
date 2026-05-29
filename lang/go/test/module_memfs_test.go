package test

import (
	"github.com/aql-lang/aql/lang/go/modules"
	"github.com/aql-lang/aql/lang/go/native"
	"testing"

	"github.com/aql-lang/aql/eng/go/parser"
	"github.com/aql-lang/aql/lang/go/capabilities"
)

// runMemFSModuleSteps sets up an in-memory filesystem with pre-populated files,
// enables __sys.fs.mem=true, and runs AQL steps against it.
// This validates the full pipeline: folder + write + import on in-memory FS.
func runMemFSModuleSteps(t *testing.T, files map[string]string, steps []string) ([]native.Value, error) {
	t.Helper()

	reg, err := native.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	reg.SetParseFunc(parser.Parse)
	modules.InstallResolver(reg) // production module wiring (lang.New)

	// Create an in-memory FS and pre-populate it with module files.
	mem := capabilities.NewMem()
	for path, content := range files {
		mem.Files[path] = []byte(content)
	}
	if err := reg.Capabilities.Set(native.CapMemFileOps, capabilities.FileOps(mem)); err != nil {
		t.Fatalf("set capability: %v", err)
	}

	// Enable in-memory FS via __sys.fs.mem = true.
	eng := native.New(reg)
	_, err = eng.Run([]native.Value{
		native.NewWord("context"), native.NewWord("get"), native.NewWord("__sys"),
		native.NewWord("get"), native.NewWord("fs"),
		native.NewWord("set"), native.NewWord("mem"), native.NewBoolean(true),
	})
	if err != nil {
		t.Fatalf("enable mem fs: %v", err)
	}

	var result []native.Value
	for _, step := range steps {
		vals, err := parser.Parse(step)
		if err != nil {
			return nil, err
		}
		result, err = eng.Run(vals)
		if err != nil {
			return nil, err
		}
	}
	return result, nil
}

// --- Basic module import on in-memory FS ---

func TestMemFSModuleBasicImport(t *testing.T) {
	files := map[string]string{
		"config.aql": `export "Config" {version:42,name:"myapp"}`,
	}
	result, err := runMemFSModuleSteps(t, files, []string{
		`import "./config.aql"`,
		`Config.version`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "42")
}

func TestMemFSModuleStringExport(t *testing.T) {
	files := map[string]string{
		"config.aql": `export "Config" {version:42,name:"myapp"}`,
	}
	result, err := runMemFSModuleSteps(t, files, []string{
		`import "./config.aql"`,
		`Config.name`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "'myapp'")
}

// --- Module with function export ---

func TestMemFSModuleFunctionExport(t *testing.T) {
	files := map[string]string{
		"math.aql": `def double fn [[n:Integer] [Integer] [n add n]]
export "Math" {double:double/r}`,
	}
	result, err := runMemFSModuleSteps(t, files, []string{
		`import "./math.aql"`,
		`5 Math.double`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "10")
}

// A 0-arg function exported via /r is stored as the function (not fired
// while the export map is built) and dispatches when accessed as
// `pkg.fn`. The export map auto-evaluates, so a bare `zero` would fire
// its 0-arg signature there and freeze the export to zero's result; the
// /r ref is resolved to the fn value directly instead.
func TestMemFSModuleZeroArgFunctionExport(t *testing.T) {
	files := map[string]string{
		"z.aql": `def zero fn [[] [Integer] [42]]
export "Z" {zero:zero/r}`,
	}
	result, err := runMemFSModuleSteps(t, files, []string{
		`import "./z.aql"`,
		`Z.zero`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "42")
}

// Negative: a bare function export (no /r) is auto-evaluated and
// dispatched while the export map is built — a fn needing args has no
// matching 0-arg signature, so it errors rather than silently exporting
// a reference. Functions must be exported with /r.
func TestMemFSModuleBareFunctionExportErrors(t *testing.T) {
	files := map[string]string{
		"math.aql": `def double fn [[n:Integer] [Integer] [n add n]]
export "Math" {double:double}`,
	}
	_, err := runMemFSModuleSteps(t, files, []string{
		`import "./math.aql"`,
	})
	if err == nil {
		t.Fatal("expected error: a bare fn export dispatches at build time; use /r")
	}
}

// --- Module with directory structure (.aql/aql.json) ---

func TestMemFSModuleWithAqlJson(t *testing.T) {
	files := map[string]string{
		"mymod/index.aql":     `export "API" {x:42}`,
		"mymod/.aql/aql.json": `{"name":"mymod","main":"index.aql"}`,
	}
	result, err := runMemFSModuleSteps(t, files, []string{
		`import "./mymod"`,
		`API.x`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "42")
}

// --- Module with custom main file ---

func TestMemFSModuleCustomMain(t *testing.T) {
	files := map[string]string{
		"lib/core.aql":      `export "Core" {pi:3}`,
		"lib/.aql/aql.json": `{"name":"lib","main":"core.aql"}`,
	}
	result, err := runMemFSModuleSteps(t, files, []string{
		`import "./lib"`,
		`Core.pi`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "3")
}

// --- Two modules imported ---

func TestMemFSModuleTwoImports(t *testing.T) {
	files := map[string]string{
		"math.aql": `def add1 fn [[n:Integer] [Integer] [n add 1]]
export "Math" {add1:add1/r}`,
		"strings.aql": `def greet fn [[s:String] [String] ["hello " add s]]
export "Strings" {greet:greet/r}`,
	}
	result, err := runMemFSModuleSteps(t, files, []string{
		`import "./math.aql"`,
		`import "./strings.aql"`,
		`9 Math.add1`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "10")
}

// --- Module with folder + write (full pipeline) ---

func TestMemFSModuleFolderWriteImport(t *testing.T) {
	// Start with empty FS — use folder and write to create everything from AQL.
	result, err := runMemFSModuleSteps(t, nil, []string{
		// Create module directory structure using Path
		`folder (make Path ["mymod"])`,
		`folder (make Path ["mymod" ".aql"])`,
		// Write aql.json using Path
		`write (make Path ["mymod" ".aql" "aql.json"]) "{\"name\":\"mymod\",\"main\":\"index.aql\"}"`,
		// Write module source using Path
		`write (make Path ["mymod" "index.aql"]) "export \"API\" {answer:42}"`,
		// Import and use
		`import "./mymod"`,
		`API.answer`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "42")
}

// --- Verify module exports are isolated ---

func TestMemFSModuleExportIsolation(t *testing.T) {
	files := map[string]string{
		"mod.aql": `def secret 999
export "Pub" {visible:1}`,
	}
	result, err := runMemFSModuleSteps(t, files, []string{
		`import "./mod.aql"`,
		`Pub.visible`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "1")
}
