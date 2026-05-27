package help

func init() {
	register(&Entry{
		Word:    "convert",
		Summary: "Convert a value to a different type.",
		Description: "Converts the first argument to the target type. Supports integer, decimal, " +
			"string, boolean conversions. An optional third argument provides settings " +
			"like base for numeric conversions.",
	})

	register(&Entry{
		Word:        "typeof",
		Summary:     "Return the type name of a value.",
		Description: "Consumes the top value and pushes its type name as an atom.",
	})

	register(&Entry{
		Word:    "inspect",
		Summary: "Return a detailed map describing a word, value, or type.",
		Description: "For a word: a map with name, kind, and signatures. For a value or " +
			"type: a map with type ('Type' for a type value, otherwise the value's " +
			"type leaf), struct (the underlying-structure leaf, for types), kind, and " +
			"kind-specific fields (fields / alternatives / child / signatures / …).",
	})

	register(&Entry{
		Word:    "make",
		Summary: "Create a value conforming to a type.",
		Description: "Constructs a value of the given type from the provided data. " +
			"For tables, creates table rows from list data.",
	})

	register(&Entry{
		Word:    "refine",
		Summary: "Construct a (sub)type from a base type.",
		Description: "Builds a (sub)type: `refine Object {fields}` an object type, " +
			"`refine ParentType {fields}` an object subtype, `refine Record [pairs]` " +
			"a record type, `refine Table recordtype` a table type, `refine List` " +
			"a bare nominal subtype. Pair with `def Name (refine …)` to bind it.",
	})

	register(&Entry{
		Word:    "base",
		Summary: "Return the zero/default value for the type of a value.",
		Description: "Consumes a value and returns the zero value for its type: 0 for integers, " +
			"0.0 for decimals, empty string for strings, false for booleans, empty list for lists.",
	})

	register(&Entry{
		Word:    "tor",
		Summary: "Construct a disjunct (union) type from two values.",
		Description: "Returns a disjunct that matches either alternative. " +
			"Flattens nested disjuncts and applies carrier widening. " +
			"Use to build optional fields and union type literals " +
			"(e.g. `string tor none`).",
	})

	register(&Entry{
		Word:    "tand",
		Summary: "Combine two values by conjunction.",
		Description: "For two concrete maps, merges keys (unifying values for " +
			"keys present in both). For other shapes, returns the unification " +
			"of the two arguments. Errors if the values cannot be combined " +
			"(e.g. conflicting concrete values for the same key).",
	})

	register(&Entry{
		Word:    "tany",
		Summary: "Apply `tor` across a list, building a flattened disjunct.",
		Description: "Folds the list with `tor` semantics: every element becomes " +
			"an alternative of the resulting disjunct. Existing disjunct elements " +
			"are flattened. A single-element list returns that element unchanged. " +
			"Errors on an empty list.",
	})

	register(&Entry{
		Word:    "tall",
		Summary: "Apply `tand` across a list, folding via map-merge / unify.",
		Description: "Folds the list with `tand` semantics: concrete maps are " +
			"merged key-by-key (overlapping keys are unified); other shapes are " +
			"unified pairwise. A single-element list returns that element " +
			"unchanged. Errors on an empty list or unifiable failure.",
	})

	register(&Entry{
		Word:    "teq",
		Summary: "Strict type equality (lattice node identity, not subtype).",
		Description: "Returns true iff both args are types AND refer to the same " +
			"lattice node. Distinct from `is`, which is subtype membership: " +
			"`Integer is Number` is true, `Integer teq Number` is false. " +
			"Non-type arguments return false on either side.",
	})

	register(&Entry{
		Word:    "tpartial",
		Summary: "Wrap every field of a Record or Object type in `T | None`.",
		Description: "Returns a new type where each field's value type is " +
			"replaced with the disjunct of itself and None. Idempotent — a " +
			"field already including None is unchanged. For Object types, " +
			"inherited fields are flattened into the result.",
	})
}
