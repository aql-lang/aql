package test

import (
	"strings"
	"testing"

	"github.com/metsitaba/voxgig-exp/aql/internal/engine"
	"github.com/metsitaba/voxgig-exp/aql/internal/fileops"
	"github.com/metsitaba/voxgig-exp/aql/internal/parser"
)

// runModuleSteps creates a registry with in-memory files and ParseFunc set,
// then executes a sequence of AQL steps on a shared engine.
func runModuleSteps(t *testing.T, files map[string]string, steps []string) ([]engine.Value, error) {
	t.Helper()
	mem := fileops.NewMem()
	for path, content := range files {
		mem.Files[path] = []byte(content)
	}

	reg, err := engine.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	reg.SetFileOps(mem)
	reg.SetParseFunc(parser.Parse)

	eng := engine.New(reg)
	var result []engine.Value
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
		"config.aql": `export Config {version:42,name:"myapp"}`,
	}

	result, err := runModuleSteps(t, files, []string{
		`import "./config.aql"`,
		`Config version .`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "42")
}

func TestImportFileStringValue(t *testing.T) {
	files := map[string]string{
		"config.aql": `export Config {version:42,name:"myapp"}`,
	}

	result, err := runModuleSteps(t, files, []string{
		`import "./config.aql"`,
		`Config name .`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "'myapp'")
}

// --- Multiple exports from one file ---

func TestImportFileMultipleExports(t *testing.T) {
	files := map[string]string{
		"data.aql": `export A {x:1}
export B {y:2}`,
	}

	result, err := runModuleSteps(t, files, []string{
		`import "./data.aql"`,
		`A x .`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "1")
}

func TestImportFileMultipleExportsSecond(t *testing.T) {
	files := map[string]string{
		"data.aql": `export A {x:1}
export B {y:2}`,
	}

	result, err := runModuleSteps(t, files, []string{
		`import "./data.aql"`,
		`B y .`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "2")
}

// --- File import with renaming ---

func TestImportFileRename(t *testing.T) {
	files := map[string]string{
		"data.aql": `export Orig {x:99}`,
	}

	result, err := runModuleSteps(t, files, []string{
		`import [Orig Renamed] "./data.aql"`,
		`Renamed x .`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "99")
}

func TestImportFileMultiRename(t *testing.T) {
	files := map[string]string{
		"data.aql": `export A {x:1}
export B {y:2}`,
	}

	result, err := runModuleSteps(t, files, []string{
		`import [[A AA] [B BB]] "./data.aql"`,
		`AA x .`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "1")
}

func TestImportFileMultiRenameSecond(t *testing.T) {
	files := map[string]string{
		"data.aql": `export A {x:1}
export B {y:2}`,
	}

	result, err := runModuleSteps(t, files, []string{
		`import [[A AA] [B BB]] "./data.aql"`,
		`BB y .`,
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
export M {x:1}`,
	}

	result, err := runModuleSteps(t, files, []string{
		`import "./mod.aql"`,
		`secret`,
	})
	if err != nil {
		t.Fatal(err)
	}
	// "secret" should be an unresolved atom, not 42.
	assertResult(t, result, "secret")
}

func TestImportFileIsolationFromParent(t *testing.T) {
	// Parent defs should not be visible inside the file's module.
	files := map[string]string{
		"mod.aql": `export M {val:foo}`,
	}

	result, err := runModuleSteps(t, files, []string{
		`def foo 99`,
		`import "./mod.aql"`,
		`M val .`,
	})
	if err != nil {
		t.Fatal(err)
	}
	got := formatStack(result)
	// "foo" should be an atom (or string), not 99.
	if got == "99" {
		t.Error("parent def 'foo' leaked into file module")
	}
}

// --- File with def that resolves in export ---

func TestImportFileDefExport(t *testing.T) {
	files := map[string]string{
		"lib.aql": `def myval 42
export Lib {myval:myval}`,
	}

	result, err := runModuleSteps(t, files, []string{
		`import "./lib.aql"`,
		`Lib myval .`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "42")
}

// --- File with computed map export ---

func TestImportFileMapExport(t *testing.T) {
	files := map[string]string{
		"comp.aql": `export Vals {x:10,y:20}`,
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
		"simple.aql": `export Simple {a:1,b:2,c:3}`,
	}

	result, err := runModuleSteps(t, files, []string{
		`import "./simple.aql"`,
		`Simple c .`,
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
export Fns {inc:inc}`,
	}

	result, err := runModuleSteps(t, files, []string{
		`import "./fns.aql"`,
		`Fns inc .`,
	})
	if err != nil {
		t.Fatal(err)
	}
	// The exported value should be the list [1 add].
	if len(result) != 1 || !result[0].VType.Equal(engine.TList) {
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
		"mod.aql": `export A {x:1}`,
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
export Math {pi:pi,e:e}`,
	}

	result, err := runModuleSteps(t, files, []string{
		`import "./math.aql"`,
		`Math pi .`,
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
export Math {pi:pi,e:e}`,
	}

	result, err := runModuleSteps(t, files, []string{
		`import "./math.aql"`,
		`Math e .`,
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
	if len(result) != 1 || !result[0].VType.Equal(engine.TMap) {
		t.Fatalf("expected map on stack, got %v", result)
	}
	assertResult(t, result, "{diameter:12756,name:'Earth'}")
}

func TestImportJSONFileAccess(t *testing.T) {
	files := map[string]string{
		"data.json": `{"name":"Earth","diameter":12756}`,
	}

	result, err := runModuleSteps(t, files, []string{
		`import "./data.json" name .`,
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
	if len(result) != 1 || !result[0].VType.Equal(engine.TList) {
		t.Fatalf("expected list on stack, got %v", result)
	}
	assertResult(t, result, "[1,2,3]")
}

func TestImportJsonicFile(t *testing.T) {
	files := map[string]string{
		"config.jsonic": `{name:Earth, diameter:12756}`,
	}

	result, err := runModuleSteps(t, files, []string{
		`import "./config.jsonic" name .`,
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
		`import "./nested.json" planet . name .`,
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
		".aql/mylib/index.aql": `export Lib {version:1,name:"mylib"}`,
	}
	result, err := runModuleSteps(t, files, []string{
		`import "mylib"`,
		`Lib version .`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "1")
}

func TestImportBareModuleStringField(t *testing.T) {
	files := map[string]string{
		".aql/mylib/index.aql": `export Lib {version:1,name:"mylib"}`,
	}
	result, err := runModuleSteps(t, files, []string{
		`import "mylib"`,
		`Lib name .`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "'mylib'")
}

func TestImportBareModuleWithRename(t *testing.T) {
	files := map[string]string{
		".aql/mylib/index.aql": `export Orig {val:42}`,
	}
	result, err := runModuleSteps(t, files, []string{
		`import [Orig Renamed] "mylib"`,
		`Renamed val .`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "42")
}

func TestImportBareModuleMultipleExports(t *testing.T) {
	files := map[string]string{
		".aql/stuff/index.aql": `export A {x:1}
export B {y:2}`,
	}
	result, err := runModuleSteps(t, files, []string{
		`import "stuff"`,
		`B y .`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "2")
}
