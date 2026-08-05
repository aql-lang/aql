// Tests for the shared spec-runner scaffolding: the .tsv walking/parsing
// runners in specrunner.go (positive rows, expected-error rows, and every
// malformed-row rejection) and the q-fixture words in qfixtures.go
// (driven through a kernel registry exactly the way the engspec runner
// installs them — every fixture word and dispatch branch is exercised by
// a spec row whose expected value is pinned from the existing golden
// corpus under eng/spec and lang/spec).
package specfix

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	eng "github.com/boru-lang/boru/eng/go"
	"github.com/boru-lang/boru/eng/go/parser"
)

// qRun returns a Run backed by a fresh kernel registry with the shared
// q-fixtures installed — the same recipe the engspec runner uses
// (registry + fixtures + root context; parse then evaluate).
func qRun(t *testing.T) Run {
	t.Helper()
	r, err := eng.NewRegistry()
	if err != nil {
		t.Fatalf("eng.NewRegistry: %v", err)
	}
	RegisterQFixtures(r)
	r.InitRootContext()
	return func(input string) ([]eng.Value, error) {
		values, perr := parser.Parse(input)
		if perr != nil {
			return nil, perr
		}
		return eng.NewTop(r).Run(values)
	}
}

func writeSpec(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
	return path
}

// arithRows exercises the numeric fixtures: both numericBinary branches
// (int fast path, float promotion via toFloat in both argument orders),
// negq's two branches, and the two expected-error forms (`ERROR:<code>`
// and the bare any-error `ERROR:`).
const arithRows = `# arithmetic q-fixture probes
1 2 addq	3	int fast path
1 2.5 addq	3.5	int/float promotes through toFloat
2.5 1 addq	3.5	float/int promotes through toFloat
0.1 0.2 addq	0.30000000000000004	float path
5 2 subq	3	handler computes b-a over sig order
2.5 1 subq	1.5	float branch of subq
2 3 mulq	6
2.5 3 mulq	7.5	float branch of mulq

5 negq	-5	integer branch
-3.5 negq	3.5	float branch
'hi' negq	ERROR:signature_error	String rejected by the Number signature
negq	ERROR:	bare ERROR: matches any failure
1 1 addq 1 addq 1 addq 1 addq 1 addq 1 addq	7	long input exercises subtest-name truncation
`

// dispatchRows exercises every remaining fixture word: multi-signature
// dispatch (describeq/tagq/flexq), pattern-matched signatures
// (factq/codeq/routeq), the barrier/arity ladder, and the list probes
// (firstq on both the empty and non-empty branch).
const dispatchRows = `# dispatch, pattern, barrier-arity and list q-fixture probes
'hello' ' world' concatq	'hello world'
7 describeq	'int:7'
's' describeq	'str:s'
5 tagq	'specific'
true tagq	'any'
0 factq	1	pattern arm
5 factq	5	general arm
99 codeq	'ninety-nine'	pattern arm
7 codeq	'general'
'admin' routeq	'matched-admin'	pattern arm
'bob' routeq	'other'
1 2 3 tripq	'3,2,1'
1 2 pairq	'2:1'
nilq	'nil'
5 flexq	'one:5'	unary signature
1 2 flexq	'two:2,1'	binary signature
1 2 3 tri1q	'3,2,1'
1 2 3 tri2q	'3,2,1'
1 2 3 4 quad1q	'4,3,2,1'
1 2 3 4 quadq	'4,3,2,1'
1 2 3 4 quad3q	'4,3,2,1'
1 2 3 4 5 quintq	'5,4,3,2,1'
1 2 3 4 5 6 hexq	'6,5,4,3,2,1'
1 2 3 4 5 6 7 septq	'7,6,5,4,3,2,1'
[1 2] lengthq	2
[1 2] firstq	1
[] firstq	none	empty-list branch
`

