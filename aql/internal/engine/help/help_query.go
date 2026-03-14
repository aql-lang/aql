package help

func init() {
	register(&Entry{
		Word:    "select",
		Summary: "Select columns from a table query.",
		Signatures: []string{"[query list] -> [query]"},
		Description: "Specifies which columns to include in the query result. Supports " +
			"renaming, casting, and aggregate functions (sum, avg, min, max, count).",
		Examples: []string{
			`"users" from [name age] select`,
			`"users" from [[name user_name]] select`,
		},
	})

	register(&Entry{
		Word:    "from",
		Summary: "Start a query from a table.",
		Signatures: []string{
			"[string] -> [query]",
			"[table] -> [query]",
		},
		Description: "Creates a new query builder targeting the named table or table value.",
		Examples: []string{
			`"users" from`,
		},
	})

	register(&Entry{
		Word:    "where",
		Summary: "Add a filter condition to a query.",
		Signatures: []string{"[query list] -> [query]"},
		Description: "Filters rows matching the condition. Supports operators: eq, neq, lt, gt, " +
			"lte, gte, like, in, between, is-null, is-not-null, and, or, not.",
		Examples: []string{
			`"users" from [age 18 gte] where`,
			`"users" from [name "Alice" eq] where`,
		},
	})

	register(&Entry{
		Word:    "order",
		Summary: "Specify sort order for query results.",
		Signatures: []string{"[query list] -> [query]"},
		Description: "Orders query results by the specified columns. Use [col desc] for descending.",
		Examples: []string{
			`"users" from [name] order`,
			`"users" from [[age desc]] order`,
		},
		Notes: []string{"Must be followed by implicit 'by' in some syntaxes."},
	})

	register(&Entry{
		Word:    "limit",
		Summary: "Limit the number of query results.",
		Signatures: []string{"[query integer] -> [query]"},
		Description: "Restricts the query to return at most n rows.",
		Examples: []string{`"users" from 10 limit`},
	})

	register(&Entry{
		Word:    "offset",
		Summary: "Skip rows in query results.",
		Signatures: []string{"[query integer] -> [query]"},
		Description: "Skips the first n rows before returning results.",
		Examples: []string{`"users" from 10 limit 5 offset`},
	})

	register(&Entry{
		Word:    "distinct",
		Summary: "Remove duplicate rows from query results.",
		Signatures: []string{"[query] -> [query]"},
		Description: "Adds SELECT DISTINCT to the query.",
		Examples: []string{`"users" from [name] select distinct`},
	})

	register(&Entry{
		Word:    "group",
		Summary: "Group query results by columns.",
		Signatures: []string{"[query list] -> [query]"},
		Description: "Groups rows by the specified columns for aggregate queries.",
		Examples: []string{
			`"sales" from [category] group [category [sum amount total]] select`,
		},
	})

	register(&Entry{
		Word:    "having",
		Summary: "Filter grouped query results.",
		Signatures: []string{"[query list] -> [query]"},
		Description: "Filters groups after GROUP BY, like WHERE but for aggregated values.",
		Examples: []string{
			`"sales" from [category] group [[sum amount] 100 gt] having`,
		},
	})

	register(&Entry{
		Word:    "as",
		Summary: "Alias a table in a query.",
		Signatures: []string{"[query string] -> [query]"},
		Description: "Sets an alias for the query's source table.",
		Examples: []string{`"users" from "u" as`},
	})

	register(&Entry{
		Word:    "star",
		Summary: "Push a wildcard column selector.",
		Signatures: []string{"[] -> [atom]"},
		Description: "Pushes the * selector for use in SELECT * queries.",
		Examples: []string{`"users" from star select`},
	})

	register(&Entry{
		Word:    "unify",
		Summary: "Unify two values using structural pattern matching.",
		Signatures: []string{"[any any] -> [boolean]"},
		Description: "Attempts to unify two values. Returns true if they match, binding " +
			"variables as needed.",
		Examples: []string{`{a: 1} {a: 1} unify => true`},
	})

	register(&Entry{
		Word:    "module",
		Summary: "Define a module with exported words.",
		Signatures: []string{"[atom list] -> []"},
		Description: "Creates a named module. The list is evaluated in an isolated scope and " +
			"exported words become available under the module name.",
		Examples: []string{`mymod [double [2 mul] def] module`},
	})

	register(&Entry{
		Word:    "import",
		Summary: "Import a module from a file or definition.",
		Signatures: []string{
			"[string] -> []",
			"[list string] -> []",
		},
		Description: "Loads and executes a .aql file as a module, making exported words available. " +
			"Use a list argument to rename imports.",
		Examples: []string{
			`"utils.aql" import`,
			`[Orig Renamed] "utils.aql" import`,
		},
	})
}
