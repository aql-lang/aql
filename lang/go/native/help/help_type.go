package help

func init() {
	register(&Entry{
		Word:    "convert",
		Summary: "Convert a value to a different type.",
		Description: "Converts the first argument to the target type. Supports integer, float, " +
			"string, boolean conversions. An optional third argument provides settings " +
			"like base for numeric conversions.",
	})

	register(&Entry{
		Word:        "typeof",
		Summary:     "Return the type name of a value.",
		Description: "Consumes the top value and pushes its type name as an atom.",
	})

	register(&Entry{
		Word:    "inspect",
		Summary: "Return a detailed map describing a word, value, or type.",
		Description: "For a word: a map with name, kind, and signatures. For a value or " +
			"type: a map with type ('Type' for a type value, otherwise the value's " +
			"type leaf), struct (the underlying-structure leaf, for types), kind, and " +
			"kind-specific fields (fields / alternatives / child / signatures / …).",
	})

	register(&Entry{
		Word:    "make",
		Summary: "Create a value conforming to a type.",
		Description: "Constructs a value of the given type from the provided data. " +
			"For tables, creates table rows from list data.",
		Examples: []string{
			`make Point {}          ;# fresh instance, all fields at their defaults`,
			`make Point {x: 3 y: 4} ;# instance with field overrides`,
			`(make Counter {}).count ;# parenthesise to read a field off a fresh make`,
		},
	})

	register(&Entry{
		Word:    "refine",
		Summary: "Construct a (sub)type from a base type.",
		Description: "Builds a (sub)type: `refine ParentClass {fields}` a subclass " +
			"(classes are defined with `class {fields}`), `refine Record [pairs]` " +
			"a record type, `refine Table recordtype` a table type, `refine List` " +
			"a bare nominal subtype. Pair with `def Name (refine …)` to bind it.",
	})

	register(&Entry{
		Word:    "class",
		Summary: "Define a class: a sealed nominal record type.",
		Description: "`def Foo class {schema}` mints a class type under Ideal/Class. " +
			"Schema entries: a TYPE value ({name:String}) declares a REQUIRED field; a " +
			"CONCRETE value ({retries:3}) declares a default, typed by the value itself. " +
			"Instances (`make Foo {\u2026}`) are flat \u2014 every field, own and inherited, " +
			"resolves at make \u2014 and sealed: writing an undeclared field raises " +
			"sealed_field. Subclass with refine: `def Bar refine Foo {\u2026}`. Class " +
			"instances are NOT Objects \u2014 classes and the open keyed container are " +
			"separate branches.",
		Examples: []string{
			`def Point class {x:1, y:2}     ;# x,y default to 1,2`,
			`def p (make Point {x:9})       ;# p.x => 9, p.y => 2`,
			`def Point3 refine Point {z:3}  ;# subclass, inherits x,y`,
			`p set w 5                      ;# ERROR sealed_field`,
		},
	})

	register(&Entry{
		Word:    "const",
		Summary: "Make a singleton type: a value whose TYPE has one inhabitant.",
		Description: "`const v` mints an interned singleton type under v's own type and " +
			"returns v tagged with it — `typeof (const 1)` renders as `1`. In a class " +
			"schema, `{kind:(const 'point')}` declares a field that can only ever hold " +
			"'point': the default is forced, the exact value is accepted at make/set, " +
			"anything else is a loud type error. Membership is same-base strict (1.0 is " +
			"not a member of (const 1)). Exemplars must be immutable (scalars, Lists, " +
			"Maps); NaN is rejected. With tor unions this gives discriminated records: " +
			"def Circle class {kind:(const 'circle'), r:0.0}.",
		Examples: []string{
			`def S class {kind:(const 'point'), x:1}`,
			`make S {}                     ;# kind='point' — forced default`,
			`make S {kind:'other'}         ;# ERROR — only 'point' inhabits the type`,
		},
	})

	register(&Entry{
		Word:    "base",
		Summary: "Return the zero/default value for the type of a value.",
		Description: "Consumes a value and returns the zero value for its type: 0 for integers, " +
			"0.0 for floats, empty string for strings, false for booleans, empty list for lists.",
	})

	register(&Entry{
		Word:    "tor",
		Summary: "Construct a disjunct (union) type from two values.",
		Description: "Returns a disjunct that matches either alternative. " +
			"Flattens nested disjuncts and applies carrier widening. " +
			"Use to build optional fields and union type literals " +
			"(e.g. `string tor none`).",
	})

	register(&Entry{
		Word:    "tand",
		Summary: "Combine two values by conjunction.",
		Description: "For two concrete maps, merges keys (unifying values for " +
			"keys present in both). For other shapes, returns the unification " +
			"of the two arguments. Errors if the values cannot be combined " +
			"(e.g. conflicting concrete values for the same key).",
	})

	register(&Entry{
		Word:    "tany",
		Summary: "Apply `tor` across a list, building a flattened disjunct.",
		Description: "Folds the list with `tor` semantics: every element becomes " +
			"an alternative of the resulting disjunct. Existing disjunct elements " +
			"are flattened. A single-element list returns that element unchanged. " +
			"Errors on an empty list.",
	})

	register(&Entry{
		Word:    "tall",
		Summary: "Apply `tand` across a list, folding via map-merge / unify.",
		Description: "Folds the list with `tand` semantics: concrete maps are " +
			"merged key-by-key (overlapping keys are unified); other shapes are " +
			"unified pairwise. A single-element list returns that element " +
			"unchanged. Errors on an empty list or unifiable failure.",
	})

	register(&Entry{
		Word:    "teq",
		Summary: "Strict type equality (lattice node identity, not subtype).",
		Description: "Returns true iff both args are types AND refer to the same " +
			"lattice node. Distinct from `is`, which is subtype membership: " +
			"`Integer is Number` is true, `Integer teq Number` is false. " +
			"Non-type arguments return false on either side.",
	})

	register(&Entry{
		Word:    "tpartial",
		Summary: "Wrap every field of a Record or Object type in `T | None`.",
		Description: "Returns a new type where each field's value type is " +
			"replaced with the disjunct of itself and None. Idempotent — a " +
			"field already including None is unchanged. For Object types, " +
			"inherited fields are flattened into the result.",
	})
}
