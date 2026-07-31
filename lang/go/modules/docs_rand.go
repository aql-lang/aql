package modules

func init() {
	registerDocs("boru:rand", map[string]string{
		"int":       "Random integer in the half-open range [lo, hi).",
		"bool":      "Random boolean.",
		"float":     "Random float in [0.0, 1.0).",
		"string":    "Random string of length N drawn from a charset.",
		"one-of":    "Pick one random element of a list.",
		"list-of":   "Build a list by running a generator body N times.",
		"map-from":  "Build a map by running each value's generator body.",
		"with-seed": "Make a fresh seeded, reproducible generator instance.",
	})

	// The point of this module is the SEEDED generator, which the flat
	// signatures hide entirely: the module words draw from a shared
	// source, while `with-seed` hands back a reproducible one whose
	// methods are reached with dot access.
	registerExamples("boru:rand", map[string][]string{
		"with-seed": {
			`def r (Rand.with-seed 42)`,
			`r.int 0 100                                      ;# 75 — same seed, same sequence, every run`,
			`r.bool`,
			`r.one-of [10 20 30]`,
			`;# Use this for tests and property checks; the bare Rand.* words`,
			`;# draw from a shared unseeded source and are NOT reproducible.`,
		},
		"int":     {`Rand.int 0 100                                   ;# in [0,100)`},
		"float":   {`Rand.float                                       ;# in [0.0,1.0)`},
		"bool":    {`Rand.bool`},
		"one-of":  {`Rand.one-of [10 20 30]                           ;# one element, uniformly`},
		"string":  {`Rand.string "abc" 4                              ;# 4 characters drawn from the ALPHABET "abc"`},
		"list-of": {`Rand.list-of [Rand.int 0 100] 3                  ;# the BODY is re-run per element`},
		"map-from": {
			`Rand.map-from {a:[Rand.int 0 10]}                ;# {a:7} — each value is a body, run once`,
		},
	})
}
