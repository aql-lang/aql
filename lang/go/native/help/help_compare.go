package help

func init() {
	// The ordering words (lt / gt / lte / gte / cmp) are family-
	// restricted: they compare only same-type values, or values a
	// shared same-family comparer handles (Integer-vs-Float, two Dates,
	// …). A cross-family pair (Integer-vs-String, List-vs-Map) raises
	// [aql/incomparable]; use tcmp for a cross-type total order.
	const restrictNote = "Compares only same-family values (e.g. Integer " +
		"and Float, two strings); a cross-family pair raises " +
		"[aql/incomparable] — use tcmp for a cross-type total order."

	register(&Entry{
		Word:        "lt",
		Summary:     "Test if the first value is less than the second.",
		Description: "Numbers are compared numerically; strings lexicographically. " + restrictNote,
	})

	register(&Entry{
		Word:        "gt",
		Summary:     "Test if the first value is greater than the second.",
		Description: "Numbers are compared numerically; strings lexicographically. " + restrictNote,
	})

	register(&Entry{
		Word:        "lte",
		Summary:     "Test if the first value is less than or equal to the second.",
		Description: "Less-than-or-equal comparison. " + restrictNote,
	})

	register(&Entry{
		Word:        "gte",
		Summary:     "Test if the first value is greater than or equal to the second.",
		Description: "Greater-than-or-equal comparison. " + restrictNote,
	})

	register(&Entry{
		Word:    "cmp",
		Summary: "Three-way comparison of two same-family values.",
		Description: "Returns -1 when the first value sorts before the second, 0 when " +
			"they tie, and 1 when it sorts after, surfaced as an Integer. " + restrictNote,
	})

	register(&Entry{
		Word:    "tcmp",
		Summary: "Three-way comparison across any two values (total order).",
		Description: "Like cmp, but unrestricted: compares ANY two values via the " +
			"unified lattice total order (the order sort and the collection words " +
			"use), returning -1 / 0 / 1. Use it for cross-type ordering that cmp " +
			"refuses, e.g. `1 tcmp \"a\"`.",
	})

	register(&Entry{
		Word:        "eq",
		Summary:     "Test if two values are equal.",
		Description: "Compares two values for equality. Numbers, strings, booleans, and atoms are compared by value.",
	})

	register(&Entry{
		Word:        "neq",
		Summary:     "Test if two values are not equal.",
		Description: "Returns true if the two values are different.",
	})

	register(&Entry{
		Word:        "deq",
		Summary:     "Deep equality test for two values.",
		Description: "Recursively compares two values including nested lists and maps.",
	})
}
