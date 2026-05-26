package native

import (
	"fmt"

	eng "github.com/aql-lang/aql/eng/go"
)

// refNatives registers the two words that complete AQL's first-class
// function-value pipeline:
//
//   - `ref name`  — resolves a name to its bound value without
//     invoking; companion to the `/r` word suffix that
//     lives in the parser+stepWord path.
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
		Name:        "ref",
		ForwardArgs: true,
		Signatures: []NativeSig{{
			// /q on the name slot lets the parser capture the upcoming
			// Word as an Atom rather than executing it. `ref add` then
			// arrives here with args[0] = Atom(add).
			Args:           []*Type{TAtom},
			QuoteArgs:      map[int]bool{0: true},
			Handler:        refHandler,
			Returns:        []*Type{TAny},
			RunInCheckMode: true,
		}},
	},
	{
		Name: "apply",
		// Stack-only: `args... fn apply` reads as "take the function
		// off the stack and apply it to the preceding values." Forward
		// collection would force callers to put fn-args after the fn,
		// which fights AQL's left-to-right stack flow.
		ForwardArgs: false,
		Signatures: []NativeSig{{
			Args:    []*Type{TFunction},
			Handler: applyHandler,
			Returns: []*Type{TAny},
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
