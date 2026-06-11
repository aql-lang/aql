package help

func init() {
	register(&Entry{
		Word:    "set",
		Summary: "Set a key in a container (Store, Object, Array, or Map).",
		Description: "Stores a value under a key. On the MUTABLE containers — a Store " +
			"(copy-on-write context layer), an Object instance (in-place field write), or " +
			"an Array (in-place indexed write) — set mutates the receiver and returns " +
			"nothing. On an immutable MAP, set returns a NEW map with the key bound and " +
			"leaves the receiver untouched (the same copy-returning contract as push), so " +
			"calls chain: `{} set a 1 set b 2`. Keys are strings or atoms (integers for " +
			"Array); wrap a variable in parens for a computed key: `m set (k) v`.",
		// Canonical order: `receiver value key set`.
		Examples: []string{
			`ctx 42 "x" set         ;# store 42 under key "x" in a Store/context`,
			`c 1 "count" set        ;# Object: set field count (c.count := 1)`,
			`{a:1} set b 2          ;# => {a:1 b:2} — new map; {a:1} is unchanged`,
		},
	})

	register(&Entry{
		Word:    "get",
		Summary: "Retrieve a value from a Store, Map, List, or Object.",
		Description: "Retrieves a value by key from a Store (with prototype chain resolution), " +
			"a Map (by string/atom key), a List (by integer index), or an Object instance (by field name). " +
			"The . (dot) operator is an alias for get. " +
			"Dot syntax shorthand: .fieldname is equivalent to get fieldname. " +
			"Returns None for missing keys in Maps/Objects/Lists.",
		// Canonical order: `receiver key get` (or the `.key` shorthand).
		Examples: []string{
			`{k: 9} "k" get         ;# => 9   — Map value by key`,
			`[10 20 30] 0 get       ;# => 10  — List element by index`,
			`c "count" get          ;# Object field (same as c.count)`,
		},
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

	register(&Entry{
		Word:    "keys",
		Summary: "The keys of a map, as a list (insertion order).",
		Description: "Projects a map to the list of its keys, in insertion order. The " +
			"complement to vals; for [key value] pairs use StructUtil.items.",
		Examples: []string{
			`{a:1 b:2 c:3} keys  ;# => ['a' 'b' 'c']`,
		},
	})

	register(&Entry{
		Word:    "vals",
		Summary: "The values of a map, as a list (insertion order).",
		Description: "Projects a map to the list of its values, in insertion order. The " +
			"complement to keys; for [key value] pairs use StructUtil.items.",
		Examples: []string{
			`{a:1 b:2 c:3} vals  ;# => [1 2 3]`,
		},
	})
}

func init() {
	register(&Entry{
		Word:        "reach",
		Summary:     "Build a reusable Reach lens from a path: `reach [keys…]`.",
		Description: "A reach is a first-class accessor over nested data. Build one from a path list, then apply it with apply, read with get, or write through it with rebind. The $.a.b syntax is sugar for a reach.",
	})
	register(&Entry{
		Word:        "apply",
		Summary:     "Apply a referenced function or a reach lens to stack arguments.",
		Description: "x inc/r apply calls the referenced fn inc on x; xs apply $.1 reads index 1 via a reach lens. The general invoke-this-fn-or-lens word.",
		Examples:    []string{`def inc fn [[n:Integer] [Integer] [n add 1]]  5 inc/r apply   ;# => 6`},
	})
	register(&Entry{
		Word:        "rebind",
		Summary:     "Bind a reach lens to a receiver, returning the bound lens.",
		Description: "`rebind $.name p` returns a Reach anchored at p — the writeable counterpart of apply/get. typeof of the result is Reach.",
	})
	register(&Entry{
		Word:        "ref",
		Summary:     "Reference a word's function value without invoking it.",
		Description: "`ref add` yields add's fn value as data (like the /r suffix), so it can be passed around and later applied.",
	})
	register(&Entry{
		Word:        "referent",
		Summary:     "Resolve what a quoted atom refers to in the current scope.",
		Description: "`referent add/q` returns the value currently bound to the name add — the def, fn, or word it denotes.",
	})
}
