package modules

func init() {
	registerDocs("aql:type-util", map[string]string{
		"alts":      "The alternatives of a disjunct as a List of types.",
		"arityof":   "Count of a function type's required parameters.",
		"brand":     "Mint a nominal subtype of a base type tagged by an atom.",
		"exclude":   "Remove a type (and its subtypes) from a disjunct.",
		"extract":   "Keep only the alternatives matching the given type.",
		"lca":       "Least common ancestor (supertype) of two types.",
		"merge":     "Combine two record/object types, unifying overlaps.",
		"nominal":   "Mint an anonymous nominal alias of a base type.",
		"omit":      "Drop the named fields from a record/object type.",
		"paramsof":  "A function type's parameter types as a List.",
		"parent":    "The immediate supertype in the type lattice.",
		"pick":      "Keep only the named fields of a record/object type.",
		"required":  "Strip the optional (None) alternative from each field.",
		"returnsof": "A function type's return type.",
		"root":      "The branch root (top supertype) in the lattice.",
		"tpartial":  "Make every field of a record/object type optional.",
	})

	// These words take TYPES as values, so the generated permutations
	// (which substitute data) cannot demonstrate any of them. Results are
	// from verified lang/spec/module-type.tsv rows.
	registerExamples("aql:type-util", map[string][]string{
		"parent": {`TypeUtil.parent Integer                          ;# Number — one step up the lattice`},
		"root":   {`TypeUtil.root Integer                            ;# Scalar — the top of its family`},
		"lca":    {`TypeUtil.lca Integer Float                       ;# Number — least common ancestor`},
		"alts":   {`TypeUtil.alts (Integer tor String)               ;# [Integer String] — the branches of a disjunct`},
		"arityof": {
			`TypeUtil.arityof (fn [[a:Integer b:Integer] [Integer] [a add b]])   ;# 2`,
		},
		"paramsof": {
			`TypeUtil.paramsof (fn [[a:Integer b:String] [Integer] [a]])         ;# [Integer String]`,
		},
		"returnsof": {
			`TypeUtil.returnsof (fn [[a:Integer] [Integer] [a]])                 ;# Integer`,
		},
		"exclude": {
			`(Integer tor String) TypeUtil.exclude String     ;# Integer — drop a branch`,
		},
		"extract": {
			`(Integer tor String) TypeUtil.extract Integer    ;# Integer — keep only the matching branch`,
		},
		"pick": {
			`(refine Record [x:Integer y:String]) TypeUtil.pick [x/q]    ;# record{x:Integer}`,
			`;# The field names are ATOMS (x/q), not Strings.`,
		},
		"omit": {
			`(refine Record [x:Integer y:String]) TypeUtil.omit [x/q]    ;# record{y:String}`,
		},
		"merge": {
			`(refine Record [x:Integer]) TypeUtil.merge (refine Record [y:String])`,
			`;# record{x:Integer y:String}`,
		},
		"required": {
			`TypeUtil.required (refine Record [x:(Integer tor None)])    ;# record{x:Integer}`,
			`;# Strips the None branch off every optional field.`,
		},
		"brand": {
			`TypeUtil.brand userid/q Integer                  ;# brand:userid — a nominal Integer`,
			`;# Two brands over the same base do NOT unify; that is the point.`,
		},
		"nominal": {
			`(TypeUtil.nominal Integer) istype                ;# true — a fresh identity over the same shape`,
		},
		"tpartial": {
			`(TypeUtil.tpartial (refine Record [x:Integer])) istype      ;# true — every field made optional`,
		},
	})
}
