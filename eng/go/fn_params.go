package eng

import (
	"fmt"
	"strings"
)

// This file owns the canonical fn-signature parser. Both the bare
// aqleng `fn` (in core_words.go) and the production aql `def`/`fn`
// in lang/engine/native_definition_fn.go call into these
// functions. Do NOT duplicate the parser logic anywhere else; the
// optional-arg `?` rule, the barrier `|` rule, and the type-name
// resolution rule must have a single source of truth.
//
// Public surface:
//
//   ParseFnParams(r, inputSig)  ([]FnParam, int, error)
//      — walks an `[ p1 p2 … ]` list and returns the FnParams plus
//        the BarrierPos (`|` position).
//   ParseFnReturns(outputSig)   ([]*Type, error)
//      — walks an `[ T1 T2 … ]` return-type list (or a single type
//        value) and returns the Types.
//   ResolveSigType(r, v)        (*Type, *Value, error)
//      — converts a Value (from a param's type slot) into a *Type
//        plus an optional pattern Value for structural matching.
//   ResolveTypeName(name)       (*Type, error)
//      — maps a bare type-name string to its *Type. Defers to NewType
//        for non-builtin paths.
//   LookupDefType(r, name)      *Value
//      — resolves a name to a type-body value via the type stack
//        first, then the def stack. Returns nil if neither layer
//        carries a type-body for that name.
//   ResolveDefType(v)           (*Type, *Value, error)
//      — converts a def'd type value (record, options, plain type
//        literal) into a sig type + pattern.
//
// All five functions are byte-identical ports of the production
// helpers that previously lived in lang/engine/native_definition_fn.go.

// ParseFnParams extracts parameters from an input signature list.
//
// The input is a List containing some mix of:
//   - Bare type-name Words: `Integer`, `String`, …
//   - *Type-literal values
//   - Implicit-pair maps `{name:*Type}` — the standard typed-param
//     form. The name may have a trailing `?` to mark the param
//     optional, and the type slot may be a paren expression that
//     evaluates to a type or a disjunct that includes None (which
//     is also auto-treated as optional).
//   - Concrete-value patterns (Integer / Boolean / String literals)
//     that anchor the param to that exact value.
//   - The Word `?` — marks the PRECEDING param as optional. This
//     is the canonical post-name optionality marker.
//   - The Word `|` — sets the BarrierPos to the current param count.
//
// Returns the FnParam list, the BarrierPos, or a parse error.
func ParseFnParams(r *Registry, inputSig Value) ([]FnParam, int, error) {
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

		_as1, _ := AsWord(elem)
		if elem.IsWord() && _as1.Name == "?" {
			if len(params) > 0 {
				params[len(params)-1].Optional = true
			}
			continue
		}

		_as2, _ := AsWord(elem)
		if elem.IsWord() && _as2.Name == "|" {
			barrierPos = len(params)
			continue
		}

		switch {
		case elem.VType.Equal(TMap) && elem.Data != nil:
			m := elem.AsMutableMap()
			if m != nil && m.Implicit {
				keys := m.Keys()
				if len(keys) != 1 {
					return nil, 0, fmt.Errorf("function spec: parameter map must have exactly one key")
				}
				name := keys[0]
				optional := false
				if strings.HasSuffix(name, "?") {
					name = strings.TrimSuffix(name, "?")
					optional = true
				}
				typeVal, _ := m.Get(keys[0])
				if typeVal.IsParenExpr() && r != nil {
					items := typeVal.AsParenExpr()
					sub := New(r)
					input := make([]Value, 0, len(items)+2)
					input = append(input, NewOpenParen())
					input = append(input, items...)
					input = append(input, NewCloseParen())
					result, err := sub.Run(input)
					if err == nil && len(result) == 1 {
						typeVal = result[0]
					}
				}
				if typeVal.IsDisjunct() {
					_as3, _ := typeVal.AsDisjunct()
					alts := _as3.Alternatives
					for _, alt := range alts {
						if alt.VType.Equal(TNone) {
							optional = true
							break
						}
					}
					if optional {
						for _, alt := range alts {
							if !alt.VType.Equal(TNone) {
								typeVal = alt
								break
							}
						}
					}
				}
				paramType, pattern, err := ResolveSigType(r, typeVal)
				if err != nil {
					return nil, 0, fmt.Errorf("function spec: invalid type for %q: %w", name, err)
				}
				if err := ValidateWordName(name); err != nil {
					return nil, 0, fmt.Errorf("function spec: %w", err)
				}
				params = append(params, FnParam{Name: name, Type: paramType, Pattern: pattern, Optional: optional})
			} else {
				paramType, pattern, err := ResolveSigType(r, elem)
				if err != nil {
					return nil, 0, fmt.Errorf("function spec: invalid map param: %w", err)
				}
				params = append(params, FnParam{Type: paramType, Pattern: pattern})
			}

		case elem.IsWord():
			_as4, _ := AsWord(elem)
			name := _as4.Name
			// `name:*Type` colon-delimited form. Used by minimal
			// tokenizers (e.g. the aqleng spec runner, whose
			// whitespace-only lexer produces a single Word for
			// `n:Integer`). Production parsers using jsonic produce
			// the `{name:*Type}` implicit-map form instead, handled
			// in the TMap case above. Either form is accepted here
			// so a single ParseFnParams serves both consumers.
			if idx := strings.Index(name, ":"); idx > 0 {
				paramName := name[:idx]
				typeName := name[idx+1:]
				optional := false
				if strings.HasSuffix(paramName, "?") {
					paramName = strings.TrimSuffix(paramName, "?")
					optional = true
				}
				paramType, err := ResolveTypeName(typeName)
				if err != nil {
					return nil, 0, fmt.Errorf("function spec: invalid type %q: %w", typeName, err)
				}
				if err := ValidateWordName(paramName); err != nil {
					return nil, 0, fmt.Errorf("function spec: %w", err)
				}
				params = append(params, FnParam{Name: paramName, Type: paramType, Optional: optional})
				continue
			}
			// Bare type-name Word: unnamed positional param.
			paramType, err := ResolveTypeName(name)
			if err != nil {
				return nil, 0, fmt.Errorf("function spec: invalid type %q: %w", name, err)
			}
			params = append(params, FnParam{Type: paramType})

		case elem.Data == nil:
			params = append(params, FnParam{Type: elem.VType})

		case elem.VType.Matches(TInteger):
			pat := elem
			params = append(params, FnParam{Type: TInteger, Pattern: &pat})

		case elem.VType.Matches(TBoolean):
			pat := elem
			params = append(params, FnParam{Type: TBoolean, Pattern: &pat})

		case elem.VType.Matches(TString):
			pat := elem
			params = append(params, FnParam{Type: TString, Pattern: &pat})

		default:
			return nil, 0, fmt.Errorf("function spec: invalid parameter: %s", elem.String())
		}
	}

	return params, barrierPos, nil
}

