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

func TestMergeAndTally(t *testing.T) {
	blocks, err := mergeProfiles([]string{
		writeProfile(t, "a.out", profA),
		writeProfile(t, "b.out", profB),
	})
	if err != nil {
		t.Fatalf("mergeProfiles: %v", err)
	}
	perModule, all := tallyModules(blocks)
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

func TestRunPassAndFail(t *testing.T) {
	full := writeProfile(t, "full.out", "mode: set\ngithub.com/aql-lang/aql/eng/go/a.go:1.1,2.2 3 1\n")
	partial := writeProfile(t, "part.out", profA)

	var out, errb bytes.Buffer
	if code := run([]string{full}, &out, &errb); code != 0 {
		t.Fatalf("run(full) = %d, want 0; stderr %s", code, errb.String())
	}
	if !strings.Contains(out.String(), "PASS") || !strings.Contains(out.String(), "TOTAL") {
		t.Errorf("pass output missing table/PASS: %q", out.String())
	}

	out.Reset()
	errb.Reset()
	if code := run([]string{partial}, &out, &errb); code != 1 {
		t.Fatalf("run(partial) = %d, want 1", code)
	}
	if !strings.Contains(errb.String(), "below the 100.0% floor") {
		t.Errorf("fail output = %q, want floor message", errb.String())
	}

	// A lowered threshold admits the same partial profile (3/10 = 30%).
	out.Reset()
	errb.Reset()
	if code := run([]string{"-threshold", "25", partial}, &out, &errb); code != 0 {
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
