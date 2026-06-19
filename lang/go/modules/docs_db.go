package modules

func init() {
	registerDocs("aql:db", map[string]string{
		"open":   "Open a connection to an external SQL database.",
		"close":  "Close a database connection.",
		"sql":    "Run a raw SQL query and return the rows as a table.",
		"exec":   "Run a raw SQL statement (DML/DDL); returns rows affected.",
		"tables": "List the table names in a database.",
		"schema": "Return the column schema of a table as a {column: type} map.",
	})
}
