package engine

import "fmt"

// controlNatives covers the control-flow words: do, if, for, break,
// continue, error.
//
// Helpers used by these handlers (spliceArg, runForLoop, parseRange,
// forCarrierReturns, etc.) live alongside the slice in this file or
// in conditional.go / forloop.go for the helpers that are
// independently testable.
var controlNatives = []NativeFunc{
	{
		Name:        "do",
		ForwardArgs: true,
		Signatures: []NativeSig{
			{
				Args:       []*Type{TList},
				NoEvalArgs: map[int]bool{0: true},
				Handler:    doListHandler,
				ReturnsFn:  doListReturnsFn,
			},
			{
				Args:    []*Type{TMap},
				Handler: doMapHandler,
				Returns: []*Type{TAny},
			},
		},
	},
	{
		Name:        "if",
		ForwardArgs: true,
		Signatures: []NativeSig{
			{
				Args:       []*Type{TAny, TAny, TAny},
				NoEvalArgs: map[int]bool{0: true, 1: true, 2: true},
				Handler:    if3Handler,
				ReturnsFn:  if3ReturnsFn,
			},
			{
				Args:       []*Type{TAny, TAny},
				NoEvalArgs: map[int]bool{0: true, 1: true},
				Handler:    if2Handler,
				ReturnsFn:  if2ReturnsFn,
			},
			// Clause-list form: `if [c1 b1 c2 b2 … else]`. Even elements
			// are conditions, the following odd element is that clause's
			// body, and a trailing element (odd-length list) is the
			// else. Conditions are tried left-to-right; the first truthy
			// one's body runs, the rest are not evaluated. Each element
			// may be a code-body list (evaluated / spliced) or a plain
			// value (used as-is). Must be tried after if3/if2 so the
			// legacy `if <listCond> <then> [<else>]` forms still win when
			// extra args are present. See ifClause in conditional.go.
			{
				Args:       []*Type{TList},
				NoEvalArgs: map[int]bool{0: true},
				Handler:    ifListHandler,
				ReturnsFn:  ifListReturnsFn,
			},
		},
	},
	{
		Name:        "for",
		ForwardArgs: true,
		Signatures: []NativeSig{
			{
				Args:       []*Type{TInteger, TList},
				NoEvalArgs: map[int]bool{1: true},
				Handler:    forCountHandler,
				ReturnsFn:  forIntegerListReturnsFn,
			},
			{
				Args:       []*Type{TList, TList},
				NoEvalArgs: map[int]bool{1: true},
				Handler:    forRangeHandler,
				ReturnsFn:  forListListReturnsFn,
			},
		},
	},
	// break and continue signal via Registry.FlowCtrl rather than
	// returning an error. The Run loop in eng/engine.go reads the
	// signal after every step and dispatches it through the nearest
	// loop's flow-control resolver.
	{
		Name: "break",
		Signatures: []NativeSig{{
			Handler: func(_ []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
				r.FlowCtrl = FlowBreak
				return nil, nil
			},
			Returns: []*Type{},
		}},
	},
	{
		Name: "continue",
		Signatures: []NativeSig{{
			Handler: func(_ []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
				r.FlowCtrl = FlowContinue
				return nil, nil
			},
			Returns: []*Type{},
		}},
	},
	{
		Name:        "error",
		ForwardArgs: true,
		Signatures: []NativeSig{
			{
				Args:       []*Type{TList, TError},
				NoEvalArgs: map[int]bool{0: true},
				Handler:    errorHandler,
				Returns:    []*Type{TError},
			},
		},
	},
}

// ---- do handlers ----

func doListHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	if !IsConcrete(args[0]) {
		return nil, fmt.Errorf("do: argument must be a concrete list, got type literal")
	}
	_lst, _ := AsList(args[0])
	return doEvalList(r, _lst.Slice())
}

func doListReturnsFn(args []Value, r *Registry) []Value {
	body := args[0]
	if IsWord(body) {
		w, _ := AsWord(body)
		if v, ok := r.Defs.Top(w.Name); ok {
			body = v
		}
	}
	stk := RunCarrierBody(r, body)
	if len(stk) == 0 {
		return []Value{NewCarrier(TAny)}
	}
	return []Value{stk[len(stk)-1]}
}

func doMapHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	result, err := doEvalMapValue(r, args[0])
	if err != nil {
		return nil, err
	}
	return []Value{result}, nil
}

// doEvalList evaluates a top-level list of tokens in a sub-engine.
// Errors are caught and returned as a single error value on the stack.
func doEvalList(r *Registry, elems []Value) ([]Value, error) {
	sub := New(r)
	input := make([]Value, len(elems))
	copy(input, elems)
	result, err := sub.Run(input)
	if err != nil {
		return []Value{NewError(err)}, nil
	}
	return result, nil
}

