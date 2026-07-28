package run

import (
	"bytes"
	"flag"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// splitFlagSet mirrors Execute's registration closely enough for the split
// table: the eval flag, a value-taking flag in both spellings, and a
// boolean. The Execute-level tests cover the real set.
func splitFlagSet() *flag.FlagSet {
	fs := flag.NewFlagSet("aql", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.String("e", "", "")
	fs.Int64("s", 0, "")
	fs.Bool("version", false, "")
	return fs
}

func writeArgsScript(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	file := filepath.Join(dir, "args.aql")
	src := "import \"aql:io\"\nIO.args\n"
	if err := os.WriteFile(file, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	return file
}

// Positionals after the script path reach the program as IO.args.
func TestExecuteScriptArgs(t *testing.T) {
	file := writeArgsScript(t)
	var out, errb bytes.Buffer
	if code := Execute([]string{file, "notes.json", "--fast"}, strings.NewReader(""), &out, &errb); code != 0 {
		t.Fatalf("exit %d, stderr: %s", code, errb.String())
	}
	got := out.String()
	if !strings.Contains(got, "notes.json") || !strings.Contains(got, "--fast") {
		t.Errorf("stdout %q does not carry the script args", got)
	}
}

// A `-e` among the script's positionals is the program's own argument, not
// this CLI's eval flag: the whole tail reaches IO.args. Previously the split
// matched that `-e` and silently discarded everything after its next token.
func TestExecuteScriptArgsDashEIsNotEval(t *testing.T) {
	file := writeArgsScript(t)
	var out, errb bytes.Buffer
	if code := Execute([]string{file, "a", "-e", "b", "c", "d"}, strings.NewReader(""), &out, &errb); code != 0 {
		t.Fatalf("exit %d, stderr: %s", code, errb.String())
	}
	got := out.String()
	for _, want := range []string{`'a'`, `'-e'`, `'b'`, `'c'`, `'d'`} {
		if !strings.Contains(got, want) {
			t.Errorf("stdout %q dropped %s", got, want)
		}
	}
}

// The same, with a value-taking flag before the script path: reaching the
// path means stepping over `5`, so the later `-e` still is not the eval flag.
func TestExecuteScriptArgsDashEAfterValueFlag(t *testing.T) {
	file := writeArgsScript(t)
	var out, errb bytes.Buffer
	if code := Execute([]string{"-s", "5", file, "-e", "b", "c"}, strings.NewReader(""), &out, &errb); code != 0 {
		t.Fatalf("exit %d, stderr: %s", code, errb.String())
	}
	got := out.String()
	for _, want := range []string{`'-e'`, `'b'`, `'c'`} {
		if !strings.Contains(got, want) {
			t.Errorf("stdout %q dropped %s", got, want)
		}
	}
}

// No positionals → IO.args prints as an empty list, not an error.
func TestExecuteScriptArgsEmpty(t *testing.T) {
	file := writeArgsScript(t)
	var out, errb bytes.Buffer
	if code := Execute([]string{file}, strings.NewReader(""), &out, &errb); code != 0 {
		t.Fatalf("exit %d, stderr: %s", code, errb.String())
	}
	if !strings.Contains(out.String(), "[]") {
		t.Errorf("stdout %q, want an empty list", out.String())
	}
}

// In -e mode every positional is a script argument.
func TestExecuteEvalModeArgs(t *testing.T) {
	var out, errb bytes.Buffer
	code := Execute([]string{"-e", `import "aql:io"  IO.args`, "alpha"}, strings.NewReader(""), &out, &errb)
	if code != 0 {
		t.Fatalf("exit %d, stderr: %s", code, errb.String())
	}
	if !strings.Contains(out.String(), "alpha") {
		t.Errorf("stdout %q does not carry the -e args", out.String())
	}
}

// -e ends option processing (node -e / python -c): a dash-prefixed first
// script argument reaches IO.args instead of erroring as an unknown flag.
func TestExecuteEvalDashFirstArg(t *testing.T) {
	var out, errb bytes.Buffer
	code := Execute([]string{"-e", `import "aql:io"  IO.args`, "--fast", "notes.json"},
		strings.NewReader(""), &out, &errb)
	if code != 0 {
		t.Fatalf("exit %d, stderr: %s", code, errb.String())
	}
	got := out.String()
	if !strings.Contains(got, "--fast") || !strings.Contains(got, "notes.json") {
		t.Errorf("stdout %q dropped the dash-prefixed script args", got)
	}
}

// A single leading `--` — the historical separator — is stripped so
// `aql -e … -- --fast` and `aql -e … --fast` agree on IO.args.
func TestExecuteEvalSeparatorStripped(t *testing.T) {
	var out, errb bytes.Buffer
	code := Execute([]string{"-e", `import "aql:io"  IO.args`, "--", "--fast"},
		strings.NewReader(""), &out, &errb)
	if code != 0 {
		t.Fatalf("exit %d, stderr: %s", code, errb.String())
	}
	got := out.String()
	if !strings.Contains(got, "--fast") {
		t.Errorf("stdout %q lost the arg after --", got)
	}
	if strings.Count(got, "-") > strings.Count("--fast", "-") {
		t.Errorf("stdout %q kept the -- separator", got)
	}
}

// The -e=expr attached form splits its tail the same way.
func TestExecuteEvalAttachedForm(t *testing.T) {
	var out, errb bytes.Buffer
	code := Execute([]string{`-e=import "aql:io"  IO.args`, "--fast"},
		strings.NewReader(""), &out, &errb)
	if code != 0 {
		t.Fatalf("exit %d, stderr: %s", code, errb.String())
	}
	if !strings.Contains(out.String(), "--fast") {
		t.Errorf("stdout %q dropped the arg after -e=expr", out.String())
	}
}

// Flags BEFORE -e are still parsed as CLI flags (only the tail after the
// expression becomes script args).
func TestExecuteEvalFlagsBeforeExpr(t *testing.T) {
	var out, errb bytes.Buffer
	code := Execute([]string{"-s", "5", "-e", `import "aql:io"  IO.args`, "--fast"},
		strings.NewReader(""), &out, &errb)
	if code != 0 {
		t.Fatalf("exit %d, stderr: %s", code, errb.String())
	}
	if !strings.Contains(out.String(), "--fast") {
		t.Errorf("stdout %q dropped the tail after a leading -s flag", out.String())
	}
}

// A dangling `-e` (no expression) is still reported by the FlagSet, not
// silently swallowed by the split.
func TestExecuteEvalDangling(t *testing.T) {
	var out, errb bytes.Buffer
	if code := Execute([]string{"-e"}, strings.NewReader(""), &out, &errb); code != 1 {
		t.Fatalf("dangling -e: exit %d (want 1), stderr: %s", code, errb.String())
	}
}

// A `--` seen BEFORE any `-e` makes the `-e` a positional (a script path),
// not the eval flag — parsing stays normal and the missing file errors.
func TestExecuteSeparatorBeforeEval(t *testing.T) {
	var out, errb bytes.Buffer
	if code := Execute([]string{"--", "-e", "x"}, strings.NewReader(""), &out, &errb); code != 1 {
		t.Fatalf("`-- -e x`: exit %d (want 1 — -e is a positional file), stderr: %s", code, errb.String())
	}
}

// splitEvalTail unit table — pins the split points the branches above
// exercise, including the no-`-e` (script/REPL) fall-through.
func TestSplitEvalTail(t *testing.T) {
	eq := func(a, b []string) bool {
		if len(a) != len(b) {
			return false
		}
		for i := range a {
			if a[i] != b[i] {
				return false
			}
		}
		return true
	}
	cases := []struct {
		name      string
		in        []string
		wantFlags []string
		wantTail  []string
	}{
		{"bare -e with tail", []string{"-e", "x", "--fast"}, []string{"-e", "x"}, []string{"--fast"}},
		{"bare -e no tail", []string{"-e", "x"}, []string{"-e", "x"}, nil},
		{"dangling -e", []string{"-e"}, []string{"-e"}, nil},
		{"attached -e=", []string{"-e=x", "--fast"}, []string{"-e=x"}, []string{"--fast"}},
		{"double-dash e", []string{"--e", "x", "y"}, []string{"--e", "x"}, []string{"y"}},
		{"separator before -e", []string{"--", "-e", "x"}, []string{"--", "-e", "x"}, nil},
		{"no -e (script)", []string{"prog.aql", "--fast"}, []string{"prog.aql", "--fast"}, nil},

		// A `-e` AFTER the script path is the program's own argument, so
		// there is no eval split and nothing may be dropped.
		{"-e after script path", []string{"prog.aql", "a", "-e", "b", "c", "d"},
			[]string{"prog.aql", "a", "-e", "b", "c", "d"}, nil},
		{"-e=x after script path", []string{"prog.aql", "-e=x", "c"},
			[]string{"prog.aql", "-e=x", "c"}, nil},
		{"script path only", []string{"prog.aql"}, []string{"prog.aql"}, nil},

		// Reaching the script path means stepping over earlier flags'
		// values: `5` below is -s's value, not a positional.
		{"value flag before -e", []string{"-s", "5", "-e", "x", "--fast"},
			[]string{"-s", "5", "-e", "x"}, []string{"--fast"}},
		{"attached value flag before -e", []string{"-s=5", "-e", "x", "y"},
			[]string{"-s=5", "-e", "x"}, []string{"y"}},
		{"bool flag before -e", []string{"-version", "-e", "x", "y"},
			[]string{"-version", "-e", "x"}, []string{"y"}},
		{"value flag then script then -e", []string{"-s", "5", "prog.aql", "-e", "b"},
			[]string{"-s", "5", "prog.aql", "-e", "b"}, nil},
		{"bool flag then script then -e", []string{"-version", "prog.aql", "-e", "b"},
			[]string{"-version", "prog.aql", "-e", "b"}, nil},

		// An unregistered flag is assumed to take a value. fs.Parse rejects
		// it regardless, so the guess cannot change the outcome.
		{"unknown flag before -e", []string{"-zzz", "v", "-e", "x", "y"},
			[]string{"-zzz", "v", "-e", "x"}, []string{"y"}},
	}
	fs := splitFlagSet()
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			gotFlags, gotTail := splitEvalTail(fs, c.in)
			if !eq(gotFlags, c.wantFlags) || !eq(gotTail, c.wantTail) {
				t.Errorf("splitEvalTail(%v) = (%v, %v), want (%v, %v)",
					c.in, gotFlags, gotTail, c.wantFlags, c.wantTail)
			}
		})
	}
}
