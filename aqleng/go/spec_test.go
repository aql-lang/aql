package aqleng

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// Spec-driven tests. Each TSV file under ../test/spec/ describes a
// table of (input tokens, expected stack) cases that the runner
// converts into engine inputs and runs through a fresh registry
// pre-populated with a fixed set of test words. No parser is involved;
// the tokenizer in this file is intentionally minimal.

// registerSpecWords installs the word set the spec files reference.
// Keep this list small and stable — the specs are easier to read
// when there's no surprise about what each word does.
func registerSpecWords(r *Registry) {
	// add, sub, mul: integer arithmetic via forward precedence.
	r.RegisterNativeFunc(NativeFunc{
		Name:              "add",
		ForwardPrecedence: true,
		Signatures: []NativeSig{{
			Args: []Type{TInteger, TInteger},
			Handler: func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
				a, _ := args[0].AsInteger()
				b, _ := args[1].AsInteger()
				return []Value{NewInteger(a + b)}, nil
			},
			Returns: []Type{TInteger},
		}},
	})
	r.RegisterNativeFunc(NativeFunc{
		Name:              "sub",
		ForwardPrecedence: true,
		Signatures: []NativeSig{{
			Args: []Type{TInteger, TInteger},
			Handler: func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
				a, _ := args[0].AsInteger()
				b, _ := args[1].AsInteger()
				return []Value{NewInteger(a - b)}, nil
			},
			Returns: []Type{TInteger},
		}},
	})
	r.RegisterNativeFunc(NativeFunc{
		Name:              "mul",
		ForwardPrecedence: true,
		Signatures: []NativeSig{{
			Args: []Type{TInteger, TInteger},
			Handler: func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
				a, _ := args[0].AsInteger()
				b, _ := args[1].AsInteger()
				return []Value{NewInteger(a * b)}, nil
			},
			Returns: []Type{TInteger},
		}},
	})

	// neg: stack-only unary integer negation.
	r.RegisterNativeFunc(NativeFunc{
		Name: "neg",
		Signatures: []NativeSig{{
			Args: []Type{TInteger},
			Handler: func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
				n, _ := args[0].AsInteger()
				return []Value{NewInteger(-n)}, nil
			},
			Returns: []Type{TInteger},
		}},
	})

	// dup, swap, drop: stack-only ops.
	r.RegisterNativeFunc(NativeFunc{
		Name: "dup",
		Signatures: []NativeSig{{
			Args: []Type{TAny},
			Handler: func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
				return []Value{args[0], args[0]}, nil
			},
		}},
	})
	r.RegisterNativeFunc(NativeFunc{
		Name: "swap",
		Signatures: []NativeSig{{
			Args: []Type{TAny, TAny},
			// Under the unified §1.4 dispatch rule, args[0] is the
			// top of the stack and args[1] is the next-deeper. The
			// splice writes the handler's return slice in source
			// order, so emitting [args[0], args[1]] places the old
			// top at the deeper position and the old next-deeper at
			// the top — i.e. the two values are swapped on the stack.
			Handler: func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
				return []Value{args[0], args[1]}, nil
			},
		}},
	})
	r.RegisterNativeFunc(NativeFunc{
		Name: "drop",
		Signatures: []NativeSig{{
			Args: []Type{TAny},
			Handler: func(_ []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
				return nil, nil
			},
		}},
	})

	// concat: string concatenation, forward precedence.
	r.RegisterNativeFunc(NativeFunc{
		Name:              "concat",
		ForwardPrecedence: true,
		Signatures: []NativeSig{{
			Args: []Type{TString, TString},
			Handler: func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
				a, _ := args[0].AsString()
				b, _ := args[1].AsString()
				return []Value{NewString(a + b)}, nil
			},
			Returns: []Type{TString},
		}},
	})

	// not: boolean negation, forward precedence.
	r.RegisterNativeFunc(NativeFunc{
		Name:              "not",
		ForwardPrecedence: true,
		Signatures: []NativeSig{{
			Args: []Type{TBoolean},
			Handler: func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
				b, _ := args[0].AsBoolean()
				return []Value{NewBoolean(!b)}, nil
			},
			Returns: []Type{TBoolean},
		}},
	})

	// describe: two type-specific overloads, used in the dispatch spec.
	r.RegisterNativeFunc(NativeFunc{
		Name:              "describe",
		ForwardPrecedence: true,
		Signatures: []NativeSig{
			{
				Args: []Type{TInteger},
				Handler: func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
					n, _ := args[0].AsInteger()
					return []Value{NewString("int:" + strconv.FormatInt(n, 10))}, nil
				},
				Returns: []Type{TString},
			},
			{
				Args: []Type{TString},
				Handler: func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
					s, _ := args[0].AsString()
					return []Value{NewString("str:" + s)}, nil
				},
				Returns: []Type{TString},
			},
		},
	})

	// tag: Any vs Integer overloads — exercises specificity scoring.
	r.RegisterNativeFunc(NativeFunc{
		Name:              "tag",
		ForwardPrecedence: true,
		Signatures: []NativeSig{
			{
				Args: []Type{TAny},
				Handler: func(_ []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
					return []Value{NewString("any")}, nil
				},
				Returns: []Type{TString},
			},
			{
				Args: []Type{TInteger},
				Handler: func(_ []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
					return []Value{NewString("specific")}, nil
				},
				Returns: []Type{TString},
			},
		},
	})
}