// doEvalDataList evaluates a list from data context (inside a map).
// Strings that name registered functions are promoted to words.
func doEvalDataList(r *Registry, elems []Value) ([]Value, error) {
	sub := New(r)
	input := make([]Value, len(elems))
	for i, e := range elems {
		input[i] = doPromoteToWord(r, e)
	}
	return sub.Run(input)
}

// doPromoteToWord converts a string or atom value to a word if it
// names a registered function.
func doPromoteToWord(r *Registry, v Value) Value {
	if v.VType.Matches(TString) || v.VType.Matches(TAtom) {
		name, _ := AsString(v)
		if r.Lookup(name) != nil {
			return NewWord(name)
		}
	}
	return v
}

// doEvalMapValue recursively evaluates list values within a map. Used
// by `do` to walk a map literal and evaluate any embedded code lists.
func doEvalMapValue(r *Registry, v Value) (Value, error) {
	if v.VType.Equal(TList) && v.Data != nil && !IsTypedList(v) && !IsTableType(v) {
		_lst, _ := AsList(v)
		results, err := doEvalDataList(r, _lst.Slice())
		if err != nil {
			return Value{}, err
		}
		if len(results) == 1 {
			return results[0], nil
		}
		return NewList(results), nil
	}
	if v.VType.Equal(TMap) && v.Data != nil && !IsTypedMap(v) && !IsRecordType(v) && !IsOptionsType(v) {
		m, _ := AsMap(v)
		out := NewOrderedMap()
		for _, key := range m.Keys() {
			val, _ := m.Get(key)
			evaluated, err := doEvalMapValue(r, val)
			if err != nil {
				return Value{}, err
			}
			out.Set(key, evaluated)
		}
		return NewMap(out), nil
	}
	return v, nil
}

// ---- if handlers ----

func if3Handler(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
	cond := args[0]
	thenBranch := spliceArg(args[1])
	elseBranch := spliceArg(args[2])

	if cond.VType.Equal(TList) && cond.Data != nil && !IsTypedList(cond) && !IsTableType(cond) {
		_lst, _ := AsList(cond)
		condSlice := _lst.Slice()
		id := NextMarkID()
		tokens := make([]Value, 0, len(condSlice)+2)
		tokens = append(tokens, NewMark(id, condSlice...))
		tokens = append(tokens, condSlice...)
		tokens = append(tokens, NewMoveIf(id, "if", &IfCont{
			Then: thenBranch,
			Else: elseBranch,
		}))
		return tokens, nil
	}

	if CoerceBoolean(cond) {
		return thenBranch, nil
	}
	return elseBranch, nil
}

func if2Handler(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
	cond := args[0]
	thenBranch := spliceArg(args[1])

	if cond.VType.Equal(TList) && cond.Data != nil && !IsTypedList(cond) && !IsTableType(cond) {
		_lst, _ := AsList(cond)
		condSlice := _lst.Slice()
		id := NextMarkID()
		tokens := make([]Value, 0, len(condSlice)+2)
		tokens = append(tokens, NewMark(id, condSlice...))
		tokens = append(tokens, condSlice...)
		tokens = append(tokens, NewMoveIf(id, "if", &IfCont{
			Then: thenBranch,
			Else: nil,
		}))
		return tokens, nil
	}

	if CoerceBoolean(cond) {
		return thenBranch, nil
	}
	return nil, nil
}

func if3ReturnsFn(args []Value, r *Registry) []Value {
	if lit, ok := LiteralCondValue(args[0]); ok {
		branch := "else"
		if !lit {
			branch = "then"
		}
		r.Check.AddDiagnostic(CheckDiagnostic{
			Code:     "unreachable_branch",
			Detail:   "if condition is a constant " + BoolWord(lit) + "; " + branch + "-branch is unreachable",
			Severity: SeverityWarning,
		})
		if lit {
			restoreThen := ApplyGuardNarrowing(r, args[0])
			stk, defs := RunCarrierBodyWithDefs(r, args[1])
			restoreThen()
			InstallJoinedDefs(r, defs, nil)
			if len(stk) == 0 {
				return nil
			}
			return []Value{stk[len(stk)-1]}
		}
		restoreElse := ApplyComplementNarrowing(r, args[0])
		stk, defs := RunCarrierBodyWithDefs(r, args[2])
		restoreElse()
		InstallJoinedDefs(r, nil, defs)
		if len(stk) == 0 {
			return nil
		}
		return []Value{stk[len(stk)-1]}
	}
	restoreThen := ApplyGuardNarrowing(r, args[0])
	thenStk, thenDefs := RunCarrierBodyWithDefs(r, args[1])
	restoreThen()
	restoreElse := ApplyComplementNarrowing(r, args[0])
	elseStk, elseDefs := RunCarrierBodyWithDefs(r, args[2])
	restoreElse()
	InstallJoinedDefs(r, thenDefs, elseDefs)
	joined := JoinCarrierStacks(thenStk, elseStk)
	if len(joined) == 0 {
		return nil
	}
	return []Value{joined[len(joined)-1]}
}

