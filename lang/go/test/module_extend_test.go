package test

import (
	"strings"
	"testing"
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

// TestModuleExtendFileDiamond pins diamond-import idempotence: the same
// module file imported twice carries the same ref, so the second
// transplant of an identical tuple installs nothing and errors nothing.
// The tuple must be built from a SHARED type (the external builtin
// Date): a re-run of the module body re-mints its own user types (there
// is no module cache), so only shared-type tuples can collide at all.
func TestModuleExtendFileDiamond(t *testing.T) {
	files := map[string]string{
		"dates.aql": `def add fn [[a:Date b:Date] [Boolean] [true]]
export "DateExt" {add: add/r}`,
	}
	result, err := runMemFSModuleSteps(t, files, []string{
		`import "./dates.aql"`,
		`import "./dates.aql"`,
		`import "aql:time-util"`,
		`add TimeUtil.today TimeUtil.today`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "true")
}

// TestModuleExtendFileConflict pins the §4.4 collision: two DIFFERENT
// module files extending `add` with the same shared-type tuple raise
// [aql/extend_conflict] at the second import.
func TestModuleExtendFileConflict(t *testing.T) {
	files := map[string]string{
		"d1.aql": `def add fn [[a:Date b:Date] [Boolean] [true]]
export "D1" {add: add/r}`,
		"d2.aql": `def add fn [[a:Date b:Date] [Boolean] [false]]
export "D2" {add: add/r}`,
	}
	_, err := runMemFSModuleSteps(t, files, []string{
		`import "./d1.aql"`,
		`import "./d2.aql"`,
	})
	if err == nil {
		t.Fatal("expected [aql/extend_conflict], got no error")
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
