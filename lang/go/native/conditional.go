package native

// spliceArg returns tokens for a branch value. If the value is a list,
// its elements are returned wrapped in parens so the main engine evaluates
// them as a sub-expression. Scalars are returned as-is.
func spliceArg(v Value) []Value {
	if v.Parent.Equal(TList) && v.Data != nil && !IsTypedList(v) && !IsTableType(v) {
		elems, _ := AsList(v)
		// Append element-wise: Slice() would copy the backing once into a
		// throwaway slice and append would copy it AGAIN into result —
		// this runs per selected if/for branch, so the intermediate copy
		// was a per-branch allocation.
		result := make([]Value, 0, elems.Len()+2)
		result = append(result, NewOpenParen())
		for i := 0; i < elems.Len(); i++ {
			result = append(result, elems.Get(i))
		}
		result = append(result, NewCloseParen())
		return result
	}
	return []Value{v}
}

// isCodeBody reports whether v is a plain (non-typed, non-table) concrete
// list — i.e. something to be evaluated as a code body rather than used
// as a literal value.
func isCodeBody(v Value) bool {
	return v.Parent.Equal(TList) && v.Data != nil && !IsTypedList(v) && !IsTableType(v)
}

// ifClause turns the element slice of a clause-list `[c1 b1 c2 b2 … else]`
// into the token stream the engine should run for it.
//
// Walking two at a time: elems[2k] is a condition, elems[2k+1] is that
// clause's body; a trailing odd element is the else clause. A condition
// that is a code-body list is evaluated lazily via the mark / move-if
// machinery (so later clauses don't run once one matches and so side
// effects in the condition only happen when reached); a scalar condition
// is decided immediately with CoerceBoolean. A body is spliced via
// spliceArg (code-body list → `( … )`, scalar → as-is).
//
// Empty slice → no tokens. One element → just that element's tokens (a
// lone else). The else branch of clause k is, recursively, ifClause of
// elems[2k+2:].
func ifClause(elems []Value) []Value {
	switch len(elems) {
	case 0:
		return nil
	case 1:
		return spliceArg(elems[0])
	}

	cond := elems[0]
	thenBranch := spliceArg(elems[1])
	elseBranch := ifClause(elems[2:])

	if isCodeBody(cond) {
		_lst, _ := AsList(cond)
		condSlice := _lst.Slice()
		id := NextMarkID()
		tokens := make([]Value, 0, len(condSlice)+2)
		tokens = append(tokens, NewMark(id, condSlice...))
		tokens = append(tokens, condSlice...)
		tokens = append(tokens, NewMoveIf(id, "if", &IfCont{Then: thenBranch, Else: elseBranch}))
		return tokens
	}

	if CoerceBoolean(cond) {
		return thenBranch
	}
	return elseBranch
}

// caseClauses runs the `case` word's clause walk (see caseHandler):
// v is the captured value; elems are the raw clause-list elements —
// match/block pairs with an optional trailing default. Returns the
// matched block's result stack.
func caseClauses(r *Registry, v Value, elems []Value) ([]Value, error) {
	i := 0
	for ; i+1 < len(elems); i += 2 {
		match := elems[i]
		matched := false
		if isCodeBody(match) {
			// Predicate match: the match list executes as if the value
			// were already on the stack ([gt 3] runs as `v gt 3`), and
			// the result coerces to boolean.
			out, err := runCaseBody(r, v, match)
			if err != nil {
				return nil, err
			}
			if len(out) > 0 {
				matched = CoerceBoolean(out[len(out)-1])
			}
		} else {
			// Value / type match: the clause unifies with the value —
			// equal scalars and atoms match, a type literal (builtin or
			// def'd) matches its members, a map pattern matches
			// structurally. A bare word resolves through the def table
			// first so user types and def'd values work as matches.
			m := match
			if IsWord(m) {
				w, _ := AsWord(m)
				if bound, ok := r.ResolveTypedName(w.Name); ok {
					m = bound
				} else {
					m = ResolveWordValue(m)
				}
			}
			// A parenthesised match — `case b [(Box of [Integer]) […]]`
			// — evaluates inline, the same contract paren annotations
			// follow in typed defs. Generic instantiations are the main
			// client; any expression producing one value works. An
			// evaluation error or multi-value result keeps the raw
			// ParenExpr (which then simply fails to unify).
			if IsParenExpr(m) {
				if toks, perr := AsParenExpr(m); perr == nil {
					input := make([]Value, 0, len(toks)+2)
					input = append(input, NewOpenParen())
					input = append(input, toks...)
					input = append(input, NewCloseParen())
					sub := New(r)
					if out, rerr := sub.Run(input); rerr == nil && len(out) == 1 {
						m = out[0]
					}
				}
			}
			_, ok := UnifyR(m, v, r)
			matched = ok
		}
		if matched {
			return runCaseBody(r, v, elems[i+1])
		}
	}
	if i < len(elems) {
		// Trailing odd element: the default clause.
		return runCaseBody(r, v, elems[i])
	}
	return nil, nil
}

