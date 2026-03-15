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
			`[1 2 add] do                                => 3`,
			`[3 4 mul] do                                => 12`,
			`["hello" upper] do                          => 'HELLO'`,
			`[1 2 3] do                                  => 1 2 3`,
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
		Description: "If the boolean is true, evaluates the first list (then branch). " +
			"If false and a second list is provided, evaluates that instead (else branch).",
		Examples: []string{
			`true [1] if                                  => 1`,
			`false [1] [2] if                             => 2`,
			`true ["yes"] if                              => 'yes'`,
			`false ["yes"] ["no"] if                      => 'no'`,
			`(3 5 lt) [10] [20] if                        => 10`,
		},
	})

	register(&Entry{
		Word:    "for",
		Summary: "Iterate over a numeric range.",
		Signatures: []string{
			"[integer list] -> [any...]",
			"[list list] -> [any...]",
		},
		Description: "Iterates over a range and evaluates the body list for each step. " +
			"With an integer n, iterates from 0 to n-1. With a list, specifies " +
			"[end], [start end], or [start end step]. The loop variable i holds " +
			"the current index.",
		Examples: []string{
			`5 [i] for                                    => 0 1 2 3 4`,
			`3 [i 1 add] for                              => 1 2 3`,
			`[1 5] [i] for                                => 1 2 3 4`,
			`[0 10 3] [i] for                             => 0 3 6 9`,
			`4 [i i mul] for                              => 0 1 4 9`,
		},
		Notes: []string{
			"The loop variable is named i by default.",
			"Use break to exit early; use continue to skip to the next iteration.",
		},
	})

	register(&Entry{
		Word:    "break",
		Summary: "Exit the current for loop early.",
		Signatures: []string{"[] -> []"},
		Description: "Immediately terminates the innermost for loop. Prefix-only.",
		Examples: []string{
			`5 [(i 3 eq) [break] if i] for                => 0 1 2`,
			`10 [(i 2 eq) [break] if i] for               => 0 1`,
			`5 [(i 3 gt) [break] if i] for                => 0 1 2 3`,
			`3 [(i 0 eq) [break] if i] for                => (empty)`,
		},
	})

	register(&Entry{
		Word:    "continue",
		Summary: "Skip to the next iteration of the current for loop.",
		Signatures: []string{"[] -> []"},
		Description: "Skips the rest of the current loop body and continues with the next iteration. Prefix-only.",
		Examples: []string{
			`5 [(i 2 eq) [continue] if i] for             => 0 1 3 4`,
			`5 [(i 0 eq) [continue] if i] for             => 1 2 3 4`,
			`5 [(i 4 eq) [continue] if i] for             => 0 1 2 3`,
			`5 [(i 3 gt) [continue] if i i mul] for       => 0 1 4 9`,
		},
	})

	register(&Entry{
		Word:    "def",
		Summary: "Define a new word.",
		Signatures: []string{
			"[word any] -> []",
			"[string any] -> []",
		},
		Description: "Defines a new word with the given name and body. When the word is later " +
			"invoked, the body is evaluated. Definitions are stackable: multiple defs " +
			"for the same name stack, and undef pops the top definition. " +
			"Has suffix precedence, so both 'def name body' and 'body def name' work " +
			"via flexible argument matching.",
		Examples: []string{
			`def double [2 mul] 5 double                  => 10`,
			`def x 42 x                                  => 42`,
			`[3 mul] def triple 5 triple                  => 15`,
			`def greet ["hello"] greet                    => 'hello'`,
			`def a 1 def a 2 a                            => 2`,
			`def a 1 def a 2 a undef a a                  => 2 1`,
		},
		Notes: []string{
			"def accepts a word or string as the name.",
			"Use fn with def to define typed functions with parameters.",
			"Use undef to remove the most recent definition.",
		},
	})

	register(&Entry{
		Word:    "undef",
		Summary: "Remove the most recent definition of a word.",
		Signatures: []string{
			"[word] -> []",
			"[string] -> []",
		},
		Description: "Pops the most recent definition of the named word, restoring any previous " +
			"definition if one exists.",
		Examples: []string{
			`def x 1 def x 2 x undef x x                => 2 1`,
			`def foo [10] foo undef foo                   => 10`,
			`def a 1 a undef a                            => 1`,
			`def b 5 def b 10 b undef b b                => 10 5`,
		},
	})

	register(&Entry{
		Word:    "var",
		Summary: "Define scoped variables with automatic cleanup.",
		Signatures: []string{"[list] -> [any...]"},
		Description: "Takes a list whose first element is a list of variable declarations " +
			"and whose remaining elements are the body. Each declaration is either a bare " +
			"word (takes value from stack) or a [name value] list. Variables are automatically " +
			"undefined after the body executes.",
		Examples: []string{
			`5 var [[x] x x mul]                          => 25`,
			`var [[[x 5]] x x mul]                        => 25`,
			`3 4 var [[a b] a b add]                      => 7`,
			`var [[[x 2] [y 3]] x y mul]                  => 6`,
			`10 var [[n] n 1 add]                         => 11`,
		},
		Notes: []string{
			"Variables are scoped: they are undefined when the body finishes.",
			"Bare word declarations consume values from the stack.",
		},
	})

	register(&Entry{
		Word:    "fn",
		Summary: "Create a function value with typed parameters.",
		Signatures: []string{"[list] -> [function]"},
		Description: "Parses a list of signature triples [input-types output-types body] " +
			"into a function value. Usually used with def to bind the function to a name. " +
			"Parameters can be named ({x: Number}) or unnamed. Multiple signatures " +
			"(overloads) are supported by providing additional triples in the same list.",
		Examples: []string{
			// Basic usage with def
			`def square fn [[{x: Number}] [Number] [x x mul]]` +
				"\n  5 square                                   => 25",
			`def double fn [[{n: Number}] [Number] [n 2 mul]]` +
				"\n  7 double                                   => 14",
			// Multiple overloads
			`def inc fn [` +
				"\n    [{n: Integer}] [Integer] [n 1 add]" +
				"\n    [String] [String] [\"!\" add]" +
				"\n  ]" +
				"\n  3 inc                                      => 4" +
				"\n  \"hi\" inc                                   => 'hi!'",
			// Unnamed parameters
			`def add3 fn [[Number Number Number] [Number] [add add]]` +
				"\n  1 2 3 add3                                 => 6",
			// Recursive function
			"def fact fn [" +
				"\n    [{n: 0}] [Integer] [1]" +
				"\n    [{n: Integer}] [Integer] [n n 1 sub fact mul]" +
				"\n  ]" +
				"\n  5 fact                                     => 120",
			// Lambda with def
			`fn [[{x: Number}] [Number] [x 10 add]] def offset` +
				"\n  5 offset                                   => 15",
		},
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
		Signatures: []string{"[list] -> [any...]"},
		Description: "Takes a list and evaluates its contents as AQL code on the current stack. " +
			"Useful for invoking callback lists in higher-order patterns.",
		Examples: []string{
			`5 [dup mul] call                             => 25`,
			`2 3 [add] call                               => 5`,
			`"hello" [upper] call                         => 'HELLO'`,
			`1 2 [add 10 mul] call                        => 30`,
		},
	})

	register(&Entry{
		Word:    "args",
		Summary: "Push the current function's argument list.",
		Signatures: []string{"[] -> [list]"},
		Description: "Returns the list of arguments passed to the current fn-defined function. " +
			"Prefix-only.",
		Examples: []string{
			`def show fn [[{x: Number}] [] [args]] 42 show  => [42]`,
			`def f fn [[{a: Number} {b: Number}] [] [args]]` +
				"\n  1 2 f                                      => [1,2]",
			`def g fn [[{x: Number} {y: Number}] [] [args]]` +
				"\n  10 20 g                                    => [10,20]",
			`def h fn [[{a: Number} {b: Number}] [Number] [a b add]]` +
				"\n  3 4 h                                      => 7",
		},
	})
}
