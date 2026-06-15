package native

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
		Name: "do",

		Signatures: []NativeSig{
			{
				Args:       []*Type{TList},
				NoEvalArgs: map[int]bool{0: true},
				Handler:    doListHandler,
				ReturnsFn:  doListReturnsFn, BarrierPos: -1,
			},
			{
				Args:    []*Type{TMap},
				Handler: doMapHandler,
				Returns: []*Type{TAny}, BarrierPos: -1,
			},
		},
	},
	{
		Name: "if",

		Signatures: []NativeSig{
			{
				Args:       []*Type{TAny, TAny, TAny},
				NoEvalArgs: map[int]bool{0: true, 1: true, 2: true},
				Handler:    if3Handler,
				ReturnsFn:  if3ReturnsFn, BarrierPos: -1,
			},
			{
				Args:       []*Type{TAny, TAny},
				NoEvalArgs: map[int]bool{0: true, 1: true},
				Handler:    if2Handler,
				ReturnsFn:  if2ReturnsFn, BarrierPos:

				// Clause-list form: `if [c1 b1 c2 b2 … else]`. Even elements
				// are conditions, the following odd element is that clause's
				// body, and a trailing element (odd-length list) is the
				// else. Conditions are tried left-to-right; the first truthy
				// one's body runs, the rest are not evaluated. Each element
				// may be a code-body list (evaluated / spliced) or a plain
				// value (used as-is). Must be tried after if3/if2 so the
				// legacy `if <listCond> <then> [<else>]` forms still win when
				// extra args are present. See ifClause in conditional.go.
				-1,
			},

			{
				Args:       []*Type{TList},
				NoEvalArgs: map[int]bool{0: true},
				Handler:    ifListHandler,
				ReturnsFn:  ifListReturnsFn, BarrierPos: -1,
			},
		},
	},
	{
		// case <value> [m1 b1 m2 b2 … default] — dispatch on a value.
		// The value expression is executed and its result captured
		// (a code-body list evaluates; parens evaluate before
		// collection; a plain value is used as-is). Clauses are
		// match/block pairs with an optional trailing default. A
		// match that is a code-body list executes as if the value
		// were already on the stack ([gt 3] runs `v gt 3`) and the
		// result coerces to boolean; any other match UNIFIES with
		// the value (equal scalars/atoms, a type literal matches its
		// members). The first matching clause's block runs the same
		// way — value pushed first, like the `error [handler]` block
		// — and its result is case's result. No match and no default
		// produces nothing, like `if` without an else.
		Name: "case",

		Signatures: []NativeSig{
			// One Any/Any sig; the handler disambiguates the two call
			// shapes (forward `case v [clauses]` vs stack-value
			// `v case [clauses]`) by which arg is the clause list —
			// a type-level split mis-sorts because both shapes are
			// (List, List) when the value is itself a code body.
			{
				Args:       []*Type{TAny, TAny},
				NoEvalArgs: map[int]bool{0: true, 1: true},
				Handler:    caseHandler,
				ReturnsFn:  caseReturnsFn,
				Returns:    []*Type{TAny}, BarrierPos: -1,
			},
		},
	},
	{
		// __casematch is the internal guard the compiled `case` desugar
		// emits for a non-predicate clause: `v match __casematch` applies the
		// SAME UnifyR the interpreter's caseClauses uses, so a bare-refine
		// newtype (`Pos`) matches structurally exactly as case does — which
		// the `is` word (nominal) would not. Not user-facing.
		Name: "__casematch",
		Signatures: []NativeSig{{
			Args:       []*Type{TAny, TAny},
			Handler:    caseMatchHandler,
			Returns:    []*Type{TBoolean},
			BarrierPos: 0,
		}},
	},
	{
		Name: "for",

		Signatures: []NativeSig{
			{
				Args:       []*Type{TInteger, TList},
				NoEvalArgs: map[int]bool{1: true},
				Handler:    forCountHandler,
				ReturnsFn:  forIntegerListReturnsFn, BarrierPos: -1,
			},
			{
				Args:       []*Type{TList, TList},
				NoEvalArgs: map[int]bool{1: true},
				Handler:    forRangeHandler,
				ReturnsFn:  forListListReturnsFn, BarrierPos: -1,
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
			Returns: []*Type{}, BarrierPos: 0,
		}},
	},
	{
		Name: "continue",
		Signatures: []NativeSig{{
			Handler: func(_ []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
				r.FlowCtrl = FlowContinue
				return nil, nil
			},
			Returns: []*Type{}, BarrierPos: 0,
		}},
	},
	{
		Name: "error",

		Signatures: []NativeSig{
			{
				Args:       []*Type{TList, TError},
				NoEvalArgs: map[int]bool{0: true},
				Handler:    errorHandler,
				// The error branch leaves the HANDLER's result (the
				// caught error only when the handler passes it
				// through), so the static return is Any, like the
				// success pass-through.
				Returns: []*Type{TAny}, BarrierPos: -1,
			},
			// Success pass-through: `do [risky] error [handler]` must
			// compose when risky SUCCEEDS too — a non-Error result
			// skips the handler and passes through unchanged. Without
			// this, attaching a handler made the success path a loud
			// signature_error, so the documented pattern only worked
			// when the body failed.
			{
				Args:       []*Type{TList, TAny},
				NoEvalArgs: map[int]bool{0: true},
				Handler:    errorPassHandler,
				Returns:    []*Type{TAny}, BarrierPos: -1,
			},
		},
	},
}

