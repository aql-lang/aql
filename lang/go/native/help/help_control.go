package help

func init() {
	register(&Entry{
		Word:    "unpack",
		Summary: "Destructure entries of a map into local word bindings.",
		Description: "Extracts entries from a map (or record) and binds each to a bare word " +
			"in the current scope — AQL's analogue of JavaScript object destructuring. " +
			"Three selector forms over the same source: `unpack [names] map`, " +
			"`unpack all map`, and `unpack {renames} map`.",
		Notes: []string{
			"`unpack [a b] m` — bind the listed keys: a → m.a, b → m.b.",
			"`unpack all m` — bind every key of the source map.",
			"`unpack {a: x b: y} m` — rename: bind source key a to x, b to y.",
			"Map shorthand works: `unpack {a b} m` ≡ `{a: a b: b}` ≡ `unpack [a b] m`.",
			"A requested/renamed key absent from the source is an error (strict, like getr).",
			"Capitalised (type) names are rejected — unpack binds values only.",
			"Bindings obey scope: torn down at fn-body exit, persist at top level (like def).",
		},
	})

	register(&Entry{
		Word:    "codequote",
		Summary: "Quote a paren as code (structural quotability).",
		Description: "Like `quote`, but a forward parenthesised group is captured RAW — as " +
			"code (a paren-expression value) — instead of being evaluated. Words become " +
			"atoms and lists are kept unevaluated, exactly like `quote`; only the paren " +
			"handling differs.",
		Notes: []string{
			"`codequote (1 add 2)` — captures the paren as code (NOT the value 3).",
			"`quote (1 add 2)` — by contrast evaluates the paren, then quotes the result (3).",
			"`codequote foo` → Atom; `codequote [1 add 2]` → the list, unevaluated.",
			"For metaprogramming: capture a source expression as data to inspect or splice.",
		},
	})

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
		Word:    "raise",
		Summary: "Raise an error: construct an Ideal/Error and abort unless caught.",
		Description: "Raises the same Ideal/Error the runtime produces, so one " +
			"`do [body] error [handler]` form catches both native and user errors. " +
			"Three forms: `raise \"msg\"` (code user_error), `raise some_code \"msg\"` " +
			"(a bare word names the code), and `raise {code: c, message: m, …}` — " +
			"extra spec-map keys ride along on the Error value for the handler. " +
			"Handlers read `e.code` (atom), `e.message` (string), and any payload " +
			"keys; `convert Map e` projects the same fields.",
		Examples: []string{
			`raise "boom"                          ;# [aql/user_error]: boom`,
			`raise bad_input "expected a list"     ;# [aql/bad_input]: …`,
			`do [raise {code: nope/q, message: "m", got: 42}] error [var [[e] e.got print]]`,
		},
	})

	register(&Entry{
		Word:    "if",
		Summary: "Conditional execution.",
		Description: "If the condition is true, evaluates the then branch; with a third " +
			"argument, a false condition evaluates the else branch instead. Given a " +
			"single list `[c1 b1 c2 b2 … else]`, the even elements are conditions and " +
			"the following odd element is that clause's body; conditions are tried in " +
			"order and the first true one's body runs (a trailing element is the else).",
		Notes: []string{
			"A condition or branch that is a list is evaluated as code; a plain value is used as-is.",
			"In the clause-list form, conditions after the first match are not evaluated.",
		},
	})

	register(&Entry{
		Word:    "case",
		Summary: "Dispatch on a value: match/block pairs with an optional default.",
		Description: "`case <value> [m1 b1 m2 b2 \u2026 default]` evaluates the value (a " +
			"code-body list executes and its LAST result is captured) and walks the " +
			"clauses in order. A match that is a code-body list executes as if the " +
			"value were already on the stack ([gt 3] runs `v gt 3`) and coerces to " +
			"boolean; any other match UNIFIES with the value (equal scalars/atoms, a " +
			"type literal matches its members). The first matching block runs the " +
			"same way \u2014 value pushed first, like the error handler \u2014 so a block " +
			"list can consume it; a plain-value block is the result as-is. A trailing " +
			"odd element is the default. The stack-value form `v case [clauses]` " +
			"serves error handlers and pipelines.",
		Examples: []string{
			`case 2 [1 "one" 2 "two" "many"]            ;# 'two'`,
			`case 5 [[gt 3] "big" "small"]              ;# 'big'`,
			`case "x" [Integer "int" String "str"]      ;# 'str'`,
			`case 5 [Integer [mul 10] "other"]          ;# 50 — the block sees the value`,
			`do [risky] error [get code case [bad_input/q "rejected" "unexpected"]]`,
		},
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
			"Has forward arg collection, so both 'def name body' and 'body def name' work " +
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
		Word:    "args",
		Summary: "Push the current function's argument list.",
		Description: "Returns the list of arguments passed to the current fn-defined function. " +
			"Stack-only.",
	})
}
