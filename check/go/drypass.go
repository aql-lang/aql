package check

import (
	"errors"

	core "github.com/boru-lang/boru/core/go"
)

// Check-mode dry pass for PURE words over concrete arguments.
//
// A word whose handler is a pure function of its arguments — an assertion
// comparison, a codec decode of a literal, a bounds-checked list edit, a
// parse of a literal source — fails at runtime iff it fails on the same
// concrete arguments at check time. DryPassReturns runs the real handler
// once during analysis when EVERY argument is concrete and the position is
// the top-level straight line, and mirrors a handler error into a check
// diagnostic with the byte-identical code + detail. The modelled residual
// is unchanged either way (the declared result carriers), so the dry pass
// gates without altering typing or the compile pass.
//
// Precedents: miniMicronLitReturns' lenient dry pass (boru:minilang),
// parseSpecReturns (boru:parse), CheckMicronConstruction (make).

// CheckAtUncaughtTopLevel reports whether the current analysis position is
// the check pass's top-level straight line: outside every fn body and
// nested branch/loop/quotation body. A guaranteed runtime fault detected
// here is unconditionally reached, so it may be flagged as a program
// error; anywhere else reachability is conditional. Inside `do` bodies
// AddDiagnostic re-attributes error findings to caught info centrally,
// and the compile pass no longer needs excluding — mirror diagnostics
// (RuntimeMirror) do not trip the compile pipeline's refusal.
func CheckAtUncaughtTopLevel(r *core.Registry) bool {
	return r != nil && r.Check.IsActive() &&
		r.Check.FnBodyDepth == 0 && r.Check.NestedBodyDepth == 0
}

// DryPassReturns builds a ReturnsFunc that models the declared result
// carriers and, on the top-level straight line with all-concrete args,
// dry-runs the (pure) handler to surface its guaranteed runtime error as a
// check diagnostic. h MUST be pure: no I/O, no registry mutation, no body
// execution — the dry pass runs it during analysis. An uncoded handler
// error is reported as type_error (the CheckMicronConstruction fallback).
func DryPassReturns(h func(args []core.Value, named map[string]core.Value, body []core.Value, r *core.Registry) ([]core.Value, error), results ...*core.Type) core.ReturnsFunc {
	return DryPassWrap(h, ReturnsStatic(results...))
}

// DryPassWrap is DryPassReturns over an EXISTING result model: the base
// ReturnsFunc keeps full ownership of the residual (a precise per-arg
// narrowing like ReturnsPreserveListAt), and the dry pass only contributes
// the guaranteed-error diagnostic.
func DryPassWrap(h func(args []core.Value, named map[string]core.Value, body []core.Value, r *core.Registry) ([]core.Value, error), base core.ReturnsFunc) core.ReturnsFunc {
	return func(args []core.Value, r *core.Registry) []core.Value {
		if CheckAtUncaughtTopLevel(r) && allConcreteArgs(args) {
			var pos core.SrcPos
			if len(args) > 0 {
				pos = args[0].Pos()
			}
			if _, err := h(dryPassOperands(args), nil, nil, r); err != nil {
				code, detail := "type_error", err.Error()
				var ae *core.BoruError
				if errors.As(err, &ae) {
					code, detail = ae.Code, ae.Detail
				}
				core.CheckAddUniqueDiagnostic(r, code, detail, "", pos)
			}
		}
		return base(args, r)
	}
}

// allConcreteArgs gates the dry pass on arguments whose runtime value the
// analysis provably holds: concrete payloads, plus a strict (non-dynamic)
// none — None is a singleton, so the checked literal IS the runtime value
// (the orderingDeterminate rule) — plus a bare type node, for the same
// reason: a type literal is inert and self-evaluating, so the checked
// node IS the runtime operand (the TypeArgs slot of `convert` and its
// kin; without this the dry pass never fires for any word whose
// signature carries a type-literal slot).
func allConcreteArgs(args []core.Value) bool {
	for _, a := range args {
		if core.IsConcrete(a) || core.IsBareTypeNode(a) {
			continue
		}
		if core.IsNoneShape(a) && !a.Dynamic {
			continue
		}
		return false
	}
	return true
}

// dryPassOperands canonicalises the gated args for the handler run: a
// strict none-SHAPE (a None carrier or literal) becomes the canonical
// `none` sentinel, so the handler's payload probes (IsNone, truthiness)
// see exactly the value the runtime would hand it.
func dryPassOperands(args []core.Value) []core.Value {
	var out []core.Value
	for i, a := range args {
		if !core.IsConcrete(a) && core.IsNoneShape(a) {
			if out == nil {
				out = append([]core.Value(nil), args...)
			}
			out[i] = core.WithPos(core.NewNone(), a)
		}
	}
	if out == nil {
		return args
	}
	return out
}