// caseReturnsFn type-checks a `case` and, when bytecode emission is active,
// desugars it to a nested-`if` chain so it compiles natively instead of
// refusing as a code-body word (design doc "case clause compilation"). Each
// clause becomes `if (v match __casematch) [block] [rest]`; a code-body
// predicate match `[pred]` becomes the guard `(v pred…)`; a block runs with
// v pushed first (mirroring runCaseBody).
//
// It compiles ONLY shapes it can statically prove the single-result branch
// lowering will take — a re-pushable case value (`OperandRepushable`: tested
// against every clause guard, a computed event could not be re-pushed inside
// the else fragments) and a trailing default (so the innermost else is a
// definite value, not a 2-arg-if variadic). Every other shape returns the
// prior conservative dynamic-Any WITHOUT marking the program uncompilable, so
// the island / whole-program fallback keeps owning it and refusals never
// rise. Faithfulness rides the differential gate (runtime stays
// caseHandler/caseClauses; __casematch reuses its UnifyR).
func caseReturnsFn(args []Value, r *Registry) []Value {
	dynAny := []Value{NewDynamicCarrier(TAny)}
	v, clauses := args[0], args[1]
	if isCodeBody(v) && !isCodeBody(clauses) {
		v, clauses = clauses, v
	}
	if isCodeBody(v) {
		// A code-body scrutinee (`case [body] [clauses]`) runs the body and
		// dispatches on its last result. In compile mode:
		//   - 0-net body → the run-time case_error (caseHandler): record a TERMINAL
		//     OpTrap raising the byte-identical error. The residual count must come
		//     from the RECORDING analyseCondFragment — a type-only RunCarrierBody
		//     gives an empty-body 0-return fn (`[f 1]`) a bogus 1-value count that
		//     would miss the trap.
		//   - 1+-net body, a SINGLE clause + default, both blocks VALUES
		//     (`case [1 add 1] [2 "two" "other"]`) → desugar to a nested `if` whose
		//     guard runs the body ONCE via `do`: `if (do [body] m __casematch)
		//     [then] [default]`. Because there is exactly ONE guard and the blocks
		//     are values, `do [body]` is evaluated exactly once — so this needs no
		//     scrutinee seating and is sound for ANY body (no recompute). Any other
		//     shape (multi-clause, code-body blocks, no default) keeps the island.
		// Plain check (no emit) keeps the prior dynAny.
		if es := r.Check.Recorder(); es.Active() {
			nDiag := len(r.Check.Diagnostics)
			_, stk := analyseCondFragment(r, v)
			r.Check.TruncateDiagnostics(nDiag)
			if len(stk) == 0 {
				es.RecordTrap("case_error",
					"case: value expression produced no value to dispatch on",
					"case", "", v.Pos())
				return dynAny
			}
			if isCodeBody(clauses) {
				if lst, _ := AsList(clauses); !lst.IsNil() {
					elems := lst.Slice()
					if len(elems) == 3 && !isCodeBody(elems[1]) && !isCodeBody(elems[2]) {
						// `[do]` + the normal guard tokens: do runs the body once,
						// leaving its value as the scrutinee for the match.
						cond := NewList(append([]Value{NewWord("do")}, caseGuardTokens(v, elems[0])...))
						then := NewList(caseBlockTokens(v, elems[1]))
						rest := NewList(caseBlockTokens(v, elems[2]))
						return if3ReturnsFn([]Value{cond, then, rest}, r)
					}
				}
			}
		}
		return dynAny
	}
	if !isCodeBody(clauses) {
		// The clause argument is not a list: caseHandler raises case_error. The
		// checker is lenient (returns a carrier), but compiled mode raises the
		// byte-identical error via a TERMINAL OpTrap instead of islanding — the
		// trap keeps the events before it, drops the case dispatch (which would
		// otherwise island), and aborts exactly where the interpreter does. A
		// nested case (RecordTrap declines, frames/units != 1) keeps the island.
		r.Check.Recorder().RecordTrap("case_error",
			"case: clause list must be a concrete list of match/block pairs (optional trailing default)",
			"case", "", args[0].Pos())
		return dynAny
	}
	lst, _ := AsList(clauses)
	if lst.IsNil() {
		return dynAny
	}
	elems := lst.Slice()
	// At least one clause pair. A trailing default (odd length) makes the
	// innermost else a definite value; WITHOUT one (even length) the innermost
	// `if guard [block]` has no else and the chain is variadic (0-or-1), which
	// matches case's no-match semantics (it produces NO value) — the nested-
	// variadic branch lowering propagates that 0-or-1 to the residual.
	if len(elems) < 2 {
		return dynAny
	}
	// COMPILE desugar to a nested-`if` chain — ONLY when emission is active and
	// the case value can be seated inside every clause guard / block fragment it
	// is re-tested in. CanSeatAcrossFragment admits a const / local / type node
	// directly, and a COMPUTED top-level scrutinee (`case (1 add 1) […]`) that
	// planValueDefLocals then promotes to a frame local once the fragment reads
	// are recorded. if3ReturnsFn returns the branch-join type AND records the
	// lowering.
	if r.Check.Recorder().CanSeatAcrossFragment(v) {
		cond := NewList(caseGuardTokens(v, elems[0]))
		then := NewList(caseBlockTokens(v, elems[1]))
		rest := buildCaseChain(v, elems, 2)
		return if3ReturnsFn([]Value{cond, then, NewList(rest)}, r)
	}
	// Otherwise the island / whole-program fallback owns COMPILATION, but the
	// result TYPE is still computable: the join of every clause block's
	// residual type plus the trailing default. Decoupling the type from the
	// compile-eligibility lets a plain `aql check` (which has no emit state)
	// and a non-re-pushable case narrow instead of poisoning the result with
	// Any.
	return caseBranchJoin(r, v, elems)
}

