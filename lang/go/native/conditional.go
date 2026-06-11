package native

// spliceArg returns tokens for a branch value. If the value is a list,
// its elements are returned wrapped in parens so the main engine evaluates
// them as a sub-expression. Scalars are returned as-is.
func spliceArg(v Value) []Value {
	if v.Parent.Equal(TList) && v.Data != nil && !IsTypedList(v) && !IsTableType(v) {
		elems, _ := AsList(v)
		result := make([]Value, 0, elems.Len()+2)
		result = append(result, NewOpenParen())
		result = append(result, elems.Slice()...)
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

// runCaseBody executes a case block (or default): a code-body list
// runs in a sub-engine with the captured value pushed first — the
// same convention as the `error [handler]` block — so the block can
// consume it; any other value is the result as-is.
func runCaseBody(r *Registry, v Value, body Value) ([]Value, error) {
	if !isCodeBody(body) {
		return []Value{body}, nil
	}
	sub := New(r)
	lst, _ := AsList(body)
	input := make([]Value, 0, 1+lst.Len())
	input = append(input, v)
	input = append(input, lst.Slice()...)
	return sub.Run(input)
}
