package native

import (
	"fmt"

	eng "github.com/aql-lang/aql/eng/go"
)

// Macro system words. See design/MACROS.8.md and design/MACROS-PHASE1.10.md.
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
			// Pure mint (a per-registry counter → a fresh `tmp$G<n>` atom), so
			// it runs in check mode too: a hand-hygiene macro that binds
			// `def g (gensym)` and splices `def unquote g …` needs a REAL name
			// during the check pass, or the expansion produces `def <empty>`
			// (invalid_word_name). Running it also keeps the check-time and
			// runtime gensym counters in lockstep, so the compiled expansion is
			// byte-identical to the interpreted one.
			RunInCheckMode: true,
			Returns:        []*Type{TAtom}, BarrierPos: 0,
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
			// Pure construction, like fn/fnsig — runs in check mode so
			// the macro INSTALLS during the check pass: its uses then
			// expand on the tape (execMacro) and the checker (and the
			// bytecode recording pass) see the expanded stream, never
			// the raw-form operand span (plan R6 #29).
			Returns: []*Type{TFunction}, BarrierPos: -1,
			RunInCheckMode: true,
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
	{
		Name: "mini",
		// `mini <kind> <src> <opts?>` — embedded mini-languages
		// (design/MINILANG.5.md). A macro in effect: the call expands to the
		// STANDARD minilang call
		//
		//	MiniLang.lang_<kind> <src> <opts> end
		//
		// spliced at the call site (the trailing `end` stops the generated
		// word's forward collection, so kind-declared inputs always come
		// from the STACK and the tokens after the mini call are never
		// stolen). The kind names the expansion target and must be a
		// literal; it is resolved against the imported `MiniLang` namespace
		// at expansion time — unknown kinds fail loudly here, not
		// downstream. A missing opts is normalized to {}.
		//
		// RunInCheckMode: the handler is check-safe (the splice is built
		// from the collected values whether or not they are concrete), so
		// the checker steps the expansion and validates the call against
		// the kind's standard signature.
		Signatures: []NativeSig{
			{
				Args:           []*Type{TAtom, TString, TMap},
				QuoteArgs:      map[int]bool{0: true},
				Handler:        miniHandler,
				RunInCheckMode: true,
				Returns:        []*Type{TAny}, BarrierPos: -1,
			},
			{
				Args:           []*Type{TAtom, TString},
				QuoteArgs:      map[int]bool{0: true},
				Handler:        miniHandler,
				RunInCheckMode: true,
				Returns:        []*Type{TAny}, BarrierPos: -1,
			},
		},
	},
	{
		Name: "emit",
		// `emit <kind> <opts?> <data>` (explicit) and `emit <opts?> <data>`
		// (auto) — the symmetric inverse of `parse`. The call expands to the
		// STANDARD emitter call
		//
		//	EmitLang get emit_<kind|auto> <data> <opts> end
		//
		// spliced at the call site. `data` is the required LAST surface
		// argument; `opts` is the optional middle one (disambiguated by
		// arity). Unlike `parse`, the kind is OPTIONAL: when the leading
		// operand is not a bare word naming a registered emit kind, the call
		// routes to emit_auto (the value's natural format). The data is often
		// a bare variable word, so position 0 is /q-captured (word→atom) and
		// reconstructed as a Word for the splice when it is data, not a kind —
		// so a variable evaluates normally at the call site.
		// Two sig families. The EXPLICIT-kind family declares slot 0 as TAtom
		// with QuoteArgs{0}: when the next surface token is a bare word (the
		// kind, or a bare-variable data word), the matcher's preferWordSig path
		// selects these and the /q forward-quote intercept captures the word as
		// an Atom (instead of dispatching it — exactly how `parse`'s TAtom slot
		// works). The AUTO family declares slot 0 as TAny (no quote): it matches
		// when the leading operand is a literal map/list/scalar (data-first,
		// no kind). The handler then classifies an Atom slot 0 as kind-or-data.
		Signatures: []NativeSig{
			{
				Args:           []*Type{TAtom, TAny, TAny},
				QuoteArgs:      map[int]bool{0: true},
				Handler:        emitHandler,
				RunInCheckMode: true,
				Returns:        []*Type{TString}, BarrierPos: -1,
			},
			{
				Args:           []*Type{TAtom, TAny},
				QuoteArgs:      map[int]bool{0: true},
				Handler:        emitHandler,
				RunInCheckMode: true,
				Returns:        []*Type{TString}, BarrierPos: -1,
			},
			{
				Args:           []*Type{TAtom},
				QuoteArgs:      map[int]bool{0: true},
				Handler:        emitHandler,
				RunInCheckMode: true,
				Returns:        []*Type{TString}, BarrierPos: -1,
			},
			{
				Args:           []*Type{TAny, TAny},
				Handler:        emitHandler,
				RunInCheckMode: true,
				Returns:        []*Type{TString}, BarrierPos: -1,
			},
			{
				Args:           []*Type{TAny},
				Handler:        emitHandler,
				RunInCheckMode: true,
				Returns:        []*Type{TString}, BarrierPos: -1,
			},
		},
	},
	{
		Name: "parse",
		// `parse <kind> <opts?> <source>` — named parsers (the sibling of
		// `mini`; design/MINILANG.5.md + the ParseLang module). A macro in
		// effect: the call expands to the STANDARD parser call
		//
		//	ParseLang.parse_<kind> <source> <opts> end
		//
		// spliced at the call site. Unlike `mini`, the `source` is the
		// REQUIRED LAST surface argument (a String or a `{src:…}` Source
		// map) and `opts` is the optional MIDDLE one — disambiguated by
		// arity, so a Map source never collides with a Map opts. The kind
		// names the expansion target and must be a literal; it is resolved
		// against the imported `ParseLang` namespace at expansion time —
		// unknown kinds fail loudly here. A parser returns Any (an AST, a
		// transduction, …, per the language).
		Signatures: []NativeSig{
			{
				Args:           []*Type{TAtom, TMap, TAny},
				QuoteArgs:      map[int]bool{0: true},
				Handler:        parseHandler,
				RunInCheckMode: true,
				Returns:        []*Type{TAny}, BarrierPos: -1,
			},
			{
				Args:           []*Type{TAtom, TAny},
				QuoteArgs:      map[int]bool{0: true},
				Handler:        parseHandler,
				RunInCheckMode: true,
				Returns:        []*Type{TAny}, BarrierPos: -1,
			},
		},
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
	// its values un-evaluated). See design/MACROS-PHASE1.10.md §3/§3.1.
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

// miniHandler expands `mini <kind> <src> <opts?>` into the standard minilang
// call `MiniLang.lang_<kind> <src> <opts> end`, returned as an __SP splice so
// the generated tokens re-step at the call site (the `word` mechanism).
// args[0]=kind (Atom via /q — must be literal), args[1]=src, args[2]=opts
// (2-arg form normalizes to {}). src/opts are spliced as collected — they may
// be carriers in check mode; only the kind must be concrete.
func miniHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	kind, err := args[0].AsConcreteAtom()
	if err != nil {
		return nil, r.AqlErrorHint("mini_error",
			"mini: the kind must be a literal name", "mini",
			"write the kind as a bare word: mini re '[a-z]+'")
	}
	target := "lang_" + kind

	// Resolve the kind against the imported MiniLang namespace NOW —
	// unknown kinds are expansion-time errors at the call site.
	if !miniKindRegistered(r, target) {
		if r.Check.IsActive() && !miniNamespaceBound(r) {
			// The import may be outside the checked fragment; degrade to a
			// dynamic value rather than a false-positive diagnostic. A bound
			// namespace WITHOUT the kind is a real bug in any mode. The
			// carrier is dynamic(Any) — the documented escape-hatch
			// modality — not strict Any, which would poison every typed
			// consumer downstream (checker-accuracy-review.10.md A8).
			//
			// For the COMPILE pass the interpreter raises mini_unknown_lang here
			// at runtime, so record a TERMINAL trap (top-level only) — the
			// compiled program then raises the byte-identical error instead of
			// refusing on the dynamic carrier downstream. A nested call declines
			// the trap and keeps the lenient fallback.
			r.Check.Emit.RecordTrap("mini_unknown_lang",
				fmt.Sprintf("mini: no mini-language %q is registered", kind), "mini",
				`import "aql:minilang" first; register custom kinds with MiniLang.register; MiniLang.kinds lists what is loaded`,
				args[0].Pos)
			return []Value{NewDynamicCarrier(TAny)}, nil
		}
		return nil, r.AqlErrorHint("mini_unknown_lang",
			fmt.Sprintf("mini: no mini-language %q is registered", kind), "mini",
			`import "aql:minilang" first; register custom kinds with MiniLang.register; MiniLang.kinds lists what is loaded`)
	}

	opts := NewMap(NewOrderedMap())
	if len(args) == 3 {
		opts = args[2]
	}

	// Compile-hook path (design/MINILANG.5.md §13). When the kind registered
	// a compile hook AND src is concrete, compile at the call site and splice
	// the hook's tokens instead of the standard call. NOT in check mode: the
	// checker validates the standard `lang_<kind>` call (the semantic
	// reference), and a check-mode src is a carrier with no text to compile.
	// `mini` has no expansion cache, so the hook re-runs whenever the call is
	// stepped — hooks memoize their compile (as `re` does). A non-concrete
	// runtime src falls back to the standard transducer call.
	if !r.Check.IsActive() {
		goHook, hasGo := miniGoHook(r, kind)
		aqlHook, hasAQL := miniCompileExport(r, kind)
		if hasGo || hasAQL {
			if src, serr := args[1].AsConcreteString(); serr == nil {
				var hookToks []Value
				var herr error
				if hasGo {
					hookToks, herr = goHook(src, opts, r)
				} else {
					hookToks, herr = miniInvokeAQLCompile(r, kind, aqlHook, src, opts)
				}
				if herr != nil {
					return nil, herr
				}
				return []Value{NewSplice(NewList(hookToks))}, nil
			}
		}
	}

	toks := []Value{
		NewWord("MiniLang"), NewWord("dot"), NewWord(target),
		args[1], opts, NewEnd(),
	}
	return []Value{NewSplice(NewList(toks))}, nil
}

