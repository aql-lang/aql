package help

func init() {
	register(&Entry{
		Word:    "and",
		Summary: "Logical AND of two booleans.",
		Signatures: []string{"[boolean boolean] -> [boolean]"},
		Description: "Returns true only when both operands are true.",
		Examples: []string{`true true and => true`, `true false and => false`},
	})

	register(&Entry{
		Word:    "or",
		Summary: "Logical OR of two booleans.",
		Signatures: []string{"[boolean boolean] -> [boolean]"},
		Description: "Returns true when at least one operand is true.",
		Examples: []string{`false true or => true`, `false false or => false`},
	})

	register(&Entry{
		Word:    "not",
		Summary: "Logical NOT of a boolean.",
		Signatures: []string{"[boolean] -> [boolean]"},
		Description: "Inverts the boolean value.",
		Examples: []string{`true not => false`, `false not => true`},
	})

	register(&Entry{
		Word:    "xor",
		Summary: "Logical XOR of two booleans.",
		Signatures: []string{"[boolean boolean] -> [boolean]"},
		Description: "Returns true when exactly one operand is true.",
		Examples: []string{`true false xor => true`, `true true xor => false`},
	})

	register(&Entry{
		Word:    "nand",
		Summary: "Logical NAND of two booleans.",
		Signatures: []string{"[boolean boolean] -> [boolean]"},
		Description: "Returns false only when both operands are true (NOT AND).",
		Examples: []string{`true true nand => false`, `true false nand => true`},
	})

	register(&Entry{
		Word:    "implies",
		Summary: "Logical implication of two booleans.",
		Signatures: []string{"[boolean boolean] -> [boolean]"},
		Description: "Returns false only when the first is true and the second is false: a b implies is (!a || b).",
		Examples: []string{`true false implies => false`, `false false implies => true`},
	})
}
