package help

func init() {
	register(&Entry{
		Word:    "dup",
		Summary: "Duplicate the top stack value.",
		Signatures: []string{"[a] -> [a a]"},
		Description: "Copies the top value, pushing the copy onto the stack.",
		Examples: []string{`3 dup => 3 3`},
	})

	register(&Entry{
		Word:    "swap",
		Summary: "Swap the top two stack values.",
		Signatures: []string{"[a b] -> [b a]"},
		Description: "Exchanges the positions of the top two values.",
		Examples: []string{`1 2 swap => 2 1`},
	})

	register(&Entry{
		Word:    "drop",
		Summary: "Remove the top stack value.",
		Signatures: []string{"[a] -> []"},
		Description: "Discards the top value from the stack.",
		Examples: []string{`1 2 drop => 1`},
	})

	register(&Entry{
		Word:    "over",
		Summary: "Copy the second value to the top.",
		Signatures: []string{"[a b] -> [a b a]"},
		Description: "Copies the value below the top and pushes it on top.",
		Examples: []string{`1 2 over => 1 2 1`},
	})

	register(&Entry{
		Word:    "rot",
		Summary: "Rotate the top three values.",
		Signatures: []string{"[a b c] -> [b c a]"},
		Description: "Moves the third value to the top: a b c rot becomes b c a.",
		Examples: []string{`1 2 3 rot => 2 3 1`},
	})

	register(&Entry{
		Word:    "nip",
		Summary: "Remove the second stack value.",
		Signatures: []string{"[a b] -> [b]"},
		Description: "Discards the value below the top. Equivalent to swap drop.",
		Examples: []string{`1 2 nip => 2`},
	})

	register(&Entry{
		Word:    "tuck",
		Summary: "Copy the top value below the second value.",
		Signatures: []string{"[a b] -> [b a b]"},
		Description: "Copies the top value and inserts it below the second. Equivalent to swap over.",
		Examples: []string{`1 2 tuck => 2 1 2`},
	})

	register(&Entry{
		Word:    "2dup",
		Summary: "Duplicate the top two stack values.",
		Signatures: []string{"[a b] -> [a b a b]"},
		Description: "Copies the top two values as a pair.",
		Examples: []string{`1 2 2dup => 1 2 1 2`},
	})

	register(&Entry{
		Word:    "2swap",
		Summary: "Swap the top two pairs of values.",
		Signatures: []string{"[a b c d] -> [c d a b]"},
		Description: "Exchanges the top pair with the pair below.",
		Examples: []string{`1 2 3 4 2swap => 3 4 1 2`},
	})

	register(&Entry{
		Word:    "2drop",
		Summary: "Remove the top two stack values.",
		Signatures: []string{"[a b] -> []"},
		Description: "Discards the top two values.",
		Examples: []string{`1 2 3 2drop => 1`},
	})

	register(&Entry{
		Word:    "2over",
		Summary: "Copy the second pair to the top.",
		Signatures: []string{"[a b c d] -> [a b c d a b]"},
		Description: "Copies the pair below the top pair and pushes them on top.",
		Examples: []string{`1 2 3 4 2over => 1 2 3 4 1 2`},
	})

	register(&Entry{
		Word:    "depth",
		Summary: "Push the current stack depth.",
		Signatures: []string{"[] -> [integer]"},
		Description: "Pushes the number of values currently on the stack.",
		Examples: []string{`1 2 3 depth => 1 2 3 3`},
	})

	register(&Entry{
		Word:    "pick",
		Summary: "Copy the nth value from the top.",
		Signatures: []string{"[integer] -> [value]"},
		Description: "Copies the value at depth n (0 = top) onto the top. " +
			"0 pick is equivalent to dup.",
		Examples: []string{`1 2 3 0 pick => 1 2 3 3`, `1 2 3 2 pick => 1 2 3 1`},
		Notes: []string{"Index out of range produces an error."},
	})

	register(&Entry{
		Word:    "roll",
		Summary: "Move the nth value to the top.",
		Signatures: []string{"[integer] -> [value]"},
		Description: "Removes the value at depth n and places it on top. " +
			"1 roll is equivalent to swap; 2 roll is equivalent to rot.",
		Examples: []string{`1 2 3 2 roll => 2 3 1`},
		Notes: []string{"Index out of range produces an error."},
	})
}