// miniCompileExport returns kind's AQL compile-hook fn (the `compile_<kind>`
// export of the bound MiniLang namespace), if present.
func miniCompileExport(r *Registry, kind string) (Value, bool) {
	top, ok := r.Defs.Top("MiniLang")
	if !ok {
		return Value{}, false
	}
	info, ok := asModuleExportInfo(top)
	if !ok || info.Fields == nil {
		return Value{}, false
	}
	return info.Fields.Get("compile_" + kind)
}

// miniInvokeAQLCompile runs an AQL compile hook at expansion time. An AQL hook
// is a MACRO (template with quote/unquote): the language's list literals don't
// capture locals, so a plain fn can't build a value-injected token list — the
// macro template is the vehicle. It is expanded against [src, opts] and the
// resulting tokens are what `mini` splices.
func miniInvokeAQLCompile(r *Registry, kind string, fn Value, src string, opts Value) ([]Value, error) {
	fnDef, ok := fn.Data.(FnDefInfo)
	if !ok || !fnDef.Macro || len(fnDef.Signatures) == 0 {
		return nil, r.AqlErrorHint("mini_bad_compiler",
			fmt.Sprintf("compile_%s must be a macro", kind), "mini",
			"register it as MiniLang.register-compiled "+kind+" (macro [[src opts] [ quote [ … ] ]])")
	}
	return eng.ExpandMacroWith(r, &fnDef, []Value{NewString(src), opts})
}

