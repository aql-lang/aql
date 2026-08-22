package modules

func init() {
	registerDocs("boru:fn-util", map[string]string{
		"compose":  "Compose two functions: (compose f g) x = f (g x).",
		"const":    "A one-argument function that always returns the captured value.",
		"curry":    "Turn a single-signature n-ary function into a chain of unary functions.",
		"flip":     "Reverse a function's signature argument order (the /u wrapper as a word).",
		"identity": "Return the argument unchanged — any kind, functions included.",
		"memoize":  "Wrap a function behind a canon-keyed result cache.",
		"on":       "Apply a binary function through a unary projection: (on b u) x y = b (u x) (u y).",
		"partial":  "Bind a function's first parameter, returning a function of the rest.",
		"pipe":     "Compose two functions in pipeline order: (pipe f g) x = g (f x).",
	})

	// These words take FUNCTIONS as values, so the generated permutations
	// (which substitute data) cannot demonstrate them. Results are from
	// verified lang/spec/module-fn.tsv rows.
	registerExamples("boru:fn-util", map[string][]string{
		"compose": {`def h (FnUtil.compose addone/v double/v) end (h 5)   ;# 11 — addone (double 5)`},
		"pipe":    {`def h (FnUtil.pipe addone/v double/v) end (h 5)      ;# 12 — double (addone 5)`},
		"const":   {`def k (FnUtil.const 7) end (k 99)                    ;# 7 — the argument is ignored`},
		"flip":    {`def fs (FnUtil.flip sub2/v) end (fs 3 10)            ;# 7 — sub2 10 3`},
		"curry":   {`def c (FnUtil.curry sub2/v) end def c10 (c 10) end (c10 3)  ;# 7 — one argument at a time`},
		"partial": {`def p (FnUtil.partial sub2/v 10) end (p 3)           ;# 7 — slot 0 bound to 10`},
		"on":      {`def bigger (FnUtil.on gt2/v sq/v) end (bigger 3 5)   ;# false — gt2 (sq 3) (sq 5)`},
		"memoize": {`def m (FnUtil.memoize slowfn/v) end (m 4) (m 4)      ;# second call answers from the cache`},
	})
}
