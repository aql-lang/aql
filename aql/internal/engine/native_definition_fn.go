package engine

import (
	"fmt"
	"strings"
)

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
	fnHandler := func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
		list := args[0]
		if !list.VType.Equal(TList) {
			return nil, fmt.Errorf("fn: argument must be a list")
		}
		if list.Data == nil {
			return nil, fmt.Errorf("fn: argument must be a concrete list, got type literal")
		}
		elems := list.AsList().Slice()
		if len(elems) == 0 {
			return nil, fmt.Errorf("fn: list must not be empty")
		}
		// Triples (def mode) take precedence when divisible by 3.
		if len(elems)%3 == 0 {
			fnDef, err := parseFnDef(r, elems)
			if err != nil {
				return nil, err
			}
			return []Value{NewFunction(fnDef)}, nil
		}
		// Pairs (undef mode) when divisible by 2.
		if len(elems)%2 == 0 {
			undefInfo, err := parseFnUndefSpec(r, elems)
			if err != nil {
				return nil, err
			}
			return []Value{NewFnUndef(undefInfo)}, nil
		}
		return nil, fmt.Errorf("fn: list length must be a multiple of 3 (def) or 2 (undef spec)")
	}

	r.RegisterNativeFunc(NativeFunc{
		Name:              "fn",
		ForwardPrecedence: true,
		Signatures: []NativeSig{{
			Args:       []Type{TList},
			NoEvalArgs: map[int]bool{0: true},
			Handler:    fnHandler,
			Returns: []Type{TFunction},
		}},
	})
}

// parseFnDef parses a function specification list into FnDefInfo.
// The list contains signature triples: [input-sig, output-sig, body] ...
// Each element of a triple may be abbreviated: a non-list value is treated
// as a single-element list (e.g., `string` is equivalent to `[string]`).
func parseFnDef(r *Registry, list []Value) (FnDefInfo, error) {
	var sigs []FnSig
	for i := 0; i < len(list); i += 3 {
		inputSig := list[i]
		outputSig := list[i+1]
		body := list[i+2]

		// Abbreviation: non-list input sig is treated as [inputSig].
		if !inputSig.VType.Equal(TList) {
			inputSig = NewList([]Value{inputSig})
		}

		params, barrierPos, err := parseFnParams(r, inputSig)
		if err != nil {
			return FnDefInfo{}, err
		}

		// Check if all output values are concrete (non-type). If so, they
		// are literal return values to append to the body, not type
		// declarations for return checking.
		concreteReturns := outputSigIsConcreteReturns(outputSig)

		var returns []Type
		if !concreteReturns {
			returns, err = parseFnReturns(outputSig)
			if err != nil {
				return FnDefInfo{}, err
			}
		}

		// Abbreviation: non-list body is treated as [body].
		var bodyElems []Value
		if body.VType.Equal(TList) && body.Data != nil {
			bodyElems = body.AsList().Slice()
		} else {
			bodyElems = []Value{body}
		}

		// Append concrete return values to the body with an end separator.
		// Set returns to [Any, Any, ...] so the ReturnCheck still fires
		// to clean up unconsumed unnamed args.
		if concreteReturns {
			retVals := outputSigValues(outputSig)
			if len(retVals) > 0 {
				bodyElems = append(bodyElems, NewWord("end"))
				bodyElems = append(bodyElems, retVals...)
				returns = make([]Type, len(retVals))
				for j := range retVals {
					returns[j] = TAny
				}
			}
		}

		sigs = append(sigs, FnSig{
			Params:     params,
			Returns:    returns,
			Body:       bodyElems,
			BarrierPos: barrierPos,
		})
	}
	return FnDefInfo{Sigs: sigs}, nil
}

// outputSigIsConcreteReturns checks whether all values in the output signature
// are concrete (non-type) values. If so, they should be appended to the body
// as literal return values rather than used for return type checking.
// An empty output list is NOT concrete returns (it means no return types).
func outputSigIsConcreteReturns(outputSig Value) bool {
	if outputSig.VType.Equal(TList) && outputSig.Data != nil {
		elems := outputSig.AsList()
		if elems.Len() == 0 {
			return false
		}
		for _, e := range elems.Slice() {
			if isSigTypeValue(e) {
				return false
			}
		}
		return true
	}
	// Single non-list value: check if it's a type.
	return !isSigTypeValue(outputSig)
}

