package test

// Spec-runner test for files under aql/test/spec/. Each TSV row is
// parsed with the production aql parser (parser.Parse) and run
// against a fresh aqleng.Registry pre-populated with aqleng's core
// words plus a fixed set of spec-runner test fixtures (q-suffixed).
//
// Architecture: the spec runner LIVES in aql/test because that's
// where the production parser is reachable, but it TESTS the aqleng
// engine kernel — only aqleng.RegisterCoreWords is installed, plus
// the q-suffixed fixtures that exercise specific dispatch paths.
// Production native words (add, upper, etc.) are intentionally NOT
// installed; the q-fixtures cover the same ground in spec-stable
// minimal forms.
//
// The "q" suffix on most fixtures marks them as SPEC-RUNNER FIXTURES,
// distinct from production AQL words of the same root name. Language-
// fundamental keywords (def, fn, quote, args, type, untype, typeof,
// is, none, end, …) keep their bare names because what's being tested
// IS the keyword itself, not a fixture for it.

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/metsitaba/voxgig-exp/aql/internal/engine"
	"github.com/metsitaba/voxgig-exp/aql/internal/native"
	"github.com/metsitaba/voxgig-exp/aql/internal/parser"
	"github.com/metsitaba/voxgig-exp/aqleng"
)

// specReplayCounter is bumped per call to the `replayq` test fixture
// so each Mark/Move pair gets a unique ID across a spec file.
var specReplayCounter int

