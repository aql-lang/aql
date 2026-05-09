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

// Counter for unique mark IDs emitted by the `replayq` test word.
var replayCounter int

// Spec-driven tests. Each TSV file under ../test/spec/ describes a
// table of (input tokens, expected stack) cases that the runner
// converts into engine inputs and runs through a fresh registry
// pre-populated with a fixed set of test words. No parser is involved;
// the tokenizer in this file is intentionally minimal.
//
// NAMING — most test words in this file end in `q` (addq, subq,
// mulq, …) to make it unmistakable that they are SPEC-RUNNER
// FIXTURES, not the production AQL words of the same root name. The
// production engine has its own add/sub/mul/etc. with richer
// semantics; the q-suffixed words here are intentionally minimal,
// single-overload versions tailored for spec coverage of the
// dispatch / value / type-lattice core.
//
// EXCEPTIONS — language-fundamental keywords keep their bare name
// because they ARE the construct being tested, not a fixture for it:
//
//   def, fn       — name binding, function literal
//   end           — structural keyword (handled by stepEnd, not
//                   registered)
//   quote, args   — first-class quotation and per-fn argument frame
//   if, for, type — control flow / type definition (not yet registered
//                   in the spec runner; reserved for future ports)
//
// These are core to the language design (LANGREF.10.md), so the spec
// runner's versions of `def` and `fn` MUST be named `def` and `fn` —
// any other name would be misleading because what we're actually
// testing is the binding / function-literal mechanism itself.
//
// To add a new test word: if it's a fundamental keyword, use the bare
// name and document it here. Otherwise pick the production word's
// root name and append `q`. Reuse the production handler convention
// (mirror the production engine's body verbatim where possible —
// see the math block below for an example referencing native_math.go).

