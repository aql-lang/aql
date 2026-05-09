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

// Counter for unique mark IDs emitted by the `replay` test word.
var replayCounter int

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

	// fact, code, route: §1.1 literal-pattern dispatch via Patterns.
	// Each declares a specific-value overload first plus a catch-all.
	r.RegisterNativeFunc(NativeFunc{
		Name:              "fact",
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
		Name:              "code",
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
		Name:              "route",
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

	// trip: 3-arg integer formatter. Default barrier = N so all
	// position-mixing arrangements (all-forward through all-stack)
	// bind sig[0..2] to the same source-order args.
	r.RegisterNativeFunc(NativeFunc{
		Name:              "trip",
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

	// pair: mixed-barrier sig [Integer | Integer]. Forward fills
	// sig[0]; sig[1] must come from the stack. The handler formats
	// "args[0]:args[1]" so the binding is visible in the output.
	r.RegisterNativeFunc(NativeFunc{
		Name:              "pair",
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

	// length, first: list-aware test words for list.tsv.
	r.RegisterNativeFunc(NativeFunc{
		Name:              "length",
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
		Name:              "first",
		ForwardPrecedence: true,
		Signatures: []NativeSig{{
			Args: []Type{TList},
			Handler: func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
				lst := args[0].AsList()
				if lst.Len() == 0 {
					return []Value{NewTypeLiteral(TNone)}, nil
				}
				return []Value{lst.Get(0)}, nil
			},
			Returns: []Type{TAny},
		}},
	})

	// def: spec-subset code-body binding. Captures `def NAME body`
	// where NAME arrives as a Word token (no /q machinery in this
	// runner's tokenizer) and body is any value — typically a List
	// literal that becomes a callable code body. The handler pushes
	// the body onto the def stack under NAME.
	r.RegisterNativeFunc(NativeFunc{
		Name:              "def",
		ForwardPrecedence: true,
		Signatures: []NativeSig{{
			Args:       []Type{TWord, TAny},
			NoEvalArgs: map[int]bool{1: true},
			Handler: func(args []Value, _ map[string]Value, _ []Value, reg *Registry) ([]Value, error) {
				w, _ := args[0].AsWord()
				// FnDef body: register a synthesised native that
				// matches the param types, binds them onto the def
				// stack, runs the body via a sub-engine, then pops.
				// Bypassing InstallFnDef avoids that helper's
				// dependency on internal `__pa` / paren-marker words
				// which only exist in the production aql/internal
				// engine, not the bare aqleng spec runner.
				if info, ok := args[1].Data.(FnDefInfo); ok && len(info.Sigs) == 1 {
					installSpecFnDef(reg, w.Name, info.Sigs[0])
					return []Value{}, nil
				}
				reg.PushDef(w.Name, args[1])
				return []Value{}, nil
			},
			Returns: []Type{},
		}},
	})

	// fn: builds a function definition value from
	// `fn [ params ] [ returns ] [ body ]`. Each param is a single
	// Word token of the form `name:TypeName` — the spec tokenizer is
	// whitespace-only so a typed param arrives as one Word and the
	// handler splits on `:` to recover the (name, type) pair.
	r.RegisterNativeFunc(NativeFunc{
		Name:              "fn",
		ForwardPrecedence: true,
		Signatures: []NativeSig{{
			Args:       []Type{TList, TList, TList},
			NoEvalArgs: map[int]bool{0: true, 1: true, 2: true},
			Handler: func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
				paramsList := args[0].AsList()
				returnsList := args[1].AsList()
				body := args[2].AsList()

				params := make([]FnParam, paramsList.Len())
				for i := 0; i < paramsList.Len(); i++ {
					p, err := parseSpecFnParam(paramsList.Get(i))
					if err != nil {
						return nil, err
					}
					params[i] = p
				}
				returns := make([]Type, returnsList.Len())
				for i := 0; i < returnsList.Len(); i++ {
					t, err := parseSpecFnReturn(returnsList.Get(i))
					if err != nil {
						return nil, err
					}
					returns[i] = t
				}

				info := FnDefInfo{
					Sigs: []FnSig{{
						Params:  params,
						Returns: returns,
						Body:    body.Slice(),
					}},
				}
				return []Value{NewFnDef(info)}, nil
			},
			Returns: []Type{TFunction},
		}},
	})

	// quote: capture the next forward token as data.
	//   sig [TWord]: convert Word→Atom so `quote dup` yields
	//                atom(dup) even when dup is registered.
	//   sig [TAny]:  catch-all passthrough.
	r.RegisterNativeFunc(NativeFunc{
		Name:              "quote",
		ForwardPrecedence: true,
		Signatures: []NativeSig{
			{
				Args: []Type{TWord},
				Handler: func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
					w, _ := args[0].AsWord()
					return []Value{NewAtom(w.Name)}, nil
				},
				Returns: []Type{TAtom},
			},
			{
				// NoEvalArgs keeps the list raw (not auto-evaluated)
				// so we can flag it Quoted=true and have the def-sub
				// path treat it as data instead of splicing it as a
				// code body. Without this, `def y quote [1 add 2]`
				// would bind y to a code list and `y` would inline-
				// execute its tokens.
				Args:       []Type{TAny},
				NoEvalArgs: map[int]bool{0: true},
				Handler: func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
					v := args[0]
					if v.VType.Equal(TList) && v.Data != nil {
						v.Quoted = true
					}
					return []Value{v}, nil
				},
				Returns: []Type{TAny},
			},
		},
	})

	// `end` is a structural keyword the engine handles directly via
	// stepEnd in aqleng/go/engine.go — no spec registration needed.

	// args: 0-arg word that returns the current fn-call's argument
	// frame as a list. dispatchFnDef pushes the matched args (in sig
	// order) before running the body.
	r.RegisterNativeFunc(NativeFunc{
		Name: "args",
		Signatures: []NativeSig{{
			Args: []Type{},
			Handler: func(_ []Value, _ map[string]Value, _ []Value, reg *Registry) ([]Value, error) {
				if top, ok := reg.TopArgs(); ok {
					return []Value{top}, nil
				}
				return []Value{NewList(nil)}, nil
			},
			Returns: []Type{TList},
		}},
	})

	// replay: emit Mark + body + Move so the body executes once,
	// then the Move triggers a one-shot replay (body runs twice).
	r.RegisterNativeFunc(NativeFunc{
		Name:              "replay",
		ForwardPrecedence: true,
		Signatures: []NativeSig{{
			Args:       []Type{TList},
			NoEvalArgs: map[int]bool{0: true},
			Handler: func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
				body := args[0].AsList().Slice()
				replayCounter++
				id := fmt.Sprintf("__replay_%d", replayCounter)
				out := make([]Value, 0, len(body)+2)
				out = append(out, NewMark(id, body...))
				out = append(out, body...)
				out = append(out, NewMove(id, "replay"))
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

// installSpecFnDef wires a single fn signature into the spec runner's
// registry as a synthesised native. The handler binds each named
// param onto the def stack, runs the body in a fresh sub-engine, then
// pops the bindings. Mirrors the param-binding portion of the full
// engine's InstallFnDef without pulling in __pa / paren-marker
// machinery that the spec runner's word table doesn't carry.
func installSpecFnDef(r *Registry, name string, sig FnSig) {
	argTypes := make([]Type, len(sig.Params))
	for i, p := range sig.Params {
		argTypes[i] = p.Type
	}
	bodyCopy := append([]Value{}, sig.Body...)
	r.RegisterNativeFunc(NativeFunc{
		Name:              name,
		ForwardPrecedence: true,
		Signatures: []NativeSig{{
			Args: argTypes,
			Handler: func(args []Value, _ map[string]Value, _ []Value, reg *Registry) ([]Value, error) {
				for i, p := range sig.Params {
					reg.PushDef(p.Name, args[i])
				}
				// Push the args list so the body can read positional
				// args via the `args` word. Mirrors the InstallFnDef
				// path in core_helpers.go that does the same.
				argsCopy := append([]Value{}, args...)
				reg.PushArgs(NewList(argsCopy))
				defer func() {
					reg.PopArgs()
					for i := len(sig.Params) - 1; i >= 0; i-- {
						reg.PopDef(sig.Params[i].Name)
					}
				}()
				sub := New(reg)
				input := append([]Value{}, bodyCopy...)
				return sub.Run(input)
			},
		}},
	})
}

// parseSpecFnParam splits a `name:TypeName` Word into an FnParam.
// The spec tokenizer is whitespace-only, so a typed param arrives as
// one Word token; we recover the (name, type) pair here.
func parseSpecFnParam(v Value) (FnParam, error) {
	if !v.IsWord() {
		return FnParam{}, fmt.Errorf("fn: expected param Word, got %s", v.String())
	}
	w, _ := v.AsWord()
	idx := strings.Index(w.Name, ":")
	if idx < 0 {
		return FnParam{}, fmt.Errorf("fn: param %q missing ':TypeName' suffix", w.Name)
	}
	name := w.Name[:idx]
	typeName := w.Name[idx+1:]
	t, err := parseSpecTypeName(typeName)
	if err != nil {
		return FnParam{}, err
	}
	return FnParam{Name: name, Type: t}, nil
}

func parseSpecFnReturn(v Value) (Type, error) {
	if !v.IsWord() {
		return Type{}, fmt.Errorf("fn: expected return-type Word, got %s", v.String())
	}
	w, _ := v.AsWord()
	return parseSpecTypeName(w.Name)
}

func parseSpecTypeName(name string) (Type, error) {
	tn, ok := TypeNameTable()[name]
	if !ok {
		return Type{}, fmt.Errorf("fn: unknown type %q", name)
	}
	return tn, nil
}

// tokenizeSpec converts a single space-separated input string from a
// TSV row into a slice of engine values. It is intentionally minimal:
// integers, decimals, "quoted" strings, true/false/null, list
// literals via `[` ... `]`, and bare identifiers (treated as words).
// `(` and `)` are not collected here — they remain as Word values so
// the engine's paren pre-evaluation handles them at runtime.
func tokenizeSpec(s string) ([]Value, error) {
	// stack[0] is the top-level token stream; deeper entries are list
	// literals being collected. `[` opens a new frame, `]` closes the
	// current one and pushes the assembled List into the parent.
	stack := [][]Value{nil}
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
			continue
		}
		if tok == "]" {
			if top == 0 {
				return nil, fmt.Errorf("tokenize: unmatched ']' at %d", i-1)
			}
			collected := stack[top]
			if collected == nil {
				collected = []Value{}
			}
			stack = stack[:top]
			parent := len(stack) - 1
			// Tokenizer-built lists carry Eval=true so the engine's
			// auto-evaluation paths (execMatch and end-of-Run drain)
			// fire on them, mirroring the parser's NewEvalList output
			// in the production aql package.
			stack[parent] = append(stack[parent], NewEvalList(collected))
			continue
		}

		switch tok {
		case "true":
			stack[top] = append(stack[top], NewBoolean(true))
		case "false":
			stack[top] = append(stack[top], NewBoolean(false))
		case "null":
			stack[top] = append(stack[top], NewTypeLiteral(TNone))
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
		return nil, fmt.Errorf("tokenize: unterminated list literal '['")
	}
	return stack[0], nil
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
