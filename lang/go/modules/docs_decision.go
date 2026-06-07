package modules

func init() {
	registerDocs("aql:decision", map[string]string{
		"cond":        "Build a Cond record from a field, op, and value.",
		"all-of":      "Build an all-of predicate where every child must hold.",
		"any-of":      "Build an any-of predicate where one child must hold.",
		"not-of":      "Build a not predicate that negates a single condition.",
		"make-rule":   "Build a Rule record pairing a when condition with a then result.",
		"make-table":  "Build a decision table from rules (defaults to 'first' hit policy).",
		"with-policy": "Return a copy of a table with a different hit policy.",
		"make-branch": "Build a BranchNode record from an id and its branches.",
		"make-leaf":   "Build a LeafNode record from an id and its result.",
		"make-tree":   "Build a DTree record from a root id and its nodes.",
		"apply-op":    "Apply a comparison operator to two values, returning a Bool.",
		"eval-cond":   "Evaluate one condition against an input map.",
		"eval-pred":   "Evaluate a compound predicate against an input map.",
		"eval-table":  "Evaluate a decision table, returning the matching rule result.",
		"eval-tree":   "Walk a decision tree to its matching leaf result.",
		"decide":      "Top-level evaluator dispatching on model kind (table or tree).",
	})
}
