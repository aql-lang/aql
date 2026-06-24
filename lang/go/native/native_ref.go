package native

import (
	"fmt"

	eng "github.com/aql-lang/aql/eng/go"
)

// refNatives registers the two words that complete AQL's first-class
// function-value pipeline:
//
//   - `ref name`  — resolves a function word to its bound value
//     without invoking; companion to the `/r` word suffix
//     that lives in the parser+stepWord path. Both are legal
//     only for function words — referencing a non-fn binding
//     raises [aql/illegal_ref] (a bare value name already
//     pushes its value, so there is nothing to reference).
//   - `apply fn`  — invokes a captured function value against the
//     preceding stack args. The opposite-direction
//     complement of `ref`: ref converts a call site
//     into a value, apply converts a value back into a
//     call site.
//
// Both words sit in lang because every other built-in does (eng
// ships only kernel-level shapes and parser features). The actual
// name-resolution algorithm lives in eng.ResolveRef so that
// stepWord's `/r` short-circuit and this `ref` handler share one
// definition.
var refNatives = []NativeFunc{
	{
		Name: "ref",

		Signatures: []NativeSig{{
			// /q on the name slot lets the parser capture the upcoming
			// Word as an Atom rather than executing it. `ref add` then
			// arrives here with args[0] = Atom(add).
			Args:      []*Type{TAtom},
			QuoteArgs: map[int]bool{0: true},
			Handler:   refHandler,
			Returns:   []*Type{TAny},
			// ParkResult: leave the resolved Function value as inert data at
			// the call site instead of re-stepping it — so `ref f` behaves
			// exactly like `f/r`, never auto-invoking (not even a 0-arg fn).
			// The value still dispatches when re-stepped elsewhere (from a
			// map, a paren), matching `(f/r)` / `ops.f a b`.
			ParkResult:     true,
			RunInCheckMode: true, BarrierPos: -1,
		}},
	},
	{
		Name: "apply",
		// Stack-only: `args... fn apply` reads as "take the function
		// off the stack and apply it to the preceding values." Forward
		// collection would force callers to put fn-args after the fn,
		// which fights AQL's left-to-right stack flow.

		Signatures: []NativeSig{
			{
				Args:    []*Type{TFunction},
				Handler: applyHandler,
				// Check mode: return the applied fn VALUE concrete (identity,
				// not widened to Any) so the check engine re-steps it exactly as
				// runtime — the fn then dispatches against its preceding stack
				// args and records as an ordinary CALL_USER, letting the bytecode
				// compiler lower `…args fn apply`. Runtime is unchanged
				// (applyHandler still re-steps the fn).
				ReturnsFn: ReturnsIdentity(0),
				Returns:   []*Type{TAny}, BarrierPos: 0,
			},
			// Apply a Reach (a lens) to a receiver: `apply $.name person`
			// rebinds the reach's receiver to `person` and evaluates it —
			// the lens "get". Forward-eligible so the reach reads first.
			{
				Args:    []*Type{TReach, TAny},
				Handler: applyReachHandler,
				Returns: []*Type{TAny}, BarrierPos: -1,
			},
		},
	},
	{
		Name: "rebind",
		// Compose, don't evaluate: `rebind $.name person` returns a NEW
		// Reach with `person` as the receiver (inert data — a bound lens).
		// Apply it / read it later with `apply` or `getpath`. For an
		// already-bound reach it swaps the receiver. Forward-eligible.
		Signatures: []NativeSig{{
			Args:    []*Type{TReach, TAny},
			Handler: rebindHandler,
			Returns: []*Type{TReach}, BarrierPos: -1,
		}},
	},
	{
		Name: "usurp",
		// Forward-eligible: `usurp fn` reads as "wrap this fn". Returns a
		// Function value, so it dispatches immediately when args follow
		// (`usurp (ref f) a b`) and stays inert under quote.
		//
		// Two overloads, mirroring how `ref` and `/u` accept a name:
		//   - [Function]      `usurp (ref f)` / `usurp (f/r)` — a value.
		//   - [Atom] (/q)     `usurp f` — capture the word as a name and
		//                     resolve it to its bound function (the
		//                     function-form companion of the `/u` suffix).
		// When the next token is a Word, matchSignature prefers the /q
		// Atom sig; a parenthesised Function value takes the [Function] sig.
		Signatures: []NativeSig{
			{
				Args:           []*Type{TFunction},
				Handler:        usurpHandler,
				Returns:        []*Type{TFunction},
				RunInCheckMode: true, BarrierPos: -1,
			},
			{
				Args:           []*Type{TAtom},
				QuoteArgs:      map[int]bool{0: true},
				Handler:        usurpAtomHandler,
				Returns:        []*Type{TFunction},
				RunInCheckMode: true, BarrierPos: -1,
			},
		},
	},
	{
		Name: "stack-args",
		// The function-form companion of the `/s` modifier: wrap a function
		// so it dispatches in STACK form. Like `usurp`, returns a new
		// Function and accepts a value ([Function]) or a name ([Atom] /q).
		Signatures: []NativeSig{
			{
				Args:           []*Type{TFunction},
				Handler:        stackArgsHandler,
				Returns:        []*Type{TFunction},
				RunInCheckMode: true, BarrierPos: -1,
			},
			{
				Args:           []*Type{TAtom},
				QuoteArgs:      map[int]bool{0: true},
				Handler:        stackArgsAtomHandler,
				Returns:        []*Type{TFunction},
				RunInCheckMode: true, BarrierPos: -1,
			},
		},
	},
	{
		Name: "force-arity",
		// The function-form companion of the `/N` modifier: wrap a function
		// so it dispatches with exactly N args. `force-arity N fn` returns a
		// new Function. Like usurp, accepts a function value or a name:
		//   - [Integer, Function]  `force-arity 2 (f/r)`
		//   - [Integer, Atom] (/q) `force-arity 2 f`
		Signatures: []NativeSig{
			{
				Args:           []*Type{TInteger, TFunction},
				Handler:        forceArityHandler,
				Returns:        []*Type{TFunction},
				RunInCheckMode: true, BarrierPos: -1,
			},
			{
				Args:           []*Type{TInteger, TAtom},
				QuoteArgs:      map[int]bool{1: true},
				Handler:        forceArityAtomHandler,
				Returns:        []*Type{TFunction},
				RunInCheckMode: true, BarrierPos: -1,
			},
		},
	},
	{
		Name: "forward-args",
		// The function-form companion of the `/f` modifier: wrap a function
		// so it dispatches in FORWARD form.
		Signatures: []NativeSig{
			{
				Args:           []*Type{TFunction},
				Handler:        forwardArgsHandler,
				Returns:        []*Type{TFunction},
				RunInCheckMode: true, BarrierPos: -1,
			},
			{
				Args:           []*Type{TAtom},
				QuoteArgs:      map[int]bool{0: true},
				Handler:        forwardArgsAtomHandler,
				Returns:        []*Type{TFunction},
				RunInCheckMode: true, BarrierPos: -1,
			},
		},
	},
}