// registerSpecWords installs the word set the spec files reference.
// Keep this list small and stable — the specs are easier to read
// when there's no surprise about what each word does. Test fixtures
// have a `q` suffix; language fundamentals (def, fn, quote, args)
// live in core_words.go and are installed via RegisterCoreWords.
// See the file header for the rationale.
func registerSpecWords(r *Registry) {
	// Install language fundamentals first: def, fn, quote, args.
	// These are the canonical aqleng implementations — any spec
	// that exercises name binding, function literals, data quotation,
	// or the per-fn args frame is testing the production code, not
	// a test fixture.
	RegisterCoreWords(r)

	// addq, subq, mulq: numeric arithmetic via forward precedence.
	//
	// Sig is [TNumber, TNumber] so Integer and Decimal both match
	// (Integer/Decimal are subtypes of Number). The handler dispatches
	// on the runtime VType of each arg:
	//   - both Integer  → integer op, returns NewInteger
	//   - any Decimal   → float op,   returns NewDecimal
	// This is the "decimal contagion" rule: any decimal in either slot
	// promotes the result to Decimal. Mirrors the production engine's
	// numericBinaryHandler in aql/internal/engine/native_math.go.
	//
	// Handler convention: result = b OP a where a = args[0] and
	// b = args[1]. This is the production-engine convention (see
	// native_math.go) — for non-commutative ops it makes the natural
	// infix reading work:
	//
	//   10 subq 3  → matching algo: forward [3], stack [10]
	//                → sig[0]=3 (fwd), sig[1]=10 (stk top)
	//                → args = [3, 10]
	//                → handler returns b - a = 10 - 3 = 7
	//
	// The same matching algorithm gives sig=[3, 10] for `subq 3 10`
	// (forward in source order) and `10 3 subq` (stack top-down), so
	// all three forms produce 7. The other three arrangements
	// (`subq 10 3`, `3 subq 10`, `3 10 subq`) produce sig=[10, 3]
	// and thus -7. There is no "swap form" — every form is an equally
	// valid surface arrangement; the matching algorithm just maps
	// each one to a specific (args[0], args[1]) pair.
	toFloat := func(v Value) float64 {
		if v.VType.Matches(TInteger) {
			n, _ := v.AsInteger()
			return float64(n)
		}
		f, _ := v.AsDecimal()
		return f
	}
	numericBinary := func(intOp func(a, b int64) int64, floatOp func(a, b float64) float64) Handler {
		return func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
			if args[0].VType.Matches(TInteger) && args[1].VType.Matches(TInteger) {
				a, _ := args[0].AsInteger()
				b, _ := args[1].AsInteger()
				return []Value{NewInteger(intOp(a, b))}, nil
			}
			return []Value{NewDecimal(floatOp(toFloat(args[0]), toFloat(args[1])))}, nil
		}
	}
	r.RegisterNativeFunc(NativeFunc{
		Name:              "addq",
		ForwardPrecedence: true,
		Signatures: []NativeSig{{
			Args:    []Type{TNumber, TNumber},
			Handler: numericBinary(func(a, b int64) int64 { return b + a }, func(a, b float64) float64 { return b + a }),
			Returns: []Type{TNumber},
		}},
	})
	r.RegisterNativeFunc(NativeFunc{
		Name:              "subq",
		ForwardPrecedence: true,
		Signatures: []NativeSig{{
			Args:    []Type{TNumber, TNumber},
			Handler: numericBinary(func(a, b int64) int64 { return b - a }, func(a, b float64) float64 { return b - a }),
			Returns: []Type{TNumber},
		}},
	})
	r.RegisterNativeFunc(NativeFunc{
		Name:              "mulq",
		ForwardPrecedence: true,
		Signatures: []NativeSig{{
			Args:    []Type{TNumber, TNumber},
			Handler: numericBinary(func(a, b int64) int64 { return b * a }, func(a, b float64) float64 { return b * a }),
			Returns: []Type{TNumber},
		}},
	})

	// negq: forward-prec unary negation, sig [Number|] under the
	// unified §1.4 dispatch model — BarrierPos = N (= 1 here) makes
	// the single arg forward-eligible with stack fallback. Both
	// `negq 5` (forward) and `5 negq` (stack-via-fallback) work.
	// Mirrors aql/internal/nativemod/math.go::negate. Decimal in,
	// Decimal out — preserves the type tag.
	r.RegisterNativeFunc(NativeFunc{
		Name:              "negq",
		ForwardPrecedence: true,
		Signatures: []NativeSig{{
			Args:       []Type{TNumber},
			BarrierPos: 1,
			Handler: func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
				if args[0].VType.Matches(TInteger) {
					n, _ := args[0].AsInteger()
					return []Value{NewInteger(-n)}, nil
				}
				f, _ := args[0].AsDecimal()
				return []Value{NewDecimal(-f)}, nil
			},
			Returns: []Type{TNumber},
		}},
	})

	// `dup`, `swap`, `drop`, `over`, `rot`, `nip`, `tuck`, `2dup`,
	// `2swap`, `2drop`, `2over` — installed via RegisterCoreWords
	// above. See aqleng/go/core_words.go::registerCoreStack.

	// concatq: string concatenation, forward precedence.
	// Handler convention: result = b + a (= args[1] + args[0]) so
	// the natural infix reading `"hello" concatq " world" → "hello world"`
	// works. Same pattern as subq above. Mirrors the production engine's
	// addConcatHandler in aql/internal/engine/native_math.go.
	r.RegisterNativeFunc(NativeFunc{
		Name:              "concatq",
		ForwardPrecedence: true,
		Signatures: []NativeSig{{
			Args: []Type{TString, TString},
			Handler: func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
				a, _ := args[0].AsString()
				b, _ := args[1].AsString()
				return []Value{NewString(b + a)}, nil
			},
			Returns: []Type{TString},
		}},
	})

	// `not` — installed as a core word via RegisterCoreWords above.
	// See aqleng/go/core_boolean.go::registerCoreNot.

	// describeq: two type-specific overloads, used in the dispatch spec.
	r.RegisterNativeFunc(NativeFunc{
		Name:              "describeq",
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

	// tagq: Any vs Integer overloads — exercises specificity scoring.
	r.RegisterNativeFunc(NativeFunc{
		Name:              "tagq",
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

	// factq, codeq, routeq: §1.1 literal-pattern dispatch via Patterns.
	// Each declares a specific-value overload first plus a catch-all.
	r.RegisterNativeFunc(NativeFunc{
		Name:              "factq",
		ForwardPrecedence: true,
		Signatures: []NativeSig{
			{
				Args:     []Type{TInteger},
				Patterns: map[int]Value{0: NewInteger(0)},
				Handler: func(_ []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
					return []Value{NewInteger(1)}, nil
				},
				Returns: []Type{TInteger},
			},
			{
				Args: []Type{TInteger},
				Handler: func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
					n, _ := args[0].AsInteger()
					return []Value{NewInteger(n)}, nil
				},
				Returns: []Type{TInteger},
			},
		},
	})
	r.RegisterNativeFunc(NativeFunc{
		Name:              "codeq",
		ForwardPrecedence: true,
		Signatures: []NativeSig{
			{
				Args:     []Type{TInteger},
				Patterns: map[int]Value{0: NewInteger(99)},
				Handler: func(_ []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
					return []Value{NewString("ninety-nine")}, nil
				},
				Returns: []Type{TString},
			},
			{
				Args: []Type{TInteger},
				Handler: func(_ []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
					return []Value{NewString("general")}, nil
				},
				Returns: []Type{TString},
			},
		},
	})
	r.RegisterNativeFunc(NativeFunc{
		Name:              "routeq",
		ForwardPrecedence: true,
		Signatures: []NativeSig{
			{
				Args:     []Type{TString},
				Patterns: map[int]Value{0: NewString("admin")},
				Handler: func(_ []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
					return []Value{NewString("matched-admin")}, nil
				},
				Returns: []Type{TString},
			},
			{
				Args: []Type{TString},
				Handler: func(_ []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
					return []Value{NewString("other")}, nil
				},
				Returns: []Type{TString},
			},
		},
	})

	// tripq: 3-arg integer formatter. Default barrier = N so all
	// position-mixing arrangements (all-forward through all-stack)
	// bind sig[0..2] to the same source-order args.
	r.RegisterNativeFunc(NativeFunc{
		Name:              "tripq",
		ForwardPrecedence: true,
		Signatures: []NativeSig{{
			Args: []Type{TInteger, TInteger, TInteger},
			Handler: func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
				a, _ := args[0].AsInteger()
				b, _ := args[1].AsInteger()
				c, _ := args[2].AsInteger()
				return []Value{NewString(fmt.Sprintf("%d,%d,%d", a, b, c))}, nil
			},
			Returns: []Type{TString},
		}},
	})

	// pairq: mixed-barrier sig [Integer | Integer]. Forward fills
	// sig[0]; sig[1] must come from the stack. The handler formats
	// "args[0]:args[1]" so the binding is visible in the output.
	r.RegisterNativeFunc(NativeFunc{
		Name:              "pairq",
		ForwardPrecedence: true,
		Signatures: []NativeSig{{
			Args:       []Type{TInteger, TInteger},
			BarrierPos: 1,
			Handler: func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
				a, _ := args[0].AsInteger()
				b, _ := args[1].AsInteger()
				return []Value{NewString(fmt.Sprintf("%d:%d", a, b))}, nil
			},
			Returns: []Type{TString},
		}},
	})

	// lengthq, firstq: list-aware test words for list.tsv.
	r.RegisterNativeFunc(NativeFunc{
		Name:              "lengthq",
		ForwardPrecedence: true,
		Signatures: []NativeSig{{
			Args: []Type{TList},
			Handler: func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
				lst := args[0].AsList()
				return []Value{NewInteger(int64(lst.Len()))}, nil
			},
			Returns: []Type{TInteger},
		}},
	})
	r.RegisterNativeFunc(NativeFunc{
		Name:              "firstq",
		ForwardPrecedence: true,
		Signatures: []NativeSig{{
			Args: []Type{TList},
			Handler: func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
				lst := args[0].AsList()
				if lst.Len() == 0 {
					return []Value{NewNone()}, nil
				}
				return []Value{lst.Get(0)}, nil
			},
			Returns: []Type{TAny},
		}},
	})

	// `def`, `fn`, `quote`, `args`, `end` — installed via
	// RegisterCoreWords above. See aqleng/go/core_words.go.

	// replayq: emit Mark + body + Move so the body executes once,
	// then the Move triggers a one-shot replay (body runs twice).
	r.RegisterNativeFunc(NativeFunc{
		Name:              "replayq",
		ForwardPrecedence: true,
		Signatures: []NativeSig{{
			Args:       []Type{TList},
			NoEvalArgs: map[int]bool{0: true},
			Handler: func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
				body := args[0].AsList().Slice()
				replayCounter++
				id := fmt.Sprintf("__replayq_%d", replayCounter)
				out := make([]Value, 0, len(body)+2)
				out = append(out, NewMark(id, body...))
				out = append(out, body...)
				out = append(out, NewMove(id, "replayq"))
				return out, nil
			},
		}},
	})

	// Simple-value defs the def.tsv spec references. A word whose name
	// is in the def stack is substituted by its value before normal
	// dispatch, provided the value isn't an FnDef / ObjectType.
	r.PushDef("pi", NewInteger(3))
	r.PushDef("tau", NewInteger(6))
	r.PushDef("greeting", NewString("hello"))
}

