package test

// Spec-runner test for files under lang/test/spec/. Each TSV row is
// parsed with the production aql parser (parser.Parse) and run
// against a fresh eng.Registry pre-populated with aqleng's core
// words plus a fixed set of spec-runner test fixtures (q-suffixed).
//
// Architecture: the spec runner LIVES in aql/test because that's
// where the production parser is reachable, but it TESTS the aqleng
// engine kernel — only eng.RegisterCoreWords is installed, plus
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

	"github.com/metsitaba/voxgig-exp/lang/internal/engine"
	"github.com/metsitaba/voxgig-exp/lang/internal/native"
	"github.com/metsitaba/voxgig-exp/lang/internal/parser"
	"github.com/metsitaba/voxgig-exp/eng"
)

// specReplayCounter is bumped per call to the `replayq` test fixture
// so each Mark/Move pair gets a unique ID across a spec file.
var specReplayCounter int

// registerSpecWords installs the aqleng core words plus the spec-
// runner test fixtures on a registry. The fixtures are minimal,
// single-overload variants tailored for spec coverage of the
// dispatch / value / type-lattice core.
func registerSpecWords(r *eng.Registry) {
	eng.RegisterCoreWords(r)

	toFloat := func(v eng.Value) float64 {
		if v.VType.Matches(eng.TInteger) {
			n, _ := v.AsInteger()
			return float64(n)
		}
		f, _ := v.AsDecimal()
		return f
	}
	numericBinary := func(intOp func(a, b int64) int64, floatOp func(a, b float64) float64) eng.Handler {
		return func(args []eng.Value, _ map[string]eng.Value, _ []eng.Value, _ *eng.Registry) ([]eng.Value, error) {
			if args[0].VType.Matches(eng.TInteger) && args[1].VType.Matches(eng.TInteger) {
				a, _ := args[0].AsInteger()
				b, _ := args[1].AsInteger()
				return []eng.Value{eng.NewInteger(intOp(a, b))}, nil
			}
			return []eng.Value{eng.NewDecimal(floatOp(toFloat(args[0]), toFloat(args[1])))}, nil
		}
	}
	numberPair := []eng.Type{eng.TNumber, eng.TNumber}

	r.RegisterNativeFunc(eng.NativeFunc{
		Name: "addq", ForwardPrecedence: true,
		Signatures: []eng.NativeSig{{
			Args:    numberPair,
			Handler: numericBinary(func(a, b int64) int64 { return b + a }, func(a, b float64) float64 { return b + a }),
			Returns: []eng.Type{eng.TNumber},
		}},
	})
	r.RegisterNativeFunc(eng.NativeFunc{
		Name: "subq", ForwardPrecedence: true,
		Signatures: []eng.NativeSig{{
			Args:    numberPair,
			Handler: numericBinary(func(a, b int64) int64 { return b - a }, func(a, b float64) float64 { return b - a }),
			Returns: []eng.Type{eng.TNumber},
		}},
	})
	r.RegisterNativeFunc(eng.NativeFunc{
		Name: "mulq", ForwardPrecedence: true,
		Signatures: []eng.NativeSig{{
			Args:    numberPair,
			Handler: numericBinary(func(a, b int64) int64 { return b * a }, func(a, b float64) float64 { return b * a }),
			Returns: []eng.Type{eng.TNumber},
		}},
	})
	r.RegisterNativeFunc(eng.NativeFunc{
		Name: "negq", ForwardPrecedence: true,
		Signatures: []eng.NativeSig{{
			Args: []eng.Type{eng.TNumber}, BarrierPos: 1,
			Handler: func(args []eng.Value, _ map[string]eng.Value, _ []eng.Value, _ *eng.Registry) ([]eng.Value, error) {
				if args[0].VType.Matches(eng.TInteger) {
					n, _ := args[0].AsInteger()
					return []eng.Value{eng.NewInteger(-n)}, nil
				}
				f, _ := args[0].AsDecimal()
				return []eng.Value{eng.NewDecimal(-f)}, nil
			},
			Returns: []eng.Type{eng.TNumber},
		}},
	})
	r.RegisterNativeFunc(eng.NativeFunc{
		Name: "concatq", ForwardPrecedence: true,
		Signatures: []eng.NativeSig{{
			Args: []eng.Type{eng.TString, eng.TString},
			Handler: func(args []eng.Value, _ map[string]eng.Value, _ []eng.Value, _ *eng.Registry) ([]eng.Value, error) {
				a, _ := args[0].AsString()
				b, _ := args[1].AsString()
				return []eng.Value{eng.NewString(b + a)}, nil
			},
			Returns: []eng.Type{eng.TString},
		}},
	})
	r.RegisterNativeFunc(eng.NativeFunc{
		Name: "describeq", ForwardPrecedence: true,
		Signatures: []eng.NativeSig{
			{
				Args: []eng.Type{eng.TInteger},
				Handler: func(args []eng.Value, _ map[string]eng.Value, _ []eng.Value, _ *eng.Registry) ([]eng.Value, error) {
					n, _ := args[0].AsInteger()
					return []eng.Value{eng.NewString("int:" + strconv.FormatInt(n, 10))}, nil
				},
				Returns: []eng.Type{eng.TString},
			},
			{
				Args: []eng.Type{eng.TString},
				Handler: func(args []eng.Value, _ map[string]eng.Value, _ []eng.Value, _ *eng.Registry) ([]eng.Value, error) {
					s, _ := args[0].AsString()
					return []eng.Value{eng.NewString("str:" + s)}, nil
				},
				Returns: []eng.Type{eng.TString},
			},
		},
	})
	r.RegisterNativeFunc(eng.NativeFunc{
		Name: "tagq", ForwardPrecedence: true,
		Signatures: []eng.NativeSig{
			{Args: []eng.Type{eng.TAny}, Handler: func(_ []eng.Value, _ map[string]eng.Value, _ []eng.Value, _ *eng.Registry) ([]eng.Value, error) {
				return []eng.Value{eng.NewString("any")}, nil
			}, Returns: []eng.Type{eng.TString}},
			{Args: []eng.Type{eng.TInteger}, Handler: func(_ []eng.Value, _ map[string]eng.Value, _ []eng.Value, _ *eng.Registry) ([]eng.Value, error) {
				return []eng.Value{eng.NewString("specific")}, nil
			}, Returns: []eng.Type{eng.TString}},
		},
	})
	r.RegisterNativeFunc(eng.NativeFunc{
		Name: "factq", ForwardPrecedence: true,
		Signatures: []eng.NativeSig{
			{
				Args: []eng.Type{eng.TInteger}, Patterns: map[int]eng.Value{0: eng.NewInteger(0)},
				Handler: func(_ []eng.Value, _ map[string]eng.Value, _ []eng.Value, _ *eng.Registry) ([]eng.Value, error) {
					return []eng.Value{eng.NewInteger(1)}, nil
				},
				Returns: []eng.Type{eng.TInteger},
			},
			{
				Args: []eng.Type{eng.TInteger},
				Handler: func(args []eng.Value, _ map[string]eng.Value, _ []eng.Value, _ *eng.Registry) ([]eng.Value, error) {
					n, _ := args[0].AsInteger()
					return []eng.Value{eng.NewInteger(n)}, nil
				},
				Returns: []eng.Type{eng.TInteger},
			},
		},
	})
	r.RegisterNativeFunc(eng.NativeFunc{
		Name: "codeq", ForwardPrecedence: true,
		Signatures: []eng.NativeSig{
			{
				Args: []eng.Type{eng.TInteger}, Patterns: map[int]eng.Value{0: eng.NewInteger(99)},
				Handler: func(_ []eng.Value, _ map[string]eng.Value, _ []eng.Value, _ *eng.Registry) ([]eng.Value, error) {
					return []eng.Value{eng.NewString("ninety-nine")}, nil
				},
				Returns: []eng.Type{eng.TString},
			},
			{
				Args: []eng.Type{eng.TInteger},
				Handler: func(_ []eng.Value, _ map[string]eng.Value, _ []eng.Value, _ *eng.Registry) ([]eng.Value, error) {
					return []eng.Value{eng.NewString("general")}, nil
				},
				Returns: []eng.Type{eng.TString},
			},
		},
	})
	r.RegisterNativeFunc(eng.NativeFunc{
		Name: "routeq", ForwardPrecedence: true,
		Signatures: []eng.NativeSig{
			{
				Args: []eng.Type{eng.TString}, Patterns: map[int]eng.Value{0: eng.NewString("admin")},
				Handler: func(_ []eng.Value, _ map[string]eng.Value, _ []eng.Value, _ *eng.Registry) ([]eng.Value, error) {
					return []eng.Value{eng.NewString("matched-admin")}, nil
				},
				Returns: []eng.Type{eng.TString},
			},
			{
				Args: []eng.Type{eng.TString},
				Handler: func(_ []eng.Value, _ map[string]eng.Value, _ []eng.Value, _ *eng.Registry) ([]eng.Value, error) {
					return []eng.Value{eng.NewString("other")}, nil
				},
				Returns: []eng.Type{eng.TString},
			},
		},
	})
	r.RegisterNativeFunc(eng.NativeFunc{
		Name: "tripq", ForwardPrecedence: true,
		Signatures: []eng.NativeSig{{
			Args: []eng.Type{eng.TInteger, eng.TInteger, eng.TInteger},
			Handler: func(args []eng.Value, _ map[string]eng.Value, _ []eng.Value, _ *eng.Registry) ([]eng.Value, error) {
				a, _ := args[0].AsInteger()
				b, _ := args[1].AsInteger()
				c, _ := args[2].AsInteger()
				return []eng.Value{eng.NewString(fmt.Sprintf("%d,%d,%d", a, b, c))}, nil
			},
			Returns: []eng.Type{eng.TString},
		}},
	})
	r.RegisterNativeFunc(eng.NativeFunc{
		Name: "pairq", ForwardPrecedence: true,
		Signatures: []eng.NativeSig{{
			Args:       []eng.Type{eng.TInteger, eng.TInteger},
			BarrierPos: 1,
			Handler: func(args []eng.Value, _ map[string]eng.Value, _ []eng.Value, _ *eng.Registry) ([]eng.Value, error) {
				a, _ := args[0].AsInteger()
				b, _ := args[1].AsInteger()
				return []eng.Value{eng.NewString(fmt.Sprintf("%d:%d", a, b))}, nil
			},
			Returns: []eng.Type{eng.TString},
		}},
	})
	r.RegisterNativeFunc(eng.NativeFunc{
		Name: "lengthq", ForwardPrecedence: true,
		Signatures: []eng.NativeSig{{
			Args: []eng.Type{eng.TList},
			Handler: func(args []eng.Value, _ map[string]eng.Value, _ []eng.Value, _ *eng.Registry) ([]eng.Value, error) {
				lst := args[0].AsList()
				return []eng.Value{eng.NewInteger(int64(lst.Len()))}, nil
			},
			Returns: []eng.Type{eng.TInteger},
		}},
	})
	r.RegisterNativeFunc(eng.NativeFunc{
		Name: "firstq", ForwardPrecedence: true,
		Signatures: []eng.NativeSig{{
			Args: []eng.Type{eng.TList},
			Handler: func(args []eng.Value, _ map[string]eng.Value, _ []eng.Value, _ *eng.Registry) ([]eng.Value, error) {
				lst := args[0].AsList()
				if lst.Len() == 0 {
					return []eng.Value{eng.NewNone()}, nil
				}
				return []eng.Value{lst.Get(0)}, nil
			},
			Returns: []eng.Type{eng.TAny},
		}},
	})
	r.RegisterNativeFunc(eng.NativeFunc{
		Name: "replayq", ForwardPrecedence: true,
		Signatures: []eng.NativeSig{{
			Args:       []eng.Type{eng.TList},
			NoEvalArgs: map[int]bool{0: true},
			Handler: func(args []eng.Value, _ map[string]eng.Value, _ []eng.Value, _ *eng.Registry) ([]eng.Value, error) {
				body := args[0].AsList().Slice()
				specReplayCounter++
				id := fmt.Sprintf("__replayq_%d", specReplayCounter)
				out := make([]eng.Value, 0, len(body)+2)
				out = append(out, eng.NewMark(id, body...))
				out = append(out, body...)
				out = append(out, eng.NewMove(id, "replayq"))
				return out, nil
			},
		}},
	})

	r.PushDef("pi", eng.NewInteger(3))
	r.PushDef("tau", eng.NewInteger(6))
	r.PushDef("greeting", eng.NewString("hello"))
}

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

// specRegistryBuilder constructs a fresh registry + ready-to-run
// engine for one spec row. Different runners use different builders:
// the aqleng-only spec uses registerSpecWords; the production spec
// uses engine.DefaultRegistry+native.Register.
type specRegistryBuilder func(t *testing.T) (engine.Value, func(values []engine.Value) ([]engine.Value, error))

func aqlengSpecBuilder(t *testing.T) (engine.Value, func(values []engine.Value) ([]engine.Value, error)) {
	t.Helper()
	r, err := eng.NewRegistry()
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	registerSpecWords(r)
	r.InitRootContext()
	return engine.Value{}, func(values []engine.Value) ([]engine.Value, error) {
		return eng.NewTop(r).Run(values)
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
