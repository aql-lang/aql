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
			`"sales" from [[sum amount total]] select`,
			`"users" from star select`,
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
			`"orders" from [id total] select`,
			`"products" from [name] select 10 limit`,
			`"logs" from [timestamp message] select`,
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
			`"orders" from [total 100 gt] where`,
			`"products" from [price 10 gte price 50 lte and] where`,
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
			`"orders" from [date [total desc]] order`,
			`"products" from [[price desc] name] order`,
		},
	})

	register(&Entry{
		Word:    "limit",
		Summary: "Limit the number of query results.",
		Signatures: []string{"[query integer] -> [query]"},
		Description: "Restricts the query to return at most n rows.",
		Examples: []string{
			`"users" from 10 limit`,
			`"orders" from 5 limit`,
			`"products" from [name] select 3 limit`,
			`"logs" from 100 limit [timestamp] order`,
		},
	})

	register(&Entry{
		Word:    "offset",
		Summary: "Skip rows in query results.",
		Signatures: []string{"[query integer] -> [query]"},
		Description: "Skips the first n rows before returning results.",
		Examples: []string{
			`"users" from 10 limit 5 offset`,
			`"orders" from 20 limit 10 offset`,
			`"products" from 10 limit 0 offset`,
			`"logs" from 50 limit 100 offset`,
		},
	})

	register(&Entry{
		Word:    "distinct",
		Summary: "Remove duplicate rows from query results.",
		Signatures: []string{"[query] -> [query]"},
		Description: "Adds SELECT DISTINCT to the query.",
		Examples: []string{
			`"users" from [name] select distinct`,
			`"orders" from [status] select distinct`,
			`"products" from [category] select distinct`,
			`"logs" from [level] select distinct`,
		},
	})

	register(&Entry{
		Word:    "group",
		Summary: "Group query results by columns.",
		Signatures: []string{"[query list] -> [query]"},
		Description: "Groups rows by the specified columns for aggregate queries.",
		Examples: []string{
			`"sales" from [category] group [category [sum amount total]] select`,
			`"orders" from [status] group [status [count id n]] select`,
			`"users" from [role] group [role [avg age avg_age]] select`,
			`"products" from [category] group`,
		},
	})

	register(&Entry{
		Word:    "having",
		Summary: "Filter grouped query results.",
		Signatures: []string{"[query list] -> [query]"},
		Description: "Filters groups after GROUP BY, like WHERE but for aggregated values.",
		Examples: []string{
			`"sales" from [category] group [[sum amount] 100 gt] having`,
			`"orders" from [status] group [[count id] 5 gte] having`,
			`"users" from [role] group [[avg age] 30 lt] having`,
			`"products" from [category] group [[min price] 10 gt] having`,
		},
	})

	register(&Entry{
		Word:    "as",
		Summary: "Alias a table in a query.",
		Signatures: []string{"[query string] -> [query]"},
		Description: "Sets an alias for the query's source table.",
		Examples: []string{
			`"users" from "u" as`,
			`"orders" from "o" as`,
			`"products" from "p" as [name price] select`,
			`"long_table_name" from "t" as`,
		},
	})

	register(&Entry{
		Word:    "star",
		Summary: "Push a wildcard column selector.",
		Signatures: []string{"[] -> [atom]"},
		Description: "Pushes the * selector for use in SELECT * queries. Prefix-only.",
		Examples: []string{
			`"users" from star select`,
			`"orders" from star select 10 limit`,
			`"products" from star select [name] order`,
			`"logs" from star select distinct`,
		},
	})

	register(&Entry{
		Word:    "unify",
		Summary: "Unify two values using structural pattern matching.",
		Signatures: []string{"[any any] -> [any boolean]"},
		Description: "Attempts to unify two values. Pushes the unified value and a boolean. " +
			"Returns true if they match structurally.",
		Examples: []string{
			`{a: 1} {a: 1} unify           => {a:1} true`,
			`1 2 unify                     => '~unify-fail' false`,
			`[1 2] [1 2] unify             => [1,2] true`,
			`1 1 unify                     => 1 true`,
		},
	})

	register(&Entry{
		Word:    "module",
		Summary: "Define a module with exported words.",
		Signatures: []string{"[atom list] -> []"},
		Description: "Creates a named module. The list is evaluated in an isolated scope and " +
			"exported words become available under the module name.",
		Examples: []string{
			`def mymod [def double [2 mul]] module`,
			`def utils [def inc [1 add] def dec [1 sub]] module`,
			`def math [def square fn [[{x: Number}] [Number] [x x mul]]] module`,
			`def helpers [def greet ["hello"]] module`,
		},
	})

	register(&Entry{
		Word:    "import",
		Summary: "Import a module from a file.",
		Signatures: []string{
			"[string] -> []",
			"[list string] -> []",
		},
		Description: "Loads and executes a .aql file as a module, making exported words available. " +
			"Use a list argument to rename imports.",
		Examples: []string{
			`"utils.aql" import`,
			`"helpers.aql" import`,
			`[Orig Renamed] "utils.aql" import`,
			`[[A AA] [B BB]] "data.aql" import`,
		},
	})
}
