package help

func init() {
	register(&Entry{
		Word:        "dup",
		Summary:     "Duplicate the top stack value.",
		Description: "Copies the top value, pushing the copy onto the stack. Stack-only.",
	})

	register(&Entry{
		Word:        "swap",
		Summary:     "Swap the top two stack values.",
		Description: "Exchanges the positions of the top two values. Stack-only.",
	})

	register(&Entry{
		Word:        "drop",
		Summary:     "Remove the top stack value.",
		Description: "Discards the top value from the stack. Stack-only.",
	})

	register(&Entry{
		Word:        "over",
		Summary:     "Copy the second value to the top.",
		Description: "Copies the value below the top and pushes it on top. Stack-only.",
	})

	register(&Entry{
		Word:        "rot",
		Summary:     "Rotate the top three values.",
		Description: "Moves the third value to the top: a b c rot becomes b c a. Stack-only.",
	})

	register(&Entry{
		Word:        "nip",
		Summary:     "Remove the second stack value.",
		Description: "Discards the value below the top. Equivalent to swap drop. Stack-only.",
	})

	register(&Entry{
		Word:        "tuck",
		Summary:     "Copy the top value below the second value.",
		Description: "Copies the top value and inserts it below the second. Equivalent to swap over. Stack-only.",
	})

	register(&Entry{
		Word:        "2dup",
		Summary:     "Duplicate the top two stack values.",
		Description: "Copies the top two values as a pair. Stack-only.",
	})

	register(&Entry{
		Word:        "2swap",
		Summary:     "Swap the top two pairs of values.",
		Description: "Exchanges the top pair with the pair below. Stack-only.",
	})

	register(&Entry{
		Word:        "2drop",
		Summary:     "Remove the top two stack values.",
		Description: "Discards the top two values. Stack-only.",
	})

	register(&Entry{
		Word:        "2over",
		Summary:     "Copy the second pair to the top.",
		Description: "Copies the pair below the top pair and pushes them on top. Stack-only.",
	})

	register(&Entry{
		Word:        "depth",
		Summary:     "Push the current stack depth.",
		Description: "Pushes the number of values currently on the stack. Stack-only.",
	})

	register(&Entry{
		Word:    "pick",
		Summary: "Copy the nth value from the top.",
		Description: "Copies the value at depth n (0 = top) onto the top. " +
			"0 pick is equivalent to dup. Stack-only.",
		Notes: []string{"Index out of range produces an error."},
	})

	register(&Entry{
		Word:    "roll",
		Summary: "Move the nth value to the top.",
		Description: "Removes the value at depth n and places it on top. " +
			"1 roll is equivalent to swap; 2 roll is equivalent to rot. Stack-only.",
		Notes: []string{"Index out of range produces an error."},
	})
}
