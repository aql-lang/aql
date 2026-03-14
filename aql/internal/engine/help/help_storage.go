package help

func init() {
	register(&Entry{
		Word:    "set",
		Summary: "Store a value in the key-value store.",
		Signatures: []string{"[any any] -> []"},
		Description: "Stores the second value under the key derived from the first value. " +
			"The key is typically a string or atom.",
		Examples: []string{
			`"x" 42 set "x" get            => 42`,
			`("n" "Alice" set) "n" get     => 'Alice'`,
			`("a" 1 set) ("b" 2 set) "a" get => 1`,
			`"flag" true set "flag" get     => true`,
		},
	})

	register(&Entry{
		Word:    "get",
		Summary: "Retrieve a value from the key-value store.",
		Signatures: []string{"[any] -> [any]"},
		Description: "Retrieves the value stored under the given key. Returns None if the key does not exist.",
		Examples: []string{
			`"x" 42 set "x" get            => 42`,
			`"a" 1 set "a" get             => 1`,
			`"flag" true set "flag" get     => true`,
			`"x" 10 set "x" 20 set "x" get => 20`,
		},
	})

	register(&Entry{
		Word:    "dot",
		Summary: "Access a field in a map or table.",
		Signatures: []string{
			"[map atom] -> [any]",
			"[map string] -> [any]",
		},
		Description: "Retrieves the value of a field from a map. Dot syntax shorthand: " +
			".fieldname is equivalent to \"fieldname\" dot.",
		Examples: []string{
			`{name: "Alice" age: 30} .name  => 'Alice'`,
			`{name: "Alice" age: 30} .age   => 30`,
			`{x: 1 y: 2} .x                => 1`,
			`{flag: true} .flag             => true`,
		},
	})

	register(&Entry{
		Word:    "context",
		Summary: "Dispatch context set or context get for scoped storage.",
		Signatures: []string{
			"[word] -> []",
		},
		Description: "Dispatches to context-set or context-get. Use 'context set key value' " +
			"to store a value, and 'context get key' to retrieve it. Context values are " +
			"scoped to the current execution context.",
		Examples: []string{
			`context set "x" 42 context get "x"       => 42`,
			`context set "a" 10 context get "a"        => 10`,
			`context set "flag" true context get "flag" => true`,
			`context get "missing"                      => None`,
		},
	})
}
