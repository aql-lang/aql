package help

func init() {
	register(&Entry{
		Word:    "do",
		Summary: "Evaluate a list or map as code.",
		Description: "Evaluates the elements of a list as AQL code. For maps, recursively " +
			"evaluates all values. Used to execute deferred expressions.",
		Notes: []string{
			"Typed lists, typed maps, and record types are not evaluated.",
		},
	})

	register(&Entry{
		Word:    "if",
		Summary: "Conditional execution.",
		Description: "If the boolean is true, evaluates the first list (then branch). " +
			"If false and a second list is provided, evaluates that instead (else branch).",
	})

	register(&Entry{
		Word:    "for",
		Summary: "Iterate over a numeric range.",
		Description: "Iterates over a range and evaluates the body list for each step. " +
			"With an integer n, iterates from 0 to n-1. With a list, specifies " +
			"[end], [start end], or [start end step]. The loop variable i holds " +
			"the current index.",
		Notes: []string{
			"The loop variable is named i by default.",
			"Use break to exit early; use continue to skip to the next iteration.",
		},
	})

	register(&Entry{
		Word:        "break",
		Summary:     "Exit the current for loop early.",
		Description: "Immediately terminates the innermost for loop. Stack-only.",
	})

	register(&Entry{
		Word:        "continue",
		Summary:     "Skip to the next iteration of the current for loop.",
		Description: "Skips the rest of the current loop body and continues with the next iteration. Stack-only.",
	})

	register(&Entry{
		Word:    "def",
		Summary: "Define a new word.",
		Description: "Defines a new word with the given name and body. When the word is later " +
			"invoked, the body is evaluated. Definitions are stackable: multiple defs " +
			"for the same name stack, and undef pops the top definition. " +
			"Has forward precedence, so both 'def name body' and 'body def name' work " +
			"via flexible argument matching.",
		Notes: []string{
			"def accepts a word or string as the name.",
			"Use fn with def to define typed functions with parameters.",
			"Use undef to remove the most recent definition.",
		},
	})

	register(&Entry{
		Word:    "undef",
		Summary: "Remove the most recent definition of a word.",
		Description: "Pops the most recent definition of the named word, restoring any previous " +
			"definition if one exists.",
	})

	register(&Entry{
		Word:    "var",
		Summary: "Define scoped variables with automatic cleanup.",
		Description: "Takes a list whose first element is a list of variable declarations " +
			"and whose remaining elements are the body. Each declaration is either a bare " +
			"word (takes value from stack) or a [name value] list. Variables are automatically " +
			"undefined after the body executes.",
		Notes: []string{
			"Variables are scoped: they are undefined when the body finishes.",
			"Bare word declarations consume values from the stack.",
		},
	})

	register(&Entry{
		Word:    "fn",
		Summary: "Create a function value with typed parameters.",
		Description: "Parses a list of signature triples [input-types output-types body] " +
			"into a function value. Usually used with def to bind the function to a name. " +
			"Parameters can be named ({x: Number}) or unnamed. Multiple signatures " +
			"(overloads) are supported by providing additional triples in the same list.",
		Notes: []string{
			"fn takes a single list argument. The list length must be divisible by 3.",
			"Each triple is: [input-params] [output-types] [body].",
			"Named params use map syntax: {name: Type}. Unnamed params are bare types.",
			"Literal values (like 0) can be used as type constraints for pattern matching.",
			"Use with def to bind: def name fn [...] or fn [...] def name.",
		},
	})

	register(&Entry{
		Word:    "call",
		Summary: "Evaluate a list as code.",
		Description: "Takes a list and evaluates its contents as AQL code on the current stack. " +
			"Useful for invoking callback lists in higher-order patterns.",
	})

	register(&Entry{
		Word:    "args",
		Summary: "Push the current function's argument list.",
		Description: "Returns the list of arguments passed to the current fn-defined function. " +
			"Stack-only.",
	})
}
