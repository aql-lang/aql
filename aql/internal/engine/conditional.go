package engine

// spliceArg returns tokens for a branch value. If the value is a list,
// its elements are returned wrapped in parens so the main engine evaluates
// them as a sub-expression. Scalars are returned as-is.
func spliceArg(v Value) []Value {
	if v.VType.Equal(TList) && v.Data != nil && !v.IsTypedList() && !v.IsTableType() {
		elems := v.AsList()
		result := make([]Value, 0, elems.Len()+2)
		result = append(result, NewOpenParen())
		result = append(result, elems.Slice()...)
		result = append(result, NewWord(")"))
		return result
	}
	return []Value{v}
}

func RegisterIf(r *Registry) {
	// if: [any, any, any] -> [any] — 3-arg
	// "if cond then else": args=[cond, then, else]
	if3Handler := func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
		cond := args[0]
		thenBranch := spliceArg(args[1])
		elseBranch := spliceArg(args[2])

		// If condition is a list, use mark/move to evaluate it in-place.
		if cond.VType.Equal(TList) && cond.Data != nil && !cond.IsTypedList() && !cond.IsTableType() {
			condSlice := cond.AsList().Slice()
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

		// Scalar condition: evaluate immediately.
		if CoerceBoolean(cond) {
			return thenBranch, nil
		}
		return elseBranch, nil
	}

	// if: [any, any] -> [any] — 2-arg
	// "if cond then": args=[cond, then]
	if2Handler := func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
		cond := args[0]
		thenBranch := spliceArg(args[1])

		// If condition is a list, use mark/move to evaluate it in-place.
		if cond.VType.Equal(TList) && cond.Data != nil && !cond.IsTypedList() && !cond.IsTableType() {
			condSlice := cond.AsList().Slice()
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

		// Scalar condition: evaluate immediately.
		if CoerceBoolean(cond) {
			return thenBranch, nil
		}
		return nil, nil
	}

	r.RegisterNativeFunc(NativeFunc{
		Name:              "if",
		ForwardPrecedence: true,
		Signatures: []NativeSig{
			{
				Args:       []Type{TAny, TAny, TAny},
				NoEvalArgs: map[int]bool{0: true, 1: true, 2: true},
				Handler:    if3Handler,
				// Branch-aware check: analyse both branches in a
				// sub-engine in check mode, then widen the top-of-
				// stack carriers into a disjunction. args[0]=cond
				// (already a carrier; ignored), args[1]=then body,
				// args[2]=else body. Forward precedence + arg order
				// means both branches are parser-produced code
				// lists with Eval=true (NoEvalArgs suppresses
				// auto-eval so we get the raw body).
				ReturnsFn: func(args []Value) []Value {
					// Flow typing: detect `x is Type` in the
					// condition and narrow x in the then-branch;
					// apply the complement in the else-branch.
					// Def-joining: any new defs either branch
					// creates are snapshotted/restored per branch,
					// then joined via InstallJoinedDefs so a
					// following word sees the post-if binding.
					//
					// Unreachable-branch warning: when the
					// condition is a statically known boolean
					// literal, the opposite branch never runs.
					// Emit a warning and skip that branch's
					// contribution so downstream types stay tight.
					if lit, ok := LiteralCondValue(args[0]); ok {
						branch := "else"
						if !lit {
							branch = "then"
						}
						r.AddCheckDiagnostic(CheckDiagnostic{
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
				},
			},
			{
				Args:       []Type{TAny, TAny},
				NoEvalArgs: map[int]bool{0: true, 1: true},
				Handler:    if2Handler,
				// 2-arg if: no else branch means "then or nothing".
				// Widen the then-branch top with TNone. Any defs the
				// then-branch created are joined with the pre-branch
				// binding (or dropped if the name wasn't bound
				// before) — mirrors the 3-arg semantics.
				ReturnsFn: func(args []Value) []Value {
					if lit, ok := LiteralCondValue(args[0]); ok && !lit {
						r.AddCheckDiagnostic(CheckDiagnostic{
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
				},
			},
		},
	})
}
