package native

import (
	"github.com/metsitaba/voxgig-exp/aql/internal/engine"
)
// RegisterQuote registers the "quote" word.
//
// quote prevents auto-evaluation of its argument and returns it as-is.
// It has forward precedence and takes a single argument:
//
//	quote [1 add 2]  →  [Integer(1), Word(add), Integer(2)]
//	quote a          →  Atom(a)
//	quote 99         →  99
//	quote "hello"    →  "hello"
//
// For words (known functions), quote captures them as literals and
// converts to atoms. For all other values, quote returns them unchanged,
// preventing list/map auto-evaluation.
//
// For signature matching purposes, quote is transparent: it returns
// the type of the quotation target (atom for words, identity for rest).
func RegisterQuote(r *engine.Registry) {
	// TWord signature: captures words as literals via
	// hasPendingForwardExpectingWord(), preventing execution.
	// Converts the word to an atom.
	r.RegisterNativeFunc(engine.NativeFunc{
		Name:              "quote",
		ForwardPrecedence: true,
		Signatures: []engine.NativeSig{
			{
				Args: []engine.Type{engine.TWord},
				Handler: func(args []engine.Value, _ map[string]engine.Value, _ []engine.Value, _ *engine.Registry) ([]engine.Value, error) {
					w, _ := args[0].AsWord()
					v := engine.NewAtom(w.Name)
					v.Quoted = true
					return []engine.Value{v}, nil
				},
				Returns: []engine.Type{engine.TAtom},
			},
			// TAny signature: catches all non-word values (lists, maps,
			// scalars). Returns the value with Quoted=true to prevent
			// auto-evaluation at end of execution. NoEvalArgs prevents
			// list auto-evaluation before the handler runs.
			{
				Args:       []engine.Type{engine.TAny},
				NoEvalArgs: map[int]bool{0: true},
				Handler: func(args []engine.Value, _ map[string]engine.Value, _ []engine.Value, _ *engine.Registry) ([]engine.Value, error) {
					v := args[0]
					v.Quoted = true
					return []engine.Value{v}, nil
				},
				// quote has a semantic side-effect — Quoted=true
				// prevents downstream auto-evaluation. Run the
				// handler in check mode so the flag is preserved
				// on stored list values; otherwise a later
				// simple-value substitution would expand the
				// list and drop the do-ability.
				RunInCheckMode: true,
				ReturnsFn:      engine.ReturnsIdentity(0),
			},
		},
	})
}
