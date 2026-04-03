package help

func init() {
	register(&Entry{
		Word:    "and",
		Summary: "Logical AND of two booleans.",
		Description: "Returns true only when both operands are true.",
	})

	register(&Entry{
		Word:    "or",
		Summary: "Logical OR of two booleans.",
		Description: "Returns true when at least one operand is true.",
	})

	register(&Entry{
		Word:    "not",
		Summary: "Logical NOT of a boolean.",
		Description: "Inverts the boolean value.",
	})

	register(&Entry{
		Word:    "xor",
		Summary: "Logical XOR of two booleans.",
		Description: "Returns true when exactly one operand is true.",
	})

	register(&Entry{
		Word:    "nand",
		Summary: "Logical NAND of two booleans.",
		Description: "Returns false only when both operands are true (NOT AND).",
	})

	register(&Entry{
		Word:    "implies",
		Summary: "Logical implication of two booleans.",
		Description: "Returns false only when the first is true and the second is false: " +
			"a b implies computes (!a || b).",
	})
}