// stack-args / forward-args mirror usurp: a value form (wrap a Function) and
// a by-name form (resolve a function word, then wrap).
func stackArgsHandler(args []Value, _ map[string]Value, _ []Value, reg *Registry) ([]Value, error) {
	return rebarrierResult(eng.ForceStackFunction, args[0], "stack-args", reg)
}
func forwardArgsHandler(args []Value, _ map[string]Value, _ []Value, reg *Registry) ([]Value, error) {
	return rebarrierResult(eng.ForceForwardFunction, args[0], "forward-args", reg)
}
func stackArgsAtomHandler(args []Value, _ map[string]Value, _ []Value, reg *Registry) ([]Value, error) {
	return rebarrierAtom(eng.ForceStackFunction, args, "stack-args", reg)
}
func forwardArgsAtomHandler(args []Value, _ map[string]Value, _ []Value, reg *Registry) ([]Value, error) {
	return rebarrierAtom(eng.ForceForwardFunction, args, "forward-args", reg)
}

func forceArityHandler(args []Value, _ map[string]Value, _ []Value, reg *Registry) ([]Value, error) {
	n, _ := args[0].AsConcreteInteger()
	wrapped, ok := eng.ForceArityFunction(args[1], int(n))
	if !ok {
		if out, gradual := checkModeGradualFn(reg, args[1]); gradual {
			return out, nil
		}
		detail := "force-arity requires a non-negative arity and a function value"
		if reg != nil {
			return nil, reg.AqlError("illegal_ref", detail, "force-arity")
		}
		return nil, fmt.Errorf("%s", detail)
	}
	return []Value{wrapped}, nil
}

