package engine

import "fmt"

// registerFn registers the "fn" word, which parses a list of signature
// triples into a function value (lambda). Used standalone as a lambda:
//
//	(fn [[x:number] [number] [x mul x]])
//
// Or with def to bind a name:
//
//	def square fn [[x:number] [number] [x mul x]]
//
// When the list length is divisible by 2 but not 3, it is parsed as pairs
// (input+output, no body) producing a FnUndefInfo for targeted undef.
func registerFn(r *Registry) {
	fnHandler := func(args []Value) ([]Value, error) {
		list := args[0]
		if !list.VType.Equal(TList) {
			return nil, fmt.Errorf("fn: argument must be a list")
		}
		elems := list.AsList()
		if len(elems) == 0 {
			return nil, fmt.Errorf("fn: list must not be empty")
		}
		// Triples (def mode) take precedence when divisible by 3.
		if len(elems)%3 == 0 {
			fnDef, err := parseFnDef(elems)
			if err != nil {
				return nil, err
			}
			return []Value{NewFunction(fnDef)}, nil
		}
		// Pairs (undef mode) when divisible by 2.
		if len(elems)%2 == 0 {
			undefInfo, err := parseFnUndefSpec(elems)
			if err != nil {
				return nil, err
			}
			return []Value{NewFnUndef(undefInfo)}, nil
		}
		return nil, fmt.Errorf("fn: list length must be a multiple of 3 (def) or 2 (undef spec)")
	}

	r.Register("fn", Signature{
		Args:    []Type{TList},
		Handler: fnHandler,
	})
}

// parseFnDef parses a function specification list into FnDefInfo.
// The list contains signature triples: [input-sig, output-sig, body] ...
// Each element of a triple may be abbreviated: a non-list value is treated
// as a single-element list (e.g., `string` is equivalent to `[string]`).
func parseFnDef(list []Value) (FnDefInfo, error) {
	var sigs []FnSig
	for i := 0; i < len(list); i += 3 {
		inputSig := list[i]
		outputSig := list[i+1]
		body := list[i+2]

		// Abbreviation: non-list input sig is treated as [inputSig].
		if !inputSig.VType.Equal(TList) {
			inputSig = NewList([]Value{inputSig})
		}

		params, err := parseFnParams(inputSig)
		if err != nil {
			return FnDefInfo{}, err
		}

		returns, err := parseFnReturns(outputSig)
		if err != nil {
			return FnDefInfo{}, err
		}

		// Abbreviation: non-list body is treated as [body].
		var bodyElems []Value
		if body.VType.Equal(TList) {
			bodyElems = body.AsList()
		} else {
			bodyElems = []Value{body}
		}

		sigs = append(sigs, FnSig{
			Params:  params,
			Returns: returns,
			Body:    bodyElems,
		})
	}
	return FnDefInfo{Sigs: sigs}, nil
}

// parseFnUndefSpec parses a list of signature pairs (input+output, no body)
// into a FnUndefInfo for targeted undef. Used when fn receives a list whose
// length is divisible by 2 but not 3.
func parseFnUndefSpec(list []Value) (FnUndefInfo, error) {
	var sigs []FnSigSpec
	for i := 0; i < len(list); i += 2 {
		inputSig := list[i]
		outputSig := list[i+1]

		if !inputSig.VType.Equal(TList) {
			inputSig = NewList([]Value{inputSig})
		}

		params, err := parseFnParams(inputSig)
		if err != nil {
			return FnUndefInfo{}, err
		}

		returns, err := parseFnReturns(outputSig)
		if err != nil {
			return FnUndefInfo{}, err
		}

		sigs = append(sigs, FnSigSpec{
			Params:  params,
			Returns: returns,
		})
	}
	return FnUndefInfo{Sigs: sigs}, nil
}

// parseFnReturns extracts return types from an output signature.
// A non-list value is treated as a single-element list.
// An empty list means no return type checking.
func parseFnReturns(outputSig Value) ([]Type, error) {
	if !outputSig.VType.Equal(TList) {
		// Abbreviation: single value treated as [value].
		t, _, err := resolveSigType(outputSig)
		if err != nil {
			return nil, err
		}
		return []Type{t}, nil
	}
	elems := outputSig.AsList()
	if len(elems) == 0 {
		return nil, nil
	}
	types := make([]Type, len(elems))
	for i, e := range elems {
		var err error
		types[i], _, err = resolveSigType(e)
		if err != nil {
			return nil, err
		}
	}
	return types, nil
}

