package help

func init() {
	register(&Entry{
		Word:    "dup",
		Summary: "Duplicate the top stack value.",
		Signatures: []string{"[a] -> [a a]"},
		Description: "Copies the top value, pushing the copy onto the stack. Prefix-only.",
		Examples: []string{
			`5 dup                => 5 5`,
			`"hi" dup             => 'hi' 'hi'`,
			`1 2 dup              => 1 2 2`,
			`true dup             => true true`,
		},
	})

	register(&Entry{
		Word:    "swap",
		Summary: "Swap the top two stack values.",
		Signatures: []string{"[a b] -> [b a]"},
		Description: "Exchanges the positions of the top two values. Prefix-only.",
		Examples: []string{
			`1 2 swap             => 2 1`,
			`"a" "b" swap         => 'b' 'a'`,
			`3 5 swap sub         => 2`,
			`true false swap      => false true`,
		},
	})

	register(&Entry{
		Word:    "drop",
		Summary: "Remove the top stack value.",
		Signatures: []string{"[a] -> []"},
		Description: "Discards the top value from the stack. Prefix-only.",
		Examples: []string{
			`1 2 drop             => 1`,
			`1 2 3 drop           => 1 2`,
			`"hello" drop         => (empty stack)`,
			`1 2 3 drop drop      => 1`,
		},
	})

	register(&Entry{
		Word:    "over",
		Summary: "Copy the second value to the top.",
		Signatures: []string{"[a b] -> [a b a]"},
		Description: "Copies the value below the top and pushes it on top. Prefix-only.",
		Examples: []string{
			`1 2 over             => 1 2 1`,
			`3 5 over             => 3 5 3`,
			`"a" "b" over         => 'a' 'b' 'a'`,
			`10 20 over add       => 10 30`,
		},
	})

	register(&Entry{
		Word:    "rot",
		Summary: "Rotate the top three values.",
		Signatures: []string{"[a b c] -> [b c a]"},
		Description: "Moves the third value to the top: a b c rot becomes b c a. Prefix-only.",
		Examples: []string{
			`1 2 3 rot            => 2 3 1`,
			`"a" "b" "c" rot      => 'b' 'c' 'a'`,
			`10 20 30 rot         => 20 30 10`,
			`1 2 3 rot rot        => 3 1 2`,
		},
	})

	register(&Entry{
		Word:    "nip",
		Summary: "Remove the second stack value.",
		Signatures: []string{"[a b] -> [b]"},
		Description: "Discards the value below the top. Equivalent to swap drop. Prefix-only.",
		Examples: []string{
			`1 2 nip              => 2`,
			`"a" "b" nip          => 'b'`,
			`10 20 30 nip         => 10 30`,
			`1 2 3 nip nip        => 3`,
		},
	})

	register(&Entry{
		Word:    "tuck",
		Summary: "Copy the top value below the second value.",
		Signatures: []string{"[a b] -> [b a b]"},
		Description: "Copies the top value and inserts it below the second. Equivalent to swap over. Prefix-only.",
		Examples: []string{
			`1 2 tuck             => 2 1 2`,
			`"a" "b" tuck         => 'b' 'a' 'b'`,
			`10 20 tuck           => 20 10 20`,
			`3 5 tuck add         => 3 8`,
		},
	})

	register(&Entry{
		Word:    "2dup",
		Summary: "Duplicate the top two stack values.",
		Signatures: []string{"[a b] -> [a b a b]"},
		Description: "Copies the top two values as a pair. Prefix-only.",
		Examples: []string{
			`1 2 2dup             => 1 2 1 2`,
			`3 4 2dup add         => 3 4 7`,
			`"a" "b" 2dup         => 'a' 'b' 'a' 'b'`,
			`10 20 2dup mul       => 10 20 200`,
		},
	})

	register(&Entry{
		Word:    "2swap",
		Summary: "Swap the top two pairs of values.",
		Signatures: []string{"[a b c d] -> [c d a b]"},
		Description: "Exchanges the top pair with the pair below. Prefix-only.",
		Examples: []string{
			`1 2 3 4 2swap        => 3 4 1 2`,
			`"a" "b" "c" "d" 2swap => 'c' 'd' 'a' 'b'`,
			`10 20 30 40 2swap    => 30 40 10 20`,
			`1 2 3 4 2swap 2swap  => 1 2 3 4`,
		},
	})

	register(&Entry{
		Word:    "2drop",
		Summary: "Remove the top two stack values.",
		Signatures: []string{"[a b] -> []"},
		Description: "Discards the top two values. Prefix-only.",
		Examples: []string{
			`1 2 3 2drop          => 1`,
			`1 2 3 4 2drop        => 1 2`,
			`"a" "b" 2drop        => (empty stack)`,
			`1 2 3 4 5 2drop      => 1 2 3`,
		},
	})

	register(&Entry{
		Word:    "2over",
		Summary: "Copy the second pair to the top.",
		Signatures: []string{"[a b c d] -> [a b c d a b]"},
		Description: "Copies the pair below the top pair and pushes them on top. Prefix-only.",
		Examples: []string{
			`1 2 3 4 2over        => 1 2 3 4 1 2`,
			`10 20 30 40 2over    => 10 20 30 40 10 20`,
			`"a" "b" "c" "d" 2over => 'a' 'b' 'c' 'd' 'a' 'b'`,
			`5 6 7 8 2over add    => 5 6 7 8 11`,
		},
	})

	register(&Entry{
		Word:    "depth",
		Summary: "Push the current stack depth.",
		Signatures: []string{"[] -> [integer]"},
		Description: "Pushes the number of values currently on the stack. Prefix-only.",
		Examples: []string{
			`depth                => 0`,
			`1 depth              => 1 1`,
			`1 2 depth            => 1 2 2`,
			`1 2 3 depth          => 1 2 3 3`,
		},
	})

	register(&Entry{
		Word:    "pick",
		Summary: "Copy the nth value from the top.",
		Signatures: []string{"[integer] -> [value]"},
		Description: "Copies the value at depth n (0 = top) onto the top. " +
			"0 pick is equivalent to dup. Prefix-only.",
		Examples: []string{
			`1 2 3 0 pick         => 1 2 3 3`,
			`1 2 3 1 pick         => 1 2 3 2`,
			`1 2 3 2 pick         => 1 2 3 1`,
			`10 20 30 40 3 pick   => 10 20 30 40 10`,
		},
		Notes: []string{"Index out of range produces an error."},
	})

	register(&Entry{
		Word:    "roll",
		Summary: "Move the nth value to the top.",
		Signatures: []string{"[integer] -> [value]"},
		Description: "Removes the value at depth n and places it on top. " +
			"1 roll is equivalent to swap; 2 roll is equivalent to rot. Prefix-only.",
		Examples: []string{
			`1 2 3 1 roll         => 1 3 2`,
			`1 2 3 2 roll         => 2 3 1`,
			`10 20 30 40 3 roll   => 20 30 40 10`,
			`"a" "b" "c" 2 roll   => 'b' 'c' 'a'`,
		},
		Notes: []string{"Index out of range produces an error."},
	})
}
