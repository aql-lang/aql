package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeProfile(t *testing.T, name, content string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(p, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	return p
}

// fixtureRoot writes repo-relative source files under a temp dir and returns
// it, for tests that exercise //covergate:allow pragma scanning.
func fixtureRoot(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	for rel, src := range files {
		p := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

const profA = `mode: set
github.com/aql-lang/aql/eng/go/a.go:1.1,2.2 3 1
github.com/aql-lang/aql/eng/go/a.go:3.1,4.2 2 0
github.com/aql-lang/aql/lang/go/b.go:1.1,2.2 5 0
`

// profB covers the block profA missed (dedupe must take the max count)
// and an out-of-repo file (bucketed as "other").
const profB = `mode: set
github.com/aql-lang/aql/eng/go/a.go:3.1,4.2 2 7
example.com/x/y.go:1.1,2.2 1 1
`

// plainFiles are source files matching profA/profB's repo paths with no
// pragmas — enough lines to back every block's start line.
func plainRoot(t *testing.T) string {
	return fixtureRoot(t, map[string]string{
		"eng/go/a.go":  "package a\nvar _ = 1\nvar _ = 2\nvar _ = 3\n",
		"lang/go/b.go": "package b\nvar _ = 1\n",
	})
}

func TestMergeAndTally(t *testing.T) {
	blocks, err := mergeProfiles([]string{
		writeProfile(t, "a.out", profA),
		writeProfile(t, "b.out", profB),
	})
	if err != nil {
		t.Fatalf("mergeProfiles: %v", err)
	}
	perModule, all, _ := tallyModules(blocks, nil)
	if got := perModule["eng/go"]; got.covered != 5 || got.total != 5 {
		t.Errorf("eng/go tally = %+v, want 5/5 (second profile covers the gap)", got)
	}
	if got := perModule["lang/go"]; got.covered != 0 || got.total != 5 {
		t.Errorf("lang/go tally = %+v, want 0/5", got)
	}
	if got := perModule["other"]; got.covered != 1 || got.total != 1 {
		t.Errorf("other tally = %+v, want 1/1", got)
	}
	if all.covered != 6 || all.total != 11 {
		t.Errorf("total tally = %+v, want 6/11", all)
	}
}

func TestModuleOf(t *testing.T) {
	cases := map[string]string{
		"github.com/aql-lang/aql/eng/go/engine.go": "eng/go",
		"github.com/aql-lang/aql/wpg/serve/m.go":   "wpg/serve",
		"github.com/aql-lang/aql/single.go":        "single.go",
		"example.com/other/pkg/f.go":               "other",
	}
	for in, want := range cases {
		if got := moduleOf(in); got != want {
			t.Errorf("moduleOf(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestPctEmptyIsFull(t *testing.T) {
	if got := pct(tally{}); got != 100 {
		t.Errorf("pct(empty) = %v, want 100 (no statements = nothing uncovered)", got)
	}
}

func TestBlockStart(t *testing.T) {
	got := blockStart("github.com/aql-lang/aql/eng/go/a.go:42.7,45.3")
	want := pragmaKey{"github.com/aql-lang/aql/eng/go/a.go", 42}
	if got != want {
		t.Errorf("blockStart = %+v, want %+v", got, want)
	}
}

func TestMergeRejections(t *testing.T) {
	cases := []struct{ name, content, wantErr string }{
		{"missing fields", "mode: set\nnot-a-profile-line\n", "malformed profile line"},
		{"bad stmt count", "github.com/aql-lang/aql/e/g/a.go:1.1,2.2 x 1\n", "malformed statement count"},
		{"bad hit count", "github.com/aql-lang/aql/e/g/a.go:1.1,2.2 1 y\n", "malformed hit count"},
	}
	for _, c := range cases {
		_, err := mergeProfiles([]string{writeProfile(t, "bad.out", c.content)})
		if err == nil || !strings.Contains(err.Error(), c.wantErr) {
			t.Errorf("%s: err = %v, want %q", c.name, err, c.wantErr)
		}
	}
	if _, err := mergeProfiles([]string{filepath.Join(t.TempDir(), "absent.out")}); err == nil {
		t.Error("missing profile file: want error, got none")
	}
}

func TestScanPragmas(t *testing.T) {
	root := fixtureRoot(t, map[string]string{
		"eng/go/a.go": "package a\nvar _ = 1 //covergate:allow dead arm\nvar _ = 2\n",
	})
	blocks := map[string]block{
		"github.com/aql-lang/aql/eng/go/a.go:2.1,3.2": {stmts: 1},
		"example.com/out/z.go:1.1,2.2":                {stmts: 1}, // out-of-repo → skipped
	}
	pr, err := scanPragmas(root, blocks)
	if err != nil {
		t.Fatalf("scanPragmas: %v", err)
	}
	if len(pr) != 1 || pr[pragmaKey{"github.com/aql-lang/aql/eng/go/a.go", 2}] != "dead arm" {
		t.Fatalf("pragmas = %v, want line 2 -> 'dead arm'", pr)
	}

	// A marker with no reason is rejected.
	bad := fixtureRoot(t, map[string]string{
		"eng/go/a.go": "package a\nvar _ = 1 //covergate:allow\n",
	})
	if _, err := scanPragmas(bad, map[string]block{
		"github.com/aql-lang/aql/eng/go/a.go:2.1,2.2": {stmts: 1},
	}); err == nil || !strings.Contains(err.Error(), "needs a reason") {
		t.Errorf("empty-reason err = %v, want 'needs a reason'", err)
	}

	// A marker MENTIONED in a doc comment or a string literal is NOT a
	// pragma (comment-token exact via go/parser), so it excludes nothing.
	mention := fixtureRoot(t, map[string]string{
		"eng/go/a.go": "package a\n\n// docs about //covergate:allow markers\nvar _ = \"//covergate:allow x\"\n",
	})
	if pr, err := scanPragmas(mention, map[string]block{
		"github.com/aql-lang/aql/eng/go/a.go:3.1,4.2": {stmts: 1},
	}); err != nil || len(pr) != 0 {
		t.Errorf("mention scan = %v, %v; want no pragmas", pr, err)
	}

	// A referenced source file missing under root is an error.
	if _, err := scanPragmas(t.TempDir(), map[string]block{
		"github.com/aql-lang/aql/eng/go/gone.go:1.1,2.2": {stmts: 1},
	}); err == nil || !strings.Contains(err.Error(), "pragma scan") {
		t.Errorf("missing-file err = %v, want 'pragma scan'", err)
	}

	// A source file that does not parse is an error.
	broken := fixtureRoot(t, map[string]string{"eng/go/a.go": "package a\nfunc {\n"})
	if _, err := scanPragmas(broken, map[string]block{
		"github.com/aql-lang/aql/eng/go/a.go:1.1,2.2": {stmts: 1},
	}); err == nil || !strings.Contains(err.Error(), "pragma scan") {
		t.Errorf("parse-error err = %v, want 'pragma scan'", err)
	}
}

func TestRunPassAndFail(t *testing.T) {
	root := plainRoot(t)
	full := writeProfile(t, "full.out", "mode: set\ngithub.com/aql-lang/aql/eng/go/a.go:1.1,2.2 3 1\n")
	partial := writeProfile(t, "part.out", profA)

	var out, errb bytes.Buffer
	if code := run([]string{"-root", root, full}, &out, &errb); code != 0 {
		t.Fatalf("run(full) = %d, want 0; stderr %s", code, errb.String())
	}
	if !strings.Contains(out.String(), "PASS") || !strings.Contains(out.String(), "TOTAL") {
		t.Errorf("pass output missing table/PASS: %q", out.String())
	}

	out.Reset()
	errb.Reset()
	if code := run([]string{"-root", root, partial}, &out, &errb); code != 1 {
		t.Fatalf("run(partial) = %d, want 1", code)
	}
	if !strings.Contains(errb.String(), "below the 100.0% floor") {
		t.Errorf("fail output = %q, want floor message", errb.String())
	}

	// A lowered threshold admits the same partial profile (3/10 = 30%).
	out.Reset()
	errb.Reset()
	if code := run([]string{"-root", root, "-threshold", "25", partial}, &out, &errb); code != 0 {
		t.Fatalf("run(-threshold 25) = %d, want 0; stderr %s", code, errb.String())
	}
}

func TestRunUsageErrors(t *testing.T) {
	var out, errb bytes.Buffer
	if code := run(nil, &out, &errb); code != 2 {
		t.Errorf("run() = %d, want 2 (no profiles)", code)
	}
	if code := run([]string{"-nope"}, &out, &errb); code != 2 {
		t.Errorf("run(-nope) = %d, want 2 (bad flag)", code)
	}
	if code := run([]string{filepath.Join(t.TempDir(), "absent.out")}, &out, &errb); code != 2 {
		t.Errorf("run(absent) = %d, want 2 (unreadable profile)", code)
	}
}

func TestPragmaExcludes(t *testing.T) {
	blocks, err := mergeProfiles([]string{
		writeProfile(t, "a.out", profA),
		writeProfile(t, "b.out", profB),
	})
	if err != nil {
		t.Fatal(err)
	}
	// Exclude lang/go's uncovered block via a pragma on its opening line;
	// the remaining tree is fully covered, so 6/6.
	pragmas := map[pragmaKey]string{
		{"github.com/aql-lang/aql/lang/go/b.go", 1}: "dead",
	}
	perModule, all, ex := tallyModules(blocks, pragmas)
	if _, ok := perModule["lang/go"]; ok {
		t.Errorf("excluded lang/go block still tallied: %+v", perModule["lang/go"])
	}
	if all.covered != 6 || all.total != 6 {
		t.Errorf("tally with pragma = %+v, want 6/6", all)
	}
	if ex.stmts != 5 || ex.blocks != 1 {
		t.Errorf("excluded = %d stmts / %d blocks, want 5/1", ex.stmts, ex.blocks)
	}

	// A pragma whose line's blocks are all covered is reported for removal.
	covered := map[pragmaKey]string{
		{"github.com/aql-lang/aql/eng/go/a.go", 1}: "dead",
	}
	if _, _, ex := tallyModules(blocks, covered); len(ex.nowCovered) != 1 {
		t.Errorf("nowCovered = %v, want the covered a.go line", ex.nowCovered)
	}
	// A pragma line that opens no profiled block is reported as stale.
	stale := map[pragmaKey]string{
		{"github.com/aql-lang/aql/eng/go/gone.go", 9}: "dead",
	}
	if _, _, ex := tallyModules(blocks, stale); len(ex.stale) != 1 {
		t.Errorf("stale = %v, want the gone.go entry", ex.stale)
	}

	// Line collision: a pragma line holding BOTH a covered reaching block
	// and an uncovered guard body excludes only the guard; the covered
	// sibling is tallied, and there is no false now-covered/stale alarm.
	collide, err := mergeProfiles([]string{writeProfile(t, "c.out",
		"mode: set\n"+
			"github.com/aql-lang/aql/eng/go/c.go:5.2,5.14 1 3\n"+
			"github.com/aql-lang/aql/eng/go/c.go:5.14,7.3 1 0\n")})
	if err != nil {
		t.Fatal(err)
	}
	cp := map[pragmaKey]string{{"github.com/aql-lang/aql/eng/go/c.go", 5}: "guard"}
	_, cAll, cEx := tallyModules(collide, cp)
	if len(cEx.nowCovered) != 0 || len(cEx.stale) != 0 {
		t.Errorf("collision: nowCovered=%v stale=%v, want none", cEx.nowCovered, cEx.stale)
	}
	if cEx.blocks != 1 || cEx.stmts != 1 {
		t.Errorf("collision: excluded %d stmts / %d blocks, want 1/1 (guard only)", cEx.stmts, cEx.blocks)
	}
	if cAll.covered != 1 || cAll.total != 1 {
		t.Errorf("collision tally = %+v, want 1/1 (reaching block counted, covered)", cAll)
	}
}

func TestRunWithPragmas(t *testing.T) {
	partial := writeProfile(t, "part.out", profA) // one uncovered lang/go block
	var out, errb bytes.Buffer

	// A pragma on the uncovered block's opening line lets the gate pass.
	okRoot := fixtureRoot(t, map[string]string{
		"eng/go/a.go":  "package a\nvar _ = 1\nvar _ = 2 //covergate:allow dead\nvar _ = 3\n",
		"lang/go/b.go": "package b //covergate:allow dead\nvar _ = 1\n",
	})
	if code := run([]string{"-root", okRoot, partial}, &out, &errb); code != 0 {
		t.Fatalf("run with pragmas = %d, want 0; stderr %s", code, errb.String())
	}
	if !strings.Contains(out.String(), "allowlisted") {
		t.Errorf("pass output missing exclusion note: %q", out.String())
	}

	// A pragma on a now-covered block fails the gate (must be graduated).
	out.Reset()
	errb.Reset()
	nowCov := fixtureRoot(t, map[string]string{
		"eng/go/a.go":  "package a //covergate:allow was-dead\nvar _ = 1\nvar _ = 2\nvar _ = 3\n",
		"lang/go/b.go": "package b //covergate:allow dead\nvar _ = 1\n",
	})
	if code := run([]string{"-root", nowCov, partial}, &out, &errb); code != 1 {
		t.Fatalf("run with now-covered pragma = %d, want 1", code)
	}
	if !strings.Contains(errb.String(), "now covered") {
		t.Errorf("stderr = %q, want now-covered notice", errb.String())
	}

	// A pragma on a line that opens no block fails as stale.
	out.Reset()
	errb.Reset()
	staleRoot := fixtureRoot(t, map[string]string{
		"eng/go/a.go":  "package a\nvar _ = 1\nvar _ = 2 //covergate:allow dead\nvar _ = 3 //covergate:allow nothing-here\n",
		"lang/go/b.go": "package b //covergate:allow dead\nvar _ = 1\n",
	})
	if code := run([]string{"-root", staleRoot, partial}, &out, &errb); code != 1 {
		t.Fatalf("run with stale pragma = %d, want 1", code)
	}
	if !strings.Contains(errb.String(), "stale") {
		t.Errorf("stderr = %q, want stale notice", errb.String())
	}

	// A referenced source file missing under -root is a usage error.
	out.Reset()
	errb.Reset()
	if code := run([]string{"-root", t.TempDir(), partial}, &out, &errb); code != 2 {
		t.Errorf("run with unresolvable root = %d, want 2", code)
	}
}

// main is covered through the osExit seam (design/TEST-SEAMS.10.md).
func TestMainExitsThroughSeam(t *testing.T) {
	prevArgs, prevExit := os.Args, osExit
	t.Cleanup(func() { os.Args, osExit = prevArgs, prevExit })

	var code int
	osExit = func(c int) { code = c }
	os.Args = []string{"covergate"} // no profiles → usage error
	main()
	if code != 2 {
		t.Errorf("main with no args exited %d, want 2", code)
	}
}
