package eng

// Pure helpers settled at the carve.

// Pure helpers settled at the carve.

// Pure helpers settled at the carve.

// Pure helpers settled at the carve.

// Pure helpers settled at the carve.

// Pure helpers settled at the carve.

// Pure helpers settled at the carve.

// Pure helpers settled at the carve.

// Pure helpers settled at the carve.

// Pure helpers settled at the carve.

// Pure helpers settled at the carve.

// Pure helpers settled at the carve.

// Pure helpers settled at the carve.

// Pure helpers settled at the carve.

// Pure helpers settled at the carve.

// Pure helpers settled at the carve.

// Pure helpers settled at the carve.

// Pure helpers settled at the carve.

// Pure helpers settled at the carve.

// Pure helpers settled at the carve.

// Pure helpers settled at the carve.

// Pure helpers settled at the carve.

// Pure helpers settled at the carve.

// Pure helpers settled at the carve.

// Pure helpers settled at the carve.

// Pure helpers settled at the carve.

// Pure helpers settled at the carve.

// Pure helpers settled at the carve.

// Pure helpers settled at the carve.

// Pure helpers settled at the carve.

// Pure helpers settled at the carve.

// Pure helpers settled at the carve.

// Pure helpers settled at the carve.

// Pure helpers settled at the carve.

// Pure helpers settled at the carve.

// Pure helpers settled at the carve.

// Pure helpers settled at the carve.

// Pure helpers settled at the carve.

// Pure helpers settled at the carve.

// Pure helpers settled at the carve.

// Pure helpers settled at the carve.

// Pure helpers settled at the carve.

// Pure helpers settled at the carve.

// Pure helpers settled at the carve.

// Pure helpers settled at the carve.

// Pure helpers settled at the carve.

// Pure helpers settled at the carve.

// Pure helpers settled at the carve.

// Pure helpers settled at the carve.

// Pure helpers settled at the carve.

// Pure helpers settled at the carve.

// Pure helpers settled at the carve.

import (
	core "github.com/boru-lang/boru/core/go"
)

// Pure helpers settled at the carve.

// anonFnVal builds an anonymous Function value the way `afn`/`=>` does:
// Anonymous=true, authored sigs with a boru body. compileFnDef attaches
// the body-runner handler at dispatch time.
func anonFnVal(params []core.FnParam, returns []*core.Type, body []core.Value) core.Value {
	return core.NewFunction(core.FnDefInfo{
		Anonymous: true,
		Signatures: []core.Signature{{
			Params:     params,
			Returns:    returns,
			Impl:       core.Boru(body),
			BarrierPos: core.BarrierAllForward,
		}},
	})
}

func registerUserPoly2(r *core.Registry) {
	registerDynWords(r)
	// Both overloads declare IDENTICAL returns — the poly-arm gate.
	core.InstallFnDef(r, "upoly2", core.FnDefInfo{
		Signatures: []core.Signature{
			{
				Params:     []core.FnParam{{Name: "n", Type: core.TInteger}},
				Returns:    []*core.Type{core.TInteger},
				Impl:       core.Boru(parenBody(core.NewWord("cadd"), core.NewWord("n"), core.NewInteger(1))),
				BarrierPos: core.BarrierAllForward,
			},
			{
				Params:     []core.FnParam{{Name: "s", Type: core.TString}},
				Returns:    []*core.Type{core.TInteger},
				Impl:       core.Boru(parenBody(core.NewInteger(-1))),
				BarrierPos: core.BarrierAllForward,
			},
		},
	})
}
