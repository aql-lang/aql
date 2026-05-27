package help

func init() {
	register(&Entry{
		Word:        "and",
		Summary:     "Logical AND of two booleans.",
		Description: "Returns true only when both operands are true.",
	})

	register(&Entry{
		Word:        "or",
		Summary:     "Logical OR of two booleans.",
		Description: "Returns true when at least one operand is true.",
	})

	register(&Entry{
		Word:        "not",
		Summary:     "Logical NOT of a boolean.",
		Description: "Inverts the boolean value.",
	})

	register(&Entry{
		Word:        "xor",
		Summary:     "Logical XOR of two booleans.",
		Description: "Returns true when exactly one operand is true.",
	})

	register(&Entry{
		Word:        "nand",
		Summary:     "Logical NAND of two booleans.",
		Description: "Returns false only when both operands are true (NOT AND).",
	})

	register(&Entry{
		Word:    "implies",
		Summary: "Logical implication of two booleans.",
		Description: "Returns false only when the first is true and the second is false: " +
			"a b implies computes (!a || b).",
	})

	register(&Entry{
		Word:    "nor",
		Summary: "Logical NOR (NOT OR).",
		Description: "Returns true only when both operands are false. " +
			"Non-boolean inputs are coerced via `convert boolean` rules.",
	})

	register(&Entry{
		Word:    "iff",
		Summary: "Logical biconditional (XNOR / equivalence).",
		Description: "Returns true when both operands have the same truth value. " +
			"Non-boolean inputs are coerced via `convert boolean` rules.",
	})

	register(&Entry{
		Word:    "xnor",
		Summary: "Logical XNOR (NOT XOR / equivalence).",
		Description: "Returns true when both operands have the same truth value. " +
			"Synonym for `iff`. Non-boolean inputs are coerced via " +
			"`convert boolean` rules.",
	})

	register(&Entry{
		Word:    "otherwise",
		Summary: "Null-coalescing: returns left operand if not None, else right.",
		Description: "Distinct from `or`, which short-circuits on falsy. " +
			"`0 otherwise 5` returns 0 (since 0 is not None), but " +
			"`0 or 5` returns 5 (since 0 is falsy).",
	})

	register(&Entry{
		Word:    "any",
		Summary: "Apply `or` across a list; returns true iff any element is truthy.",
		Description: "Coerces each element via `convert boolean` rules and " +
			"short-circuits on the first truthy element. Returns `false` for an " +
			"empty list (the identity for OR).",
	})

	register(&Entry{
		Word:    "all",
		Summary: "Apply `and` across a list; returns true iff every element is truthy.",
		Description: "Coerces each element via `convert boolean` rules and " +
			"short-circuits on the first falsy element. Returns `true` for an " +
			"empty list (the identity for AND).",
	})
}
