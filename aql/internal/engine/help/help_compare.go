package help

func init() {
	register(&Entry{
		Word:    "lt",
		Summary: "Test if the first value is less than the second.",
		Signatures: []string{"[any any] -> [boolean]"},
		Description: "Compares two values. Numbers are compared numerically; strings lexicographically.",
		Examples: []string{
			`1 2 lt               => true`,
			`2 1 lt               => false`,
			`3 3 lt               => false`,
			`"a" "b" lt           => true`,
		},
	})

	register(&Entry{
		Word:    "gt",
		Summary: "Test if the first value is greater than the second.",
		Signatures: []string{"[any any] -> [boolean]"},
		Description: "Compares two values. Numbers are compared numerically; strings lexicographically.",
		Examples: []string{
			`2 1 gt               => true`,
			`1 2 gt               => false`,
			`3 3 gt               => false`,
			`"b" "a" gt           => true`,
		},
	})

	register(&Entry{
		Word:    "lte",
		Summary: "Test if the first value is less than or equal to the second.",
		Signatures: []string{"[any any] -> [boolean]"},
		Description: "Less-than-or-equal comparison.",
		Examples: []string{
			`1 2 lte              => true`,
			`2 2 lte              => true`,
			`3 2 lte              => false`,
			`"a" "a" lte          => true`,
		},
	})

	register(&Entry{
		Word:    "gte",
		Summary: "Test if the first value is greater than or equal to the second.",
		Signatures: []string{"[any any] -> [boolean]"},
		Description: "Greater-than-or-equal comparison.",
		Examples: []string{
			`2 1 gte              => true`,
			`2 2 gte              => true`,
			`1 2 gte              => false`,
			`"b" "a" gte          => true`,
		},
	})

	register(&Entry{
		Word:    "eq",
		Summary: "Test if two values are equal.",
		Signatures: []string{"[any any] -> [boolean]"},
		Description: "Compares two values for equality. Numbers, strings, booleans, and atoms are compared by value.",
		Examples: []string{
			`1 1 eq               => true`,
			`1 2 eq               => false`,
			`"a" "a" eq           => true`,
			`true true eq         => true`,
		},
	})

	register(&Entry{
		Word:    "neq",
		Summary: "Test if two values are not equal.",
		Signatures: []string{"[any any] -> [boolean]"},
		Description: "Returns true if the two values are different.",
		Examples: []string{
			`1 2 neq              => true`,
			`1 1 neq              => false`,
			`"a" "b" neq          => true`,
			`true false neq       => true`,
		},
	})

	register(&Entry{
		Word:    "deq",
		Summary: "Deep equality test for two values.",
		Signatures: []string{"[any any] -> [boolean]"},
		Description: "Recursively compares two values including nested lists and maps.",
		Examples: []string{
			`[1 2] [1 2] deq      => true`,
			`[1 2] [1 3] deq      => false`,
			`1 1 deq              => true`,
			`"a" "a" deq          => true`,
		},
	})
}