// tokenizeSpec converts a single space-separated input string from a
// TSV row into a slice of engine values. It is intentionally minimal:
// integers, decimals, "quoted" strings, true/false/null, and bare
// identifiers (treated as words).
func tokenizeSpec(s string) ([]Value, error) {
	var out []Value
	i := 0
	for i < len(s) {
		// Skip whitespace.
		for i < len(s) && (s[i] == ' ' || s[i] == '\t') {
			i++
		}
		if i >= len(s) {
			break
		}

		// Quoted string.
		if s[i] == '"' {
			j := i + 1
			for j < len(s) && s[j] != '"' {
				j++
			}
			if j >= len(s) {
				return nil, fmt.Errorf("unterminated string at %d", i)
			}
			out = append(out, NewString(s[i+1:j]))
			i = j + 1
			continue
		}

		// Unquoted token.
		j := i
		for j < len(s) && s[j] != ' ' && s[j] != '\t' {
			j++
		}
		tok := s[i:j]
		i = j

		switch tok {
		case "true":
			out = append(out, NewBoolean(true))
		case "false":
			out = append(out, NewBoolean(false))
		case "null":
			out = append(out, NewTypeLiteral(TNone))
		default:
			if n, err := strconv.ParseInt(tok, 10, 64); err == nil {
				out = append(out, NewInteger(n))
			} else if f, err := strconv.ParseFloat(tok, 64); err == nil {
				out = append(out, NewDecimal(f))
			} else {
				// /s and /f trailing modifiers — at the call site, force
				// stack-only or forward-only dispatch regardless of the
				// sig's declared BarrierPos. Mirrors the lexer's
				// handling of these modifiers in the full parser.
				name := tok
				forceStack := false
				forceForward := false
				if strings.HasSuffix(name, "/s") {
					forceStack = true
					name = name[:len(name)-2]
				} else if strings.HasSuffix(name, "/f") {
					forceForward = true
					name = name[:len(name)-2]
				}
				if forceStack || forceForward {
					out = append(out, NewWordModified(name, -1, forceStack, forceForward))
				} else {
					out = append(out, NewWord(name))
				}
			}
		}
	}
	return out, nil
}

// renderStack renders a result stack back into the same syntax used
// by the spec's expected column. Integer/string/boolean/none cover
// everything our spec words produce.
func renderStack(stack []Value) string {
	parts := make([]string, len(stack))
	for i, v := range stack {
		parts[i] = renderValue(v)
	}
	return strings.Join(parts, " ")
}

func renderValue(v Value) string {
	switch {
	case v.Data == nil && v.VType.Equal(TNone):
		return "null"
	case v.VType.Matches(TInteger):
		n, _ := v.AsInteger()
		return strconv.FormatInt(n, 10)
	case v.VType.Matches(TDecimal):
		f, _ := v.AsDecimal()
		return strconv.FormatFloat(f, 'g', -1, 64)
	case v.VType.Matches(TString):
		s, _ := v.AsString()
		return "\"" + s + "\""
	case v.VType.Matches(TBoolean):
		b, _ := v.AsBoolean()
		if b {
			return "true"
		}
		return "false"
	default:
		return v.String()
	}
}

// runSpecFile walks a TSV file and emits one t.Run subtest per row.
func runSpecFile(t *testing.T, path string) {
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

		name := fmt.Sprintf("L%d_%s", lineNum, sanitiseName(input))
		t.Run(name, func(t *testing.T) {
			values, err := tokenizeSpec(input)
			if err != nil {
				t.Fatalf("tokenize: %v", err)
			}

			r, err := NewRegistry()
			if err != nil {
				t.Fatalf("NewRegistry: %v", err)
			}
			registerSpecWords(r)
			r.InitRootContext()

			out, runErr := NewTop(r).Run(values)

			if strings.HasPrefix(expected, "ERROR:") {
				want := expected[len("ERROR:"):]
				if runErr == nil {
					t.Fatalf("expected error containing %q, got result %v", want, renderStack(out))
				}
				if want != "" && !strings.Contains(runErr.Error(), want) {
					t.Errorf("error %q does not contain %q", runErr.Error(), want)
				}
				return
			}

			if runErr != nil {
				t.Fatalf("unexpected error: %v", runErr)
			}
			got := renderStack(out)
			if got != expected {
				t.Errorf("got %q, want %q", got, expected)
			}
		})
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("scanner error in %s: %v", path, err)
	}
}

// sanitiseName trims the input down to a t.Run-friendly subtest name.
func sanitiseName(s string) string {
	s = strings.ReplaceAll(s, " ", "_")
	if len(s) > 40 {
		s = s[:40]
	}
	return s
}

func TestSpec(t *testing.T) {
	specDir := filepath.Join("..", "test", "spec")
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
