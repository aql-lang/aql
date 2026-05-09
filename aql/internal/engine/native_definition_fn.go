package engine

// This file contains the helpers and parsers used by the `fn` word.
// The fn handler itself lives in native_definition.go.

// parseFnDef, outputSigIsConcreteReturns, isSigTypeValue,
// outputSigValues: re-exported from aqleng via aliases.go (canonical
// implementations live in aqleng/go/fn_def.go).

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