// miniNamespaceBound reports whether the `MiniLang` namespace is bound to a
// ModuleExport in the current scope.
func miniNamespaceBound(r *Registry) bool {
	top, ok := r.Defs.Top("MiniLang")
	if !ok {
		return false
	}
	_, ok = asModuleExportInfo(top)
	return ok
}

// miniKindRegistered reports whether `lang_<kind>` is an export of the bound
// MiniLang namespace.
func miniKindRegistered(r *Registry, target string) bool {
	top, ok := r.Defs.Top("MiniLang")
	if !ok {
		return false
	}
	info, ok := asModuleExportInfo(top)
	if !ok || info.Fields == nil {
		return false
	}
	_, ok = info.Fields.Get(target)
	return ok
}

// parseHandler expands `parse <kind> <opts?> <source>` into the standard
// parser call `ParseLang.parse_<kind> <source> <opts> end`, returned as an
// __SP splice so the generated tokens re-step at the call site (the same
// mechanism as `mini`). args[0]=kind (Atom via /q — must be literal); the
// `source` is the required LAST surface arg, `opts` the optional middle one
// (2-arg form normalizes to {}). source/opts are spliced as collected — they
// may be carriers in check mode; only the kind must be concrete.
func parseHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	kind, err := args[0].AsConcreteAtom()
	if err != nil {
		return nil, r.AqlErrorHint("parse_error",
			"parse: the kind must be a literal name", "parse",
			"write the kind as a bare word: parse calc 'x + y'")
	}
	target := "parse_" + kind

	// Resolve the kind against the imported ParseLang namespace NOW —
	// unknown kinds are expansion-time errors at the call site.
	if !parseKindRegistered(r, target) {
		if r.Check.IsActive() && parseKindDeferred(r, kind) {
			// A Parse.register kind (aql:parse): its grammar is built by Parse
			// builder words whose result is NOT a concrete value during analysis,
			// so the parser can't be installed for the checker — but it IS
			// registered at run time by the Parse.register call. Degrade to a
			// dynamic value WITHOUT a trap (unlike the unbound-namespace case
			// below): the runtime resolves the kind, so the compiled program must
			// NOT raise parse_unknown_lang. The check-mode hook that records the
			// deferral is aql:parse's parse-register ReturnsFn.
			return []Value{NewDynamicCarrier(TAny)}, nil
		}
		if r.Check.IsActive() && !parseNamespaceBound(r) {
			// The import may be outside the checked fragment; degrade to a
			// dynamic value rather than a false-positive diagnostic (mirror
			// of miniHandler). For the COMPILE pass record a TERMINAL trap
			// (top-level only) so the compiled program raises the byte-identical
			// parse_unknown_lang the interpreter raises here; a nested call
			// declines and keeps the lenient fallback.
			r.Check.Emit.RecordTrap("parse_unknown_lang",
				fmt.Sprintf("parse: no parser %q is registered", kind), "parse",
				`import "aql:parselang" first; register parsers with ParseLang.register; ParseLang.kinds lists what is loaded`,
				args[0].Pos)
			return []Value{NewDynamicCarrier(TAny)}, nil
		}
		return nil, r.AqlErrorHint("parse_unknown_lang",
			fmt.Sprintf("parse: no parser %q is registered", kind), "parse",
			`import "aql:parselang" first; register parsers with ParseLang.register; ParseLang.kinds lists what is loaded`)
	}

	// Surface: parse <kind> <opts?> <source>. source is the required last
	// arg; opts is optional. Map surface→emission to the standard parser
	// call shape [source opts].
	var opts, source Value
	if len(args) == 3 {
		opts = args[1]
		source = args[2]
	} else {
		opts = NewMap(NewOrderedMap())
		source = args[1]
	}
	toks := []Value{
		NewWord("ParseLang"), NewWord("dot"), NewWord(target),
		source, opts, NewEnd(),
	}
	return []Value{NewSplice(NewList(toks))}, nil
}

