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
}
