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

		Signatures: []NativeSig{{
			Args:    []*Type{TFunction},
			Handler: applyHandler,
			Returns: []*Type{TAny}, BarrierPos: 0,
		}},
	},
	{
		Name: "usurp",
		// Forward-eligible: `usurp fn` reads as "wrap this fn". Returns a
		// Function value, so it dispatches immediately when args follow
		// (`usurp (ref f) a b`) and stays inert under quote.
		Signatures: []NativeSig{{
			Args:           []*Type{TFunction},
			Handler:        usurpHandler,
			Returns:        []*Type{TFunction},
			RunInCheckMode: true, BarrierPos: -1,
		}},
	},
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

// usurpHandler wraps a Function value so its signature argument order is
// reversed: the wrapper called `usurped a b c` dispatches the original as
// `f c b a`. Mirrors the /u modifier (eng.UsurpFunction). The wrapper is
// returned unquoted, so — like a bare function word — it dispatches
// immediately when args follow, and stays inert when captured with quote.
func usurpHandler(args []Value, _ map[string]Value, _ []Value, reg *Registry) ([]Value, error) {
	wrapped, ok := eng.UsurpFunction(args[0])
	if !ok {
		detail := "usurp requires a function value, got " + args[0].Parent.String()
		if reg != nil {
			return nil, reg.AqlError("illegal_ref", detail, "usurp")
		}
		return nil, fmt.Errorf("%s", detail)
	}
	return []Value{wrapped}, nil
}
