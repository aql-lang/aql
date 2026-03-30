package help

func init() {
	register(&Entry{
		Word:    "convert",
		Summary: "Convert a value to a different type.",
		Signatures: []string{
			"[any type] -> [any]",
			"[any type any] -> [any]",
		},
		Description: "Converts the first argument to the target type. Supports integer, decimal, " +
			"string, boolean conversions. An optional third argument provides settings " +
			"like base for numeric conversions.",
		Examples: []string{
			`"42" Integer convert           => 42`,
			`42 String convert              => '42'`,
			`"3.14" Decimal convert         => 3.14`,
			`1 Boolean convert              => true`,
		},
	})

	register(&Entry{
		Word:    "typeof",
		Summary: "Return the type name of a value.",
		Signatures: []string{"[any] -> [atom]"},
		Description: "Consumes the top value and pushes its type name as an atom.",
		Examples: []string{
			`42 typeof                      => Number`,
			`"hello" typeof                 => String`,
			`true typeof                    => Boolean`,
			`[1 2 3] typeof                 => List`,
		},
	})

	register(&Entry{
		Word:    "inspect",
		Summary: "Return a detailed map describing a registered word.",
		Signatures: []string{"[word] -> [map]"},
		Description: "Returns a map with name, kind, forward_precedence, and signatures for " +
			"the named word. Useful for introspecting built-in and user-defined words.",
		Examples: []string{
			`add inspect .name              => 'add'`,
			`add inspect .kind              => builtin`,
			`sub inspect .kind              => builtin`,
			`upper inspect .kind            => builtin`,
		},
	})

	register(&Entry{
		Word:    "record",
		Summary: "Create a record type from a list of field definitions.",
		Signatures: []string{"[list] -> [record-type]"},
		Description: "Creates a record type schema from a list of {name: type} maps. " +
			"Record types are used to define table schemas.",
		Examples: []string{
			`[{name: String} {age: Integer}] record`,
			`[{id: Integer} {value: String}] record`,
			`[{x: Decimal} {y: Decimal}] record`,
			`[{flag: Boolean}] record`,
		},
	})

	register(&Entry{
		Word:    "table",
		Summary: "Create a table type from a record type.",
		Signatures: []string{"[record-type] -> [table-type]"},
		Description: "Creates a table type from a record type definition. Tables hold rows " +
			"matching the record schema.",
		Examples: []string{
			`[{name: String} {age: Integer}] record table`,
			`[{id: Integer} {value: String}] record table`,
			`[{x: Decimal} {y: Decimal}] record table`,
			`[{flag: Boolean} {label: String}] record table`,
		},
	})

	register(&Entry{
		Word:    "make",
		Summary: "Create a value conforming to a type.",
		Signatures: []string{
			"[type any] -> [any]",
			"[type any map] -> [any]",
		},
		Description: "Constructs a value of the given type from the provided data. " +
			"For tables, creates table rows from list data.",
		Examples: []string{
			`[{name: String}] record table [{name: "Alice"} {name: "Bob"}] make`,
			`Integer "42" make              => 42`,
			`String 42 make                 => '42'`,
			`Boolean 1 make                 => true`,
		},
	})

	register(&Entry{
		Word:    "type",
		Summary: "Define a named type.",
		Signatures: []string{"[atom any] -> []"},
		Description: "Registers a named type definition for later use.",
		Examples: []string{
			`type Age Integer`,
			`type Name String`,
			`type Point [{x: Decimal} {y: Decimal}] record`,
			`type ID Integer`,
		},
	})

	register(&Entry{
		Word:    "base",
		Summary: "Return the zero/default value for the type of a value.",
		Signatures: []string{
			"[any] -> [any]",
		},
		Description: "Consumes a value and returns the zero value for its type: 0 for integers, " +
			"0.0 for decimals, empty string for strings, false for booleans, empty list for lists.",
		Examples: []string{
			`42 base                      => 0`,
			`"hello" base                 => ''`,
			`true base                    => false`,
			`[1 2 3] base                 => []`,
		},
	})
}
