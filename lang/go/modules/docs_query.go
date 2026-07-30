package modules

func init() {
	registerDocs("aql:query", map[string]string{
		"select":    "Seed a query, projecting the named columns (empty = all).",
		"from":      "Set the source table, resolved by name from context.",
		"where":     "Attach a WHERE condition clause to filter rows.",
		"order":     "Attach an ORDER BY sort clause.",
		"group":     "Attach GROUP BY grouping keys.",
		"having":    "Filter groups after grouping (HAVING).",
		"limit":     "Cap the result to at most N rows.",
		"offset":    "Skip the leading N rows.",
		"distinct":  "Deduplicate the result rows.",
		"join":      "Join another table into the query.",
		"innerjoin": "Inner-join another table into the query.",
		"leftjoin":  "Left-outer-join another table into the query.",
		"crossjoin": "Cross-join (cartesian product) another table.",
		"on":        "Attach a join predicate to the preceding join.",
		"using":     "Join on a shared column by name.",
		"union":     "Union two queries, dropping duplicates.",
		"unionall":  "Union two queries, keeping duplicates.",
		"intersect": "Intersection of two queries.",
		"except":    "Set difference (rows in first, not in second).",
	})

	// Query words COMPOSE: each takes the query built so far and returns
	// it, so a query is written as a left-to-right chain. That shape is
	// the entire API and no per-word signature shows it, which is why
	// every word here carries the same worked chain.
	registerExamples("aql:query", map[string][]string{
		"select": {
			`Query.select [name age] Query.from people Query.where [age gt 25]`,
			`  Query.order [age desc] Query.limit 10`,
			`;# select seeds the chain; an empty list [] projects every column.`,
			`;# ` + "`from`" + ` resolves the table BY NAME from context — it is not a value.`,
		},
		"from":     {`Query.select [] Query.from people                ;# every column of the people table`},
		"where":    {`Query.select [name] Query.from people Query.where [age gt 25]`},
		"order":    {`Query.select [name] Query.from people Query.order [age desc]`},
		"limit":    {`Query.select [name] Query.from people Query.limit 10`},
		"offset":   {`Query.select [name] Query.from people Query.offset 20 Query.limit 10   ;# page 3`},
		"distinct": {`Query.select [city] Query.from people Query.distinct`},
		"group": {
			`Query.select [city] Query.from people Query.group [city] Query.having [cnt gt 1]`,
			`;# having filters GROUPS, where filters rows — they are not interchangeable.`,
		},
		"having":    {`Query.select [city] Query.from people Query.group [city] Query.having [cnt gt 1]`},
		"join":      {`Query.select [name] Query.from people Query.join visits Query.on [name eq who]`},
		"innerjoin": {`Query.select [name] Query.from people Query.innerjoin visits Query.using [name]`},
		"leftjoin":  {`Query.select [name] Query.from people Query.leftjoin visits Query.using [name]`},
		"crossjoin": {`Query.select [name] Query.from people Query.crossjoin visits`},
		"on":        {`Query.select [name] Query.from people Query.join visits Query.on [name eq who]`},
		"using":     {`Query.select [name] Query.from people Query.join visits Query.using [name]`},
		"union":     {`Query.union (Query.select [name] Query.from a) (Query.select [name] Query.from b)`},
		"unionall":  {`Query.unionall (Query.select [name] Query.from a) (Query.select [name] Query.from b)   ;# keeps duplicates`},
		"intersect": {`Query.intersect (Query.select [name] Query.from a) (Query.select [name] Query.from b)`},
		"except":    {`Query.except (Query.select [name] Query.from a) (Query.select [name] Query.from b)`},
	})
}
