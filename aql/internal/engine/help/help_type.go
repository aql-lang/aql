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
			`"42" Integer convert   => 42`,
			`42 String convert      => "42"`,
		},
	})

	register(&Entry{
		Word:    "typeof",
		Summary: "Push the type of a value.",
		Signatures: []string{"[any] -> [any type]"},
		Description: "Returns the type of the top value as a type value, without consuming it.",
		Examples: []string{
			`42 typeof   => 42 Number/Integer`,
		},
	})

	register(&Entry{
		Word:    "inspect",
		Summary: "Return a detailed map describing a value.",
		Signatures: []string{"[any] -> [map]"},
		Description: "Returns a map with type information and metadata about the value.",
		Examples: []string{`42 inspect`},
	})

	register(&Entry{
		Word:    "record",
		Summary: "Create a record type from a list of field definitions.",
		Signatures: []string{"[list] -> [record-type]"},
		Description: "Creates a record type schema from a list of {name: type} maps. " +
			"Record types are used to define table schemas.",
		Examples: []string{
			`[{name: String} {age: Integer}] record`,
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
			`Integer 42 make`,
		},
	})

	register(&Entry{
		Word:    "type",
		Summary: "Define a named type.",
		Signatures: []string{"[atom any] -> []"},
		Description: "Registers a named type definition for later use.",
		Examples: []string{
			`Age Integer type`,
		},
	})

	register(&Entry{
		Word:    "base",
		Summary: "Convert an integer to/from a base representation.",
		Signatures: []string{
			"[integer string] -> [string]",
			"[string integer] -> [integer]",
		},
		Description: "Converts between integers and their string representations in different bases.",
		Examples: []string{
			`255 "hex" base   => "ff"`,
		},
	})
}