// caseBranchJoin computes a `case` result TYPE as the join of every clause
// BLOCK's residual carrier (a block run with the case value pushed, per
// caseBlockTokens) plus the trailing default — the abstract analogue of the
// nested-`if` chain's branch join, without the emit desugar. A block that
// produces no value contributes None. Type-only: RunCarrierBody pauses any
// active recording, so this is safe whether or not emission is live.
func caseBranchJoin(r *Registry, v Value, elems []Value) []Value {
	var out Value
	have := false
	join := func(block Value) {
		stk := RunCarrierBody(r, NewList(caseBlockTokens(v, block)))
		res := NewCarrier(TNone)
		if len(stk) > 0 {
			res = stk[len(stk)-1]
		}
		if have {
			out = JoinCarriers(out, res)
		} else {
			out, have = res, true
		}
	}
	i := 0
	for ; i+1 < len(elems); i += 2 {
		join(elems[i+1]) // clause block (odd-indexed; the even index is the match)
	}
	if i < len(elems) {
		join(elems[i]) // trailing default block
	}
	if !have {
		return []Value{NewDynamicCarrier(TAny)}
	}
	return []Value{out}
}

// caseGuardTokens builds the guard body for one clause: a code-body
// predicate `[pred]` runs as `v pred…` (matching runCaseBody), any other
// match dispatches `v match __casematch` (the same UnifyR caseClauses uses).
func caseGuardTokens(v Value, m Value) []Value {
	if isCodeBody(m) {
		ml, _ := AsList(m)
		return append([]Value{v}, ml.Slice()...)
	}
	return []Value{v, m, NewWord("__casematch")}
}

// caseBlockTokens returns the tokens a clause block contributes: a code body
// runs with v pushed first (runCaseBody's convention), a plain value is
// itself.
func caseBlockTokens(v Value, b Value) []Value {
	if isCodeBody(b) {
		bl, _ := AsList(b)
		return append([]Value{v}, bl.Slice()...)
	}
	return []Value{b}
}

// buildCaseChain builds the token stream for the nested-`if` form of the
// clause list from index i: `if (v match __casematch) [block] [<rest>]`,
// recursing into the else body so a later clause only evaluates when the
// earlier guard fails. A trailing odd element is the default block.
func buildCaseChain(v Value, elems []Value, i int) []Value {
	if i+1 >= len(elems) {
		if i < len(elems) {
			return caseBlockTokens(v, elems[i]) // trailing default
		}
		return nil
	}
	guard := []Value{NewOpenParen()}
	guard = append(guard, caseGuardTokens(v, elems[i])...)
	guard = append(guard, NewCloseParen())
	out := []Value{NewWord("if")}
	out = append(out, guard...)
	out = append(out, NewList(caseBlockTokens(v, elems[i+1]))) // then [block]
	if rest := buildCaseChain(v, elems, i+2); rest != nil {
		out = append(out, NewList(rest)) // else [rest] — lazy
	}
	return out
}

// caseMatchHandler is the runtime of __casematch: UnifyR(match, value) → ok.
// args[0] is the match (stack top), args[1] the case value (BarrierPos 0).
func caseMatchHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	_, ok := UnifyR(args[0], args[1], r)
	return []Value{NewBoolean(ok)}, nil
}

// runCaseBody executes a case block (or default): a code-body list
// runs in a sub-engine with the captured value pushed first — the
// same convention as the `error [handler]` block — so the block can
// consume it; any other value is the result as-is.
func runCaseBody(r *Registry, v Value, body Value) ([]Value, error) {
	if !isCodeBody(body) {
		return []Value{body}, nil
	}
	lst, _ := AsList(body)
	// The captured value is a resolved input: it enters as stack data the
	// block consumes, never re-stepped (arguments are inert; RunResolved).
	return RunResolved(r, []Value{v}, lst.Slice())
}
