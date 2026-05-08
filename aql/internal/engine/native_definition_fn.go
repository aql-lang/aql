package engine

import (
	"fmt"
	"strings"
)

// This file contains the helpers and parsers used by the `fn` word.
// The fn handler itself lives in native_definition.go.

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

		if !inputSig.VType.Equal(TList) {
			inputSig = NewList([]Value{inputSig})
		}

		params, barrierPos, err := parseFnParams(r, inputSig)
		if err != nil {
			return FnDefInfo{}, err
		}

		concreteReturns := outputSigIsConcreteReturns(outputSig)

		var returns []Type
		if !concreteReturns {
			returns, err = parseFnReturns(outputSig)
			if err != nil {
				return FnDefInfo{}, err
			}
		}

		var bodyElems []Value
		if body.VType.Equal(TList) && body.Data != nil {
			bodyElems = body.AsList().Slice()
		} else {
			bodyElems = []Value{body}
		}

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

// outputSigIsConcreteReturns checks whether all values in the output
// signature are concrete (non-type) values.
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
	return !isSigTypeValue(outputSig)
}

// isSigTypeValue returns true if v looks like a type in a signature
// context.
func isSigTypeValue(v Value) bool {
	if v.Data == nil && !v.VType.Equal(TNone) {
		return true
	}
	if v.IsOptionsType() || v.IsRecordType() || v.IsTypedList() ||
		v.IsTypedMap() || v.IsTableType() || v.IsObjectType() {
		return true
	}
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

// parseFnUndefSpec parses a list of signature pairs (input+output, no
// body) into a FnUndefInfo for targeted undef.
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
func parseFnReturns(outputSig Value) ([]Type, error) {
	if !outputSig.VType.Equal(TList) || outputSig.Data == nil {
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

		_as1, _ := elem.AsWord()
		if elem.IsWord() && _as1.Name == "?" {
			if len(params) > 0 {
				params[len(params)-1].Optional = true
			}
			continue
		}

		_as2, _ := elem.AsWord()
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
					input = append(input, NewWord(")"))
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
				paramType, pattern, err := resolveSigType(r, typeVal)
				if err != nil {
					return nil, 0, fmt.Errorf("function spec: invalid type for %q: %w", name, err)
				}
				params = append(params, FnParam{Name: name, Type: paramType, Pattern: pattern, Optional: optional})
			} else {
				paramType, pattern, err := resolveSigType(r, elem)
				if err != nil {
					return nil, 0, fmt.Errorf("function spec: invalid map param: %w", err)
				}
				params = append(params, FnParam{Type: paramType, Pattern: pattern})
			}

		case elem.IsWord():
			_as4, _ := elem.AsWord()
			typeName := _as4.Name
			paramType, err := resolveTypeName(typeName)
			if err != nil {
				return nil, 0, fmt.Errorf("function spec: invalid type %q: %w", typeName, err)
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

// resolveSigType converts a Value (from a pair's value side) to a Type.
func resolveSigType(r *Registry, v Value) (Type, *Value, error) {
	if v.Data == nil {
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
	if v.VType.Matches(TAtom) {
		name, _ := v.AsString()
		if defVal := lookupDefType(r, name); defVal != nil {
			return resolveDefType(*defVal)
		}
		t, err := resolveTypeName(name)
		return t, nil, err
	}
	if v.Data != nil && (v.VType.Matches(TInteger) ||
		v.VType.Matches(TDecimal) ||
		v.VType.Matches(TBoolean) ||
		v.VType.Matches(TString) ||
		v.VType.Matches(TAtom)) {
		pattern := v
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
	if v.VType.Equal(TMap) {
		return TMap, &v, nil
	}
	if v.VType.Equal(TList) {
		return TList, &v, nil
	}
	return TAny, nil, nil
}

// lookupDefType resolves a name to its type value.
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

// resolveDefType converts a def'd type value into a signature type +
// pattern.
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

// MatchFnSig finds the first FnSig in a FnDef value whose params match
// the given args. Returns nil if no signature matches.
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

// ExpandOptionalSigs: re-exported from aqleng via aliases.go
// InstallFnDef: re-exported from aqleng via aliases.go
// CallAQL: re-exported from aqleng via aliases.go
// InstallDef: re-exported from aqleng via aliases.go
// FnDefsOverlap: re-exported from aqleng via aliases.go
// UninstallDef: re-exported from aqleng via aliases.go
// UninstallFnSigs: re-exported from aqleng via aliases.go
