package help

func init() {
	// The ordering words (lt / gt / lte / gte / cmp) are family-
	// restricted: they compare only same-type values, or values a
	// shared same-family comparer handles (Integer-vs-Float, two Dates,
	// …). A cross-family pair (Integer-vs-String, List-vs-Map) raises
	// [boru/incomparable]; use tcmp for a cross-type total order.
	const restrictNote = "Compares only same-family values (e.g. Integer " +
		"and Float, two strings); a cross-family pair raises " +
		"[boru/incomparable] — use tcmp for a cross-type total order."

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
		Word:    "eq",
		Summary: "Test if two values are equal (scalars by value, compounds by identity).",
		Description: "Scalars — numbers (cross-leaf magnitude allowed, 1 eq 1.0), " +
			"strings, booleans, atoms — compare by value. Compound values " +
			"(lists, maps) compare by IDENTITY, not structure: two distinct " +
			"equal-looking lists are NOT eq. This is by design — use deq for a " +
			"deep, structural comparison of contents.",
		Notes: []string{
			"`{a:1} eq {a:1}` is `false` and `[\"a\" \"b\"] eq [\"a\" \"b\"]` is `false` " +
				"(distinct compound values, identity compare); `deq` returns `true` " +
				"for both — in tests, assert maps/lists with `deq`, not `eq`.",
		},
	})

	register(&Entry{
		Word:        "neq",
		Summary:     "Test if two values are not equal (the negation of eq).",
		Description: "Returns true exactly when eq is false; carries eq's identity-for-compounds semantics (see eq).",
	})

	register(&Entry{
		Word:    "deq",
		Summary: "Deep, structural equality of two values.",
		Description: "Recursively compares two values by structure, including nested " +
			"lists and maps, so equal-looking compounds are deq-equal regardless " +
			"of identity. The structural counterpart to eq (which compares " +
			"compounds by identity).",
	})
}