// forceArityAtomHandler is the by-name form `force-arity N foo`: it
// resolves the captured atom to its bound function (sharing usurp's rules)
// and wraps it. The function-form companion of the `/N` suffix.
func forceArityAtomHandler(args []Value, _ map[string]Value, _ []Value, reg *Registry) ([]Value, error) {
	name, err := AsAtom(args[1])
	if err != nil {
		return nil, fmt.Errorf("force-arity: expected an atom name, got %s", args[1].Parent.String())
	}
	v, ok := eng.ResolveRef(reg, name)
	if !ok {
		if reg != nil {
			return nil, reg.AqlError("undefined_word", "force-arity: name "+name+" is not bound", name)
		}
		return nil, fmt.Errorf("force-arity: name %s is not bound", name)
	}
	if !eng.IsFunctionRef(v) {
		detail := "force-arity requires a function word: " + name + " is bound to " + v.Parent.String()
		if reg != nil {
			return nil, reg.AqlError("illegal_ref", detail, name)
		}
		return nil, fmt.Errorf("%s", detail)
	}
	return forceArityHandler([]Value{args[0], v}, nil, nil, reg)
}

func rebarrierResult(wrap func(Value) (Value, bool), v Value, word string, reg *Registry) ([]Value, error) {
	wrapped, ok := wrap(v)
	if !ok {
		if out, gradual := checkModeGradualFn(reg, v); gradual {
			return out, nil
		}
		detail := word + " requires a function value, got " + v.Parent.String()
		if reg != nil {
			return nil, reg.AqlError("illegal_ref", detail, word)
		}
		return nil, fmt.Errorf("%s", detail)
	}
	return []Value{wrapped}, nil
}

func rebarrierAtom(wrap func(Value) (Value, bool), args []Value, word string, reg *Registry) ([]Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("%s: missing name", word)
	}
	name, err := AsAtom(args[0])
	if err != nil {
		return nil, fmt.Errorf("%s: expected an atom name, got %s", word, args[0].Parent.String())
	}
	v, ok := eng.ResolveRef(reg, name)
	if !ok {
		if reg != nil {
			return nil, reg.AqlError("undefined_word", word+": name "+name+" is not bound", name)
		}
		return nil, fmt.Errorf("%s: name %s is not bound", word, name)
	}
	if !eng.IsFunctionRef(v) {
		detail := word + " requires a function word: " + name + " is bound to " + v.Parent.String()
		if reg != nil {
			return nil, reg.AqlError("illegal_ref", detail, name)
		}
		return nil, fmt.Errorf("%s", detail)
	}
	return rebarrierResult(wrap, v, word, reg)
}

// usurpAtomHandler is the by-name form `usurp foo`: it resolves the
// captured atom to its bound function value (sharing `ref`'s rules — an
// unbound name raises undefined_word, a non-fn binding raises
// illegal_ref) and then returns the argument-reversed wrapper. It is the
// function-form companion of the `/u` word suffix; `usurp (ref foo)` /
// `usurp (foo/r)` pass the value directly via the [Function] overload.
func usurpAtomHandler(args []Value, _ map[string]Value, _ []Value, reg *Registry) ([]Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("usurp: missing name")
	}
	name, err := AsAtom(args[0])
	if err != nil {
		return nil, fmt.Errorf("usurp: expected an atom name, got %s", args[0].Parent.String())
	}
	v, ok := eng.ResolveRef(reg, name)
	if !ok {
		if reg != nil {
			return nil, reg.AqlError("undefined_word", "usurp: name "+name+" is not bound", name)
		}
		return nil, fmt.Errorf("usurp: name %s is not bound", name)
	}
	if !eng.IsFunctionRef(v) {
		detail := "usurp requires a function word: " + name + " is bound to " + v.Parent.String()
		if reg != nil {
			return nil, reg.AqlError("illegal_ref", detail, name)
		}
		return nil, fmt.Errorf("%s", detail)
	}
	return usurpHandler([]Value{v}, nil, nil, reg)
}

// refHandler resolves the captured atom name to its bound value and
// returns it. Failure to bind raises an undefined_word error, the
// same code stepWord raises for an unbound bare word — so the two
// surfaces report identical errors.
func refHandler(args []Value, _ map[string]Value, _ []Value, reg *Registry) ([]Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("ref: missing name")
	}
	name, err := AsAtom(args[0])
	if err != nil {
		return nil, fmt.Errorf("ref: expected an atom name, got %s", args[0].Parent.String())
	}
	v, ok := eng.ResolveRef(reg, name)
	if !ok {
		if reg != nil {
			return nil, reg.AqlError("undefined_word", "ref: name "+name+" is not bound", name)
		}
		return nil, fmt.Errorf("ref: name %s is not bound", name)
	}
	// `ref` is the function-form companion of the `/r` suffix and shares
	// its rule: only function words may be referenced. A non-fn binding
	// (plain value, type body) is rejected so both surfaces behave alike.
	if !eng.IsFunctionRef(v) {
		detail := "ref requires a function word: " + name + " is bound to " + v.Parent.String()
		if reg != nil {
			return nil, reg.AqlError("illegal_ref", detail, name)
		}
		return nil, fmt.Errorf("%s", detail)
	}
	return []Value{v}, nil
}

