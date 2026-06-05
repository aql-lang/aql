package native

import (
	"fmt"

	eng "github.com/aql-lang/aql/eng/go"
)

// Macro system words. See design/MACROS.0.md and design/MACROS-PHASE1.0.md.
//
// - gensym (Phase 0): fresh non-colliding atoms.
// - macro (1c): the definer — an fn the expander runs on UNEVALUATED operand
//   forms, whose returned token list is spliced into the call site.
// - unquote / splice (1d): template escapes, recognized by the expander
//   (eng/go/macro_expand.go); they error if stepped outside an expansion.
// - macroexpand (1e): introspection — return the expansion without splicing.

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
	{
		Name: "macro",
		// `macro [[params] [body]]` builds a macro: an fn whose every param is
		// raw-capture (FormArgs + NoEvalArgs + NoEvalMapArgs) and whose body
		// runs at expansion time, the returned template spliced into the call
		// site. NoEvalArgs{0}: the [[params][body]] list must not be evaluated
		// (the template body must not run at definition time).
		Signatures: []NativeSig{{
			Args:       []*Type{TList},
			NoEvalArgs: map[int]bool{0: true},
			Handler:    macroHandler,
			Returns:    []*Type{TFunction}, BarrierPos: -1,
		}},
	},
	{
		Name: "unquote",
		// Template escape — recognized by the expander. Stepped outside an
		// expansion it is a misuse and errors.
		Signatures: []NativeSig{{
			Args:    []*Type{TAny},
			Handler: unquoteOutsideMacroHandler,
			Returns: []*Type{TAny}, BarrierPos: -1,
		}},
	},
	{
		Name: "splice",
		Signatures: []NativeSig{{
			Args:    []*Type{TAny},
			Handler: spliceOutsideMacroHandler,
			Returns: []*Type{TAny}, BarrierPos: -1,
		}},
	},
	{
		Name: "macroexpand",
		// `macroexpand (mac args…)` — return the expanded token list as data
		// (a List) WITHOUT splicing/running it. Introspection for testing and
		// debugging. The macro call is captured raw (FormArgs{0}); see
		// macroexpandHandler.
		Signatures: []NativeSig{{
			Args:     []*Type{TAny},
			FormArgs: map[int]bool{0: true},
			Handler:  macroexpandHandler,
			Returns:  []*Type{TList}, BarrierPos: -1,
		}},
	},
}

// gensymHandler returns a fresh atom whose name is guaranteed not to collide
// with any prior gensym in this registry (`tmp$G1`, `tmp$G2`, …).
func gensymHandler(_ []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	return []Value{NewAtom(r.NextGensym())}, nil
}

// macroHandler builds a macro FnDef from `[[params] [body]]`. Mirrors the afn
// definer, but flags Macro=true and sets FormArgs/NoEvalArgs/NoEvalMapArgs on
// every param so operands arrive as raw code.
func macroHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	spec, err := RequireConcreteList(args[0], "macro")
	if err != nil {
		return nil, err
	}
	elems := spec.Slice()
	if len(elems) != 2 {
		return nil, r.AqlError("macro_error",
			fmt.Sprintf("macro: expected [[params] [body]] (2 elements), got %d", len(elems)), "macro")
	}
	// Macro params are plain NAMES of type Any (operands arrive as raw code,
	// so there is nothing to type-match). `[cond body]` → two named Any
	// params. (parseFnParams treats bare words as type names, which is wrong
	// here; typed macro params are a later extension.)
	paramsSpec := elems[0]
	if !paramsSpec.Parent.Equal(TList) || !IsConcrete(paramsSpec) {
		return nil, r.AqlError("macro_error", "macro: params must be a list of names", "macro")
	}
	pl, _ := AsList(paramsSpec)
	params := make([]FnParam, 0, pl.Len())
	for i := 0; i < pl.Len(); i++ {
		el := pl.Get(i)
		if !IsWord(el) {
			return nil, r.AqlError("macro_error",
				"macro: each param must be a bare name (typed params are a later extension)", "macro")
		}
		w, _ := AsWord(el)
		params = append(params, FnParam{Name: w.Name, Type: TAny})
	}
	if len(params) == 0 {
		return nil, r.AqlError("macro_error", "macro: needs at least one parameter", "macro")
	}
	barrierPos := len(params) // all operands forward-eligible

	// Body: a list runs as the template-producing program; a bare value is
	// wrapped (mirrors afn).
	var bodyElems []Value
	if elems[1].Parent.Equal(TList) && IsConcrete(elems[1]) {
		bl, _ := AsList(elems[1])
		bodyElems = bl.Slice()
	} else {
		bodyElems = []Value{elems[1]}
	}

	// Every param is raw-capture: FormArgs (word/paren/literal raw), NoEvalArgs
	// (a list operand stays un-evaluated), NoEvalMapArgs (a map operand keeps
	// its values un-evaluated). See design/MACROS-PHASE1.0.md §3/§3.1.
	form := make(map[int]bool, len(params))
	for i := range params {
		form[i] = true
	}
	sig := FnSig{
		Params:        params,
		Returns:       []*Type{TAny},
		Body:          bodyElems,
		BarrierPos:    barrierPos,
		FormArgs:      form,
		NoEvalArgs:    form,
		NoEvalMapArgs: form,
	}
	fnDef := FnDefInfo{
		Signatures: []FnSig{sig},
		Macro:      true,
		Captured:   eng.ComputeCaptures(r, &sig),
	}
	// (Re)constructing a macro invalidates any memoized expansions: a
	// redefined macro must re-expand at its call sites.
	r.MacroCacheClear()
	return []Value{NewFunction(fnDef)}, nil
}

// unquoteOutsideMacroHandler / spliceOutsideMacroHandler fire only when the
// word is stepped outside a macro template (the expander consumes them by name
// during expansion and they never reach dispatch there).
func unquoteOutsideMacroHandler(_ []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	return nil, r.AqlError("unquote_error", "unquote: only valid inside a macro template", "unquote")
}

func spliceOutsideMacroHandler(_ []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	return nil, r.AqlError("splice_error", "splice: only valid inside a macro template", "splice")
}

// macroexpandHandler returns the expansion of a macro call as a List, without
// splicing or running it. args[0] is the raw `(mac operand…)` form (FormArgs);
// it delegates to the kernel expander via eng.ExpandMacroForm.
func macroexpandHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	toks, err := eng.ExpandMacroForm(r, args[0])
	if err != nil {
		return nil, err
	}
	return []Value{NewList(toks)}, nil
}
