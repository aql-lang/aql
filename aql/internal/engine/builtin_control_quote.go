package engine

// registerQuote registers the "quote" word.
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
func registerQuote(r *Registry) {
	// TWord signature: captures words as literals via
	// hasPendingForwardExpectingWord(), preventing execution.
	// Converts the word to an atom.
	r.Register("quote",
		Signature{
			Args: []Type{TWord},
			Handler: func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
				w := args[0].AsWord()
				v := NewAtom(w.Name)
				v.Quoted = true
				return []Value{v}, nil
			},
		},
		// TAny signature: catches all non-word values (lists, maps,
		// scalars). Returns the value with Quoted=true to prevent
		// auto-evaluation at end of execution.
		Signature{
			Args: []Type{TAny},
			Handler: func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
				v := args[0]
				v.Quoted = true
				return []Value{v}, nil
			},
		},
	)
}
