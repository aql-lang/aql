package check

import core "github.com/boru-lang/boru/core/go"

// Predicate-implied guard typing. A user predicate whose whole job is an
// `x is T` test lets a 2-token guard `if (is-T x) [then] [else]` narrow x to T
// in the THEN branch (extractGuardClauses' predicate rule). This file derives
// the T a predicate implies, structurally, from the predicate's own body —
// there is no runtime execution and no registry mutation.
//
// Two body shapes qualify:
//
//	def is-T fn [[x:Any] [Boolean] [x is T]]                     (shape A)
//	def is-T fn [[x:Any] [Boolean] [if (x is T) [then] [false]]] (shape B)
//
// Shape B is sound for ANY then-arm: a false cond returns the `false` else-arm,
// so `is-T x` can only be true when `x is T` held — pred(x) true ⇒ x is T.

// predicateImpliedType returns the type a single-argument Boolean predicate
// named `name` implies about its argument in the true case, or (nil, false)
// when `name` is not a recognisable is-T predicate.
func predicateImpliedType(r *core.Registry, name string) (*core.Type, bool) {
	if r == nil || name == "" {
		return nil, false
	}
	fn := r.Lookup(name)
	if fn == nil {
		return nil, false
	}
	// Exactly one REAL overload. An aggregate Lookup carries a synthetic
	// Fallback catch-all sig alongside the real one; skip it. A genuinely
	// multi-overload word is not a simple is-test.
	var sig *core.Signature
	for i := range fn.Signatures {
		if fn.Signatures[i].Fallback {
			continue
		}
		if sig != nil {
			return nil, false
		}
		sig = &fn.Signatures[i]
	}
	if sig == nil {
		return nil, false
	}
	boru, ok := sig.Impl.(*core.BoruImpl)
	if !ok {
		return nil, false
	}
	// One named parameter, a declared Boolean return.
	if len(sig.Params) != 1 || sig.Params[0].Name == "" {
		return nil, false
	}
	if len(sig.Returns) != 1 || !sig.Returns[0].Equal(core.TBoolean) {
		return nil, false
	}
	return predicateBodyImpliedType(r, sig.Params[0].Name, boru.Body)
}

// predicateBodyImpliedType matches the qualifying body shapes over the
// marker-stripped body tokens.
func predicateBodyImpliedType(r *core.Registry, param string, body []core.Value) (*core.Type, bool) {
	toks := stripGuardMarkers(body)
	// Shape A: the body IS `param is T`.
	if len(toks) == 3 {
		return guardTripleType(r, param, toks[0], toks[1], toks[2])
	}
	// Shape B: `if (param is T) [then] [false]`. The cond may still be a raw
	// (unexpanded) ParenExpr token — normalise it to the 6-token marker form
	// `[if param is T [then] [else]]`.
	if len(toks) == 4 && isWordNamed(toks[0], "if") {
		if pe, ok := toks[1].Data.(core.ParenExprPayload); ok {
			inner := stripGuardMarkers(pe.Toks)
			if len(inner) == 3 {
				toks = []core.Value{toks[0], inner[0], inner[1], inner[2], toks[2], toks[3]}
			}
		}
	}
	if len(toks) == 6 && isWordNamed(toks[0], "if") {
		t, ok := guardTripleType(r, param, toks[1], toks[2], toks[3])
		if !ok {
			return nil, false
		}
		// Both arms are single-token bodies; the ELSE arm must be `false` — the
		// structure that makes `pred x` true imply `x is T`.
		if !toks[4].Parent.Equal(core.TList) || !toks[5].Parent.Equal(core.TList) {
			return nil, false
		}
		elseArm, err := core.AsList(toks[5])
		if err != nil || elseArm.IsNil() || elseArm.Len() != 1 {
			return nil, false
		}
		ev := elseArm.Get(0)
		if core.IsConcrete(ev) && ev.Parent.ConformsTo(core.TBoolean) {
			if b, berr := core.AsBoolean(ev); berr == nil && !b {
				return t, true
			}
		}
		if isWordNamed(ev, "false") {
			return t, true
		}
	}
	return nil, false
}

// guardTripleType resolves the `param is T` triple to T. w0 must name param,
// w1 must be `is`, and tv must resolve to a bare type node (not None). Type
// names appear as bare Words in a fn body, so tv is resolved through the def
// table, then the kernel name table, then the external-builtin path resolver —
// the same cascade stepWord uses.
func guardTripleType(r *core.Registry, param string, w0, w1, tv core.Value) (*core.Type, bool) {
	if !isWordNamed(w0, param) || !isWordNamed(w1, "is") {
		return nil, false
	}
	if core.IsWord(tv) {
		inner, err := core.AsWord(tv)
		if err != nil {
			return nil, false
		}
		switch {
		case defsHasType(r, inner.Name):
			e, _ := r.Defs.TopEntry(inner.Name)
			tv = e.Body
		default:
			t, ok := core.ResolveBuiltinTypeName(inner.Name)
			if !ok {
				return nil, false
			}
			tv = core.NewTypeLiteral(t)
		}
	}
	if !core.IsBareTypeNode(tv) || tv.Is(core.TNone) {
		return nil, false
	}
	return core.CanonicalType(r, core.ValueType(tv)), true
}

// defsHasType reports whether name has a live def-table binding.
func defsHasType(r *core.Registry, name string) bool {
	_, ok := r.Defs.TopEntry(name)
	return ok
}

// isWordNamed reports whether v is a concrete Word whose name is `name`. The
// concreteness probe uses IsConcrete rather than a raw `Data == nil` (the
// data_nil_gate discipline) — a Word carrier is not a literal word token.
func isWordNamed(v core.Value, name string) bool {
	if !v.Parent.Equal(core.TWord) || !core.IsConcrete(v) {
		return false
	}
	w, err := core.AsWord(v)
	return err == nil && w.Name == name
}

// stripGuardMarkers drops the structural paren / statement-end markers that
// wrap a group's tokens, leaving only the semantic operands.
func stripGuardMarkers(toks []core.Value) []core.Value {
	out := make([]core.Value, 0, len(toks))
	for _, t := range toks {
		if core.IsOpenParen(t) || core.IsCloseParen(t) || core.IsEnd(t) {
			continue
		}
		out = append(out, t)
	}
	return out
}
