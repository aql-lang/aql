package test

import (
	"strings"
	"testing"

	"github.com/aql-lang/aql/eng/go/parser"
	"github.com/aql-lang/aql/lang/go/capabilities"
	"github.com/aql-lang/aql/lang/go/modules"
	"github.com/aql-lang/aql/lang/go/native"
)

// runModuleSteps creates a registry with in-memory files and ParseFunc set,
// then executes a sequence of AQL steps on a shared native.
func runModuleSteps(t *testing.T, files map[string]string, steps []string) ([]native.Value, error) {
	t.Helper()
	mem := capabilities.NewMem()
	for path, content := range files {
		mem.Files[path] = []byte(content)
	}

	reg, err := native.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	native.SetHostFileOps(reg, mem)
	reg.SetParseFunc(parser.Parse)
	modules.InstallResolver(reg) // production module wiring (lang.New)

	eng := native.New(reg)
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

// --- Basic file import ---

func TestImportFileBasic(t *testing.T) {
	files := map[string]string{
		"config.aql": `export "Config" {version:42,name:"myapp"}`,
	}

	result, err := runModuleSteps(t, files, []string{
		`import "./config.aql"`,
		`Config.version`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "42")
}

func TestImportFileStringValue(t *testing.T) {
	files := map[string]string{
		"config.aql": `export "Config" {version:42,name:"myapp"}`,
	}

	result, err := runModuleSteps(t, files, []string{
		`import "./config.aql"`,
		`Config.name`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "'myapp'")
}

// --- Multiple exports from one file ---

func TestImportFileMultipleExports(t *testing.T) {
	files := map[string]string{
		"data.aql": `export "A" {x:1}
export "B" {y:2}`,
	}

	result, err := runModuleSteps(t, files, []string{
		`import "./data.aql"`,
		`A.x`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "1")
}

func TestImportFileMultipleExportsSecond(t *testing.T) {
	files := map[string]string{
		"data.aql": `export "A" {x:1}
export "B" {y:2}`,
	}

	result, err := runModuleSteps(t, files, []string{
		`import "./data.aql"`,
		`B.y`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "2")
}

// --- File import with renaming ---

func TestImportFileRename(t *testing.T) {
	files := map[string]string{
		"data.aql": `export "Orig" {x:99}`,
	}

	result, err := runModuleSteps(t, files, []string{
		`import [Orig Renamed] "./data.aql"`,
		`Renamed.x`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "99")
}

func TestImportFileMultiRename(t *testing.T) {
	files := map[string]string{
		"data.aql": `export "A" {x:1}
export "B" {y:2}`,
	}

	result, err := runModuleSteps(t, files, []string{
		`import [[A AA] [B BB]] "./data.aql"`,
		`AA.x`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "1")
}

func TestImportFileMultiRenameSecond(t *testing.T) {
	files := map[string]string{
		"data.aql": `export "A" {x:1}
export "B" {y:2}`,
	}

	result, err := runModuleSteps(t, files, []string{
		`import [[A AA] [B BB]] "./data.aql"`,
		`BB.y`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "2")
}

// --- File import isolation ---

func TestImportFileIsolation(t *testing.T) {
	// Internal defs should not leak to parent.
	files := map[string]string{
		"mod.aql": `def secret 42
export "M" {x:1}`,
	}

	_, err := runModuleSteps(t, files, []string{
		`import "./mod.aql"`,
		`secret`,
	})
	// "secret" is undefined (not exported) — should error.
	if err == nil {
		t.Fatal("expected error for undefined word 'secret', got nil")
	}
}

func TestImportFileIsolationFromParent(t *testing.T) {
	// Parent defs should not be visible inside the file's module.
	// Use a string value so the map doesn't error on undefined word.
	files := map[string]string{
		"mod.aql": `export "M" {val:"foo"}`,
	}

	result, err := runModuleSteps(t, files, []string{
		`def foo 99`,
		`import "./mod.aql"`,
		`M.val`,
	})
	if err != nil {
		t.Fatal(err)
	}
	got := formatStack(result)
	// "foo" should be the string, not 99.
	if got == "99" {
		t.Error("parent def 'foo' leaked into file module")
	}
}

// --- File with def that resolves in export ---

func TestImportFileDefExport(t *testing.T) {
	files := map[string]string{
		"lib.aql": `def myval 42
export "Lib" {myval:myval}`,
	}

	result, err := runModuleSteps(t, files, []string{
		`import "./lib.aql"`,
		`Lib.myval`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "42")
}

// --- File with computed map export ---

func TestImportFileMapExport(t *testing.T) {
	files := map[string]string{
		"comp.aql": `export "Vals" {x:10,y:20}`,
	}

	result, err := runModuleSteps(t, files, []string{
		`import "./comp.aql"`,
		`Vals`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "{x:10,y:20}")
}

// --- No module word needed (just exports) ---

func TestImportFileNoModuleWord(t *testing.T) {
	files := map[string]string{
		"simple.aql": `export "Simple" {a:1,b:2,c:3}`,
	}

	result, err := runModuleSteps(t, files, []string{
		`import "./simple.aql"`,
		`Simple.c`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "3")
}

// --- File with function list export ---

func TestImportFileFunctionListExport(t *testing.T) {
	files := map[string]string{
		"fns.aql": `def inc [1 add]
export "Fns" {inc:inc}`,
	}

	result, err := runModuleSteps(t, files, []string{
		`import "./fns.aql"`,
		`Fns.inc`,
	})
	if err != nil {
		t.Fatal(err)
	}
	// The exported value should be the list [1 add].
	if len(result) != 1 || !result[0].Parent.Equal(native.TList) {
		t.Errorf("expected list, got %v", result)
	}
}

// --- Error cases ---

func TestImportFileMissing(t *testing.T) {
	_, err := runModuleSteps(t, map[string]string{}, []string{
		`import "./missing.aql"`,
	})
	if err == nil {
		t.Fatal("expected error for missing file")
	}
	if !strings.Contains(err.Error(), "import") {
		t.Errorf("expected import error, got: %v", err)
	}
}

func TestImportFileParseError(t *testing.T) {
	files := map[string]string{
		"bad.aql": `((( invalid`,
	}
	_, err := runModuleSteps(t, files, []string{
		`import "./bad.aql"`,
	})
	if err == nil {
		t.Fatal("expected error for parse failure")
	}
}

func TestImportFileRenameNotFound(t *testing.T) {
	files := map[string]string{
		"mod.aql": `export "A" {x:1}`,
	}
	_, err := runModuleSteps(t, files, []string{
		`import [NoSuch Renamed] "./mod.aql"`,
	})
	if err == nil {
		t.Fatal("expected error for missing export in rename")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected 'not found' error, got: %v", err)
	}
}

// --- File with multiple defs and exports ---

func TestImportFileMultipleDefs(t *testing.T) {
	files := map[string]string{
		"math.aql": `def pi 3
def e 2
export "Math" {pi:pi,e:e}`,
	}

	result, err := runModuleSteps(t, files, []string{
		`import "./math.aql"`,
		`Math.pi`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "3")
}

func TestImportFileMultipleDefsSecond(t *testing.T) {
	files := map[string]string{
		"math.aql": `def pi 3
def e 2
export "Math" {pi:pi,e:e}`,
	}

	result, err := runModuleSteps(t, files, []string{
		`import "./math.aql"`,
		`Math.e`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "2")
}

// --- JSON / Jsonic data file import ---

func TestImportJSONFile(t *testing.T) {
	files := map[string]string{
		"data.json": `{"name":"Earth","diameter":12756}`,
	}

	result, err := runModuleSteps(t, files, []string{
		`import "./data.json"`,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result) != 1 || !result[0].Parent.Equal(native.TMap) {
		t.Fatalf("expected map on stack, got %v", result)
	}
	assertResult(t, result, "{diameter:12756,name:'Earth'}")
}

func TestImportJSONFileAccess(t *testing.T) {
	files := map[string]string{
		"data.json": `{"name":"Earth","diameter":12756}`,
	}

	result, err := runModuleSteps(t, files, []string{
		`import "./data.json" . name`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "'Earth'")
}

func TestImportJSONFileList(t *testing.T) {
	files := map[string]string{
		"items.json": `[1,2,3]`,
	}

	result, err := runModuleSteps(t, files, []string{
		`import "./items.json"`,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result) != 1 || !result[0].Parent.Equal(native.TList) {
		t.Fatalf("expected list on stack, got %v", result)
	}
	assertResult(t, result, "[1,2,3]")
}

func TestImportJsonicFile(t *testing.T) {
	files := map[string]string{
		"config.jsonic": `{name:Earth, diameter:12756}`,
	}

	result, err := runModuleSteps(t, files, []string{
		`import "./config.jsonic" . name`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "'Earth'")
}

func TestImportJSONFileNested(t *testing.T) {
	files := map[string]string{
		"nested.json": `{"planet":{"name":"Mars","moons":["Phobos","Deimos"]}}`,
	}

	result, err := runModuleSteps(t, files, []string{
		`import "./nested.json" get planet get name`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "'Mars'")
}

func TestImportJSONFileRenameError(t *testing.T) {
	files := map[string]string{
		"data.json": `{"x":1}`,
	}

	_, err := runModuleSteps(t, files, []string{
		`import [A B] "./data.json"`,
	})
	if err == nil {
		t.Fatal("expected error for rename on data file")
	}
}

// --- CSV / TSV data file import ---

func TestImportCSVFile(t *testing.T) {
	files := map[string]string{
		"people.csv": "name,age\nAlice,30\nBob,25\n",
	}

	result, err := runModuleSteps(t, files, []string{
		`import "./people.csv"`,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
}

func TestImportTSVFile(t *testing.T) {
	files := map[string]string{
		"data.tsv": "x\ty\n1\t2\n3\t4\n",
	}

	result, err := runModuleSteps(t, files, []string{
		`import "./data.tsv"`,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
}

func TestImportCSVFileRenameError(t *testing.T) {
	files := map[string]string{
		"data.csv": "x,y\n1,2\n",
	}

	_, err := runModuleSteps(t, files, []string{
		`import [A B] "./data.csv"`,
	})
	if err == nil {
		t.Fatal("expected error for rename on data file")
	}
}

// --- Path validation ---

func TestImportBareModuleNotFoundError(t *testing.T) {
	_, err := runModuleSteps(t, map[string]string{}, []string{
		`import "config"`,
	})
	if err == nil {
		t.Fatal("expected error for missing bare module")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected module not found error, got: %v", err)
	}
}

func TestImportBareModuleRenameNotFoundError(t *testing.T) {
	_, err := runModuleSteps(t, map[string]string{}, []string{
		`import [A B] "config"`,
	})
	if err == nil {
		t.Fatal("expected error for missing bare module")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected module not found error, got: %v", err)
	}
}

// --- Bare module import (CommonJS-style .aql/ resolution) ---

func TestImportBareModuleBasic(t *testing.T) {
	files := map[string]string{
		".aql/mylib/index.aql": `export "Lib" {version:1,name:"mylib"}`,
	}
	result, err := runModuleSteps(t, files, []string{
		`import "mylib"`,
		`Lib.version`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "1")
}

func TestImportBareModuleStringField(t *testing.T) {
	files := map[string]string{
		".aql/mylib/index.aql": `export "Lib" {version:1,name:"mylib"}`,
	}
	result, err := runModuleSteps(t, files, []string{
		`import "mylib"`,
		`Lib.name`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "'mylib'")
}

func TestImportBareModuleWithRename(t *testing.T) {
	files := map[string]string{
		".aql/mylib/index.aql": `export "Orig" {val:42}`,
	}
	result, err := runModuleSteps(t, files, []string{
		`import [Orig Renamed] "mylib"`,
		`Renamed.val`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "42")
}

func TestImportBareModuleMultipleExports(t *testing.T) {
	files := map[string]string{
		".aql/stuff/index.aql": `export "A" {x:1}
export "B" {y:2}`,
	}
	result, err := runModuleSteps(t, files, []string{
		`import "stuff"`,
		`B.y`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "2")
}

// runModuleStepsWithCwd creates a registry with a simulated working directory,
// in-memory files, and ParseFunc set, then executes AQL steps.
func runModuleStepsWithCwd(t *testing.T, cwd string, files map[string]string, steps []string) ([]native.Value, error) {
	t.Helper()
	mem := capabilities.NewMem()
	mem.Cwd = cwd
	for path, content := range files {
		mem.Files[path] = []byte(content)
	}

	reg, err := native.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	native.SetHostFileOps(reg, mem)
	reg.SetParseFunc(parser.Parse)
	modules.InstallResolver(reg) // production module wiring (lang.New)

	eng := native.New(reg)
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

// --- Bare module: parent directory walk (1–7 levels) ---

func TestBareModuleResolveLevel1(t *testing.T) {
	// Module at CWD level: /project/.aql/foo/index.aql
	files := map[string]string{
		"/project/.aql/foo/index.aql": `export "Foo" {level:1}`,
	}
	result, err := runModuleStepsWithCwd(t, "/project", files, []string{
		`import "foo"`, `Foo.level`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "1")
}

func TestBareModuleResolveLevel2(t *testing.T) {
	// Module one level up: /project/.aql/foo/index.aql, CWD = /project/src
	files := map[string]string{
		"/project/.aql/foo/index.aql": `export "Foo" {level:2}`,
	}
	result, err := runModuleStepsWithCwd(t, "/project/src", files, []string{
		`import "foo"`, `Foo.level`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "2")
}

func TestBareModuleResolveLevel3(t *testing.T) {
	files := map[string]string{
		"/project/.aql/foo/index.aql": `export "Foo" {level:3}`,
	}
	result, err := runModuleStepsWithCwd(t, "/project/src/sub", files, []string{
		`import "foo"`, `Foo.level`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "3")
}

func TestBareModuleResolveLevel4(t *testing.T) {
	files := map[string]string{
		"/project/.aql/foo/index.aql": `export "Foo" {level:4}`,
	}
	result, err := runModuleStepsWithCwd(t, "/project/a/b/c", files, []string{
		`import "foo"`, `Foo.level`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "4")
}

func TestBareModuleResolveLevel5(t *testing.T) {
	files := map[string]string{
		"/project/.aql/foo/index.aql": `export "Foo" {level:5}`,
	}
	result, err := runModuleStepsWithCwd(t, "/project/a/b/c/d", files, []string{
		`import "foo"`, `Foo.level`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "5")
}

func TestBareModuleResolveLevel6(t *testing.T) {
	files := map[string]string{
		"/project/.aql/foo/index.aql": `export "Foo" {level:6}`,
	}
	result, err := runModuleStepsWithCwd(t, "/project/a/b/c/d/e", files, []string{
		`import "foo"`, `Foo.level`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "6")
}

func TestBareModuleResolveLevel7(t *testing.T) {
	files := map[string]string{
		"/project/.aql/foo/index.aql": `export "Foo" {level:7}`,
	}
	result, err := runModuleStepsWithCwd(t, "/project/a/b/c/d/e/f", files, []string{
		`import "foo"`, `Foo.level`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "7")
}

func TestBareModuleResolveAtRoot(t *testing.T) {
	// Module at filesystem root: /.aql/foo/index.aql
	files := map[string]string{
		"/.aql/rootmod/index.aql": `export "Root" {found:true}`,
	}
	result, err := runModuleStepsWithCwd(t, "/a/b/c", files, []string{
		`import "rootmod"`, `Root.found`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "true")
}

// --- Bare module: closest wins (shadowing) ---

func TestBareModuleClosestWins(t *testing.T) {
	// Module exists at both CWD and parent — CWD version wins.
	files := map[string]string{
		"/project/src/.aql/foo/index.aql": `export "Foo" {loc:"child"}`,
		"/project/.aql/foo/index.aql":     `export "Foo" {loc:"parent"}`,
	}
	result, err := runModuleStepsWithCwd(t, "/project/src", files, []string{
		`import "foo"`, `Foo.loc`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "'child'")
}

func TestBareModuleClosestWinsDeep(t *testing.T) {
	// Module at level 2 and level 5 — level 2 (closer) wins.
	files := map[string]string{
		"/a/b/.aql/mod/index.aql":     `export "Mod" {loc:"level2"}`,
		"/a/b/c/d/.aql/mod/index.aql": `export "Mod" {loc:"level4"}`,
	}
	result, err := runModuleStepsWithCwd(t, "/a/b/c/d/e", files, []string{
		`import "mod"`, `Mod.loc`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "'level4'")
}

func TestBareModuleFallsThroughToParent(t *testing.T) {
	// Module only at parent, not at CWD.
	files := map[string]string{
		"/project/.aql/util/index.aql": `export "Util" {val:99}`,
	}
	result, err := runModuleStepsWithCwd(t, "/project/src", files, []string{
		`import "util"`, `Util.val`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "99")
}

// --- Bare module: siblings (different modules at same level) ---

func TestBareModuleSiblings(t *testing.T) {
	// Two different modules in the same .aql/ directory.
	files := map[string]string{
		"/project/.aql/alpha/index.aql": `export "Alpha" {id:"a"}`,
		"/project/.aql/beta/index.aql":  `export "Beta" {id:"b"}`,
	}
	result, err := runModuleStepsWithCwd(t, "/project", files, []string{
		`import "alpha"`,
		`import "beta"`,
		`Beta.id`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "'b'")
}

func TestBareModuleSiblingsAccessBoth(t *testing.T) {
	files := map[string]string{
		"/project/.aql/alpha/index.aql": `export "Alpha" {id:"a"}`,
		"/project/.aql/beta/index.aql":  `export "Beta" {id:"b"}`,
	}
	result, err := runModuleStepsWithCwd(t, "/project", files, []string{
		`import "alpha"`,
		`import "beta"`,
		`Alpha.id`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "'a'")
}

func TestBareModuleSiblingsAtDifferentLevels(t *testing.T) {
	// alpha at CWD level, beta at parent level.
	files := map[string]string{
		"/project/src/.aql/alpha/index.aql": `export "Alpha" {id:"child-a"}`,
		"/project/.aql/beta/index.aql":      `export "Beta" {id:"parent-b"}`,
	}
	result, err := runModuleStepsWithCwd(t, "/project/src", files, []string{
		`import "alpha"`,
		`import "beta"`,
		`Alpha.id`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "'child-a'")
}

func TestBareModuleSiblingsAtDifferentLevelsSecond(t *testing.T) {
	files := map[string]string{
		"/project/src/.aql/alpha/index.aql": `export "Alpha" {id:"child-a"}`,
		"/project/.aql/beta/index.aql":      `export "Beta" {id:"parent-b"}`,
	}
	result, err := runModuleStepsWithCwd(t, "/project/src", files, []string{
		`import "alpha"`,
		`import "beta"`,
		`Beta.id`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "'parent-b'")
}

// --- Bare module: same name, different parents, different functionality ---

func TestBareModuleSameNameDifferentParents(t *testing.T) {
	// "utils" exists at two different directory levels with different content.
	// The closest one (child) should win.
	files := map[string]string{
		"/project/src/.aql/utils/index.aql": `export "Utils" {scope:"local",ver:2}`,
		"/project/.aql/utils/index.aql":     `export "Utils" {scope:"global",ver:1}`,
	}
	result, err := runModuleStepsWithCwd(t, "/project/src", files, []string{
		`import "utils"`, `Utils.scope`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "'local'")
}

func TestBareModuleSameNameDifferentParentsVersion(t *testing.T) {
	files := map[string]string{
		"/project/src/.aql/utils/index.aql": `export "Utils" {scope:"local",ver:2}`,
		"/project/.aql/utils/index.aql":     `export "Utils" {scope:"global",ver:1}`,
	}
	result, err := runModuleStepsWithCwd(t, "/project/src", files, []string{
		`import "utils"`, `Utils.ver`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "2")
}

func TestBareModuleSameNameParentWinsWhenChildAbsent(t *testing.T) {
	// "utils" only at the parent level — should be found via upward walk.
	files := map[string]string{
		"/project/.aql/utils/index.aql": `export "Utils" {scope:"global",ver:1}`,
	}
	result, err := runModuleStepsWithCwd(t, "/project/src", files, []string{
		`import "utils"`, `Utils.scope`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "'global'")
}

func TestBareModuleSameNameThreeLevels(t *testing.T) {
	// "config" at three levels; the closest should win.
	files := map[string]string{
		"/a/b/c/.aql/config/index.aql": `export "Config" {env:"dev"}`,
		"/a/b/.aql/config/index.aql":   `export "Config" {env:"staging"}`,
		"/a/.aql/config/index.aql":     `export "Config" {env:"prod"}`,
	}
	result, err := runModuleStepsWithCwd(t, "/a/b/c", files, []string{
		`import "config"`, `Config.env`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "'dev'")
}

func TestBareModuleSameNameThreeLevelsMidWins(t *testing.T) {
	// "config" at root and mid but NOT at CWD. Mid level wins.
	files := map[string]string{
		"/a/b/.aql/config/index.aql": `export "Config" {env:"staging"}`,
		"/a/.aql/config/index.aql":   `export "Config" {env:"prod"}`,
	}
	result, err := runModuleStepsWithCwd(t, "/a/b/c", files, []string{
		`import "config"`, `Config.env`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "'staging'")
}

// --- Bare module: error cases ---

func TestBareModuleNotFoundDeep(t *testing.T) {
	// No .aql/ directory anywhere in the hierarchy.
	_, err := runModuleStepsWithCwd(t, "/a/b/c/d/e/f/g", map[string]string{}, []string{
		`import "nonexistent"`,
	})
	if err == nil {
		t.Fatal("expected error for missing module")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected not found error, got: %v", err)
	}
}

func TestBareModuleWrongNameNotFound(t *testing.T) {
	// "bar" exists but we ask for "baz".
	files := map[string]string{
		"/project/.aql/bar/index.aql": `export "Bar" {x:1}`,
	}
	_, err := runModuleStepsWithCwd(t, "/project", files, []string{
		`import "baz"`,
	})
	if err == nil {
		t.Fatal("expected error for wrong module name")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected not found error, got: %v", err)
	}
}

// --- Bare module: mixed with file path imports ---

func TestBareModuleAndFilePathImportCoexist(t *testing.T) {
	files := map[string]string{
		"/project/.aql/bare/index.aql": `export "Bare" {src:"bare"}`,
		"/project/local.aql":           `export "Local" {src:"file"}`,
	}
	result, err := runModuleStepsWithCwd(t, "/project", files, []string{
		`import "bare"`,
		`import "./local.aql"`,
		`Bare.src`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "'bare'")
}

// --- Bare module: rename variants ---

func TestBareModuleWithMultiRename(t *testing.T) {
	files := map[string]string{
		"/project/.aql/lib/index.aql": `export "A" {x:1}
export "B" {y:2}`,
	}
	result, err := runModuleStepsWithCwd(t, "/project", files, []string{
		`import [[A X][B Y]] "lib"`,
		`X.x`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "1")
}

func TestBareModuleWithMultiRenameSecond(t *testing.T) {
	files := map[string]string{
		"/project/.aql/lib/index.aql": `export "A" {x:1}
export "B" {y:2}`,
	}
	result, err := runModuleStepsWithCwd(t, "/project", files, []string{
		`import [[A X][B Y]] "lib"`,
		`Y.y`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "2")
}

// --- Bare module: complex module content ---

func TestBareModuleWithDefs(t *testing.T) {
	files := map[string]string{
		"/project/.aql/math/index.aql": `
def pi 3
def e 2
export "Math" {pi:pi,e:e}`,
	}
	result, err := runModuleStepsWithCwd(t, "/project", files, []string{
		`import "math"`, `Math.pi`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "3")
}

func TestBareModuleImportsJsonReExportsAsMap(t *testing.T) {
	files := map[string]string{
		"/project/.aql/planets/data.json": `{"earth":{"diameter":12756},"mars":{"diameter":6792}}`,
		"/project/.aql/planets/index.aql": `import "./data.json" def data end
export "Planets" {catalog:data}`,
	}
	result, err := runModuleStepsWithCwd(t, "/project", files, []string{
		`import "planets"`,
		`Planets get catalog get earth get diameter`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "12756")
}

func TestBareModuleInternalDefsDoNotLeak(t *testing.T) {
	files := map[string]string{
		"/project/.aql/secret/index.aql": `
def internal 42
export "Public" {val:internal}`,
	}
	result, err := runModuleStepsWithCwd(t, "/project", files, []string{
		`import "secret"`, `Public.val`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "42")
}