// applyHandler unquotes the captured Function value and returns it.
// The engine's stepLiteral check then fires execFnDefLiteral, which
// dispatches the function against whatever stack args precede it.
//
// For AQL-defined fns the dispatch uses the captured FnDef's own
// Sigs table, so the call is stable even when the original binding
// has been redefined or undef'd.
//
// For native fns the captured payload has Signatures but no Sigs,
// and execFnDefLiteral's pure-stack path is FnSig-based — those will
// reach apply, unquote, but fall back to passing through. Native fn
// captures still serve as TFunction-slot args to higher-order words
// (filter, walk, behave) where the consumer's handler calls into the
// engine directly via CallAQL.
func applyHandler(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
	v := args[0]
	if !v.Parent.Equal(TFunction) && !v.Parent.Equal(TFnDef) {
		return nil, fmt.Errorf("apply: expected Function, got %s", v.Parent.String())
	}
	if _, ok := v.Data.(FnDefInfo); !ok {
		return nil, fmt.Errorf("apply: function value carries no FnDefInfo (got %T)", v.Data)
	}
	v.Quoted = false
	return []Value{v}, nil
}

// applyReachHandler applies a Reach (a lens) to a receiver value: it rebinds
// the reach's receiver to args[1] and evaluates the segment walk (the lens
// "get"). getr strictness and computed keys behave exactly as bare m.a.b.
func applyReachHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	info, err := AsReach(args[0])
	if err != nil {
		return nil, fmt.Errorf("apply: %w", err)
	}
	out, err := ApplyReach(r, info, args[1])
	if err != nil {
		return nil, err
	}
	return []Value{out}, nil
}

// rebindHandler returns a new inert Reach with its receiver swapped to
// args[1] — composition, not evaluation. The result is a bound lens (data);
// `apply` / `getpath` read through it.
func rebindHandler(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
	info, err := AsReach(args[0])
	if err != nil {
		return nil, fmt.Errorf("rebind: %w", err)
	}
	bound := ReachInfo{
		Receiver: []Value{args[1]},
		Segments: info.Segments,
		Eval:     false,
	}
	return []Value{NewReach(bound)}, nil
}

// usurpHandler wraps a Function value so its signature argument order is
// reversed: the wrapper called `usurped a b c` dispatches the original as
// `f c b a`. Mirrors the /u modifier (eng.UsurpFunction). The wrapper is
// returned unquoted, so — like a bare function word — it dispatches
// immediately when args follow, and stays inert when captured with quote.
func usurpHandler(args []Value, _ map[string]Value, _ []Value, reg *Registry) ([]Value, error) {
	wrapped, ok := eng.UsurpFunction(args[0])
	if !ok {
		if out, gradual := checkModeGradualFn(reg, args[0]); gradual {
			return out, nil
		}
		detail := "usurp requires a function value, got " + args[0].Parent.String()
		if reg != nil {
			return nil, reg.AqlError("illegal_ref", detail, "usurp")
		}
		return nil, fmt.Errorf("%s", detail)
	}
	return []Value{wrapped}, nil
}

// checkModeGradualFn handles a dispatch-modifier word (usurp / stack-args /
// forward-args / force-arity) applied to a NON-CONCRETE function-value carrier
// in check mode. A stored fn-ref read via dot-access (`m.a` where
// `m = {a:add/r}`) is statically dynamic(Any) — getNodeReturns deliberately
// cannot narrow a dispatch-bearing field — but a real Function at run time.
// Rather than the strict handler rejecting the Any carrier (a false no_signature
// / illegal_ref, path-modifier.tsv), return a gradual Function carrier so the
// downstream arg dispatch types optimistically. Only fires for a carrier the
// real wrapper already declined (a concrete Function still wraps normally).
func checkModeGradualFn(reg *Registry, v Value) ([]Value, bool) {
	if reg == nil || !reg.Check.IsActive() || IsConcrete(v) || v.Parent == nil {
		return nil, false
	}
	// Only a STATICALLY-UNKNOWN carrier (a dynamic Any from a dispatch-bearing
	// field read like `m.a`) or an actual Function-typed carrier is plausibly a
	// function at run time. A concrete-TYPED carrier (an Integer / String
	// binding, e.g. `def x 5 x/u`) is provably NOT a function — let the strict
	// handler flag it (a real illegal_ref), so the fallback never masks a
	// genuine non-fn modifier error.
	if (v.Dynamic && v.Parent.Equal(TAny)) || v.Parent.ConformsTo(TFunction) {
		return []Value{NewDynamicCarrier(TFunction)}, true
	}
	return nil, false
}
