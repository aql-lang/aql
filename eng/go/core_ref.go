package eng

// ResolveRef looks up name in the registry and returns the value-form
// of its current binding without invoking. Resolution mirrors the
// priority used by stepWord:
//
//  1. A type binding (capitalised def, refine-prefab) returns its
//     stored body — typically a type literal.
//  2. A value binding returns the bound value: an FnDef binding is
//     wrapped as a Function value; every other binding is returned
//     as-is.
//
// The returned Function value is UNQUOTED. Under the dispatch rules
// of this engine, an unquoted Function on the stack is a live call
// site — full signature matching (forward + stack) applies the next
// time the engine processes it. To capture as inert data, wrap with
// `quote` at the call site.
//
// The second return is false when the name is not bound at all. The
// caller decides how to report the failure — the `ref` word raises
// an undefined_word error, the /r short-circuit in stepWord does the
// same.
//
// Lives in eng because stepWord's /r path needs it during the run
// loop; the `ref` word itself is registered in the language layer.
func ResolveRef(r *Registry, name string) (Value, bool) {
	if r == nil {
		return Value{}, false
	}
	if tv, ok := r.TopTypeBody(name); ok {
		return tv, true
	}
	top, ok := r.Defs.Top(name)
	if !ok {
		return Value{}, false
	}
	if _, ok := top.Data.(FnDefInfo); ok {
		// Wrap the aggregate dispatch view so the reference carries every
		// overload of the name, not just the topmost entry's own sigs.
		if fnDef := r.Lookup(name); fnDef != nil {
			return NewFunction(*fnDef), true
		}
	}
	return top, true
}

// IsFunctionRef reports whether a value resolved by ResolveRef is a
// function word — the only binding kind `/r` and `ref` are permitted to
// reference. The reference surfaces exist to break the asymmetry between
// value bindings (a bare name already pushes the value) and fn bindings
// (a bare name invokes); for a non-fn binding there is no asymmetry to
// break, so referencing it is meaningless and rejected. ResolveRef wraps
// every FnDef binding as a Function value (Parent == TFunction), so the
// predicate is a single Parent check; plain values and type bodies come
// back with their own Parent and are illegal ref targets.
func IsFunctionRef(v Value) bool {
	return v.Parent.Equal(TFunction)
}

// UsurpFunction wraps a function value so its signature argument order is
// reversed: a wrapped fn called `usurped a b c` dispatches the original as
// `f c b a`. It returns false when v is not a function value.
//
// The wrapper is a fresh TFunction whose signatures each copy an original
// signature with the per-position Params REVERSED (so the wrapper accepts
// the caller's args in usurped order, with Pattern / Optional / types still
// aligned to each slot) and an all-forward BarrierPos so the natural
// `usurped a b c` forward form matches. Each wrapper sig carries a Go
// handler that re-dispatches the ORIGINAL function with the matched args in
// reverse order (see usurpDispatchHandler). Because the original sig runs
// unchanged on re-dispatch, every original behaviour (barriers, quoting,
// return checks, closures, module scope) is preserved; usurp only reverses
// which caller argument lands in which original slot.
func UsurpFunction(v Value) (Value, bool) {
	if !v.Parent.Equal(TFunction) && !v.Parent.Equal(TFnDef) {
		return Value{}, false
	}
	fnDef, ok := v.Data.(FnDefInfo)
	if !ok {
		return Value{}, false
	}
	orig := NewFunction(fnDef)
	own := fnDef.OwnSigs()
	wrapped := make([]Signature, 0, len(own))
	for i := range own {
		src := own[i]
		rev := src // copy Returns (output shape is unchanged) and other flags
		rev.Params = reverseParams(src.Params)
		rev.BarrierPos = len(rev.Params) // all-forward: caller writes usurped a b c
		// Drop per-position fields that would now be mis-indexed; the
		// original sig re-applies them on re-dispatch. Args/Patterns are
		// rebuilt from Params by normalizeSig at registration time.
		rev.Args = nil
		rev.Patterns = nil
		rev.QuoteArgs = nil
		rev.TypeArgs = nil
		rev.NoEvalArgs = nil
		rev.NoEvalMapArgs = nil
		rev.Body = nil
		rev.ReturnsFn = nil
		rev.CheckFullStackFn = nil
		rev.Fallback = false
		rev.Handler = usurpDispatchHandler(orig)
		normalizeSig(&rev)
		wrapped = append(wrapped, rev)
	}
	SortSignatures(wrapped)
	return NewFunction(FnDefInfo{
		Name:           fnDef.Name,
		Signatures:     wrapped,
		MaxForwardArgs: calcMaxForwardArgs(wrapped),
		Registry:       fnDef.Registry,
	}), true
}

// reverseParams returns a new slice with the params in reverse order.
func reverseParams(params []FnParam) []FnParam {
	out := make([]FnParam, len(params))
	for i, p := range params {
		out[len(params)-1-i] = p
	}
	return out
}

// usurpDispatchHandler builds the handler for one usurped signature. The
// wrapper sig collected the caller's args in usurp order (args[0] is the
// first value the caller wrote). To realise usurped a b c ≡ f c b a, the
// handler emits the ORIGINAL function value followed by the args in REVERSE
// order, paren-wrapped: ( origFn argN-1 … arg1 arg0 ). The engine re-steps
// this; origFn forward-collects the reversed args, so the original's slot 0
// receives the caller's last-written arg. The original sig runs unchanged,
// preserving every original behaviour (barriers, return checks, closures,
// module scope). The paren keeps the re-dispatch atomic so an outer forward
// can't grab an intermediate value before the original call completes.
func usurpDispatchHandler(orig Value) Handler {
	return func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
		out := make([]Value, 0, len(args)+3)
		out = append(out, NewOpenParen())
		out = append(out, orig)
		for i := len(args) - 1; i >= 0; i-- {
			out = append(out, args[i])
		}
		out = append(out, NewCloseParen())
		return out, nil
	}
}

// ResolveUsurp resolves name to its bound function value and returns the
// usurped wrapper (see UsurpFunction). The second return is false when the
// name is unbound; an ok-but-non-function binding is returned as-is so the
// caller's IsFunctionRef check raises illegal_ref (mirrors ResolveRef).
func ResolveUsurp(r *Registry, name string) (Value, bool) {
	v, ok := ResolveRef(r, name)
	if !ok {
		return Value{}, false
	}
	if !IsFunctionRef(v) {
		return v, true
	}
	wrapped, _ := UsurpFunction(v)
	return wrapped, true
}
