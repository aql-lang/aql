package test

// Spec-runner test for files under lang/test/spec/. Each TSV row is
// parsed with the AQL parser (eng/parser) and run against a fresh
// production registry (engine.DefaultRegistry + native.Register) — the
// full language layer, so these specs can exercise any registered word
// (record / object / make / get / length / …) and the builtin
// Resource / Entity types installed by installResourceTypes.
//
// The kernel-only spec suite (q-suffixed fixtures, eng.RegisterCoreWords)
// lives in eng/go/spec_test.go — it tests the engine kernel in
// isolation. The render helpers below are duplicated there because the
// two runners are in separate Go modules (eng vs lang).

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/metsitaba/voxgig-exp/eng"
	"github.com/metsitaba/voxgig-exp/eng/parser"
	"github.com/metsitaba/voxgig-exp/lang/internal/engine"
	"github.com/metsitaba/voxgig-exp/lang/internal/native"
)

// renderSpecValue renders a value in the spec format. The spec format
// diverges from Value.String for clarity in expected columns: strings
// double-quoted, atoms as `atom(name)`, lists as space-separated
// `[a b c]`, maps as `{k:v k:v}`, type literals as their leaf, and
// `none` lowercase.
func renderSpecValue(v eng.Value) string {
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
		return "\"" + s + "\""
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
			parts[i] = renderSpecValue(lst.Get(i))
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
			parts[i] = k + ":" + renderSpecValue(val)
		}
		return "{" + strings.Join(parts, " ") + "}"
	default:
		return v.String()
	}
}

func renderSpecStack(stack []eng.Value) string {
	parts := make([]string, len(stack))
	for i, v := range stack {
		parts[i] = renderSpecValue(v)
	}
	return strings.Join(parts, " ")
}

func sanitiseSpecName(s string) string {
	s = strings.ReplaceAll(s, " ", "_")
	if len(s) > 40 {
		s = s[:40]
	}
	return s
}

func runProdSpecFile(t *testing.T, path string) {
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
			values, err := parser.Parse(input)
			if err != nil {
				t.Fatalf("parse: %v", err)
			}

			reg, err := engine.DefaultRegistry(native.Register)
			if err != nil {
				t.Fatalf("DefaultRegistry: %v", err)
			}
			out, runErr := engine.NewTop(reg).Run(values)

			if strings.HasPrefix(expected, "ERROR:") {
				want := expected[len("ERROR:"):]
				if runErr == nil {
					t.Fatalf("expected error containing %q, got result %v", want, renderSpecStack(out))
				}
				if want != "" && !strings.Contains(runErr.Error(), want) {
					t.Errorf("error %q does not contain %q", runErr.Error(), want)
				}
				return
			}

			if runErr != nil {
				t.Fatalf("unexpected error: %v", runErr)
			}
			got := renderSpecStack(out)
			if got != expected {
				t.Errorf("got %q, want %q", got, expected)
			}
		})
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("scanner error in %s: %v", path, err)
	}
}

// TestSpecProd runs the .tsv spec files under lang/test/spec/ against
// a production-aql registry (engine.DefaultRegistry + native.Register).
// These specs cover the production language layer — words and types
// that aren't part of the eng kernel (record, object, make, get/set
// on Stores, Resource / Entity, …).
func TestSpecProd(t *testing.T) {
	specDir := "spec"
	entries, err := os.ReadDir(specDir)
	if err != nil {
		t.Fatalf("read %s: %v", specDir, err)
	}
	ran := 0
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".tsv") {
			continue
		}
		ran++
		t.Run(strings.TrimSuffix(e.Name(), ".tsv"), func(t *testing.T) {
			runProdSpecFile(t, filepath.Join(specDir, e.Name()))
		})
	}
	if ran == 0 {
		t.Errorf("no .tsv specs found under %s", specDir)
	}
}