func if2ReturnsFn(args []Value, r *Registry) []Value {
	if lit, ok := LiteralCondValue(args[0]); ok && !lit {
		r.Check.AddDiagnostic(CheckDiagnostic{
			Code:     "unreachable_branch",
			Detail:   "if condition is a constant false; then-branch is unreachable",
			Severity: SeverityWarning,
		})
	}
	restore := ApplyGuardNarrowing(r, args[0])
	thenStk, thenDefs := RunCarrierBodyWithDefs(r, args[1])
	restore()
	InstallJoinedDefs(r, thenDefs, nil)
	if len(thenStk) == 0 {
		return []Value{NewCarrier(TNone)}
	}
	return []Value{JoinCarriers(thenStk[len(thenStk)-1], NewCarrier(TNone))}
}

// ifListHandler implements the clause-list form `if [c1 b1 c2 b2 … else]`.
// It hands the (raw, NoEval'd) list's elements to ifClause, which produces
// the token stream the engine then runs.
func ifListHandler(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
	if !IsConcrete(args[0]) {
		return nil, fmt.Errorf("if: clause-list argument must be a concrete list, got a type literal")
	}
	_lst, _ := AsList(args[0])
	return ifClause(_lst.Slice()), nil
}

// ifListReturnsFn type-checks the clause-list form: the result is the
// join of every clause body's last value plus the else clause (or None
// when there is no else, since an unmatched `if` produces nothing).
// Condition bodies are still run for their diagnostics but don't
// contribute to the return type. Unlike if3/if2 this does no per-clause
// guard narrowing — multi-clause narrowing isn't modelled.
func ifListReturnsFn(args []Value, r *Registry) []Value {
	if !IsConcrete(args[0]) || !args[0].VType.Equal(TList) {
		return []Value{NewCarrier(TAny)}
	}
	_lst, _ := AsList(args[0])
	elems := _lst.Slice()

	var joined []Value
	add := func(stk []Value) {
		if joined == nil {
			joined = stk
		} else {
			joined = JoinCarrierStacks(joined, stk)
		}
	}

	i := 0
	for ; i+1 < len(elems); i += 2 {
		if isCodeBody(elems[i]) {
			RunCarrierBody(r, elems[i]) // run the condition body for diagnostics only
		}
		add(RunCarrierBody(r, elems[i+1]))
	}
	if i < len(elems) {
		add(RunCarrierBody(r, elems[i])) // lone else
	} else {
		add([]Value{NewCarrier(TNone)}) // no else: an unmatched if yields nothing
	}

	if len(joined) == 0 {
		return nil
	}
	return []Value{joined[len(joined)-1]}
}

// ---- for / break / continue handlers ----

func forCountHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	n, _ := args[0].AsConcreteInteger()
	body := args[1]
	return runForLoop(r, 0, n, 1, "i", body)
}

func forRangeHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	if !IsConcrete(args[0]) {
		return nil, fmt.Errorf("for: range must be a concrete list, got type literal")
	}
	_lst, _ := AsList(args[0])
	rangeSpec := _lst.Slice()
	body := args[1]
	start, end, step, err := parseRange(rangeSpec)
	if err != nil {
		return nil, fmt.Errorf("for: %w", err)
	}
	return runForLoop(r, start, end, step, "i", body)
}

func forIntegerListReturnsFn(args []Value, r *Registry) []Value {
	return forCarrierAnalyse(r, "i", TInteger, args)
}

func forListListReturnsFn(args []Value, r *Registry) []Value {
	return forCarrierAnalyse(r, "i", TInteger, args)
}

// forCarrierAnalyse runs the body once with the iterator bound as a
// typed carrier and returns a typed list whose element type mirrors
// the body's residual top-of-stack.
func forCarrierAnalyse(r *Registry, iterName string, iterType *Type, args []Value) []Value {
	body := args[len(args)-1]
	r.Defs.Push(iterName, NewCarrier(iterType))
	stk, _ := RunCarrierBodyWithDefs(r, body)
	r.Defs.Pop(iterName)
	if len(stk) == 0 {
		return []Value{NewCarrier(TList)}
	}
	return []Value{NewCarrierTypedList(stk[len(stk)-1].VType)}
}

// ---- error handler ----

func errorHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	if !IsConcrete(args[0]) {
		return nil, fmt.Errorf("error: handler must be a concrete list, got type literal")
	}
	sub := New(r)
	_lst, _ := AsList(args[0])
	body := _lst.Slice()
	input := make([]Value, 0, 1+len(body))
	input = append(input, args[1])
	input = append(input, body...)
	return sub.Run(input)
}