// parseFnParams extracts parameters from an input signature list.
// Each element is either:
//   - A map with one key (named param from pair syntax): {x: type}
//   - A word (unnamed param): type name
//   - A type literal (Data==nil): already resolved type
func parseFnParams(inputSig Value) ([]FnParam, error) {
	if !inputSig.VType.Equal(TList) {
		return nil, fmt.Errorf("function spec: input signature must be a list")
	}
	elems := inputSig.AsList()
	var params []FnParam

	for _, elem := range elems {
		switch {
		case elem.VType.Equal(TMap):
			m := elem.AsMap()
			if m.Implicit {
				// Named parameter from implicit pair syntax: [x:Integer]
				keys := m.Keys()
				if len(keys) != 1 {
					return nil, fmt.Errorf("function spec: parameter map must have exactly one key")
				}
				name := keys[0]
				typeVal, _ := m.Get(name)
				paramType, pattern, err := resolveSigType(typeVal)
				if err != nil {
					return nil, fmt.Errorf("function spec: invalid type for %q: %w", name, err)
				}
				params = append(params, FnParam{Name: name, Type: paramType, Pattern: pattern})
			} else {
				// Explicit map: unnamed parameter with structural pattern
				paramType, pattern, err := resolveSigType(elem)
				if err != nil {
					return nil, fmt.Errorf("function spec: invalid map param: %w", err)
				}
				params = append(params, FnParam{Type: paramType, Pattern: pattern})
			}

		case elem.IsWord():
			// Unnamed parameter: bare word is a type name
			typeName := elem.AsWord().Name
			paramType, err := resolveTypeName(typeName)
			if err != nil {
				return nil, fmt.Errorf("function spec: invalid type %q: %w", typeName, err)
			}
			params = append(params, FnParam{Type: paramType})

		case elem.Data == nil:
			// Type literal (already resolved by parser)
			params = append(params, FnParam{Type: elem.VType})

		case elem.VType.Matches(TInteger):
			// Integer literal as type constraint (e.g., 0 matches number/integer/0)
			params = append(params, FnParam{Type: elem.VType})

		case elem.VType.Matches(TBoolean):
			// Boolean literal as type constraint
			params = append(params, FnParam{Type: elem.VType})

		case elem.VType.Matches(TString):
			// String literal as type constraint
			params = append(params, FnParam{Type: elem.VType})

		default:
			return nil, fmt.Errorf("function spec: invalid parameter: %s", elem.String())
		}
	}

	return params, nil
}

// resolveSigType converts a Value (from a pair's value side) to a Type.
// The second return value is a non-nil pattern when the value is a map or list
// literal that requires structural matching beyond basic type checking.
func resolveSigType(v Value) (Type, *Value, error) {
	if v.Data == nil {
		// Type literal (e.g., number, string) — already resolved by parser
		return v.VType, nil, nil
	}
	if v.IsWord() {
		t, err := resolveTypeName(v.AsWord().Name)
		return t, nil, err
	}
	if v.VType.Matches(TString) {
		t, err := resolveTypeName(v.AsString())
		return t, nil, err
	}
	// Atoms (unquoted text in data context) may be type names.
	if v.VType.Matches(TAtom) {
		t, err := resolveTypeName(v.AsString())
		return t, nil, err
	}
	// Literal values (integers, booleans) carry their literal type.
	if v.VType.Matches(TInteger) || v.VType.Matches(TBoolean) {
		return v.VType, nil, nil
	}
	// Map/list literals: match by type and store pattern for structural unification.
	if v.VType.Equal(TMap) {
		return TMap, &v, nil
	}
	if v.VType.Equal(TList) {
		return TList, &v, nil
	}
	return TAny, nil, nil
}

// resolveTypeName maps a type name string to its engine Type.
func resolveTypeName(name string) (Type, error) {
	switch name {
	case "Any":
		return TAny, nil
	case "None":
		return TNone, nil
	case "Number":
		return TNumber, nil
	case "Integer":
		return TInteger, nil
	case "Decimal":
		return TDecimal, nil
	case "String":
		return TString, nil
	case "Boolean":
		return TBoolean, nil
	case "List":
		return TList, nil
	case "Function":
		return TFunction, nil
	case "Map":
		return TMap, nil
	default:
		return NewType(name)
	}
}

