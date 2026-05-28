package help

func init() {
	register(&Entry{
		Word:    "band",
		Summary: "Bitwise AND of two integers.",
		Description: "Returns the bitwise AND. Operates on 64-bit twos-complement. " +
			"Distinct from boolean `and`, which short-circuits on truthiness.",
	})

	register(&Entry{
		Word:    "bor",
		Summary: "Bitwise OR of two integers.",
		Description: "Returns the bitwise OR. Operates on 64-bit twos-complement. " +
			"Distinct from boolean `or`, which short-circuits on truthiness.",
	})

	register(&Entry{
		Word:    "bxor",
		Summary: "Bitwise XOR of two integers.",
		Description: "Returns the bitwise XOR. Operates on 64-bit twos-complement. " +
			"Distinct from boolean `xor`, which coerces operands to Boolean.",
	})

	register(&Entry{
		Word:        "bnot",
		Summary:     "Bitwise complement (one's complement) of an integer.",
		Description: "Returns `~x`. Distinct from boolean `not`, which inverts truthiness.",
	})

	register(&Entry{
		Word:    "bsl",
		Summary: "Shift integer left by N bits.",
		Description: "`value bsl count` returns `value << count`. Shifts >= 64 " +
			"saturate to 0. Negative counts raise [aql/binary_error].",
	})

	register(&Entry{
		Word:    "bsr",
		Summary: "Arithmetic (sign-extending) right shift.",
		Description: "`value bsr count` returns `value >> count` with sign-fill: " +
			"the high bit is replicated so negatives stay negative. Shifts >= 64 " +
			"saturate (0 for non-negative inputs, -1 for negative). Use `busr` for " +
			"logical (zero-fill) right shift.",
	})

	register(&Entry{
		Word:    "busr",
		Summary: "Logical (zero-fill) right shift.",
		Description: "`value busr count` shifts right with zero-fill: vacated " +
			"high bits become 0 regardless of sign. Shifts >= 64 saturate to 0. " +
			"Use `bsr` for arithmetic (sign-extending) right shift.",
	})
}
