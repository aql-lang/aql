package engine

import (
	"fmt"
	"strings"
)

// RegisterFn registers the "fn" word, which parses a list of signature
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
func RegisterFn(r *Registry) {
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
			Args:           []Type{TList},
			NoEvalArgs:     map[int]bool{0: true},
			Handler:        fnHandler,
			Returns:        []Type{TFunction},
			RunInCheckMode: true,
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
		if _, ok := TypeNameTable()[name]; ok {
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
		if _, ok := TypeNameTable()[name]; ok {
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
			// Integer literal as value pattern (post §1.1 fix:
			// dispatched via Signature.Patterns, not by a
			// value-tagged type-path leaf).
			pat := elem
			params = append(params, FnParam{Type: TInteger, Pattern: &pat})

		case elem.VType.Matches(TBoolean):
			// Boolean literal as value pattern.
			pat := elem
			params = append(params, FnParam{Type: TBoolean, Pattern: &pat})

		case elem.VType.Matches(TString):
			// String literal as value pattern.
			pat := elem
			params = append(params, FnParam{Type: TString, Pattern: &pat})

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
	// Scalar literal in sig position (e.g. `_:0`, `flag:true`,
	// `name:"alice"`, `pi:3.14`). The kind goes into the param's
	// Type; the specific value goes into the Pattern slot so the
	// matcher dispatches via Signature.Patterns + Unify (which
	// compares Data when both sides have equal types). This replaces
	// the older "value-tagged subtype" path that encoded the literal
	// in the type itself.
	if v.Data != nil && (v.VType.Matches(TInteger) ||
		v.VType.Matches(TDecimal) ||
		v.VType.Matches(TBoolean) ||
		v.VType.Matches(TString) ||
		v.VType.Matches(TAtom)) {
		pattern := v
		// Normalise the param type to the kind so `Equal(TInteger)`
		// works for callers that inspect it.
		var kind Type
		switch {
		case v.VType.Matches(TInteger):
			kind = TInteger
		case v.VType.Matches(TDecimal):
			kind = TDecimal
		case v.VType.Matches(TBoolean):
			kind = TBoolean
		case v.VType.Matches(TString):
			kind = TString
		default:
			kind = TAtom
		}
		return kind, &pattern, nil
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

// lookupDefType resolves a name to its type value. Used by fn-sig
// parsing so `def f fn [[rgb:Color] …]` can bind the Color
// reference to its actual record/object/disjunct/etc. type at
// install time.
//
// Resolution order matches stepWord: r.Types (the canonical home
// for user-defined types) wins, then fall back to DefStacks for
// any legacy installer that still drops a type body there. Returns
// nil if the name is unbound or the binding isn't a type body.
func lookupDefType(r *Registry, name string) *Value {
	if r == nil {
		return nil
	}
	if tv, ok := r.TopOfTypeStack(name); ok {
		if IsTypeBody(tv) {
			return &tv
		}
	}
	val, ok := r.TopOfDefStack(name)
	if !ok {
		return nil
	}
	if !IsTypeBody(val) {
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
	case "Never":
		return TNever, nil
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

// ExpandOptionalSigs: re-exported from aqleng via aliases.go

// InstallFnDef: re-exported from aqleng via aliases.go

// CallAQL: re-exported from aqleng via aliases.go

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
					if !OpenUnifyMap(pat, args[j]) {
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