// registerSpecWords installs the aqleng core words plus the spec-
// runner test fixtures on a registry. The fixtures are minimal,
// single-overload variants tailored for spec coverage of the
// dispatch / value / type-lattice core.
func registerSpecWords(r *aqleng.Registry) {
	aqleng.RegisterCoreWords(r)

	toFloat := func(v aqleng.Value) float64 {
		if v.VType.Matches(aqleng.TInteger) {
			n, _ := v.AsInteger()
			return float64(n)
		}
		f, _ := v.AsDecimal()
		return f
	}
	numericBinary := func(intOp func(a, b int64) int64, floatOp func(a, b float64) float64) aqleng.Handler {
		return func(args []aqleng.Value, _ map[string]aqleng.Value, _ []aqleng.Value, _ *aqleng.Registry) ([]aqleng.Value, error) {
			if args[0].VType.Matches(aqleng.TInteger) && args[1].VType.Matches(aqleng.TInteger) {
				a, _ := args[0].AsInteger()
				b, _ := args[1].AsInteger()
				return []aqleng.Value{aqleng.NewInteger(intOp(a, b))}, nil
			}
			return []aqleng.Value{aqleng.NewDecimal(floatOp(toFloat(args[0]), toFloat(args[1])))}, nil
		}
	}
	numberPair := []aqleng.Type{aqleng.TNumber, aqleng.TNumber}

	r.RegisterNativeFunc(aqleng.NativeFunc{
		Name: "addq", ForwardPrecedence: true,
		Signatures: []aqleng.NativeSig{{
			Args:    numberPair,
			Handler: numericBinary(func(a, b int64) int64 { return b + a }, func(a, b float64) float64 { return b + a }),
			Returns: []aqleng.Type{aqleng.TNumber},
		}},
	})
	r.RegisterNativeFunc(aqleng.NativeFunc{
		Name: "subq", ForwardPrecedence: true,
		Signatures: []aqleng.NativeSig{{
			Args:    numberPair,
			Handler: numericBinary(func(a, b int64) int64 { return b - a }, func(a, b float64) float64 { return b - a }),
			Returns: []aqleng.Type{aqleng.TNumber},
		}},
	})
	r.RegisterNativeFunc(aqleng.NativeFunc{
		Name: "mulq", ForwardPrecedence: true,
		Signatures: []aqleng.NativeSig{{
			Args:    numberPair,
			Handler: numericBinary(func(a, b int64) int64 { return b * a }, func(a, b float64) float64 { return b * a }),
			Returns: []aqleng.Type{aqleng.TNumber},
		}},
	})
	r.RegisterNativeFunc(aqleng.NativeFunc{
		Name: "negq", ForwardPrecedence: true,
		Signatures: []aqleng.NativeSig{{
			Args: []aqleng.Type{aqleng.TNumber}, BarrierPos: 1,
			Handler: func(args []aqleng.Value, _ map[string]aqleng.Value, _ []aqleng.Value, _ *aqleng.Registry) ([]aqleng.Value, error) {
				if args[0].VType.Matches(aqleng.TInteger) {
					n, _ := args[0].AsInteger()
					return []aqleng.Value{aqleng.NewInteger(-n)}, nil
				}
				f, _ := args[0].AsDecimal()
				return []aqleng.Value{aqleng.NewDecimal(-f)}, nil
			},
			Returns: []aqleng.Type{aqleng.TNumber},
		}},
	})
	r.RegisterNativeFunc(aqleng.NativeFunc{
		Name: "concatq", ForwardPrecedence: true,
		Signatures: []aqleng.NativeSig{{
			Args: []aqleng.Type{aqleng.TString, aqleng.TString},
			Handler: func(args []aqleng.Value, _ map[string]aqleng.Value, _ []aqleng.Value, _ *aqleng.Registry) ([]aqleng.Value, error) {
				a, _ := args[0].AsString()
				b, _ := args[1].AsString()
				return []aqleng.Value{aqleng.NewString(b + a)}, nil
			},
			Returns: []aqleng.Type{aqleng.TString},
		}},
	})
	r.RegisterNativeFunc(aqleng.NativeFunc{
		Name: "describeq", ForwardPrecedence: true,
		Signatures: []aqleng.NativeSig{
			{
				Args: []aqleng.Type{aqleng.TInteger},
				Handler: func(args []aqleng.Value, _ map[string]aqleng.Value, _ []aqleng.Value, _ *aqleng.Registry) ([]aqleng.Value, error) {
					n, _ := args[0].AsInteger()
					return []aqleng.Value{aqleng.NewString("int:" + strconv.FormatInt(n, 10))}, nil
				},
				Returns: []aqleng.Type{aqleng.TString},
			},
			{
				Args: []aqleng.Type{aqleng.TString},
				Handler: func(args []aqleng.Value, _ map[string]aqleng.Value, _ []aqleng.Value, _ *aqleng.Registry) ([]aqleng.Value, error) {
					s, _ := args[0].AsString()
					return []aqleng.Value{aqleng.NewString("str:" + s)}, nil
				},
				Returns: []aqleng.Type{aqleng.TString},
			},
		},
	})
	r.RegisterNativeFunc(aqleng.NativeFunc{
		Name: "tagq", ForwardPrecedence: true,
		Signatures: []aqleng.NativeSig{
			{Args: []aqleng.Type{aqleng.TAny}, Handler: func(_ []aqleng.Value, _ map[string]aqleng.Value, _ []aqleng.Value, _ *aqleng.Registry) ([]aqleng.Value, error) {
				return []aqleng.Value{aqleng.NewString("any")}, nil
			}, Returns: []aqleng.Type{aqleng.TString}},
			{Args: []aqleng.Type{aqleng.TInteger}, Handler: func(_ []aqleng.Value, _ map[string]aqleng.Value, _ []aqleng.Value, _ *aqleng.Registry) ([]aqleng.Value, error) {
				return []aqleng.Value{aqleng.NewString("specific")}, nil
			}, Returns: []aqleng.Type{aqleng.TString}},
		},
	})
	r.RegisterNativeFunc(aqleng.NativeFunc{
		Name: "factq", ForwardPrecedence: true,
		Signatures: []aqleng.NativeSig{
			{
				Args: []aqleng.Type{aqleng.TInteger}, Patterns: map[int]aqleng.Value{0: aqleng.NewInteger(0)},
				Handler: func(_ []aqleng.Value, _ map[string]aqleng.Value, _ []aqleng.Value, _ *aqleng.Registry) ([]aqleng.Value, error) {
					return []aqleng.Value{aqleng.NewInteger(1)}, nil
				},
				Returns: []aqleng.Type{aqleng.TInteger},
			},
			{
				Args: []aqleng.Type{aqleng.TInteger},
				Handler: func(args []aqleng.Value, _ map[string]aqleng.Value, _ []aqleng.Value, _ *aqleng.Registry) ([]aqleng.Value, error) {
					n, _ := args[0].AsInteger()
					return []aqleng.Value{aqleng.NewInteger(n)}, nil
				},
				Returns: []aqleng.Type{aqleng.TInteger},
			},
		},
	})
	r.RegisterNativeFunc(aqleng.NativeFunc{
		Name: "codeq", ForwardPrecedence: true,
		Signatures: []aqleng.NativeSig{
			{
				Args: []aqleng.Type{aqleng.TInteger}, Patterns: map[int]aqleng.Value{0: aqleng.NewInteger(99)},
				Handler: func(_ []aqleng.Value, _ map[string]aqleng.Value, _ []aqleng.Value, _ *aqleng.Registry) ([]aqleng.Value, error) {
					return []aqleng.Value{aqleng.NewString("ninety-nine")}, nil
				},
				Returns: []aqleng.Type{aqleng.TString},
			},
			{
				Args: []aqleng.Type{aqleng.TInteger},
				Handler: func(_ []aqleng.Value, _ map[string]aqleng.Value, _ []aqleng.Value, _ *aqleng.Registry) ([]aqleng.Value, error) {
					return []aqleng.Value{aqleng.NewString("general")}, nil
				},
				Returns: []aqleng.Type{aqleng.TString},
			},
		},
	})
	r.RegisterNativeFunc(aqleng.NativeFunc{
		Name: "routeq", ForwardPrecedence: true,
		Signatures: []aqleng.NativeSig{
			{
				Args: []aqleng.Type{aqleng.TString}, Patterns: map[int]aqleng.Value{0: aqleng.NewString("admin")},
				Handler: func(_ []aqleng.Value, _ map[string]aqleng.Value, _ []aqleng.Value, _ *aqleng.Registry) ([]aqleng.Value, error) {
					return []aqleng.Value{aqleng.NewString("matched-admin")}, nil
				},
				Returns: []aqleng.Type{aqleng.TString},
			},
			{
				Args: []aqleng.Type{aqleng.TString},
				Handler: func(_ []aqleng.Value, _ map[string]aqleng.Value, _ []aqleng.Value, _ *aqleng.Registry) ([]aqleng.Value, error) {
					return []aqleng.Value{aqleng.NewString("other")}, nil
				},
				Returns: []aqleng.Type{aqleng.TString},
			},
		},
	})
	r.RegisterNativeFunc(aqleng.NativeFunc{
		Name: "tripq", ForwardPrecedence: true,
		Signatures: []aqleng.NativeSig{{
			Args: []aqleng.Type{aqleng.TInteger, aqleng.TInteger, aqleng.TInteger},
			Handler: func(args []aqleng.Value, _ map[string]aqleng.Value, _ []aqleng.Value, _ *aqleng.Registry) ([]aqleng.Value, error) {
				a, _ := args[0].AsInteger()
				b, _ := args[1].AsInteger()
				c, _ := args[2].AsInteger()
				return []aqleng.Value{aqleng.NewString(fmt.Sprintf("%d,%d,%d", a, b, c))}, nil
			},
			Returns: []aqleng.Type{aqleng.TString},
		}},
	})
	r.RegisterNativeFunc(aqleng.NativeFunc{
		Name: "pairq", ForwardPrecedence: true,
		Signatures: []aqleng.NativeSig{{
			Args:       []aqleng.Type{aqleng.TInteger, aqleng.TInteger},
			BarrierPos: 1,
			Handler: func(args []aqleng.Value, _ map[string]aqleng.Value, _ []aqleng.Value, _ *aqleng.Registry) ([]aqleng.Value, error) {
				a, _ := args[0].AsInteger()
				b, _ := args[1].AsInteger()
				return []aqleng.Value{aqleng.NewString(fmt.Sprintf("%d:%d", a, b))}, nil
			},
			Returns: []aqleng.Type{aqleng.TString},
		}},
	})
	r.RegisterNativeFunc(aqleng.NativeFunc{
		Name: "lengthq", ForwardPrecedence: true,
		Signatures: []aqleng.NativeSig{{
			Args: []aqleng.Type{aqleng.TList},
			Handler: func(args []aqleng.Value, _ map[string]aqleng.Value, _ []aqleng.Value, _ *aqleng.Registry) ([]aqleng.Value, error) {
				lst := args[0].AsList()
				return []aqleng.Value{aqleng.NewInteger(int64(lst.Len()))}, nil
			},
			Returns: []aqleng.Type{aqleng.TInteger},
		}},
	})
	r.RegisterNativeFunc(aqleng.NativeFunc{
		Name: "firstq", ForwardPrecedence: true,
		Signatures: []aqleng.NativeSig{{
			Args: []aqleng.Type{aqleng.TList},
			Handler: func(args []aqleng.Value, _ map[string]aqleng.Value, _ []aqleng.Value, _ *aqleng.Registry) ([]aqleng.Value, error) {
				lst := args[0].AsList()
				if lst.Len() == 0 {
					return []aqleng.Value{aqleng.NewNone()}, nil
				}
				return []aqleng.Value{lst.Get(0)}, nil
			},
			Returns: []aqleng.Type{aqleng.TAny},
		}},
	})
	r.RegisterNativeFunc(aqleng.NativeFunc{
		Name: "replayq", ForwardPrecedence: true,
		Signatures: []aqleng.NativeSig{{
			Args:       []aqleng.Type{aqleng.TList},
			NoEvalArgs: map[int]bool{0: true},
			Handler: func(args []aqleng.Value, _ map[string]aqleng.Value, _ []aqleng.Value, _ *aqleng.Registry) ([]aqleng.Value, error) {
				body := args[0].AsList().Slice()
				specReplayCounter++
				id := fmt.Sprintf("__replayq_%d", specReplayCounter)
				out := make([]aqleng.Value, 0, len(body)+2)
				out = append(out, aqleng.NewMark(id, body...))
				out = append(out, body...)
				out = append(out, aqleng.NewMove(id, "replayq"))
				return out, nil
			},
		}},
	})

	r.PushDef("pi", aqleng.NewInteger(3))
	r.PushDef("tau", aqleng.NewInteger(6))
	r.PushDef("greeting", aqleng.NewString("hello"))
}

