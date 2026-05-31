package help

func init() {
	// SQL-style query DSL words. These live in the aql:query module and
	// are accessed via dot notation after `"aql:query" import` — e.g.
	// query.from, query.where, query.select. They form a left-to-right
	// pipeline in forward form: `query.from people query.where [age gt
	// 18] query.select [name age]`.

	register(&Entry{
		Word:    "select",
		Summary: "Start a query, projecting columns (aql:query).",
		Description: "The SQL-order entry word: `select [name age] from people …`. Seeds a new " +
			"lazy query with the projection columns; an empty list [] selects every column. " +
			"Supports renaming ([col alias]), casting ([cast col type]), and aggregate functions " +
			"(sum, avg, min, max, count). The query runs only when its result is printed, " +
			"iterated, or otherwise needs rows. Imported from aql:query; call as query.select.",
	})

	register(&Entry{
		Word:    "from",
		Summary: "Set the source table of a query (aql:query).",
		Description: "Sets the table the preceding `select` reads from — required in every query. " +
			"Given a bare name it looks the table up in the context store (set via " +
			"`context set <name> <table>`); given a table value it uses that. Imported from " +
			"aql:query; call as query.from.",
	})

	register(&Entry{
		Word:    "where",
		Summary: "Add a filter condition to a query (aql:query).",
		Description: "Filters rows matching the condition. Supports operators: eq, neq, lt, gt, " +
			"lte, gte, like, in, between, is null, is not null, and, or, not. Imported from " +
			"aql:query; call as query.where.",
	})

	register(&Entry{
		Word:    "order",
		Summary: "Specify sort order for query results (aql:query).",
		Description: "Orders query results by the specified columns. Use [col desc] for descending. " +
			"Imported from aql:query; call as query.order.",
	})

	register(&Entry{
		Word:        "limit",
		Summary:     "Limit the number of query results (aql:query).",
		Description: "Restricts the query to return at most n rows. Imported from aql:query; call as query.limit.",
	})

	register(&Entry{
		Word:        "offset",
		Summary:     "Skip rows in query results (aql:query).",
		Description: "Skips the first n rows before returning results. Imported from aql:query; call as query.offset.",
	})

	register(&Entry{
		Word:        "distinct",
		Summary:     "Remove duplicate rows from query results (aql:query).",
		Description: "Adds SELECT DISTINCT to the query. Imported from aql:query; call as query.distinct.",
	})

	register(&Entry{
		Word:        "group",
		Summary:     "Group query results by columns (aql:query).",
		Description: "Groups rows by the specified columns for aggregate queries. Imported from aql:query; call as query.group.",
	})

	register(&Entry{
		Word:    "having",
		Summary: "Filter grouped query results (aql:query).",
		Description: "Filters groups after GROUP BY, like WHERE but for aggregated values. " +
			"Imported from aql:query; call as query.having.",
	})

	register(&Entry{
		Word:    "join",
		Summary: "Join another table into a query (aql:query).",
		Description: "Adds a JOIN against the named table; pair with query.on (ON condition) or " +
			"query.using (shared columns). Variants: query.join / query.innerjoin (inner), " +
			"query.leftjoin (left outer), query.crossjoin (cross). Imported from aql:query.",
	})

	register(&Entry{
		Word:    "union",
		Summary: "Combine two queries with a set operation (aql:query).",
		Description: "Appends a set operation against a right-hand query: query.union (distinct), " +
			"query.unionall (keep duplicates), query.intersect, query.except. The right-hand " +
			"query is typically a parenthesized sub-pipeline. Imported from aql:query.",
	})

	register(&Entry{
		Word:    "unify",
		Summary: "Unify two values using structural pattern matching.",
		Description: "Attempts to unify two values. Pushes the unified value and a boolean. " +
			"Returns true if they match structurally.",
	})

	register(&Entry{
		Word:    "module",
		Summary: "Define a module with exported words.",
		Description: "Creates a named module. The list is evaluated in an isolated scope and " +
			"exported words become available under the module name.",
		Examples: []string{
			`import module [export "Utils" {greet: greet/r}] end  ;# inline module`,
		},
	})

	register(&Entry{
		Word:    "import",
		Summary: "Import a module or data file.",
		Description: "Loads a file as a module or data. File paths must start with /, ./ or ../. " +
			"For .aql files, executes as an isolated module and installs exports. " +
			"For .json/.jsonic files, pushes parsed data onto the stack. " +
			"For .csv/.tsv files, loads data as a table. " +
			"Use a list argument to rename imports (not supported for data files).",
		Examples: []string{
			`"aql:math" import end   ;# native module → math.* namespace`,
			`"./lib.aql" import end  ;# sibling file's exports`,
			`import [Orig Alias] "./lib.aql" end  ;# rename on import`,
		},
	})

	register(&Entry{
		Word:    "export",
		Summary: "Publish a namespace from a module.",
		Description: "Inside a module body, `export \"Name\" {key: value …}` publishes " +
			"a namespace reachable as Name.key after import. Export functions with the " +
			"/r ref modifier so the map doesn't dispatch them. At the top level (running " +
			"a file directly) export is a no-op, so one file can both run standalone and " +
			"export when imported.",
		Examples: []string{
			`export "Utils" {pi: pi, greet: greet/r}  ;# value bare, fn with /r`,
		},
	})

	register(&Entry{
		Word:    "each",
		Summary: "Map a code body over each element of a list.",
		Description: "Runs the body once per element with the element on the stack, " +
			"collecting each body's result into a new list. The body must leave a value; " +
			"for a side-effecting loop that collects nothing, use do-each.",
		Examples: []string{
			`[1 2 3] each [dup mul]  ;# => [1 4 9]`,
			`iota 4 each [1 add]     ;# => [1 2 3 4]`,
		},
	})

	register(&Entry{
		Word:    "do-each",
		Summary: "Run a code body over each element for side effects, discarding results.",
		Description: "Like each, but discards every body result and produces nothing. " +
			"The body may leave the stack empty (no sentinel needed) — use it for " +
			"mutating loops that don't collect a value.",
		Examples: []string{
			`[1 2 3] do-each [print]   ;# prints 1,2,3; leaves nothing`,
		},
	})

	register(&Entry{
		Word:    "fold",
		Summary: "Reduce a list to a single value with an accumulator.",
		Description: "Runs the body with the accumulator and each element, threading the " +
			"result forward. With an initial value: `init fold [body] data`. Without one, " +
			"the first element seeds the accumulator: `fold [body] data`.",
		Examples: []string{
			`0 fold [add] [1 2 3 4]  ;# => 10  (with initial value)`,
			`fold [add] [1 2 3]      ;# => 6   (first element seeds it)`,
		},
	})
}
