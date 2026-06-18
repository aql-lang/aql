package help

func init() {
	register(&Entry{
		Word:    "and",
		Summary: "Logical AND of two Booleans; over other types, a truthy short-circuit that returns an operand.",
		Description: "With two Booleans, returns true only when both are true. The " +
			"`[Any Any]` overload accepts operands of any type: it coerces them via " +
			"`convert boolean` truthiness and returns one of the OPERANDS " +
			"(Python/Lisp short-circuit), not necessarily a Boolean — `and 1 2` → 1, " +
			"`and 0 9` → 0. The checker types this overload as the join of the operand " +
			"types, so `and 1 2` is accepted and inferred Integer, not rejected.",
	})

	register(&Entry{
		Word:    "or",
		Summary: "Logical OR of two Booleans; over other types, a truthy short-circuit that returns an operand.",
		Description: "With two Booleans, returns true when at least one is true. The " +
			"`[Any Any]` overload accepts operands of any type: it coerces them via " +
			"`convert boolean` truthiness and returns one of the OPERANDS " +
			"(Python/Lisp short-circuit), not necessarily a Boolean — `0 or 9` → 9, " +
			"`5 or 9` → 5. The checker types this overload as the join of the operand " +
			"types, not Boolean.",
	})

	register(&Entry{
		Word:    "not",
		Summary: "Logical NOT; coerces any operand via truthiness and always returns a Boolean.",
		Description: "With a Boolean, inverts it. The `[Any]` overload accepts any " +
			"operand and coerces it via `convert boolean` truthiness before inverting, " +
			"so `not 5` → false and `not 0` → true. Always returns a Boolean.",
	})

	register(&Entry{
		Word:    "xor",
		Summary: "Logical XOR of two Booleans; coerces other types via truthiness.",
		Description: "Returns true when exactly one operand is true. The `[Any Any]` " +
			"overload coerces non-Boolean operands via `convert boolean` rules and " +
			"always returns a Boolean (unlike `and`/`or`, which return an operand).",
	})

	register(&Entry{
		Word:    "nand",
		Summary: "Logical NAND of two Booleans; coerces other types via truthiness.",
		Description: "Returns false only when both operands are true (NOT AND). The " +
			"`[Any Any]` overload coerces non-Boolean operands via `convert boolean` " +
			"rules and always returns a Boolean.",
	})

	register(&Entry{
		Word:    "implies",
		Summary: "Logical implication of two Booleans; coerces other types via truthiness.",
		Description: "Returns false only when the first is true and the second is false: " +
			"a b implies computes (!a || b). The `[Any Any]` overload coerces non-Boolean " +
			"operands via `convert boolean` rules and always returns a Boolean.",
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
