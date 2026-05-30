package test

import (
	"os"
	"path/filepath"
	"testing"
)

// =====================================================================
// Deep dependency chain: 7 layers of nested module dependencies.
//
// Layer 7 (leaves): charops (v2.3.1), joiner (v0.4.2)
// Layer 6: wrapper (v1.1.0) → charops, tagger (v3.0.2) → joiner
// Layer 5: caser (v0.2.4) → wrapper, bracket (v1.3.0) → tagger
// Layer 4: formatter (v2.1.1) → caser + bracket
// Layer 3: decorator (v0.5.3) → formatter
// Layer 2: styler (v1.0.7) → decorator
// Layer 1: textkit (v3.2.0) → styler
// Project: wordlab → textkit
//
// Each module is nested inside its parent's .aql/ directory, forming
// a 7-level deep tree mirroring CommonJS node_modules resolution.
//
// Main branch:  textkit → styler → decorator → formatter → caser → wrapper → charops
// Other branch: formatter → bracket → tagger → joiner
// =====================================================================

func wordlabDir(t *testing.T) string {
	t.Helper()
	abs, err := filepath.Abs("module-work/wordlab")
	if err != nil {
		t.Fatal(err)
	}
	return abs
}

// --- Full 7-layer chain through main branch ---

func TestDeepTextkitProcess(t *testing.T) {
	dir := wordlabDir(t)
	// "hello" → charops.to-up → "HELLO" → wrapper.shout → "HELLO!"
	// → caser.emphasize → "*HELLO!*" → formatter.format → "*HELLO!*"
	// → decorator.decorate → ">> *HELLO!*" → styler.style → "~ >> *HELLO!* ~"
	// → textkit.process → "~ >> *HELLO!* ~"
	result, err := runRealFileSteps(t, dir, []string{
		`(import "textkit")`,
		`"hello" Textkit.process`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "'~ >> *HELLO!* ~'")
}

func TestDeepTextkitProcessDifferentInput(t *testing.T) {
	dir := wordlabDir(t)
	result, err := runRealFileSteps(t, dir, []string{
		`(import "textkit")`,
		`"world" Textkit.process`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "'~ >> *WORLD!* ~'")
}

// --- Full pipeline via project code ---

func TestDeepWordlabProjectCode(t *testing.T) {
	dir := wordlabDir(t)
	result, err := runRealFileSteps(t, dir, []string{
		`(import "textkit")`,
		`"hello" Textkit.process`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "'~ >> *HELLO!* ~'")
}

// --- Second branch: formatter.mark through bracket → tagger → joiner ---

func TestDeepFormatterMarkBranch(t *testing.T) {
	// Test the bracket → tagger → joiner branch.
	// Access formatter from inside its parent (decorator),
	// which is inside styler, which is inside textkit.
	// We can access it by importing textkit and using its chain.
	// But formatter.mark is not re-exported by the chain.
	//
	// Instead, test from decorator's directory context where formatter is a child dep.
	dir := wordlabDir(t)
	decDir := filepath.Join(dir, ".aql", "textkit", ".aql", "styler", ".aql", "decorator")
	result, err := runRealFileSteps(t, decDir, []string{
		`(import "formatter")`,
		`"tag" Formatter.mark`,
	})
	if err != nil {
		t.Fatal(err)
	}
	// joiner.add-dot("tag") = "tag."
	// tagger.tag("tag") = "[tag.]"
	// bracket.label("tag") = "<[tag.]>"
	// formatter.mark("tag") = "<[tag.]>"
	assertResult(t, result, "'<[tag.]>'")
}

func TestDeepFormatterFormatBranch(t *testing.T) {
	dir := wordlabDir(t)
	decDir := filepath.Join(dir, ".aql", "textkit", ".aql", "styler", ".aql", "decorator")
	result, err := runRealFileSteps(t, decDir, []string{
		`(import "formatter")`,
		`"word" Formatter.format`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "'*WORD!*'")
}

// --- Individual layers tested from their parent's context ---

func TestDeepCharopsFromWrapper(t *testing.T) {
	dir := wordlabDir(t)
	wrapperDir := filepath.Join(dir, ".aql", "textkit", ".aql", "styler", ".aql",
		"decorator", ".aql", "formatter", ".aql", "caser", ".aql", "wrapper")
	result, err := runRealFileSteps(t, wrapperDir, []string{
		`(import "charops")`,
		`"hello" Charops.to-up`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "'HELLO'")
}

func TestDeepCharopsToDown(t *testing.T) {
	dir := wordlabDir(t)
	wrapperDir := filepath.Join(dir, ".aql", "textkit", ".aql", "styler", ".aql",
		"decorator", ".aql", "formatter", ".aql", "caser", ".aql", "wrapper")
	result, err := runRealFileSteps(t, wrapperDir, []string{
		`(import "charops")`,
		`"WORLD" Charops.to-down`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "'world'")
}

func TestDeepJoinerFromTagger(t *testing.T) {
	dir := wordlabDir(t)
	taggerDir := filepath.Join(dir, ".aql", "textkit", ".aql", "styler", ".aql",
		"decorator", ".aql", "formatter", ".aql", "bracket", ".aql", "tagger")
	result, err := runRealFileSteps(t, taggerDir, []string{
		`(import "joiner")`,
		`"hello" Joiner.add-dot`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "'hello.'")
}

func TestDeepWrapperFromCaser(t *testing.T) {
	dir := wordlabDir(t)
	caserDir := filepath.Join(dir, ".aql", "textkit", ".aql", "styler", ".aql",
		"decorator", ".aql", "formatter", ".aql", "caser")
	result, err := runRealFileSteps(t, caserDir, []string{
		`(import "wrapper")`,
		`"hello" Wrapper.shout`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "'HELLO!'")
}

func TestDeepTaggerFromBracket(t *testing.T) {
	dir := wordlabDir(t)
	bracketDir := filepath.Join(dir, ".aql", "textkit", ".aql", "styler", ".aql",
		"decorator", ".aql", "formatter", ".aql", "bracket")
	result, err := runRealFileSteps(t, bracketDir, []string{
		`(import "tagger")`,
		`"test" Tagger.tag`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "'[test.]'")
}

func TestDeepCaserFromFormatter(t *testing.T) {
	dir := wordlabDir(t)
	fmtDir := filepath.Join(dir, ".aql", "textkit", ".aql", "styler", ".aql",
		"decorator", ".aql", "formatter")
	result, err := runRealFileSteps(t, fmtDir, []string{
		`(import "caser")`,
		`"hello" Caser.emphasize`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "'*HELLO!*'")
}

func TestDeepBracketFromFormatter(t *testing.T) {
	dir := wordlabDir(t)
	fmtDir := filepath.Join(dir, ".aql", "textkit", ".aql", "styler", ".aql",
		"decorator", ".aql", "formatter")
	result, err := runRealFileSteps(t, fmtDir, []string{
		`(import "bracket")`,
		`"test" Bracket.label`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "'<[test.]>'")
}

func TestDeepDecoratorFromStyler(t *testing.T) {
	dir := wordlabDir(t)
	stylerDir := filepath.Join(dir, ".aql", "textkit", ".aql", "styler")
	result, err := runRealFileSteps(t, stylerDir, []string{
		`(import "decorator")`,
		`"hello" Decorator.decorate`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "'>> *HELLO!*'")
}

func TestDeepDecoratorLabelItFromStyler(t *testing.T) {
	dir := wordlabDir(t)
	stylerDir := filepath.Join(dir, ".aql", "textkit", ".aql", "styler")
	result, err := runRealFileSteps(t, stylerDir, []string{
		`(import "decorator")`,
		`"test" Decorator.label-it`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "'>> <[test.]>'")
}

func TestDeepStylerFromTextkit(t *testing.T) {
	dir := wordlabDir(t)
	tkDir := filepath.Join(dir, ".aql", "textkit")
	result, err := runRealFileSteps(t, tkDir, []string{
		`(import "styler")`,
		`"hello" Styler.style`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "'~ >> *HELLO!* ~'")
}

// --- Nested file tree verification ---

func TestDeepWordlabNestedFileTree(t *testing.T) {
	dir := wordlabDir(t)

	// Project-level files.
	for _, f := range []string{"aql.jsonic", "index.aql", ".aql/aql.json"} {
		if _, err := os.Stat(filepath.Join(dir, f)); err != nil {
			t.Errorf("expected wordlab/%s: %s", f, err)
		}
	}

	// Verify the 7-level nested dependency tree.
	// Main branch: textkit → styler → decorator → formatter → caser → wrapper → charops
	type modEntry struct {
		relPath string // relative to wordlab/
		main    string
	}

	tree := []modEntry{
		// Layer 1
		{".aql/textkit", "textkit.aql"},
		// Layer 2
		{".aql/textkit/.aql/styler", "styler.aql"},
		// Layer 3
		{".aql/textkit/.aql/styler/.aql/decorator", "decorator.aql"},
		// Layer 4
		{".aql/textkit/.aql/styler/.aql/decorator/.aql/formatter", "formatter.aql"},
		// Layer 5 (main branch)
		{".aql/textkit/.aql/styler/.aql/decorator/.aql/formatter/.aql/caser", "caser.aql"},
		// Layer 5 (second branch)
		{".aql/textkit/.aql/styler/.aql/decorator/.aql/formatter/.aql/bracket", "bracket.aql"},
		// Layer 6 (main branch)
		{".aql/textkit/.aql/styler/.aql/decorator/.aql/formatter/.aql/caser/.aql/wrapper", "wrapper.aql"},
		// Layer 6 (second branch)
		{".aql/textkit/.aql/styler/.aql/decorator/.aql/formatter/.aql/bracket/.aql/tagger", "tagger.aql"},
		// Layer 7 (main branch leaf)
		{".aql/textkit/.aql/styler/.aql/decorator/.aql/formatter/.aql/caser/.aql/wrapper/.aql/charops", "charops.aql"},
		// Layer 7 (second branch leaf)
		{".aql/textkit/.aql/styler/.aql/decorator/.aql/formatter/.aql/bracket/.aql/tagger/.aql/joiner", "joiner.aql"},
	}

	for _, m := range tree {
		modDir := filepath.Join(dir, m.relPath)

		if fi, err := os.Stat(modDir); err != nil || !fi.IsDir() {
			t.Errorf("expected directory %s", m.relPath)
			continue
		}

		// Main AQL file exists.
		if _, err := os.Stat(filepath.Join(modDir, m.main)); err != nil {
			t.Errorf("expected %s/%s: %s", m.relPath, m.main, err)
		}

		// aql.jsonic exists.
		if _, err := os.Stat(filepath.Join(modDir, "aql.jsonic")); err != nil {
			t.Errorf("expected %s/aql.jsonic: %s", m.relPath, err)
		}

		// .aql/aql.json exists (prep was run).
		if _, err := os.Stat(filepath.Join(modDir, ".aql", "aql.json")); err != nil {
			t.Errorf("expected %s/.aql/aql.json: %s", m.relPath, err)
		}
	}
}

// --- Resource exports ---

func TestDeepResourceTextkitConfig(t *testing.T) {
	dir := wordlabDir(t)
	result, err := runRealFileSteps(t, dir, []string{
		`(import "textkit")`,
		`resource.config`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "{mode:'standard' version:'3.2.0'}")
}

func TestDeepResourceTextkitConfigKey(t *testing.T) {
	dir := wordlabDir(t)
	result, err := runRealFileSteps(t, dir, []string{
		`(import "textkit")`,
		`resource.config.version`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "'3.2.0'")
}

func TestDeepResourceCharopsLetters(t *testing.T) {
	dir := wordlabDir(t)
	wrapperDir := filepath.Join(dir, ".aql", "textkit", ".aql", "styler", ".aql",
		"decorator", ".aql", "formatter", ".aql", "caser", ".aql", "wrapper")
	result, err := runRealFileSteps(t, wrapperDir, []string{
		`(import "charops")`,
		`resource.letters.alpha`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "'a'")
}

func TestDeepResourceCharopsLettersAll(t *testing.T) {
	dir := wordlabDir(t)
	wrapperDir := filepath.Join(dir, ".aql", "textkit", ".aql", "styler", ".aql",
		"decorator", ".aql", "formatter", ".aql", "caser", ".aql", "wrapper")
	result, err := runRealFileSteps(t, wrapperDir, []string{
		`(import "charops")`,
		`resource.letters`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "{alpha:'a' beta:'b' gamma:'c'}")
}

func TestDeepResourceNoConflictWithExports(t *testing.T) {
	// Importing textkit should still work - resource doesn't break regular exports.
	dir := wordlabDir(t)
	result, err := runRealFileSteps(t, dir, []string{
		`(import "textkit")`,
		`"test" Textkit.process`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "'~ >> *TEST!* ~'")
}

// --- Registry zips verification ---

func TestDeepRegistryZipsExist(t *testing.T) {
	regDir, err := filepath.Abs("../test/regsrv/registry")
	if err != nil {
		t.Fatal(err)
	}

	zips := []string{
		"charops-2.3.1.zip",
		"joiner-0.4.2.zip",
		"wrapper-1.1.0.zip",
		"tagger-3.0.2.zip",
		"caser-0.2.4.zip",
		"bracket-1.3.0.zip",
		"formatter-2.1.1.zip",
		"decorator-0.5.3.zip",
		"styler-1.0.7.zip",
		"textkit-3.2.0.zip",
	}

	for _, z := range zips {
		if _, err := os.Stat(filepath.Join(regDir, z)); err != nil {
			t.Errorf("expected registry zip %s: %s", z, err)
		}
	}
}
