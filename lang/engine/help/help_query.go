package help

func init() {
	register(&Entry{
		Word:    "select",
		Summary: "Select columns from a table query.",
		Description: "Specifies which columns to include in the query result. Supports " +
			"renaming, casting, and aggregate functions (sum, avg, min, max, count).",
	})

	register(&Entry{
		Word:        "from",
		Summary:     "Start a query from a table.",
		Description: "Creates a new query builder targeting the named table or table value.",
	})

	register(&Entry{
		Word:    "where",
		Summary: "Add a filter condition to a query.",
		Description: "Filters rows matching the condition. Supports operators: eq, neq, lt, gt, " +
			"lte, gte, like, in, between, is-null, is-not-null, and, or, not.",
	})

	register(&Entry{
		Word:        "order",
		Summary:     "Specify sort order for query results.",
		Description: "Orders query results by the specified columns. Use [col desc] for descending.",
	})

	register(&Entry{
		Word:        "limit",
		Summary:     "Limit the number of query results.",
		Description: "Restricts the query to return at most n rows.",
	})

	register(&Entry{
		Word:        "offset",
		Summary:     "Skip rows in query results.",
		Description: "Skips the first n rows before returning results.",
	})

	register(&Entry{
		Word:        "distinct",
		Summary:     "Remove duplicate rows from query results.",
		Description: "Adds SELECT DISTINCT to the query.",
	})

	register(&Entry{
		Word:        "group",
		Summary:     "Group query results by columns.",
		Description: "Groups rows by the specified columns for aggregate queries.",
	})

	register(&Entry{
		Word:        "having",
		Summary:     "Filter grouped query results.",
		Description: "Filters groups after GROUP BY, like WHERE but for aggregated values.",
	})

	register(&Entry{
		Word:        "as",
		Summary:     "Alias a table in a query.",
		Description: "Sets an alias for the query's source table.",
	})

	register(&Entry{
		Word:        "star",
		Summary:     "Push a wildcard column selector.",
		Description: "Pushes the * selector for use in SELECT * queries. Stack-only.",
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
	})

	register(&Entry{
		Word:    "import",
		Summary: "Import a module or data file.",
		Description: "Loads a file as a module or data. File paths must start with /, ./ or ../. " +
			"For .aql files, executes as an isolated module and installs exports. " +
			"For .json/.jsonic files, pushes parsed data onto the stack. " +
			"For .csv/.tsv files, loads data as a table. " +
			"Use a list argument to rename imports (not supported for data files).",
	})
}
