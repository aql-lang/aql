package native

// Macro system words. See design/MACROS.0.md and design/MACROS-PHASE1.0.md.
//
// Phase 0 (this file, so far): `gensym` — a fresh non-colliding atom for
// capture-free temporaries. Later phases add `macro`, `unquote`, `splice`,
// and `macroexpand`.

// macroNatives is the macro-system word set, wired into Register (register.go).
var macroNatives = []NativeFunc{
	{
		Name: "gensym",
		// `gensym` mints a fresh, never-colliding atom (`tmp$G<n>`) — the
		// Common-Lisp temporary generator. Capture-free temporaries for
		// hand-written `word`/`__SP` macros today, and the manual-hygiene
		// tool before automatic hygiene (Phase 4) lands. Zero args.
		Signatures: []NativeSig{{
			Args:    []*Type{},
			Handler: gensymHandler,
			Returns: []*Type{TAtom}, BarrierPos: 0,
		}},
	},
}

// gensymHandler returns a fresh atom whose name is guaranteed not to collide
// with any prior gensym in this registry (`tmp$G1`, `tmp$G2`, …).
func gensymHandler(_ []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	return []Value{NewAtom(r.NextGensym())}, nil
}