// parseNamespaceBound reports whether the `ParseLang` namespace is bound to a
// ModuleExport in the current scope.
func parseNamespaceBound(r *Registry) bool {
	top, ok := r.Defs.Top("ParseLang")
	if !ok {
		return false
	}
	_, ok = asModuleExportInfo(top)
	return ok
}

// parseKindRegistered reports whether `parse_<kind>` is an export of the
// bound ParseLang namespace.
// capParseDeferredKinds holds the set of parse-kind names a Parse.register call
// will install at RUNTIME — recorded in check mode so `parse <kind>` resolves
// leniently during analysis even though the built grammar isn't concrete yet.
const capParseDeferredKinds = "engine.parse.deferred-kinds"

// MarkParseKindDeferred records (during check) that `kind` is registered at run
// time by an aql:parse `Parse.register <kind> <built-grammar>` call. The grammar
// is constructed by Parse builder words whose result is not a concrete value
// under static analysis, so the parser can't be installed for the checker; the
// `parse` macro therefore degrades `parse <kind>` to a dynamic value (no
// parse_unknown_lang, no trap) for a deferred kind. Called from aql:parse's
// parse-register check-mode ReturnsFn.
func MarkParseKindDeferred(r *Registry, kind string) {
	if r == nil || kind == "" {
		return
	}
	if m, ok, _ := eng.Cap[map[string]bool](r, capParseDeferredKinds); ok && m != nil {
		m[kind] = true
		return
	}
	_ = r.Capabilities.Set(capParseDeferredKinds, map[string]bool{kind: true})
}

// ResetParseDeferredKinds clears the deferred parse-kind set so it is scoped to
// a single check pass. The set lives in the persistent Capabilities store, so a
// reused AQL instance would otherwise carry a prior pass's `Parse.register`
// kinds into a later Check: `parse <kind>` for that kind would silently degrade
// to a dynamic value (no parse_unknown_lang) even though the kind is no longer
// registered. Called at the start of every check pass (see lang.(*AQL).Check /
// CompileCheck) so deferral is per-check-run, not per-instance lifetime.
func ResetParseDeferredKinds(r *Registry) {
	if r == nil || r.Capabilities == nil {
		return
	}
	_, _ = r.Capabilities.Delete(capParseDeferredKinds)
}

// parseKindDeferred reports whether `kind` was marked as a runtime-registered
// Parse.register kind (see MarkParseKindDeferred).
func parseKindDeferred(r *Registry, kind string) bool {
	m, ok, _ := eng.Cap[map[string]bool](r, capParseDeferredKinds)
	return ok && m != nil && m[kind]
}