// renderSpecValue renders a value in the spec format. The spec format
// diverges from Value.String for clarity in expected columns: strings
// double-quoted, atoms as `atom(name)`, lists as space-separated
// `[a b c]`, maps as `{k:v k:v}`, type literals as their leaf, and
// `none` lowercase.
func renderSpecValue(v aqleng.Value) string {
	switch {
	case v.IsNone():
		return "none"
	case v.Data == nil:
		if name := aqleng.TypeNameByID(v.VType.ID); name != "" {
			return name
		}
		return v.VType.Leaf()
	case v.VType.Matches(aqleng.TInteger):
		n, _ := v.AsInteger()
		return strconv.FormatInt(n, 10)
	case v.VType.Matches(aqleng.TDecimal):
		f, _ := v.AsDecimal()
		return aqleng.FormatDecimal(f)
	case v.VType.Matches(aqleng.TString):
		s, _ := v.AsString()
		return "\"" + s + "\""
	case v.VType.Matches(aqleng.TBoolean):
		b, _ := v.AsBoolean()
		if b {
			return "true"
		}
		return "false"
	case v.VType.Equal(aqleng.TAtom) && v.Data != nil:
		s, _ := v.AsAtom()
		return "atom(" + s + ")"
	case v.VType.Matches(aqleng.TList) && v.Data != nil:
		lst := v.AsList()
		parts := make([]string, lst.Len())
		for i := 0; i < lst.Len(); i++ {
			parts[i] = renderSpecValue(lst.Get(i))
		}
		return "[" + strings.Join(parts, " ") + "]"
	case v.VType.Equal(aqleng.TMap) && v.Data != nil:
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

func renderSpecStack(stack []aqleng.Value) string {
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

// specRegistryBuilder constructs a fresh registry + ready-to-run
// engine for one spec row. Different runners use different builders:
// the aqleng-only spec uses registerSpecWords; the production spec
// uses engine.DefaultRegistry+native.Register.
type specRegistryBuilder func(t *testing.T) (engine.Value, func(values []engine.Value) ([]engine.Value, error))

func aqlengSpecBuilder(t *testing.T) (engine.Value, func(values []engine.Value) ([]engine.Value, error)) {
	t.Helper()
	r, err := aqleng.NewRegistry()
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	registerSpecWords(r)
	r.InitRootContext()
	return engine.Value{}, func(values []engine.Value) ([]engine.Value, error) {
		return aqleng.NewTop(r).Run(values)
	}
}

func prodSpecBuilder(t *testing.T) (engine.Value, func(values []engine.Value) ([]engine.Value, error)) {
	t.Helper()
	reg, err := engine.DefaultRegistry(native.Register)
	if err != nil {
		t.Fatalf("DefaultRegistry: %v", err)
	}
	return engine.Value{}, func(values []engine.Value) ([]engine.Value, error) {
		return engine.NewTop(reg).Run(values)
	}
}

func runSpecFile(t *testing.T, path string) {
	runSpecFileWith(t, path, aqlengSpecBuilder)
}

func runSpecFileWith(t *testing.T, path string, build specRegistryBuilder) {
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

			_, run := build(t)
			out, runErr := run(values)

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

func TestSpec(t *testing.T) {
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
			runSpecFile(t, filepath.Join(specDir, e.Name()))
		})
	}
	if ran == 0 {
		t.Errorf("no .tsv specs found under %s", specDir)
	}
}

// TestSpecProd runs spec/.tsv files under spec/prod/ against a
// production-aql registry (engine.DefaultRegistry + native.Register),
// rather than the aqleng-only kernel that TestSpec uses. These specs
// can exercise any registered word — record / object / make / get /
// length / etc. — and the builtin Resource / Entity types installed
// by installResourceTypes.
func TestSpecProd(t *testing.T) {
	specDir := filepath.Join("spec", "prod")
	entries, err := os.ReadDir(specDir)
	if err != nil {
		if os.IsNotExist(err) {
			return
		}
		t.Fatalf("read %s: %v", specDir, err)
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".tsv") {
			continue
		}
		t.Run(strings.TrimSuffix(e.Name(), ".tsv"), func(t *testing.T) {
			runSpecFileWith(t, filepath.Join(specDir, e.Name()), prodSpecBuilder)
		})
	}
}