// ---- do handlers ----

func doListHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	if !IsConcrete(args[0]) {
		return nil, r.AqlError("do_error", "do: argument must be a concrete list, got type literal", "do")
	}
	// `do` runs its body with no per-call inputs and catches a body error,
	// surfacing it as an Error VALUE rather than propagating (the escape
	// hatch semantics). Routed through the InvokeBody seam so the VM can run
	// the body as a compiled closure.
	result, err := InvokeBody(r, args[0], nil)
	if err != nil {
		return []Value{NewError(err)}, nil
	}
	return result, nil
}

func doListReturnsFn(args []Value, r *Registry) []Value {
	body := args[0]
	if IsWord(body) {
		w, _ := AsWord(body)
		if v, ok := r.Defs.Top(w.Name); ok {
			body = v
		}
	}
	// Escape hatch: a computed body the checker cannot run statically (a
	// list carrier rather than concrete tokens) has a genuinely unknown
	// residual, so emit a bounded gradual dynamic(Any) — optimistically
	// usable downstream — rather than strict Carry<Any>.
	// (design/dynamic-modality-report.10.md, do/eval hatch.) A concrete
	// body is analyzed normally; one that runs to nothing stays strict.
	if !(IsConcrete(body) && body.Parent.ConformsTo(TList)) {
		return []Value{NewDynamicCarrier(TAny)}
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

// doEvalDataList evaluates a list value inside a `do` map as code.
// Unquoted words in the list arrive as Word values and run normally
// (`do {a:[add 1 2]}` → {a:3}); quoted strings and atoms are DATA and
// are left untouched — a `do {a:["if"]}` stores the string "if", it
// does not dispatch the `if` word. Respecting the quote is what keeps
// data values whose text happens to name a word (`"if"`, `"get"`,
// `"do"`) storable without boxing tricks (voxgig DX report T4).
func doEvalDataList(r *Registry, elems []Value) ([]Value, error) {
	sub := New(r)
	input := make([]Value, len(elems))
	copy(input, elems)
	return sub.Run(input)
}

// doEvalMapValue recursively evaluates list values within a map. Used
// by `do` to walk a map literal and evaluate any embedded code lists.
func doEvalMapValue(r *Registry, v Value) (Value, error) {
	if v.Parent.Equal(TList) && v.Data != nil && !IsTypedList(v) && !IsTableType(v) {
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
	if v.Parent.Equal(TMap) && v.Data != nil && !IsTypedMap(v) && !IsRecordType(v) && !IsOptionsType(v) {
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

// ifMarkMoveTokens builds the mark+move token sequence for a list-form `if`
// condition: the condition list is run inline (Mark…Move) and its result
// selects the branch via the IfCont. Returns (nil, false) when cond is not a
// runnable plain list, so the caller falls back to scalar-condition
// coercion. Shared by if2Handler (elseBranch=nil) and if3Handler.
func ifMarkMoveTokens(cond Value, thenBranch, elseBranch []Value) ([]Value, bool) {
	if !(cond.Parent.Equal(TList) && cond.Data != nil && !IsTypedList(cond) && !IsTableType(cond)) {
		return nil, false
	}
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
	return tokens, true
}

func if3Handler(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
	cond := args[0]
	thenBranch := spliceArg(args[1])
	elseBranch := spliceArg(args[2])

	if tokens, ok := ifMarkMoveTokens(cond, thenBranch, elseBranch); ok {
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

	if tokens, ok := ifMarkMoveTokens(cond, thenBranch, nil); ok {
		return tokens, nil
	}

	if CoerceBoolean(cond) {
		return thenBranch, nil
	}
	return nil, nil
}

func if3ReturnsFn(args []Value, r *Registry) []Value {
	es := &r.Check
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
		var stk []Value
		var defs map[string]Value
		if lit {
			restoreThen := ApplyGuardNarrowing(r, args[0])
			es.Emit.ArmBranchCapture()
			stk, defs = RunCarrierBodyWithDefs(r, args[1])
			restoreThen()
			InstallJoinedDefs(r, defs, nil)
		} else {
			restoreElse := ApplyComplementNarrowing(r, args[0])
			es.Emit.ArmBranchCapture()
			stk, defs = RunCarrierBodyWithDefs(r, args[2])
			restoreElse()
			InstallJoinedDefs(r, nil, defs)
		}
		frag := es.Emit.TakeFragment()
		if len(stk) == 0 {
			es.Emit.MarkUncompilable("if: branch produces no value (Stage 2 lowers single-result branches)")
			return nil
		}
		out := stk[len(stk)-1]
		taken := lit
		es.Emit.RecordBranch(BranchRecord{
			ConstCond: &taken, HasElse: true,
			Then: frag, ThenStk: stk, Out: out, Pos: args[0].Pos,
		})
		return []Value{out}
	}
	// List-form condition: when emitting, analyse the condition body
	// as its own fragment so the lowering can run it inline before
	// JMP_IF_FALSE (the checker otherwise never evaluates if3
	// conditions — guard extraction is syntactic). Emit-gated so
	// plain checks keep their diagnostics surface unchanged.
	condFrag, condStk := analyseCondFragment(r, args[0])
	restoreThen := ApplyGuardNarrowing(r, args[0])
	es.Emit.ArmBranchCapture()
	thenStk, thenDefs := RunCarrierBodyWithDefs(r, args[1])
	thenFrag := es.Emit.TakeFragment()
	restoreThen()
	// The else arm may be a `[…]` code body (captured as its own fragment)
	// or an already-evaluated VALUE (`if cond [then] 42` — a literal, a
	// def-bound value, a paren result). For a non-body else, do NOT arm a
	// capture (there is no body to run); pass the value through so the
	// lowering pushes it directly in the else arm.
	elseIsBody := IsConcrete(args[2]) && args[2].Parent.ConformsTo(TList)
	var elseFrag *EmitFragment
	var elseStk []Value
	var elseDefs map[string]Value
	var elseValue *Value
	if elseIsBody {
		restoreElse := ApplyComplementNarrowing(r, args[0])
		es.Emit.ArmBranchCapture()
		elseStk, elseDefs = RunCarrierBodyWithDefs(r, args[2])
		elseFrag = es.Emit.TakeFragment()
		restoreElse()
	} else {
		v := args[2]
		elseValue = &v
		elseStk = []Value{v}
	}
	InstallJoinedDefs(r, thenDefs, elseDefs)
	joined := JoinCarrierStacks(thenStk, elseStk)
	if len(joined) == 0 {
		es.Emit.MarkUncompilable("if: branches produce no value (Stage 2 lowers single-result branches)")
		return nil
	}
	out := joined[len(joined)-1]
	es.Emit.RecordBranch(BranchRecord{
		Cond: args[0], CondFrag: condFrag, CondStk: condStk, HasElse: true,
		Then: thenFrag, Els: elseFrag, ThenStk: thenStk, ElsStk: elseStk,
		ElsValue: elseValue, Out: out, Pos: args[0].Pos,
	})
	return []Value{out}
}

// analyseCondFragment captures a list-form `if` condition body as an
// emit fragment (nil when the condition is a pre-evaluated value, or
// when no bytecode recording is active).
func analyseCondFragment(r *Registry, cond Value) (*EmitFragment, []Value) {
	es := r.Check.Emit
	if es == nil || !IsConcrete(cond) || !cond.Parent.ConformsTo(TList) {
		return nil, nil
	}
	es.ArmBranchCapture()
	stk, _ := RunCarrierBodyWithDefs(r, cond)
	return es.TakeFragment(), stk
}

func if2ReturnsFn(args []Value, r *Registry) []Value {
	es := &r.Check
	if lit, ok := LiteralCondValue(args[0]); ok && !lit {
		r.Check.AddDiagnostic(CheckDiagnostic{
			Code:     "unreachable_branch",
			Detail:   "if condition is a constant false; then-branch is unreachable",
			Severity: SeverityWarning,
		})
	}
	condFrag, condStk := analyseCondFragment(r, args[0])
	restore := ApplyGuardNarrowing(r, args[0])
	es.Emit.ArmBranchCapture()
	thenStk, thenDefs := RunCarrierBodyWithDefs(r, args[1])
	thenFrag := es.Emit.TakeFragment()
	restore()
	InstallJoinedDefs(r, thenDefs, nil)
	var out Value
	if len(thenStk) == 0 {
		out = NewCarrier(TNone)
	} else {
		out = JoinCarriers(thenStk[len(thenStk)-1], NewCarrier(TNone))
	}
	// 2-arg if: a VARIADIC result (0 or 1 values at run time).
	es.Emit.RecordBranch(BranchRecord{
		Cond: args[0], CondFrag: condFrag, CondStk: condStk, HasElse: false,
		Then: thenFrag, ThenStk: thenStk, Out: out, Pos: args[0].Pos,
	})
	return []Value{out}
}

// ifListHandler implements the clause-list form `if [c1 b1 c2 b2 … else]`.
// It hands the (raw, NoEval'd) list's elements to ifClause, which produces
// the token stream the engine then runs.
func ifListHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	if !IsConcrete(args[0]) {
		return nil, r.AqlError("if_error", "if: clause-list argument must be a concrete list, got a type literal", "if")
	}
	_lst, _ := AsList(args[0])
	return ifClause(_lst.Slice()), nil
}

// caseHandler implements both call shapes of `case`:
//
//	case <value> [m1 b1 … default]    forward form (canonical)
//	<value> case [m1 b1 … default]    stack-value form
//
// Disambiguation: in the forward form args[0]=value, args[1]=clause
// list; in the stack form the matcher delivers args[0]=clause list
// (forward) and args[1]=value (stack). When exactly one arg is a
// plain list it is the clause list; when BOTH are lists the forward
// reading wins (args[0] is the value — so `case [1 add 1] [clauses]`
// evaluates the code body; dispatching on a LIST value requires the
// forward form). The value, if a code body, is executed and its LAST
// result captured — it must produce one, loudly.
func caseHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	v, clauses := args[0], args[1]
	if isCodeBody(v) && !isCodeBody(clauses) {
		v, clauses = clauses, v
	}
	if isCodeBody(v) {
		sub := New(r)
		lst, _ := AsList(v)
		input := make([]Value, lst.Len())
		copy(input, lst.Slice())
		out, err := sub.Run(input)
		if err != nil {
			return nil, err
		}
		if len(out) == 0 {
			return nil, r.AqlError("case_error",
				"case: value expression produced no value to dispatch on", "case")
		}
		v = out[len(out)-1]
	}
	if !isCodeBody(clauses) {
		return nil, r.AqlError("case_error",
			"case: clause list must be a concrete list of match/block pairs (optional trailing default)", "case")
	}
	lst, _ := AsList(clauses)
	return caseClauses(r, v, lst.Slice())
}

// ifListReturnsFn type-checks the clause-list form: the result is the
// join of every clause body's last value plus the else clause (or None
// when there is no else, since an unmatched `if` produces nothing).
// Condition bodies are still run for their diagnostics but don't
// contribute to the return type. Unlike if3/if2 this does no per-clause
// guard narrowing — multi-clause narrowing isn't modelled.
func ifListReturnsFn(args []Value, r *Registry) []Value {
	if !IsConcrete(args[0]) || !args[0].Parent.Equal(TList) {
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
		return nil, r.AqlError("for_error", "for: range must be a concrete list, got type literal", "for")
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
	return forCarrierAnalyse(r, "i", TInteger, args, 0)
}

func forListListReturnsFn(args []Value, r *Registry) []Value {
	return forCarrierAnalyse(r, "i", TInteger, args, 0)
}

// forCarrierAnalyse analyses the body to a bounded fixed point with
// the iterator bound as a typed carrier (AnalyseLoopBody —
// design/checker-accuracy-review.0.md A4): body rebindings like
// `def acc (acc add 0.5)` join back into the enclosing binding and
// the body re-runs until the bindings stabilise, so post-loop reads
// see Integer|Float, not the pre-loop Integer. Returns a typed list
// whose element type mirrors the final round's residual top.
//
// countArg >= 0 names the count/range operand and arms bytecode loop
// recording: the final round's events are captured as a fragment and
// RecordLoop lowers the loop (FOR_SETUP/FOR_NEXT with the iterator
// as a VM local). The count form lowers as the range [0, n, 1]; the
// range form decomposes a LITERAL integer range via parseRange
// (computed ranges record nothing and the generic path refuses).
func forCarrierAnalyse(r *Registry, iterName string, iterType *Type, args []Value, countArg int) []Value {
	body := args[len(args)-1]
	iter := NewCarrier(iterType)
	es := r.Check.Emit

	// Decompose the loop bounds for recording.
	var startV, endV, stepV Value
	lowerable := false
	if countArg >= 0 {
		cv := args[countArg]
		switch {
		case cv.Parent.ConformsTo(TInteger):
			startV, endV, stepV = NewInteger(0), cv, NewInteger(1)
			lowerable = true
		case IsConcrete(cv) && cv.Parent.ConformsTo(TList):
			if lst, err := AsList(cv); err == nil && !lst.IsNil() {
				if st, en, sp, perr := parseRange(lst.Slice()); perr == nil {
					startV, endV, stepV = NewInteger(st), NewInteger(en), NewInteger(sp)
					lowerable = true
				}
			}
		}
	}
	if lowerable {
		es.ArmLoopCapture()
	}
	stk := AnalyseLoopBody(r, body, []string{iterName}, []Value{iter})
	out := NewCarrier(TList)
	if len(stk) > 0 {
		top := stk[len(stk)-1]
		if IsDisjunct(top) {
			out = NewCarrierTypedListValue(top)
		} else {
			out = NewCarrierTypedList(top.Parent)
		}
	}
	if lowerable {
		frag := es.TakeFragment()
		es.RecordLoop(startV, endV, stepV, frag, stk, iter.ID, out, args[countArg].Pos)
	}
	return []Value{out}
}

// ---- error handler ----

// errorPassHandler is `error`'s success path: the guarded body
// produced a normal value, so the handler list is discarded and the
// value passes through unchanged.
func errorPassHandler(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
	return []Value{args[1]}, nil
}

func errorHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	if !IsConcrete(args[0]) {
		return nil, r.AqlError("error_error", "error: handler must be a concrete list, got type literal", "error")
	}
	sub := New(r)
	_lst, _ := AsList(args[0])
	body := _lst.Slice()
	input := make([]Value, 0, 1+len(body))
	input = append(input, args[1])
	input = append(input, body...)
	out, err := sub.Run(input)
	if err != nil {
		return nil, err
	}
	// Stack-neutrality (decision DX report finding 6): the caught error
	// is PUSHED so the handler can bind it (`var [[e] …]`, `get code`,
	// `dup`, …), but a handler that ignores it must not leak it beneath
	// its result — `def r do [risky] error ["fallback"]` used to bind r
	// to the error's neighbour and auto-print the stray error. If the
	// pushed error is still sitting unconsumed at the BOTTOM of the
	// handler's stack, strip it, so the error branch leaves exactly the
	// handler's result — mirroring the success pass-through. (A bare
	// `error []` handler keeps the error as the result: pass-through.
	// The identity probe compares ErrorInfo payloads; a non-ErrorInfo
	// bottom is a different dynamic type and compares false.)
	if len(out) >= 2 && out[0].Data == args[1].Data {
		out = out[1:]
	}
	return out, nil
}