// isSigTypeValue returns true if v looks like a type in a signature context.
// This handles: type literals (Data==nil), Words that are type names,
// Atoms/Strings that are type names, and structured types (Record, Options, etc).
func isSigTypeValue(v Value) bool {
	// Already a type literal (parser-resolved).
	if v.Data == nil && !v.VType.Equal(TNone) {
		return true
	}
	// Structured type values.
	if v.IsOptionsType() || v.IsRecordType() || v.IsTypedList() ||
		v.IsTypedMap() || v.IsTableType() || v.IsObjectType() {
		return true
	}
	// Word that is a type name (from token-based API).
	if v.IsWord() {
		_as0, _ := v.AsWord()
		name := _as0.Name
		if _, ok := typeNames[name]; ok {
			return true
		}
		if _, ok := ResolveTypePath(name); ok {
			return true
		}
		return false
	}
	// Atom or String that is a type name.
	if v.VType.Matches(TAtom) || v.VType.Matches(TString) {
		name, _ := v.AsString()
		if _, ok := typeNames[name]; ok {
			return true
		}
		if _, ok := ResolveTypePath(name); ok {
			return true
		}
		return false
	}
	return false
}

// outputSigValues extracts the concrete values from an output signature.
func outputSigValues(outputSig Value) []Value {
	if outputSig.VType.Equal(TList) && outputSig.Data != nil {
		elems := outputSig.AsList()
		result := elems.Slice()
		return result
	}
	return []Value{outputSig}
}

