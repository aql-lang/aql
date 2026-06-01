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
		// The re-dispatch layout must respect the ORIGINAL sig's barrier so a
		// stack-only or mixed-barrier original still receives its args in the
		// right place (the wrapper itself stays all-forward). Resolve the
		// sentinel (-1 → all-forward) and clamp to [0, N].
		origBarrier := src.BarrierPos
		if origBarrier < 0 || origBarrier > len(src.Params) {
			origBarrier = len(src.Params)
		}
		rev.Handler = usurpDispatchHandler(orig, origBarrier)
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
// first value the caller wrote). To realise usurped a0…a(N-1) ≡
// orig a(N-1)…a0, it re-emits the ORIGINAL function value with the reversed
// args laid out around it, paren-wrapped so the re-dispatch is atomic.
//
// WHERE the args go depends on the original sig's barrier B (= origBarrier):
// the original collects positions 0…B-1 from forward tokens (after orig) and
// positions B…N-1 from the stack (before orig, top-first). Its natural call
// layout is therefore  [pN-1 … pB] orig [p0 … pB-1].  Substituting the
// reversal pi = a(N-1-i) gives one layout that is correct for EVERY barrier:
//
//		( a0 a1 … a(N-1-B)   orig   a(N-1) a(N-2) … a(N-B) )
//		  └── stack part ──┘        └──── forward part ────┘
//
//	  - B == N (all-forward, the common case): stack part empty → ( orig aN-1…a0 ).
//	  - B == 0 (stack-only): forward part empty → ( a0…aN-1 orig ); orig reads
//	    the stack top-first, yielding the reversal. (Fixes usurp of stack-only
//	    words, which previously left the wrapper inert.)
//	  - 0 < B < N (mixed): both parts populated, args split at the barrier.
//
// The original sig runs unchanged, preserving every original behaviour
// (barriers, return checks, closures, module scope).
func usurpDispatchHandler(orig Value, origBarrier int) Handler {
	return func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
		n := len(args)
		b := origBarrier
		if b < 0 || b > n {
			b = n
		}
		out := make([]Value, 0, n+3)
		out = append(out, NewOpenParen())
		// Stack part: a0 … a(N-1-B), in order (bottom→top).
		for i := 0; i < n-b; i++ {
			out = append(out, args[i])
		}
		out = append(out, orig)
		// Forward part: a(N-1) down to a(N-B).
		for i := n - 1; i >= n-b; i-- {
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
