package test

import (
	"strings"
	"testing"

	"github.com/aql-lang/aql/eng/go/parser"
	"github.com/aql-lang/aql/lang/go/native"
)

// File-module battery for open words (design/OPEN-WORDS.0.md): the
// export-transplant behaviours that need REAL module files — stable
// module refs for diamond/idempotence and conflict provenance — rather
// than the inline modules lang/spec/open-words.tsv uses (each inline
// module is a distinct instance, so it can never share a ref). Runs on
// the in-memory FS via runMemFSModuleSteps (module_memfs_test.go).

// flagExtModule is the canonical extending module: a minted user type
// (the module-scope core-word rule requires one), a merged `add`
// overload whose body uses a module-PRIVATE helper, a constructor, and
// the exports that carry all of it to the importer.
const flagExtModule = `def Flag (refine Boolean)
def flip fn [[b:Boolean] [Boolean] [b not]]
def add fn [[a:Flag b:Flag] [Boolean] [flip (flip a and flip b)]]
def mk fn [[b:Boolean] [Flag] [def v:Flag b v]]
export "FlagExt" {add: add/r mk: mk/r Flag: Flag}`

// TestModuleExtendFileTransplant pins the positive transplant through a
// real file: the importer's bare `add` gains the [Flag Flag] overload,
// the module-private helper (`flip`) resolves when the body runs at the
// importer (module-closure execution), and the locked native signatures
// are untouched.
func TestModuleExtendFileTransplant(t *testing.T) {
	files := map[string]string{"ext.aql": flagExtModule}
	// flip(flip a AND flip b) == a OR b.
	result, err := runMemFSModuleSteps(t, files, []string{
		`import "./ext.aql"`,
		`add (FlagExt.mk true) (FlagExt.mk false)`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "true")

	result, err = runMemFSModuleSteps(t, files, []string{
		`import "./ext.aql"`,
		`add 40 2`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "42")
}

// TestModuleExtendFileDiamond pins diamond-import behaviour: importing
// the same module file twice neither errors nor breaks dispatch. A
// re-run of the module body re-mints its user types (there is no
// module cache), so the second import's tuple is anchored to a fresh
// mint and coexists with the first; the rebound namespace's
// constructor produces values that dispatch the second instance's
// signature.
func TestModuleExtendFileDiamond(t *testing.T) {
	files := map[string]string{"ext.aql": flagExtModule}
	result, err := runMemFSModuleSteps(t, files, []string{
		`import "./ext.aql"`,
		`import "./ext.aql"`,
		`add (FlagExt.mk true) (FlagExt.mk false)`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "true")
}

// TestModuleExtendTransplantConflictDirect pins the §4.4 collision
// machinery at the API level: the same unlocked tuple transplanted
// from two DIFFERENT module refs raises [aql/extend_conflict], while
// the same ref re-arriving is idempotent and quiet. Unreachable from
// AQL source today — a user-typed tuple is anchored to a per-import
// mint, so two source modules can never build the identical tuple —
// but the guard must hold for a future module cache (shared mints
// across importers) and for host-constructed clones.
func TestModuleExtendTransplantConflictDirect(t *testing.T) {
	reg, err := native.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	reg.SetParseFunc(parser.Parse)

	// Build a real word-extension clone by running a top-level merge
	// (unrestricted scope), then pop it so the registry is back to the
	// pristine base — the clone value is the transplant payload.
	src := `def Money (refine Integer)  def add fn [[a:Money b:Money] [Boolean] [true]]`
	vals, err := parser.Parse(src)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := native.New(reg).Run(vals); err != nil {
		t.Fatal(err)
	}
	top, ok := reg.Defs.Top("add")
	if !ok {
		t.Fatal("add unbound after merge")
	}
	clone, ok := native.IsWordExtension(top)
	if !ok {
		t.Fatal("top of add is not a word-extension clone")
	}
	native.UninstallDef(reg, "add")

	if err := native.TransplantExtension(reg, clone, "./mod-a.aql"); err != nil {
		t.Fatalf("first transplant: %v", err)
	}
	if err := native.TransplantExtension(reg, clone, "./mod-a.aql"); err != nil {
		t.Fatalf("diamond re-transplant (same ref) must be idempotent, got %v", err)
	}
	err = native.TransplantExtension(reg, clone, "./mod-b.aql")
	if err == nil {
		t.Fatal("expected [aql/extend_conflict] for the same tuple from a different module, got no error")
	}
	if !strings.Contains(err.Error(), "extend_conflict") {
		t.Fatalf("expected extend_conflict, got %v", err)
	}
}

// TestModuleExtendFileOneLevel pins the one-level rule through files: a
// middle module imports the extender (receiving the transplant in ITS
// registry) but re-exports only a constructor — the program's `add`
// must NOT gain the extension, even though Flag-conforming values reach
// it (this is the firewall idiom as a file layout).
func TestModuleExtendFileOneLevel(t *testing.T) {
	files := map[string]string{
		"ext.aql": flagExtModule,
		"mid.aql": `import "./ext.aql"
def Flag FlagExt.Flag
def omk fn [[b:Boolean] [Flag] [def v:Flag b v]]
export "Mid" {mk: omk/r}`,
	}
	_, err := runMemFSModuleSteps(t, files, []string{
		`import "./mid.aql"`,
		`add (Mid.mk true) (Mid.mk true)`,
	})
	if err == nil {
		t.Fatal("expected signature_error (extension must not cross two levels), got no error")
	}
	if !strings.Contains(err.Error(), "signature_error") {
		t.Fatalf("expected signature_error, got %v", err)
	}
}

// TestModuleExtendFileReExport pins opt-in transitivity through files:
// the middle module ALSO re-exports the word, so the program's `add`
// does gain the extension — and the body still resolves the innermost
// module's private helper.
func TestModuleExtendFileReExport(t *testing.T) {
	files := map[string]string{
		"ext.aql": flagExtModule,
		"mid.aql": `import "./ext.aql"
def Flag FlagExt.Flag
def omk fn [[b:Boolean] [Flag] [def v:Flag b v]]
export "Mid" {mk: omk/r add: add/r}`,
	}
	result, err := runMemFSModuleSteps(t, files, []string{
		`import "./mid.aql"`,
		`add (Mid.mk true) (Mid.mk false)`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "true")
}

// TestModuleExtendFileUserTypeRule pins the module-scope safety rule at
// the file boundary: a module extending a core word with an all-kernel
// tuple is refused at import with [aql/extend_user_type] — `add 1 {}`
// can never start working because of an import.
func TestModuleExtendFileUserTypeRule(t *testing.T) {
	files := map[string]string{
		"bad.aql": `def add fn [[a:Integer b:Map] [Integer] [1]]
export "Bad" {add: add/r}`,
	}
	_, err := runMemFSModuleSteps(t, files, []string{
		`import "./bad.aql"`,
	})
	if err == nil {
		t.Fatal("expected [aql/extend_user_type], got no error")
	}
	if !strings.Contains(err.Error(), "extend_user_type") {
		t.Fatalf("expected extend_user_type, got %v", err)
	}
}

// TestModuleExtendFileUndefReimport pins retraction + re-import: undef
// pops the transplant (base word intact), and importing the module
// again re-installs a working extension.
func TestModuleExtendFileUndefReimport(t *testing.T) {
	files := map[string]string{"ext.aql": flagExtModule}
	result, err := runMemFSModuleSteps(t, files, []string{
		`import "./ext.aql"`,
		`undef add`,
		`add 40 2`,
		`import "./ext.aql"`,
		`add (FlagExt.mk false) (FlagExt.mk true)`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "true")
}
