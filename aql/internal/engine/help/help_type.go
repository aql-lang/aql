package help

func init() {
	register(&Entry{
		Word:    "convert",
		Summary: "Convert a value to a different type.",
		Description: "Converts the first argument to the target type. Supports integer, decimal, " +
			"string, boolean conversions. An optional third argument provides settings " +
			"like base for numeric conversions.",
	})

	register(&Entry{
		Word:    "typeof",
		Summary: "Return the type name of a value.",
		Description: "Consumes the top value and pushes its type name as an atom.",
	})

	register(&Entry{
		Word:    "inspect",
		Summary: "Return a detailed map describing a registered word.",
		Description: "Returns a map with name, kind, forward_precedence, and signatures for " +
			"the named word. Useful for introspecting built-in and user-defined words.",
	})

	register(&Entry{
		Word:    "record",
		Summary: "Create a record type from a list of field definitions.",
		Description: "Creates a record type schema from a list of {name: type} maps. " +
			"Record types are used to define table schemas.",
	})

	register(&Entry{
		Word:    "table",
		Summary: "Create a table type from a record type.",
		Description: "Creates a table type from a record type definition. Tables hold rows " +
			"matching the record schema.",
	})

	register(&Entry{
		Word:    "make",
		Summary: "Create a value conforming to a type.",
		Description: "Constructs a value of the given type from the provided data. " +
			"For tables, creates table rows from list data.",
	})

	register(&Entry{
		Word:    "type",
		Summary: "Define a named type.",
		Description: "Registers a named type definition for later use.",
	})

	register(&Entry{
		Word:    "base",
		Summary: "Return the zero/default value for the type of a value.",
		Description: "Consumes a value and returns the zero value for its type: 0 for integers, " +
			"0.0 for decimals, empty string for strings, false for booleans, empty list for lists.",
	})

	register(&Entry{
		Word:    "tor",
		Summary: "Construct a disjunct (union) type from two values.",
		Description: "Returns a disjunct that matches either alternative. " +
			"Flattens nested disjuncts and applies carrier widening. " +
			"Use to build optional fields and union type literals " +
			"(e.g. `string tor none`).",
	})
}
