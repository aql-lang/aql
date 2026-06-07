package modules

func init() {
	registerDocs("aql:type-util", map[string]string{
		"alts":      "The alternatives of a disjunct as a List of types.",
		"arityof":   "Count of a function type's required parameters.",
		"brand":     "Mint a nominal subtype of a base type tagged by an atom.",
		"exclude":   "Remove a type (and its subtypes) from a disjunct.",
		"extract":   "Keep only the alternatives matching the given type.",
		"lca":       "Least common ancestor (supertype) of two types.",
		"merge":     "Combine two record/object types, unifying overlaps.",
		"nominal":   "Mint an anonymous nominal alias of a base type.",
		"omit":      "Drop the named fields from a record/object type.",
		"paramsof":  "A function type's parameter types as a List.",
		"parent":    "The immediate supertype in the type lattice.",
		"pick":      "Keep only the named fields of a record/object type.",
		"required":  "Strip the optional (None) alternative from each field.",
		"returnsof": "A function type's return type.",
		"root":      "The branch root (top supertype) in the lattice.",
		"tpartial":  "Make every field of a record/object type optional.",
	})
}
