package modules

func init() {
	registerDocs("aql:rand", map[string]string{
		"int":       "Random integer in the half-open range [lo, hi).",
		"bool":      "Random boolean.",
		"float":     "Random float in [0.0, 1.0).",
		"string":    "Random string of length N drawn from a charset.",
		"one-of":    "Pick one random element of a list.",
		"list-of":   "Build a list by running a generator body N times.",
		"map-from":  "Build a map by running each value's generator body.",
		"with-seed": "Make a fresh seeded, reproducible generator instance.",
	})
}
