package native

// spliceArg returns tokens for a branch value. If the value is a list,
// its elements are returned wrapped in parens so the main engine evaluates
// them as a sub-expression. Scalars are returned as-is.
func spliceArg(v Value) []Value {
	if v.VType.Equal(TList) && v.Data != nil && !IsTypedList(v) && !IsTableType(v) {
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
	return v.VType.Equal(TList) && v.Data != nil && !IsTypedList(v) && !IsTableType(v)
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
