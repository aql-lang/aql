package help

func init() {
	register(&Entry{
		Word:    "set",
		Summary: "Store a value in the key-value store.",
		Signatures: []string{"[any any] -> []"},
		Description: "Stores the second value under the key derived from the first value. " +
			"The key is typically a string or atom.",
		Examples: []string{
			`x 42 set     => (stores 42 under key "x")`,
		},
	})

	register(&Entry{
		Word:    "get",
		Summary: "Retrieve a value from the key-value store.",
		Signatures: []string{"[any] -> [any]"},
		Description: "Retrieves the value stored under the given key. Returns None if the key does not exist.",
		Examples: []string{
			`x get        => 42 (if previously set)`,
		},
	})

	register(&Entry{
		Word:    "context",
		Summary: "Access or set values in the current execution context.",
		Signatures: []string{
			"[map] -> [map]",
		},
		Description: "Provides access to the current execution context. Context values are " +
			"scoped and inherited by child contexts.",
		Examples: []string{
			`context`,
		},
	})

	register(&Entry{
		Word:    "dot",
		Summary: "Access a field in a map or table.",
		Signatures: []string{
			"[map atom] -> [any]",
			"[map string] -> [any]",
		},
		Description: "Retrieves the value of a field from a map. Can be written with dot " +
			"syntax: .fieldname is shorthand for \"fieldname\" dot.",
		Examples: []string{
			`{name: "Alice"} .name   => "Alice"`,
		},
	})

	register(&Entry{
		Word:    "dotr",
		Summary: "Access a field in a map (reversed argument order).",
		Signatures: []string{
			"[atom map] -> [any]",
			"[string map] -> [any]",
		},
		Description: "Same as dot but with reversed argument order. Also available as \"!.\".",
		Examples: []string{
			`name {name: "Alice"} dotr   => "Alice"`,
		},
	})
}
