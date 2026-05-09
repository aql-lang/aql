package engine

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

// parseFnReturns: re-exported from aqleng via aliases.go
// (canonical implementation lives in aqleng/go/fn_params.go).

// parseFnParams, resolveSigType, lookupDefType, resolveDefType,
// resolveTypeName: re-exported from aqleng via aliases.go (canonical
// implementations live in aqleng/go/fn_params.go).

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
