package test

import (
	"os"
	"path/filepath"
	"testing"
)

// =====================================================================
// Deep dependency chain: 7 layers of module dependencies.
//
// Layer 7 (leaves): charops (v2.3.1), joiner (v0.4.2)
// Layer 6: wrapper (v1.1.0) → charops, tagger (v3.0.2) → joiner
// Layer 5: caser (v0.2.4) → wrapper, bracket (v1.3.0) → tagger
// Layer 4: formatter (v2.1.1) → caser + bracket
// Layer 3: decorator (v0.5.3) → formatter
// Layer 2: styler (v1.0.7) → decorator
// Layer 1: textkit (v3.2.0) → styler
// Project: wordlab → all 10 modules
// =====================================================================

func wordlabDir(t *testing.T) string {
	t.Helper()
	abs, err := filepath.Abs("module-work/wordlab")
	if err != nil {
		t.Fatal(err)
	}
	return abs
}

// --- Layer 7: leaf modules ---

func TestDeepCharopsToUp(t *testing.T) {
	dir := wordlabDir(t)
	result, err := runRealFileSteps(t, dir, []string{
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
	result, err := runRealFileSteps(t, dir, []string{
		`(import "charops")`,
		`"WORLD" Charops.to-down`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "'world'")
}

func TestDeepJoinerAddDot(t *testing.T) {
	dir := wordlabDir(t)
	result, err := runRealFileSteps(t, dir, []string{
		`(import "joiner")`,
		`"hello" Joiner.add-dot`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "'hello.'")
}

func TestDeepJoinerAddBang(t *testing.T) {
	dir := wordlabDir(t)
	result, err := runRealFileSteps(t, dir, []string{
		`(import "joiner")`,
		`"hello" Joiner.add-bang`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "'hello!'")
}

// --- Layer 6: wrapper, tagger ---

func TestDeepWrapperShout(t *testing.T) {
	dir := wordlabDir(t)
	result, err := runRealFileSteps(t, dir, []string{
		`(import "wrapper")`,
		`"hello" Wrapper.shout`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "'HELLO!'")
}

func TestDeepWrapperWhisper(t *testing.T) {
	dir := wordlabDir(t)
	result, err := runRealFileSteps(t, dir, []string{
		`(import "wrapper")`,
		`"HELLO" Wrapper.whisper`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "'hello...'")
}

func TestDeepTaggerTag(t *testing.T) {
	dir := wordlabDir(t)
	result, err := runRealFileSteps(t, dir, []string{
		`(import "tagger")`,
		`"test" Tagger.tag`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "'[test.]'")
}

// --- Layer 5: caser, bracket ---

func TestDeepCaserEmphasize(t *testing.T) {
	dir := wordlabDir(t)
	result, err := runRealFileSteps(t, dir, []string{
		`(import "caser")`,
		`"hello" Caser.emphasize`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "'*HELLO!*'")
}

func TestDeepBracketLabel(t *testing.T) {
	dir := wordlabDir(t)
	result, err := runRealFileSteps(t, dir, []string{
		`(import "bracket")`,
		`"test" Bracket.label`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "'<[test.]>'")
}

// --- Layer 4: formatter (two branches merge) ---

func TestDeepFormatterFormat(t *testing.T) {
	dir := wordlabDir(t)
	result, err := runRealFileSteps(t, dir, []string{
		`(import "formatter")`,
		`"hello" Formatter.format`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "'*HELLO!*'")
}

func TestDeepFormatterMark(t *testing.T) {
	dir := wordlabDir(t)
	result, err := runRealFileSteps(t, dir, []string{
		`(import "formatter")`,
		`"test" Formatter.mark`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "'<[test.]>'")
}

// --- Layer 3: decorator ---

func TestDeepDecoratorDecorate(t *testing.T) {
	dir := wordlabDir(t)
	result, err := runRealFileSteps(t, dir, []string{
		`(import "decorator")`,
		`"hello" Decorator.decorate`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "'>> *HELLO!*'")
}

func TestDeepDecoratorLabelIt(t *testing.T) {
	dir := wordlabDir(t)
	result, err := runRealFileSteps(t, dir, []string{
		`(import "decorator")`,
		`"test" Decorator.label-it`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "'>> <[test.]>'")
}

// --- Layer 2: styler ---

func TestDeepStylerStyle(t *testing.T) {
	dir := wordlabDir(t)
	result, err := runRealFileSteps(t, dir, []string{
		`(import "styler")`,
		`"hello" Styler.style`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "'~ >> *HELLO!* ~'")
}

// --- Layer 1: textkit (full 7-layer chain) ---

func TestDeepTextkitProcess(t *testing.T) {
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

// --- Full pipeline via project code ---

func TestDeepWordlabProjectCode(t *testing.T) {
	dir := wordlabDir(t)
	// Run the same code as index.aql directly.
	result, err := runRealFileSteps(t, dir, []string{
		`(import "textkit")`,
		`"hello" Textkit.process`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "'~ >> *HELLO!* ~'")
}

// --- Cross-branch: format through both branches from project ---

func TestDeepWordlabBothBranches(t *testing.T) {
	dir := wordlabDir(t)
	result, err := runRealFileSteps(t, dir, []string{
		`(import "formatter")`,
		`"word" Formatter.format`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "'*WORD!*'")

	result, err = runRealFileSteps(t, dir, []string{
		`(import "formatter")`,
		`"tag" Formatter.mark`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "'<[tag.]>'")
}

// --- File tree verification ---

func TestDeepWordlabFileTree(t *testing.T) {
	dir := wordlabDir(t)

	// All 10 modules should be installed under .aql/
	modules := []struct {
		name    string
		main    string
		version string
	}{
		{"charops", "charops.aql", "2.3.1"},
		{"joiner", "joiner.aql", "0.4.2"},
		{"wrapper", "wrapper.aql", "1.1.0"},
		{"tagger", "tagger.aql", "3.0.2"},
		{"caser", "caser.aql", "0.2.4"},
		{"bracket", "bracket.aql", "1.3.0"},
		{"formatter", "formatter.aql", "2.1.1"},
		{"decorator", "decorator.aql", "0.5.3"},
		{"styler", "styler.aql", "1.0.7"},
		{"textkit", "textkit.aql", "3.2.0"},
	}

	for _, m := range modules {
		modDir := filepath.Join(dir, ".aql", m.name)

		// Module directory exists.
		if fi, err := os.Stat(modDir); err != nil || !fi.IsDir() {
			t.Errorf("expected .aql/%s/ directory", m.name)
			continue
		}

		// Main file exists.
		mainPath := filepath.Join(modDir, m.main)
		if _, err := os.Stat(mainPath); err != nil {
			t.Errorf("expected .aql/%s/%s: %s", m.name, m.main, err)
		}

		// aql.jsonic exists.
		jsonicPath := filepath.Join(modDir, "aql.jsonic")
		if _, err := os.Stat(jsonicPath); err != nil {
			t.Errorf("expected .aql/%s/aql.jsonic: %s", m.name, err)
		}

		// .aql/aql.json exists (prep was run).
		aqlJSON := filepath.Join(modDir, ".aql", "aql.json")
		if _, err := os.Stat(aqlJSON); err != nil {
			t.Errorf("expected .aql/%s/.aql/aql.json: %s", m.name, err)
		}
	}

	// Project-level files.
	if _, err := os.Stat(filepath.Join(dir, "aql.jsonic")); err != nil {
		t.Errorf("expected wordlab/aql.jsonic: %s", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "index.aql")); err != nil {
		t.Errorf("expected wordlab/index.aql: %s", err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".aql", "aql.json")); err != nil {
		t.Errorf("expected wordlab/.aql/aql.json: %s", err)
	}
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
