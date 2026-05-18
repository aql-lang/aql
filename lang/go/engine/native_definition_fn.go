package engine

// This file contains the helpers and parsers used by the `fn` word.
// The fn handler itself lives in native_definition.go.

// parseFnDef, parseFnUndefSpec: re-exported from aqleng via
// aliases.go (canonical implementations live in eng/go/fn_def.go).

// parseFnReturns: re-exported from aqleng via aliases.go
// (canonical implementation lives in eng/go/fn_params.go).

// parseFnParams, resolveSigType, resolveTypeName: re-exported from
// aqleng via aliases.go (canonical implementations live in
// eng/go/fn_params.go).

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
