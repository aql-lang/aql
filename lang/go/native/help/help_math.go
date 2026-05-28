package help

func init() {
	// --- Built-in arithmetic (always available) ---

	register(&Entry{
		Word:    "add",
		Summary: "Add two numbers, or concatenate two scalars as strings.",
		Description: "Adds two numeric values. When both are integers the result is an integer; " +
			"if either is a decimal the result is a decimal. For non-numeric scalars, " +
			"concatenates their string representations.",
	})

	register(&Entry{
		Word:    "sub",
		Summary: "Subtract: a sub b ≡ a - b.",
		Description: "All three call forms `a b sub`, `a sub b`, and `sub b a` " +
			"compute a - b. The handler returns args[1] - args[0]; under the " +
			"argument-order rule args[0] is the rightmost source-position arg.",
	})

	register(&Entry{
		Word:        "mul",
		Summary:     "Multiply two numbers.",
		Description: "Multiplies two numeric values (commutative).",
	})

	register(&Entry{
		Word:    "div",
		Summary: "Divide: a div b ≡ a / b.",
		Description: "All three call forms `a b div`, `a div b`, and `div b a` " +
			"compute a / b. Integer division truncates toward zero.",
		Notes: []string{
			"Division by zero produces an error.",
		},
	})

	register(&Entry{
		Word:    "mod",
		Summary: "Remainder: a mod b ≡ a %% b.",
		Description: "All three call forms `a b mod`, `a mod b`, and `mod b a` " +
			"compute a %% b (the remainder of integer division).",
	})

	register(&Entry{
		Word:    "pow",
		Summary: "Power: a pow b ≡ a^b.",
		Description: "All three call forms `a b pow`, `a pow b`, and `pow b a` " +
			"compute a^b.",
		Notes: []string{
			"Negative exponents produce an error for integer pow.",
		},
	})

	// --- aql:math native module (requires: \"aql:math\" import) ---

	register(&Entry{
		Word:        "abs",
		Summary:     "Return the absolute value of a number.",
		Description: "Returns the absolute (non-negative) value.",
		Notes:       []string{"Requires: \"aql:math\" import"},
	})

	register(&Entry{
		Word:        "negate",
		Summary:     "Negate a number (flip the sign).",
		Description: "Returns -n for input n.",
		Notes:       []string{"Requires: \"aql:math\" import"},
	})

	register(&Entry{
		Word:        "min",
		Summary:     "Return the smaller of two numbers.",
		Description: "Returns the minimum of two values.",
		Notes:       []string{"Requires: \"aql:math\" import"},
	})

	register(&Entry{
		Word:        "max",
		Summary:     "Return the larger of two numbers.",
		Description: "Returns the maximum of two values.",
		Notes:       []string{"Requires: \"aql:math\" import"},
	})

	register(&Entry{
		Word:        "sign",
		Summary:     "Return the sign of a number (-1, 0, or 1).",
		Description: "Returns -1 for negative, 0 for zero, 1 for positive.",
		Notes:       []string{"Requires: \"aql:math\" import"},
	})

	register(&Entry{
		Word:        "ceil",
		Summary:     "Round a decimal up to the nearest integer.",
		Description: "Returns the smallest integer value greater than or equal to the input.",
		Notes:       []string{"Requires: \"aql:math\" import"},
	})

	register(&Entry{
		Word:        "floor",
		Summary:     "Round a decimal down to the nearest integer.",
		Description: "Returns the largest integer value less than or equal to the input.",
		Notes:       []string{"Requires: \"aql:math\" import"},
	})

	register(&Entry{
		Word:        "round",
		Summary:     "Round a decimal to the nearest integer.",
		Description: "Rounds to the nearest integer; ties round away from zero.",
		Notes: []string{
			"Uses Go's math.Round: 0.5 rounds away from zero.",
			"Requires: \"aql:math\" import",
		},
	})

	register(&Entry{
		Word:        "trunc",
		Summary:     "Truncate a decimal toward zero.",
		Description: "Removes the fractional part, rounding toward zero.",
		Notes:       []string{"Requires: \"aql:math\" import"},
	})

	register(&Entry{
		Word:        "sqrt",
		Summary:     "Compute the square root.",
		Description: "Returns the square root of the input.",
		Notes:       []string{"Requires: \"aql:math\" import"},
	})

	register(&Entry{
		Word:        "cbrt",
		Summary:     "Compute the cube root.",
		Description: "Returns the cube root of the input.",
		Notes:       []string{"Requires: \"aql:math\" import"},
	})

	register(&Entry{
		Word:        "exp",
		Summary:     "Compute e raised to a power.",
		Description: "Returns e^x where e is Euler's number.",
		Notes:       []string{"Requires: \"aql:math\" import"},
	})

	register(&Entry{
		Word:        "log",
		Summary:     "Compute the natural logarithm.",
		Description: "Returns the natural logarithm (base e) of the input.",
		Notes:       []string{"Requires: \"aql:math\" import"},
	})

	register(&Entry{
		Word:        "log2",
		Summary:     "Compute the base-2 logarithm.",
		Description: "Returns the base-2 logarithm of the input.",
		Notes:       []string{"Requires: \"aql:math\" import"},
	})

	register(&Entry{
		Word:        "log10",
		Summary:     "Compute the base-10 logarithm.",
		Description: "Returns the base-10 logarithm of the input.",
		Notes:       []string{"Requires: \"aql:math\" import"},
	})

	register(&Entry{
		Word:        "sin",
		Summary:     "Compute the sine (input in radians).",
		Description: "Returns the sine of the input angle in radians.",
		Notes:       []string{"Requires: \"aql:math\" import"},
	})

	register(&Entry{
		Word:        "cos",
		Summary:     "Compute the cosine (input in radians).",
		Description: "Returns the cosine of the input angle in radians.",
		Notes:       []string{"Requires: \"aql:math\" import"},
	})

	register(&Entry{
		Word:        "tan",
		Summary:     "Compute the tangent (input in radians).",
		Description: "Returns the tangent of the input angle in radians.",
		Notes:       []string{"Requires: \"aql:math\" import"},
	})

	register(&Entry{
		Word:        "asin",
		Summary:     "Compute the arc sine (result in radians).",
		Description: "Returns the arc sine of the input. Input must be in [-1, 1].",
		Notes:       []string{"Requires: \"aql:math\" import"},
	})

	register(&Entry{
		Word:        "acos",
		Summary:     "Compute the arc cosine (result in radians).",
		Description: "Returns the arc cosine of the input. Input must be in [-1, 1].",
		Notes:       []string{"Requires: \"aql:math\" import"},
	})

	register(&Entry{
		Word:        "atan",
		Summary:     "Compute the arc tangent (result in radians).",
		Description: "Returns the arc tangent of the input.",
		Notes:       []string{"Requires: \"aql:math\" import"},
	})

	register(&Entry{
		Word:        "atan2",
		Summary:     "Compute the two-argument arc tangent.",
		Description: "Returns atan2(y, x): y x atan2. Handles quadrant correctly.",
		Notes:       []string{"Requires: \"aql:math\" import"},
	})

	register(&Entry{
		Word:        "hypot",
		Summary:     "Compute the hypotenuse length.",
		Description: "Returns sqrt(x*x + y*y) without overflow: x y hypot.",
		Notes:       []string{"Requires: \"aql:math\" import"},
	})

	register(&Entry{
		Word:        "math-pi",
		Summary:     "Push the constant pi onto the stack.",
		Description: "Pushes the mathematical constant pi (3.14159...). Stack-only.",
		Notes:       []string{"Requires: \"aql:math\" import"},
	})

	register(&Entry{
		Word:        "math-e",
		Summary:     "Push Euler's number e onto the stack.",
		Description: "Pushes the mathematical constant e (2.71828...). Stack-only.",
		Notes:       []string{"Requires: \"aql:math\" import"},
	})
}