// parseFnUndefSpec parses a list of signature pairs (input+output, no body)
// into a FnUndefInfo for targeted undef. Used when fn receives a list whose
// length is divisible by 2 but not 3.
func parseFnUndefSpec(r *Registry, list []Value) (FnUndefInfo, error) {
	var sigs []FnSigSpec
	for i := 0; i < len(list); i += 2 {
		inputSig := list[i]
		outputSig := list[i+1]

		if !inputSig.VType.Equal(TList) {
			inputSig = NewList([]Value{inputSig})
		}

		params, _, err := parseFnParams(r, inputSig)
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
	if !outputSig.VType.Equal(TList) || outputSig.Data == nil {
		// Abbreviation: single value treated as [value].
		t, _, err := resolveSigType(nil, outputSig)
		if err != nil {
			return nil, err
		}
		return []Type{t}, nil
	}
	elems := outputSig.AsList()
	if elems.Len() == 0 {
		return nil, nil
	}
	types := make([]Type, elems.Len())
	for i, e := range elems.Slice() {
		var err error
		types[i], _, err = resolveSigType(nil, e)
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
//
// Optional params are detected via:
//   - Named: key ending with "?" (from pair ? syntax): [x?:Integer]
//   - Unnamed: "?" word following a type: [Integer?]
func parseFnParams(r *Registry, inputSig Value) ([]FnParam, int, error) {
	if !inputSig.VType.Equal(TList) {
		return nil, 0, fmt.Errorf("function spec: input signature must be a list")
	}
	if inputSig.Data == nil {
		return nil, 0, fmt.Errorf("function spec: input signature must be a concrete list, got type literal")
	}
	elems := inputSig.AsList()
	var params []FnParam
	barrierPos := 0

	for i := 0; i < elems.Len(); i++ {
		elem := elems.Get(i)

		// Check if this element is a "?" marker — skip it but mark
		// the previous param as optional.
		_as1, _ := elem.AsWord()
		if elem.IsWord() && _as1.Name == "?" {
			if len(params) > 0 {
				params[len(params)-1].Optional = true
			}
			continue
		}

		// Check if this element is a "|" marker — record the barrier
		// position. Forward collection stops here; remaining args
		// are matched from the stack.
		_as2, _ := elem.AsWord()
		if elem.IsWord() && _as2.Name == "|" {
			barrierPos = len(params)
			continue
		}

		switch {
		case elem.VType.Equal(TMap) && elem.Data != nil:
			m := elem.AsMutableMap()
			if m != nil && m.Implicit {
				// Named parameter from implicit pair syntax: [x:Integer]
				keys := m.Keys()
				if len(keys) != 1 {
					return nil, 0, fmt.Errorf("function spec: parameter map must have exactly one key")
				}
				name := keys[0]
				optional := false
				// Detect optional named param: key ends with "?"
				if strings.HasSuffix(name, "?") {
					name = strings.TrimSuffix(name, "?")
					optional = true
				}
				typeVal, _ := m.Get(keys[0])
				// Evaluate ParenExpr values (e.g., (Integer or None))
				// that haven't been auto-evaluated yet.
				if typeVal.IsParenExpr() && r != nil {
					items := typeVal.AsParenExpr()
					sub := New(r)
					input := make([]Value, 0, len(items)+2)
					input = append(input, NewOpenParen())
					input = append(input, items...)
					input = append(input, NewWord(")"))
					result, err := sub.Run(input)
					if err == nil && len(result) == 1 {
						typeVal = result[0]
					}
				}
				// Detect optional from disjunct containing None:
				// either from ? syntax (key?) or explicit (Integer or None).
				if typeVal.IsDisjunct() {
					_as3, _ := typeVal.AsDisjunct()
					alts := _as3.Alternatives
					for _, alt := range alts {
						if alt.VType.Equal(TNone) {
							optional = true
							break
						}
					}
					// Extract the base type (non-None alternative).
					if optional {
						for _, alt := range alts {
							if !alt.VType.Equal(TNone) {
								typeVal = alt
								break
							}
						}
					}
				}
				paramType, pattern, err := resolveSigType(r, typeVal)
				if err != nil {
					return nil, 0, fmt.Errorf("function spec: invalid type for %q: %w", name, err)
				}
				params = append(params, FnParam{Name: name, Type: paramType, Pattern: pattern, Optional: optional})
			} else {
				// Explicit map: unnamed parameter with structural pattern
				paramType, pattern, err := resolveSigType(r, elem)
				if err != nil {
					return nil, 0, fmt.Errorf("function spec: invalid map param: %w", err)
				}
				params = append(params, FnParam{Type: paramType, Pattern: pattern})
			}

		case elem.IsWord():
			// Unnamed parameter: bare word is a type name
			_as4, _ := elem.AsWord()
			typeName := _as4.Name
			paramType, err := resolveTypeName(typeName)
			if err != nil {
				return nil, 0, fmt.Errorf("function spec: invalid type %q: %w", typeName, err)
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
			return nil, 0, fmt.Errorf("function spec: invalid parameter: %s", elem.String())
		}
	}

	return params, barrierPos, nil
}

// resolveSigType converts a Value (from a pair's value side) to a Type.
// The second return value is a non-nil pattern when the value is a map or list
// literal that requires structural matching beyond basic type checking.
// When r is non-nil, def'd types (e.g. record types) are resolved from the registry.
func resolveSigType(r *Registry, v Value) (Type, *Value, error) {
	if v.Data == nil {
		// Type literal (e.g., number, string) — already resolved by parser
		return v.VType, nil, nil
	}
	if v.IsWord() {
		_as5, _ := v.AsWord()
		name := _as5.Name
		if defVal := lookupDefType(r, name); defVal != nil {
			return resolveDefType(*defVal)
		}
		t, err := resolveTypeName(name)
		return t, nil, err
	}
	if v.VType.Matches(TString) {
		name, _ := v.AsString()
		if defVal := lookupDefType(r, name); defVal != nil {
			return resolveDefType(*defVal)
		}
		t, err := resolveTypeName(name)
		return t, nil, err
	}
	// Atoms (unquoted text in data context) may be type names.
	if v.VType.Matches(TAtom) {
		name, _ := v.AsString()
		if defVal := lookupDefType(r, name); defVal != nil {
			return resolveDefType(*defVal)
		}
		t, err := resolveTypeName(name)
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

// lookupDefType checks if a name is def'd as a type value in the registry.
// Returns nil if the registry is nil, the name is not def'd, or the value
// is not a type (record, disjunct, etc.).
func lookupDefType(r *Registry, name string) *Value {
	if r == nil {
		return nil
	}
	stack := r.DefStacks[name]
	if len(stack) == 0 {
		return nil
	}
	val := stack[len(stack)-1]
	if !isTypeValue(val) {
		return nil
	}
	return &val
}

// resolveDefType converts a def'd type value into a signature type + pattern.
// Record types become TMap with a structural map pattern so that plain maps
// with matching fields satisfy the signature.
func resolveDefType(v Value) (Type, *Value, error) {
	if v.IsRecordType() {
		rt, _ := v.AsRecordType()
		pat := NewMap(rt.Fields)
		return TMap, &pat, nil
	}
	if v.IsOptionsType() {
		_as6, _ := v.AsOptionsType()
		pat := NewOptionsType(_as6.Fields)
		return TMap, &pat, nil
	}
	// Other type values (disjuncts, type literals, etc.) use their type directly.
	if v.Data == nil {
		return v.VType, nil, nil
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

// expandOptionalSigs expands signatures with optional parameters into
// additional signatures for each combination of omitted optional params.
// Each generated sig's body calls the function with base values for the
// omitted params. Present params are referenced by name (if named) or
// via args.N (if unnamed), avoiding synthetic param names.
//
// For example:
//
//	def foo fn [[Map? Integer] [Integer] [body]]
//
// expands to add:
//
//	[Integer] [Integer] [foo {} args.0]
//
// where {} is the base value for Map, and args.0 references the first
// argument of the reduced signature.
func expandOptionalSigs(name string, sigs []FnSig) []FnSig {
	var expanded []FnSig
	for _, sig := range sigs {
		expanded = append(expanded, sig)

		// Find optional param indices.
		var optIndices []int
		for i, p := range sig.Params {
			if p.Optional {
				optIndices = append(optIndices, i)
			}
		}
		if len(optIndices) == 0 {
			continue
		}

		// Generate combinations: each subset of optional params to omit.
		// We iterate from 1 to 2^N-1 (skip 0 = no omissions, which is
		// the original sig). Bit i set means optional param i is omitted.
		numOpt := len(optIndices)
		for mask := 1; mask < (1 << numOpt); mask++ {
			// Build omitted set.
			omitted := make(map[int]bool)
			for bit := 0; bit < numOpt; bit++ {
				if mask&(1<<bit) != 0 {
					omitted[optIndices[bit]] = true
				}
			}

			// Build reduced params (only non-omitted).
			// Named params keep their names; unnamed params stay unnamed.
			var reducedParams []FnParam
			for i, p := range sig.Params {
				if !omitted[i] {
					reducedParams = append(reducedParams, FnParam{
						Name:    p.Name,
						Type:    p.Type,
						Pattern: p.Pattern,
					})
				}
			}

			// Build body: call the function with all original params,
			// inserting base values for omitted ones. Present params
			// are referenced by name or via args.N positional access.
			var body []Value
			body = append(body, NewWord(name))
			presentIdx := 0
			for i, p := range sig.Params {
				if omitted[i] {
					// Insert base value for the omitted param's type.
					bv, err := baseValue(p.Type)
					if err != nil {
						continue
					}
					body = append(body, bv)
				} else {
					if p.Name != "" {
						// Named param: reference by name.
						body = append(body, NewWord(p.Name))
					} else {
						// Unnamed param: use args.N (paren-wrapped dot access).
						body = append(body,
							NewOpenParen(),
							NewWord("args"),
							NewAtom(fmt.Sprintf("%d", presentIdx)),
							NewWord("get"),
							NewWord(")"),
						)
					}
					presentIdx++
				}
			}

			expanded = append(expanded, FnSig{
				Params:  reducedParams,
				Returns: sig.Returns,
				Body:    body,
			})
		}
	}
	return expanded
}

// installFnDef registers typed signatures for a function definition.
// For each signature, it creates a handler that binds named parameters
// via installDef, returns body tokens, and appends undef cleanup.
func installFnDef(r *Registry, name string, fnDef FnDefInfo, stackOnly ...bool) {
	isStackOnly := len(stackOnly) > 0 && stackOnly[0]
	// Expand optional parameters into additional signatures.
	fnDef.Sigs = expandOptionalSigs(name, fnDef.Sigs)
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
		handler := func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
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

			unnamedCount := 0
			for i, p := range s.Params {
				if p.Name != "" {
					arg := args[i]
					// Quote list params so they're treated as data values
					// when referenced in the body, not expanded as code bodies.
					if arg.VType.Equal(TList) && !arg.Quoted {
						arg.Quoted = true
					}
					installDef(r, p.Name, arg)
					names = append(names, p.Name)
				} else {
					// Unnamed parameter: push value back for the body to use
					result = append(result, args[i])
					unnamedCount++
				}
			}
			// Snapshot DefStacks lengths after installing named params
			// so we can clean up any defs created during body execution
			// (fixes def leakage from fn bodies — DX-REPORT Issue 2).
			defSnapshot := make(map[string]int, len(r.DefStacks))
			for dname, dstack := range r.DefStacks {
				defSnapshot[dname] = len(dstack)
			}

			body := make([]Value, len(s.Body))
			copy(body, s.Body)
			result = append(result, body...)
			// Clean up defs created during body execution, then pop
			// the args stack to restore the previous args (for nesting).
			result = append(result, NewDefCleanup(DefCleanupInfo{
				Snapshot: defSnapshot,
				Registry: r,
			}))
			result = append(result, NewWord("__pa"))
			for i := len(names) - 1; i >= 0; i-- {
				// Force forward so undef takes the name word that follows,
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
					FuncName:     name,
					Returns:      s.Returns,
					UnnamedCount: unnamedCount,
				}))
			}
			result = append(result, NewWord(")"))
			return result, nil
		}
		r.RegisterNativeFunc(NativeFunc{
			Name:              name,
			ForwardPrecedence: !isStackOnly,
			Signatures: []NativeSig{{
				Args:       argTypes,
				Handler:    handler,
				Patterns:   patterns,
				BarrierPos: s.BarrierPos,
			}},
		})
	}
}

// CallAQL invokes an AQL function value (FnDefInfo) with a pre-matched
// signature and arguments in a sub-engine. The caller is responsible for
// signature matching — use MatchFnSig to find the matching sig.
//
//	sig := MatchFnSig(fn, args)
//	result, err := r.CallAQL(sig, args)
func (r *Registry) CallAQL(sig *FnSig, args []Value) ([]Value, error) {
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
			arg := args[i]
			if arg.VType.Equal(TList) && !arg.Quoted {
				arg.Quoted = true
			}
			installDef(r, p.Name, arg)
			names = append(names, p.Name)
		} else {
			tokens = append(tokens, args[i])
		}
	}
	body := make([]Value, len(sig.Body))
	copy(body, sig.Body)
	tokens = append(tokens, body...)

	// Snapshot DefStacks lengths before body execution so we can
	// clean up any defs created during body execution (Issue 2
	// from AQL-DX-REPORT: def leakage from fn bodies).
	defSnapshot := make(map[string]int, len(r.DefStacks))
	for name, stack := range r.DefStacks {
		defSnapshot[name] = len(stack)
	}

	// Evaluate in a sub-engine with higher step limit for complex bodies.
	sub := NewTop(r)
	result, err := sub.Run(tokens)

	// Cleanup: pop args stack, undef named params, then clean up
	// any defs that were created during body execution.
	if len(r.argsStack) > 0 {
		r.argsStack = r.argsStack[:len(r.argsStack)-1]
	}
	for i := len(names) - 1; i >= 0; i-- {
		uninstallDef(r, names[i])
	}

	// Remove defs that were added during body execution.
	// Collect names first, then clean up outside the range loop
	// to avoid mutating DefStacks during iteration (uninstallDef
	// triggers installFnDef → Register → upsertFnDef which can
	// modify DefStacks entries for other names).
	var toClean []string
	for name := range r.DefStacks {
		if len(r.DefStacks[name]) > defSnapshot[name] {
			toClean = append(toClean, name)
		}
	}
	for _, name := range toClean {
		target := defSnapshot[name]
		// Pop entries down to the snapshot length. Use a bounded
		// loop to avoid infinite looping if uninstallDef's rebuild
		// creates new entries.
		for attempts := 0; attempts < 100 && len(r.DefStacks[name]) > target; attempts++ {
			uninstallDef(r, name)
		}
	}

	if err != nil {
		return nil, fmt.Errorf("CallAQL: %w", err)
	}
	return result, nil
}

// MatchFnSig finds the first FnSig in a FnDef value whose params match the
// given args. Returns nil if no signature matches.
func MatchFnSig(fn Value, args []Value) *FnSig {
	fnDef, ok := fn.Data.(FnDefInfo)
	if !ok {
		return nil
	}
	for i := range fnDef.Sigs {
		sig := &fnDef.Sigs[i]
		if len(sig.Params) != len(args) {
			continue
		}
		match := true
		for j, p := range sig.Params {
			if !args[j].VType.Matches(p.Type) {
				match = false
				break
			}
			if p.Pattern != nil {
				pat := *p.Pattern
				if pat.VType.Equal(TMap) && args[j].VType.Equal(TMap) &&
					pat.Data != nil && args[j].Data != nil {
					if !openUnifyMap(pat, args[j]) {
						match = false
						break
					}
				} else {
					if _, uOk := Unify(args[j], pat); !uOk {
						match = false
						break
					}
				}
			}
		}
		if match {
			return sig
		}
	}
	return nil
}