// tokenizeSpec converts a single space-separated input string from a
// TSV row into a slice of engine values. It is intentionally minimal:
// integers, decimals, "quoted" strings, true/false/none/null, list
// literals via `[` ... `]`, implicit-map literals via `{` ... `}` with
// `key:value` pair tokens, and bare identifiers (treated as words).
// `(` and `)` are not collected here — they remain as Word values so
// the engine's paren pre-evaluation handles them at runtime.
//
// Keyword conventions:
//
//   - `none` parses to a None type literal — None is the unit type;
//     `none` is its single inhabitant.
//   - `null` parses to an Atom("null") — represents JSON null at the
//     value level, distinct from the None type's inhabitant.
//   - `{ x:Integer y:String }` parses to an Implicit Map with the
//     declared keys; values are recursively tokenized so types
//     (`Integer`, `List`), scalars, and quoted strings all work.
func tokenizeSpec(s string) ([]Value, error) {
	// stack[0] is the top-level token stream; deeper entries are list
	// or map literals being collected. We track frame kind in parallel
	// so `]` and `}` close the right kind.
	stack := [][]Value{nil}
	kinds := []string{"top"}
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
			top := len(stack) - 1
			stack[top] = append(stack[top], NewString(s[i+1:j]))
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

		top := len(stack) - 1

		// List literal open / close.
		if tok == "[" {
			stack = append(stack, nil)
			kinds = append(kinds, "list")
			continue
		}
		if tok == "]" {
			if top == 0 || kinds[top] != "list" {
				return nil, fmt.Errorf("tokenize: unmatched ']' at %d", i-1)
			}
			collected := stack[top]
			if collected == nil {
				collected = []Value{}
			}
			stack = stack[:top]
			kinds = kinds[:top]
			parent := len(stack) - 1
			stack[parent] = append(stack[parent], NewEvalList(collected))
			continue
		}

		// Map literal open / close.
		if tok == "{" {
			stack = append(stack, nil)
			kinds = append(kinds, "map")
			continue
		}
		if tok == "}" {
			if top == 0 || kinds[top] != "map" {
				return nil, fmt.Errorf("tokenize: unmatched '}' at %d", i-1)
			}
			pairs := stack[top]
			stack = stack[:top]
			kinds = kinds[:top]
			parent := len(stack) - 1
			m := NewOrderedMap()
			for _, p := range pairs {
				name, val, err := parsePairToken(p)
				if err != nil {
					return nil, err
				}
				m.Set(name, val)
			}
			stack[parent] = append(stack[parent], NewImplicitMap(m))
			continue
		}

		switch tok {
		case "true":
			stack[top] = append(stack[top], NewBoolean(true))
		case "false":
			stack[top] = append(stack[top], NewBoolean(false))
		case "none":
			stack[top] = append(stack[top], NewNone())
		case "null":
			stack[top] = append(stack[top], NewAtom("null"))
		default:
			if n, err := strconv.ParseInt(tok, 10, 64); err == nil {
				stack[top] = append(stack[top], NewInteger(n))
			} else if f, err := strconv.ParseFloat(tok, 64); err == nil {
				stack[top] = append(stack[top], NewDecimal(f))
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
					stack[top] = append(stack[top], NewWordModified(name, -1, forceStack, forceForward))
				} else {
					stack[top] = append(stack[top], NewWord(name))
				}
			}
		}
	}
	if len(stack) > 1 {
		switch kinds[len(kinds)-1] {
		case "map":
			return nil, fmt.Errorf("tokenize: unterminated map literal '{'")
		default:
			return nil, fmt.Errorf("tokenize: unterminated list literal '['")
		}
	}
	return stack[0], nil
}

