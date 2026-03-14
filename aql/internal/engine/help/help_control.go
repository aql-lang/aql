package help

func init() {
	register(&Entry{
		Word:    "do",
		Summary: "Evaluate a list or map as code.",
		Signatures: []string{
			"[list] -> [any...]",
			"[map] -> [map]",
		},
		Description: "Evaluates the elements of a list as AQL code. For maps, recursively " +
			"evaluates all values. Used to execute deferred expressions.",
		Examples: []string{
			`[1 2 add] do          => 3`,
		},
		Notes: []string{
			"Typed lists, typed maps, and record types are not evaluated.",
		},
	})

	register(&Entry{
		Word:    "if",
		Summary: "Conditional execution.",
		Signatures: []string{
			"[boolean list] -> [any...]",
			"[boolean list list] -> [any...]",
		},
		Description: "If the boolean is true, evaluates the first list. If false and a second " +
			"list is provided, evaluates that instead (else branch).",
		Examples: []string{
			`true [1] if                => 1`,
			`false [1] [2] if           => 2`,
		},
	})

	register(&Entry{
		Word:    "for",
		Summary: "Loop over a list or range.",
		Signatures: []string{
			"[list list] -> [any...]",
			"[integer list] -> [any...]",
		},
		Description: "Iterates over the first argument (list of items, or integer count) " +
			"and evaluates the body list for each iteration. The current item is available via context.",
		Examples: []string{
			`[1 2 3] [dup add] for    => 2 4 6`,
		},
	})

	register(&Entry{
		Word:    "break",
		Summary: "Exit the current for loop early.",
		Signatures: []string{"[] -> []"},
		Description: "Immediately terminates the innermost for loop.",
		Examples: []string{`[1 2 3] [dup 2 eq [break] if] for`},
	})

	register(&Entry{
		Word:    "continue",
		Summary: "Skip to the next iteration of the current for loop.",
		Signatures: []string{"[] -> []"},
		Description: "Skips the rest of the current loop body and continues with the next iteration.",
		Examples: []string{`[1 2 3] [dup 2 eq [continue] if print] for`},
	})

	register(&Entry{
		Word:    "def",
		Summary: "Define a new word.",
		Signatures: []string{
			"[atom list] -> []",
			"[atom any] -> []",
		},
		Description: "Defines a new word with the given name and body. When the word is later " +
			"invoked, the body is evaluated. Definitions are stackable (can be overridden " +
			"and restored with undef).",
		Examples: []string{
			`double [2 mul] def   => (defines 'double')`,
			`5 double             => 10`,
		},
	})

	register(&Entry{
		Word:    "undef",
		Summary: "Remove the most recent definition of a word.",
		Signatures: []string{
			"[atom] -> []",
			"[string] -> []",
		},
		Description: "Pops the most recent definition of the named word, restoring any previous " +
			"definition if one exists.",
		Examples: []string{
			`myword undef`,
		},
	})

	register(&Entry{
		Word:    "var",
		Summary: "Define a named variable.",
		Signatures: []string{"[atom] -> []"},
		Description: "Creates a variable with the given name. The value on the stack " +
			"is stored. Use the variable name to recall its value.",
		Examples: []string{
			`42 x var   => (stores 42 in x)`,
		},
	})

	register(&Entry{
		Word:    "fn",
		Summary: "Define a named function with explicit argument handling.",
		Signatures: []string{"[list list] -> []"},
		Description: "Defines a function with a parameter list and body. Parameters are " +
			"bound from the stack when the function is called.",
		Examples: []string{
			`fn double [x] [x 2 mul]`,
		},
	})

	register(&Entry{
		Word:    "call",
		Summary: "Call a word by name.",
		Signatures: []string{"[atom] -> [any...]"},
		Description: "Looks up and invokes the word with the given name.",
		Examples: []string{`add call  (same as: add)`},
	})

	register(&Entry{
		Word:    "args",
		Summary: "Push the current function's argument list.",
		Signatures: []string{"[] -> [list]"},
		Description: "Returns the list of arguments passed to the current fn call.",
		Examples: []string{`fn show [] [args print]`},
	})
}
