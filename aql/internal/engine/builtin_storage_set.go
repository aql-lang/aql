package engine

func registerSet(r *Registry) {
	setHandler := func(args []Value) ([]Value, error) {
		key := storeKey(args[0])
		r.Store[key] = args[1]
		return nil, nil
	}

	// set: key and value → store[key] = value
	// Three signatures for different key types:
	//   [TString, TAny] — string key (also handles coerced words)
	//   [TWord, TAny]   — word key (unknown word collected as word literal)
	//   [TAny, TAny]    — fallback for integer/other keys
	// Flexible matching handles reordering: "99 set foo end" →
	// [99, foo_word] → swap → [foo_word, 99] matching [TWord, TAny].
	// Registration order matters for tiebreaking: TString first so it wins
	// when peeking gives no disambiguation (e.g. paren expressions).
	r.Register("set",
		Signature{
			Args:    []Type{TString, TAny},
			Handler: setHandler,
		},
		Signature{
			Args:    []Type{TWord, TAny},
			Handler: setHandler,
		},
		Signature{
			Args:    []Type{TAny, TAny},
			Handler: setHandler,
		},
	)
}