// TestRunDirQFixtures drives the q-fixture corpus through RunDir: every
// .tsv in the directory runs, non-.tsv entries and subdirectories are
// skipped, and every row must reproduce its pinned golden value.
func TestRunDirQFixtures(t *testing.T) {
	dir := t.TempDir()
	writeSpec(t, dir, "arith.tsv", arithRows)
	writeSpec(t, dir, "dispatch.tsv", dispatchRows)
	writeSpec(t, dir, "notes.txt", "not a spec; must be ignored\n")
	if err := os.Mkdir(filepath.Join(dir, "subdir"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	RunDir(t, dir, qRun(t))
}

// TestRunDirRendered drives the rendered-string runner over a small fake
// renderer, covering the value, ERROR:<substring>, and bare-ERROR: rows.
func TestRunDirRendered(t *testing.T) {
	render := func(input string) (string, error) {
		if rest, ok := strings.CutPrefix(input, "err:"); ok {
			return "", errors.New(rest)
		}
		return strings.ToUpper(input), nil
	}
	dir := t.TempDir()
	writeSpec(t, dir, "rendered.tsv", strings.Join([]string{
		"# rendered-runner probes",
		"hello\tHELLO",
		"two words\tTWO WORDS\twith a note",
		"err:boom\tERROR:boom",
		"err:whatever\tERROR:\tbare ERROR: matches any failure",
	}, "\n")+"\n")
	writeSpec(t, dir, "README", "ignored: no .tsv suffix\n")
	RunDirRendered(t, dir, render)
}

func TestSanitiseSpecName(t *testing.T) {
	long := strings.Repeat("a", 41)
	cases := []struct{ in, want string }{
		{"", ""},
		{"1 2 addq", "1_2_addq"},
		{strings.Repeat("a", 40), strings.Repeat("a", 40)},
		{long, long[:40]},
		{"spaces get underscored everywhere in the name here", "spaces_get_underscored_everywhere_in_the"},
	}
	for _, c := range cases {
		if got := sanitiseSpecName(c.in); got != c.want {
			t.Errorf("sanitiseSpecName(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// runInner runs f as its own top-level test via testing.RunTests,
// reporting the verdict and anything the failing run printed WITHOUT
// failing the caller. RunTests is deprecated for external use, but it is
// the only in-process way to observe a *testing.T failure — a subprocess
// re-run would not record coverage for the exercised branches.
func runInner(t *testing.T, f func(t *testing.T)) (ok bool, output string) {
	t.Helper()
	outPath := filepath.Join(t.TempDir(), "inner-output")
	fout, err := os.Create(outPath)
	if err != nil {
		t.Fatalf("create %s: %v", outPath, err)
	}
	orig := os.Stdout
	os.Stdout = fout
	func() {
		defer func() { os.Stdout = orig }()
		match := func(pat, str string) (bool, error) { return true, nil }
		ok = testing.RunTests(match, []testing.InternalTest{{Name: "inner", F: f}}) //nolint:staticcheck // see runInner doc comment
	}()
	if cerr := fout.Close(); cerr != nil {
		t.Fatalf("close %s: %v", outPath, cerr)
	}
	b, rerr := os.ReadFile(outPath)
	if rerr != nil {
		t.Fatalf("read %s: %v", outPath, rerr)
	}
	return ok, string(b)
}

// TestRunnerRejections asserts the runners FAIL (not skip) on every
// malformed or mismatching row shape, and that the failure message names
// the problem.
func TestRunnerRejections(t *testing.T) {
	run := qRun(t)
	render := func(input string) (string, error) {
		if rest, ok := strings.CutPrefix(input, "err:"); ok {
			return "", errors.New(rest)
		}
		return strings.ToUpper(input), nil
	}
	// A single line longer than bufio.Scanner's default 64KiB token limit
	// aborts the scan — the runner must surface that, not pass vacuously.
	oversized := strings.Repeat("a", 70_000) + "\tx\n"

	cases := []struct {
		name    string
		content string // the .tsv body driven through the value runner
		want    string // substring of the failure output
	}{
		{
			name: "value malformed row", content: "just-one-column\n", want: "malformed row",
		},
		{
			name: "value wrong result", content: "1 2 addq\t4\n", want: `got "3", want "4"`,
		},
		{
			name: "value unexpected error", content: "negq\t7\n", want: "unexpected error",
		},
		{
			name: "value expected error got value", content: "1 2 addq\tERROR:boom\n", want: "expected error containing",
		},
		{
			name: "value error missing substring", content: "'hi' negq\tERROR:zap\n", want: "does not contain",
		},
		{
			name: "value oversized line", content: oversized, want: "scanner error",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			path := writeSpec(t, t.TempDir(), "case.tsv", c.content)
			ok, out := runInner(t, func(t *testing.T) { RunFile(t, path, run) })
			if ok {
				t.Fatalf("RunFile accepted %q", c.content)
			}
			if !strings.Contains(out, c.want) {
				t.Errorf("failure output %q does not contain %q", out, c.want)
			}
		})
	}

	rendCases := []struct {
		name    string
		content string
		want    string
	}{
		{"rendered malformed row", "just-one-column\n", "malformed row"},
		{"rendered wrong result", "hello\tNOPE\n", `got "HELLO", want "NOPE"`},
		{"rendered unexpected error", "err:boom\tBOOM\n", "unexpected error"},
		{"rendered expected error got value", "hello\tERROR:boom\n", "expected error containing"},
		{"rendered error missing substring", "err:boom\tERROR:zap\n", "does not contain"},
		{"rendered oversized line", oversized, "scanner error"},
	}
	for _, c := range rendCases {
		t.Run(c.name, func(t *testing.T) {
			path := writeSpec(t, t.TempDir(), "case.tsv", c.content)
			ok, out := runInner(t, func(t *testing.T) { RunFileRendered(t, path, render) })
			if ok {
				t.Fatalf("RunFileRendered accepted %q", c.content)
			}
			if !strings.Contains(out, c.want) {
				t.Errorf("failure output %q does not contain %q", out, c.want)
			}
		})
	}

	t.Run("dir with no specs", func(t *testing.T) {
		empty := t.TempDir()
		if ok, out := runInner(t, func(t *testing.T) { RunDir(t, empty, run) }); ok {
			t.Fatalf("RunDir passed on a directory with no .tsv files")
		} else if !strings.Contains(out, "no .tsv specs found") {
			t.Errorf("failure output %q does not name the missing specs", out)
		}
		if ok, out := runInner(t, func(t *testing.T) { RunDirRendered(t, empty, render) }); ok {
			t.Fatalf("RunDirRendered passed on a directory with no .tsv files")
		} else if !strings.Contains(out, "no .tsv specs found") {
			t.Errorf("failure output %q does not name the missing specs", out)
		}
	})

	t.Run("missing paths", func(t *testing.T) {
		absentDir := filepath.Join(t.TempDir(), "absent")
		if ok, _ := runInner(t, func(t *testing.T) { RunDir(t, absentDir, run) }); ok {
			t.Errorf("RunDir passed on a missing directory")
		}
		if ok, _ := runInner(t, func(t *testing.T) { RunDirRendered(t, absentDir, render) }); ok {
			t.Errorf("RunDirRendered passed on a missing directory")
		}
		absentFile := filepath.Join(t.TempDir(), "absent.tsv")
		if ok, _ := runInner(t, func(t *testing.T) { RunFile(t, absentFile, run) }); ok {
			t.Errorf("RunFile passed on a missing file")
		}
		if ok, _ := runInner(t, func(t *testing.T) { RunFileRendered(t, absentFile, render) }); ok {
			t.Errorf("RunFileRendered passed on a missing file")
		}
	})
}