// parsePairToken splits a `name:value` token into the entry pieces of
// an implicit map. The value side is parsed as a Value: known type
// names become type literals, integers/decimals parse as scalars,
// quoted strings as strings, and any remaining word is captured as a
// bare Word.
func parsePairToken(v Value) (string, Value, error) {
	if !v.IsWord() {
		return "", Value{}, fmt.Errorf("map: pair token must be a Word, got %s", v.VType.String())
	}
	w, _ := v.AsWord()
	idx := strings.Index(w.Name, ":")
	if idx <= 0 {
		return "", Value{}, fmt.Errorf("map: expected `name:value`, got %q", w.Name)
	}
	name := w.Name[:idx]
	rest := w.Name[idx+1:]
	if rest == "" {
		return "", Value{}, fmt.Errorf("map: empty value side in %q", w.Name)
	}
	// Type-name word? Look up in typeNames.
	if t, ok := typeNames[rest]; ok {
		return name, NewTypeLiteral(t), nil
	}
	// Integer / Decimal literal?
	if n, err := strconv.ParseInt(rest, 10, 64); err == nil {
		return name, NewInteger(n), nil
	}
	if f, err := strconv.ParseFloat(rest, 64); err == nil {
		return name, NewDecimal(f), nil
	}
	// Boolean / none / null literal?
	switch rest {
	case "true":
		return name, NewBoolean(true), nil
	case "false":
		return name, NewBoolean(false), nil
	case "none":
		return name, NewNone(), nil
	case "null":
		return name, NewAtom("null"), nil
	}
	// Fallback: bare Word.
	return name, NewWord(rest), nil
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
	case v.IsNone():
		// The VALUE `none` — unique inhabitant of None.
		return "none"
	case v.Data == nil:
		// Type literal — render the LEAF of its type path (e.g. Integer
		// → "Integer", List → "List", None → "None"). Type names are
		// globally unique, so the leaf alone is unambiguous.
		return v.VType.Leaf()
	case v.VType.Matches(TInteger):
		n, _ := v.AsInteger()
		return strconv.FormatInt(n, 10)
	case v.VType.Matches(TDecimal):
		f, _ := v.AsDecimal()
		return formatDecimal(f)
	case v.VType.Matches(TString):
		s, _ := v.AsString()
		return "\"" + s + "\""
	case v.VType.Matches(TBoolean):
		b, _ := v.AsBoolean()
		if b {
			return "true"
		}
		return "false"
	case v.VType.Equal(TAtom) && v.Data != nil:
		// Render as `atom(name)` so the spec format matches TS's
		// Value.toString output exactly. Go's default Value.String
		// for an atom prints just the bare name.
		s, _ := v.AsAtom()
		return "atom(" + s + ")"
	case v.VType.Matches(TList) && v.Data != nil:
		// Recursive list rendering. Use space-separated elements
		// in [ ... ] so the spec format matches the TS engine's
		// toString output exactly.
		lst := v.AsList()
		parts := make([]string, lst.Len())
		for i := 0; i < lst.Len(); i++ {
			parts[i] = renderValue(lst.Get(i))
		}
		return "[" + strings.Join(parts, " ") + "]"
	case v.VType.Equal(TMap) && v.Data != nil:
		// Recursive map rendering. Use `key:value` pairs inside `{ … }`
		// matching the implicit-map and value-map source syntax. Values
		// are rendered recursively so nested type literals render via
		// the leaf branch.
		m := v.AsMap()
		if m == nil {
			return v.String()
		}
		parts := make([]string, m.Len())
		for i, k := range m.Keys() {
			val, _ := m.Get(k)
			parts[i] = k + ":" + renderValue(val)
		}
		return "{" + strings.Join(parts, " ") + "}"
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