// installFnDef registers typed signatures for a function definition.
// For each signature, it creates a handler that binds named parameters
// via installDef, returns body tokens, and appends undef cleanup.
func installFnDef(r *Registry, name string, fnDef FnDefInfo, prefixOnly ...bool) {
	isPrefixOnly := len(prefixOnly) > 0 && prefixOnly[0]
	registerFn := r.Register
	if isPrefixOnly {
		registerFn = r.RegisterPrefixOnly
	}
	for _, sig := range fnDef.Sigs {
		argTypes := make([]Type, len(sig.Params))
		var patterns map[int]Value
		for i, p := range sig.Params {
			argTypes[i] = p.Type
			if p.Pattern != nil {
				if patterns == nil {
					patterns = make(map[int]Value)
				}
				patterns[i] = *p.Pattern
			}
		}
		s := sig // capture for closure
		handler := func(args []Value) ([]Value, error) {
			var result []Value
			var names []string
			// Wrap the entire expansion (unnamed args + body + undef
			// cleanup) in parens so it evaluates as a single
			// sub-expression. Without this, an outer forward can grab
			// intermediate values from the body before the body
			// finishes executing (e.g. recursive factorial: the outer
			// mul's forward grabs x=1 from the inner body instead of
			// waiting for the full result).
			result = append(result, NewOpenParen())

			// Push args list onto the args stack for access via the
			// "args" word (args.0, args.1, etc.).
			argsCopy := make([]Value, len(args))
			copy(argsCopy, args)
			argsList := NewList(argsCopy)
			r.argsStack = append(r.argsStack, argsList)

			for i, p := range s.Params {
				if p.Name != "" {
					installDef(r, p.Name, args[i])
					names = append(names, p.Name)
				} else {
					// Unnamed parameter: push value back for the body to use
					result = append(result, args[i])
				}
			}
			body := make([]Value, len(s.Body))
			copy(body, s.Body)
			result = append(result, body...)
			// Pop the args stack to restore the previous args (for nesting).
			result = append(result, NewWord("__pop-args"))
			for i := len(names) - 1; i >= 0; i-- {
				// Force suffix so undef takes the name word that follows,
				// not a same-typed value from the prefix stack (e.g. a
				// string return value when the param is also a string).
				result = append(result,
					NewWordModified("undef", -1, false, true),
					NewWord(names[i]),
				)
			}
			// Inject return-check if return types are declared.
			if len(s.Returns) > 0 {
				result = append(result, NewReturnCheck(ReturnCheckInfo{
					FuncName: name,
					Returns:  s.Returns,
				}))
			}
			result = append(result, NewWord(")"))
			return result, nil
		}
		registerFn(name, Signature{Args: argTypes, Handler: handler, Patterns: patterns})
	}
}

// CallAQL invokes an AQL function value (FnDefInfo) with the given arguments
// in a sub-engine. This allows native Go code to call AQL callbacks.
//
//	result, err := r.CallAQL(callbackValue, []Value{someArg})
func (r *Registry) CallAQL(fn Value, args []Value) ([]Value, error) {
	fnDef, ok := fn.Data.(FnDefInfo)
	if !ok {
		return nil, fmt.Errorf("CallAQL: value is not a function")
	}

	// Find matching signature.
	for _, sig := range fnDef.Sigs {
		if len(sig.Params) != len(args) {
			continue
		}
		match := true
		for i, p := range sig.Params {
			if !args[i].VType.Matches(p.Type) {
				match = false
				break
			}
			// Check structural pattern (e.g. map literal).
			if p.Pattern != nil {
				pat := *p.Pattern
				if pat.VType.Equal(TMap) && args[i].VType.Equal(TMap) &&
					pat.Data != nil && args[i].Data != nil {
					if !openUnifyMap(pat, args[i]) {
						match = false
						break
					}
				} else {
					if _, uOk := Unify(args[i], pat); !uOk {
						match = false
						break
					}
				}
			}
		}
		if !match {
			continue
		}

		// Build token sequence (same as installFnDef handler).
		var tokens []Value
		var names []string

		// Push args list onto the args stack.
		argsCopy := make([]Value, len(args))
		copy(argsCopy, args)
		argsList := NewList(argsCopy)
		r.argsStack = append(r.argsStack, argsList)

		for i, p := range sig.Params {
			if p.Name != "" {
				installDef(r, p.Name, args[i])
				names = append(names, p.Name)
			} else {
				tokens = append(tokens, args[i])
			}
		}
		body := make([]Value, len(sig.Body))
		copy(body, sig.Body)
		tokens = append(tokens, body...)

		// Evaluate in a sub-engine.
		sub := New(r)
		result, err := sub.Run(tokens)

		// Cleanup: pop args stack, undef named params.
		if len(r.argsStack) > 0 {
			r.argsStack = r.argsStack[:len(r.argsStack)-1]
		}
		for i := len(names) - 1; i >= 0; i-- {
			uninstallDef(r, names[i])
		}

		if err != nil {
			return nil, fmt.Errorf("CallAQL: %w", err)
		}
		return result, nil
	}

	return nil, fmt.Errorf("CallAQL: no matching signature for arguments")
}
