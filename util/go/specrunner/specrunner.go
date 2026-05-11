// Package specrunner is the shared scaffolding for the .tsv spec-suite
// test runners — `eng/go/spec_test.go` (kernel) and
// `lang/test/spec_runner_test.go` (production language). Both walk a
// directory of `.tsv` files and, for each non-blank/non-comment row,
// parse the `<input><TAB><expected>[<TAB><note>]` columns, evaluate the
// input through a caller-supplied engine, and compare a rendered output
// string to `<expected>` (with a `ERROR:<wantSubstring>` form for
// expected-error rows).
//
// The caller supplies a Run function that does the parse-and-evaluate
// step. Rendering an eng.Value stack to the canonical spec format lives
// here (RenderStack / RenderValue) so the two runners share the same
// format.
package specrunner

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/aql-lang/aql/eng"
)

// Run executes one spec row's input and returns the resulting stack.
// Returning an error is the row's way of signalling that the input
// errored; a row marked `ERROR:<text>` in the .tsv passes when the
// returned error's message contains `<text>` (empty `<text>` matches
// any error).
type Run func(input string) ([]eng.Value, error)

// RunDir runs every `.tsv` file in dir as a subtest named after the
// file's basename (minus the `.tsv` suffix). Each row inside the file
// becomes its own subtest (`L<line>_<input-snippet>`). Fails the parent
// test if dir has no `.tsv` files.
func RunDir(t *testing.T, dir string, run Run) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read %s: %v", dir, err)
	}
	ran := 0
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".tsv") {
			continue
		}
		ran++
		t.Run(strings.TrimSuffix(e.Name(), ".tsv"), func(t *testing.T) {
			RunFile(t, filepath.Join(dir, e.Name()), run)
		})
	}
	if ran == 0 {
		t.Errorf("no .tsv specs found under %s", dir)
	}
}

// RunFile runs every data row of a single `.tsv` file against run.
func RunFile(t *testing.T, path string, run Run) {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open %s: %v", path, err)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	lineNum := 0
	for scanner.Scan() {
		lineNum++
		raw := scanner.Text()
		line := strings.TrimRight(raw, " \t")
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		parts := strings.Split(line, "\t")
		if len(parts) < 2 {
			t.Errorf("%s:L%d: malformed row, want at least input<TAB>expected, got %q", path, lineNum, line)
			continue
		}
		input := strings.TrimSpace(parts[0])
		expected := strings.TrimSpace(parts[1])

		name := fmt.Sprintf("L%d_%s", lineNum, sanitiseSpecName(input))
		t.Run(name, func(t *testing.T) {
			out, runErr := run(input)

			if strings.HasPrefix(expected, "ERROR:") {
				want := expected[len("ERROR:"):]
				if runErr == nil {
					t.Fatalf("expected error containing %q, got result %v", want, RenderStack(out))
				}
				if want != "" && !strings.Contains(runErr.Error(), want) {
					t.Errorf("error %q does not contain %q", runErr.Error(), want)
				}
				return
			}

			if runErr != nil {
				t.Fatalf("unexpected error: %v", runErr)
			}
			got := RenderStack(out)
			if got != expected {
				t.Errorf("got %q, want %q", got, expected)
			}
		})
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("scanner error in %s: %v", path, err)
	}
}

// RenderStack renders a slice of values as a single space-separated
// string in spec format.
func RenderStack(stack []eng.Value) string {
	parts := make([]string, len(stack))
	for i, v := range stack {
		parts[i] = RenderValue(v)
	}
	return strings.Join(parts, " ")
}

// RenderValue renders one value in the spec format. The spec format
// diverges from Value.String for clarity in expected columns: strings
// single-quoted, atoms as `atom(name)`, lists as space-separated
// `[a b c]`, maps as `{k:v k:v}`, type literals as their leaf, and
// `none` lowercase.
func RenderValue(v eng.Value) string {
	switch {
	case v.IsNone():
		return "none"
	case v.Data == nil:
		if name := eng.TypeNameByID(v.VType.ID); name != "" {
			return name
		}
		return v.VType.Leaf()
	case v.VType.Matches(eng.TInteger):
		n, _ := v.AsInteger()
		return strconv.FormatInt(n, 10)
	case v.VType.Matches(eng.TDecimal):
		f, _ := v.AsDecimal()
		return eng.FormatDecimal(f)
	case v.VType.Matches(eng.TString):
		s, _ := v.AsString()
		return "'" + s + "'"
	case v.VType.Matches(eng.TBoolean):
		b, _ := v.AsBoolean()
		if b {
			return "true"
		}
		return "false"
	case v.VType.Equal(eng.TAtom) && v.Data != nil:
		s, _ := v.AsAtom()
		return "atom(" + s + ")"
	case v.VType.Matches(eng.TList) && v.Data != nil:
		lst := v.AsList()
		parts := make([]string, lst.Len())
		for i := 0; i < lst.Len(); i++ {
			parts[i] = RenderValue(lst.Get(i))
		}
		return "[" + strings.Join(parts, " ") + "]"
	case v.VType.Equal(eng.TMap) && v.Data != nil:
		m := v.AsMap()
		if m == nil {
			return v.String()
		}
		parts := make([]string, m.Len())
		for i, k := range m.Keys() {
			val, _ := m.Get(k)
			parts[i] = k + ":" + RenderValue(val)
		}
		return "{" + strings.Join(parts, " ") + "}"
	default:
		return v.String()
	}
}

func sanitiseSpecName(s string) string {
	s = strings.ReplaceAll(s, " ", "_")
	if len(s) > 40 {
		s = s[:40]
	}
	return s
}
