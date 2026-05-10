package help

func init() {
	register(&Entry{
		Word:    "lt",
		Summary: "Test if the first value is less than the second.",
		Description: "Compares two values. Numbers are compared numerically; strings lexicographically.",
	})

	register(&Entry{
		Word:    "gt",
		Summary: "Test if the first value is greater than the second.",
		Description: "Compares two values. Numbers are compared numerically; strings lexicographically.",
	})

	register(&Entry{
		Word:    "lte",
		Summary: "Test if the first value is less than or equal to the second.",
		Description: "Less-than-or-equal comparison.",
	})

	register(&Entry{
		Word:    "gte",
		Summary: "Test if the first value is greater than or equal to the second.",
		Description: "Greater-than-or-equal comparison.",
	})

	register(&Entry{
		Word:    "eq",
		Summary: "Test if two values are equal.",
		Description: "Compares two values for equality. Numbers, strings, booleans, and atoms are compared by value.",
	})

	register(&Entry{
		Word:    "neq",
		Summary: "Test if two values are not equal.",
		Description: "Returns true if the two values are different.",
	})

	register(&Entry{
		Word:    "deq",
		Summary: "Deep equality test for two values.",
		Description: "Recursively compares two values including nested lists and maps.",
	})
}
