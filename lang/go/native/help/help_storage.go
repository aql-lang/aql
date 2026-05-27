package help

func init() {
	register(&Entry{
		Word:    "set",
		Summary: "Store a value in a Store.",
		Description: "Stores a value under a key in the given Store. " +
			"The key is typically a string or atom. The Store is the context or any Store instance.",
	})

	register(&Entry{
		Word:    "get",
		Summary: "Retrieve a value from a Store, Map, List, or Object.",
		Description: "Retrieves a value by key from a Store (with prototype chain resolution), " +
			"a Map (by string/atom key), a List (by integer index), or an Object instance (by field name). " +
			"The . (dot) operator is an alias for get. " +
			"Dot syntax shorthand: .fieldname is equivalent to get fieldname. " +
			"Returns None for missing keys in Maps/Objects/Lists.",
	})

	register(&Entry{
		Word:    "getr",
		Summary: "Strict value retrieval — errors on missing keys.",
		Description: "Like get, but returns an error when the key or index is missing, " +
			"or the parent is None, instead of silently returning None. " +
			"The !. operator is an alias for getr.",
	})

	register(&Entry{
		Word:    "context",
		Summary: "Push the current context Store onto the stack.",
		Description: "Returns the current context Store. The context is a Store (Object/Store) " +
			"that supports prototype chain resolution for nested scopes. " +
			"Use 'context set key value' to store and 'context get key' to retrieve.",
	})
}