func parseKindRegistered(r *Registry, target string) bool {
	top, ok := r.Defs.Top("ParseLang")
	if !ok {
		return false
	}
	info, ok := asModuleExportInfo(top)
	if !ok || info.Fields == nil {
		return false
	}
	_, ok = info.Fields.Get(target)
	return ok
}

// emitHandler expands `emit <kind> <opts?> <data>` (explicit) or `emit
// <opts?> <data>` (auto) into the standard emitter call `EmitLang get
// emit_<kind|auto> <data> <opts> end`, returned as an __SP splice so the
// generated tokens re-step at the call site (the same mechanism as `parse`).
//
// Position 0 is /q-captured (word→atom). Classification:
//   - leading operand is an Atom W and `emit_<W>` is registered in the bound
//     EmitLang namespace → EXPLICIT (kind=W); the remaining operands are
//     opts? then data.
//   - otherwise → AUTO (emit_auto); the leading operand is data (a /q'd bare
//     word is reconstructed as a Word so a variable evaluates) and any prior
//     operand is opts.
//
// data is always the LAST operand; opts the optional middle one. data/opts are
// spliced as collected — they may be carriers in check mode.
func emitHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	// Try to read the leading operand as a kind name (a /q'd bare word).
	kind, isWord := "", false
	if a, err := args[0].AsConcreteAtom(); err == nil {
		kind, isWord = a, true
	}

	// A leading bare word with at least one operand after it is an EXPLICIT
	// kind attempt; a lone leading word is a bare-variable `emit m`.
	leadingKind := isWord && len(args) >= 2
	explicit := leadingKind && emitKindRegistered(r, "emit_"+kind)

	// A leading bare word that is neither a registered kind nor a bound value is
	// an unknown-kind typo → loud expansion-time error (mirror parse), with the
	// check-mode dynamic-carrier fallback. A leading word that resolves to a
	// VALUE (e.g. an options map held in a variable: `def opts {…} emit opts
	// {a:1}`) is DATA/OPTS, not a kind, and falls through to the auto form.
	if leadingKind && !explicit && !r.Defs.Has(kind) {
		if r.Check.IsActive() && !emitNamespaceBound(r) {
			r.Check.Emit.RecordTrap("emit_unknown_lang",
				fmt.Sprintf("emit: no emitter %q is registered", kind), "emit",
				`import "aql:emitlang" first; register emitters with EmitLang.register; EmitLang.kinds lists what is loaded`,
				args[0].Pos)
			return []Value{NewDynamicCarrier(TString)}, nil
		}
		return nil, r.AqlErrorHint("emit_unknown_lang",
			fmt.Sprintf("emit: no emitter %q is registered", kind), "emit",
			`import "aql:emitlang" first; register emitters with EmitLang.register; EmitLang.kinds lists what is loaded`)
	}

	// Reconstruct a /q'd leading bare word as a Word so the variable it names
	// (used as data in `emit m`, or as the options map in `emit opts {a:1}`)
	// evaluates at the call site rather than being spliced as a literal atom.
	lead := args[0]
	if isWord {
		lead = NewWord(kind)
	}

	target := "emit_auto"
	var data, opts Value
	if explicit {
		target = "emit_" + kind
		if len(args) == 3 {
			opts = args[1]
			data = args[2]
		} else { // len == 2
			opts = NewMap(NewOrderedMap())
			data = args[1]
		}
	} else {
		// Auto: `emit <opts?> <data>` — data is the last operand, opts the
		// optional leading one (reconstructed above when it is a bare word).
		if len(args) >= 2 {
			opts = lead
			data = args[len(args)-1]
		} else { // len == 1: the data
			opts = NewMap(NewOrderedMap())
			data = lead
		}
	}

	toks := []Value{
		NewWord("EmitLang"), NewWord("dot"), NewWord(target),
		data, opts, NewEnd(),
	}
	return []Value{NewSplice(NewList(toks))}, nil
}

// emitNamespaceBound reports whether the `EmitLang` namespace is bound to a
// ModuleExport in the current scope.
func emitNamespaceBound(r *Registry) bool {
	top, ok := r.Defs.Top("EmitLang")
	if !ok {
		return false
	}
	_, ok = asModuleExportInfo(top)
	return ok
}

// emitKindRegistered reports whether `emit_<kind>` is an export of the bound
// EmitLang namespace.
func emitKindRegistered(r *Registry, target string) bool {
	top, ok := r.Defs.Top("EmitLang")
	if !ok {
		return false
	}
	info, ok := asModuleExportInfo(top)
	if !ok || info.Fields == nil {
		return false
	}
	_, ok = info.Fields.Get(target)
	return ok
}