// ParseFnReturns extracts return types from an output signature.
// The output may be a list of types/values or a single type/value.
func ParseFnReturns(outputSig Value) ([]*Type, error) {
	if !outputSig.VType.Equal(TList) || outputSig.Data == nil {
		t, _, err := ResolveSigType(nil, outputSig)
		if err != nil {
			return nil, err
		}
		return []*Type{t}, nil
	}
	elems := outputSig.AsList()
	if elems.Len() == 0 {
		return nil, nil
	}
	types := make([]*Type, elems.Len())
	for i, e := range elems.Slice() {
		var err error
		types[i], _, err = ResolveSigType(nil, e)
		if err != nil {
			return nil, err
		}
	}
	return types, nil
}

// ResolveSigType converts a Value (from a pair's value side) to a *Type
// plus an optional pattern Value for structural matching.
func ResolveSigType(r *Registry, v Value) (*Type, *Value, error) {
	if v.Data == nil {
		return v.VType, nil, nil
	}
	if v.IsWord() {
		_as5, _ := AsWord(v)
		name := _as5.Name
		if defVal := LookupDefType(r, name); defVal != nil {
			return ResolveDefType(*defVal)
		}
		t, err := ResolveTypeName(name)
		return t, nil, err
	}
	if v.VType.Matches(TString) {
		name, _ := AsString(v)
		if defVal := LookupDefType(r, name); defVal != nil {
			return ResolveDefType(*defVal)
		}
		t, err := ResolveTypeName(name)
		return t, nil, err
	}
	if v.VType.Matches(TAtom) {
		name, _ := AsString(v)
		if defVal := LookupDefType(r, name); defVal != nil {
			return ResolveDefType(*defVal)
		}
		t, err := ResolveTypeName(name)
		return t, nil, err
	}
	if v.Data != nil && (v.VType.Matches(TInteger) ||
		v.VType.Matches(TDecimal) ||
		v.VType.Matches(TBoolean) ||
		v.VType.Matches(TString) ||
		v.VType.Matches(TAtom)) {
		pattern := v
		var kind *Type
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
	if v.VType.Equal(TMap) {
		return TMap, &v, nil
	}
	if v.VType.Equal(TList) {
		return TList, &v, nil
	}
	return TAny, nil, nil
}

// LookupDefType resolves a name to its type value via the type stack
// first, then the def stack. Returns nil if neither carries a
// type-body for that name.
func LookupDefType(r *Registry, name string) *Value {
	if r == nil {
		return nil
	}
	if tv, ok := r.Types.TopBody(name); ok {
		if IsTypeBody(tv) {
			return &tv
		}
	}
	val, ok := r.Defs.Top(name)
	if !ok {
		return nil
	}
	if !IsTypeBody(val) {
		return nil
	}
	return &val
}

// ResolveDefType converts a def'd type value (record, options, plain
// type literal) into a sig type + pattern.
func ResolveDefType(v Value) (*Type, *Value, error) {
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
	if v.Data == nil {
		return v.VType, nil, nil
	}
	return TAny, nil, nil
}

// ResolveTypeName maps a type name string to its engine *Type.
// Special-cases the well-known names; falls back to NewType for any
// other slash-separated path.
func ResolveTypeName(name string) (*Type, error) {
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
