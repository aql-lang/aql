package help

func init() {
	register(&Entry{
		Word:    "set",
		Summary: "Store a value in the key-value store.",
		Description: "Stores the second value under the key derived from the first value. " +
			"The key is typically a string or atom.",
	})

	register(&Entry{
		Word:    "get",
		Summary: "Retrieve a value from the key-value store.",
		Description: "Retrieves the value stored under the given key. Returns None if the key does not exist.",
	})

	register(&Entry{
		Word:    "dot",
		Summary: "Access a field in a map or table.",
		Description: "Retrieves the value of a field from a map. Dot syntax shorthand: " +
			".fieldname is equivalent to \"fieldname\" dot.",
	})

	register(&Entry{
		Word:    "context",
		Summary: "Dispatch context set or context get for scoped storage.",
		Description: "Dispatches to context-set or context-get. Use 'context set key value' " +
			"to store a value, and 'context get key' to retrieve it. Context values are " +
			"scoped to the current execution context.",
	})
}
