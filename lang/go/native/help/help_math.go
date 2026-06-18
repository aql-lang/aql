package help

func init() {
	// --- Built-in arithmetic (always available) ---

	register(&Entry{
		Word:    "add",
		Summary: "Add two numbers, or concatenate when at least one operand is a String.",
		Description: "Adds two numeric values. When both are integers the result is an integer; " +
			"if either is a float the result is a float. When at least one operand is a " +
			"String, the other Scalar is coerced to its string form and the two are " +
			"concatenated. Two non-String scalars (e.g. a Boolean and a Number) match " +
			"neither overload and raise a type error rather than silently stringifying.",
		Notes: []string{
			"Integer overflow raises [aql/integer_overflow]; use a Float operand for an approximate result.",
			"Concatenation needs a String: `add 1 \"x\"` → 'x1', but `add true 1` is a type error.",
		},
	})

	register(&Entry{
		Word:    "sub",
		Summary: "Subtract: a sub b ≡ a - b.",
		Description: "All three call forms `a b sub`, `a sub b`, and `sub b a` " +
			"compute a - b. The handler returns args[1] - args[0]; under the " +
			"argument-order rule args[0] is the rightmost source-position arg.",
		Notes: []string{
			"Integer overflow raises [aql/integer_overflow]; use a Float operand for an approximate result.",
		},
	})

	register(&Entry{
		Word:        "mul",
		Summary:     "Multiply two numbers.",
		Description: "Multiplies two numeric values (commutative).",
		Notes: []string{
			"Integer overflow raises [aql/integer_overflow]; use a Float operand for an approximate result.",
		},
	})

	register(&Entry{
		Word:    "div",
		Summary: "Divide: a div b ≡ a / b.",
		Description: "All three call forms `a b div`, `a div b`, and `div b a` " +
			"compute a / b. Integer division truncates toward zero.",
		Notes: []string{
			"Integer division by zero is an error; Float division by zero is IEEE-754 (x/0 → ±inf, 0/0 → nan).",
		},
	})

	register(&Entry{
		Word:    "mod",
		Summary: "Remainder: a mod b ≡ a %% b.",
		Description: "All three call forms `a b mod`, `a mod b`, and `mod b a` " +
			"compute a %% b (the truncated remainder). For the IEEE round-to-nearest remainder, use `MathUtil.remainder`.",
		Notes: []string{
			"Integer modulo by zero is an error; Float modulo by zero is IEEE-754 nan.",
		},
	})

	register(&Entry{
		Word:    "pow",
		Summary: "Power: a pow b ≡ a^b.",
		Description: "All three call forms `a b pow`, `a pow b`, and `pow b a` " +
			"compute a^b.",
		Notes: []string{
			"Negative exponents produce an error for integer pow.",
			"Integer overflow raises [aql/integer_overflow]; use a Float operand for an approximate result.",
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
		Summary:     "Round a float up to the nearest integer.",
		Description: "Returns the smallest integer value greater than or equal to the input.",
		Notes:       []string{"Requires: \"aql:math\" import"},
	})

	register(&Entry{
		Word:        "floor",
		Summary:     "Round a float down to the nearest integer.",
		Description: "Returns the largest integer value less than or equal to the input.",
		Notes:       []string{"Requires: \"aql:math\" import"},
	})

	register(&Entry{
		Word:        "round",
		Summary:     "Round a float to the nearest integer.",
		Description: "Rounds to the nearest integer; ties round away from zero.",
		Notes: []string{
			"Uses Go's math.Round: 0.5 rounds away from zero.",
			"Requires: \"aql:math\" import",
		},
	})

	register(&Entry{
		Word:        "trunc",
		Summary:     "Truncate a float toward zero.",
		Description: "Removes the fractional part, rounding toward zero.",
		Notes:       []string{"Requires: \"aql:math\" import"},
	})

	register(&Entry{
		Word:        "round-even",
		Summary:     "Round a float to the nearest integer, ties to even.",
		Description: "Rounds to nearest; halves go to the even neighbour (IEEE-754 roundTiesToEven). `2.5 round-even` is 2, `3.5 round-even` is 4. Contrast `round`, which rounds halves away from zero.",
		Notes:       []string{"Requires: \"aql:math-util\" import"},
	})

	register(&Entry{
		Word:        "logb",
		Summary:     "The unbiased radix-2 exponent of a number.",
		Description: "Returns the exponent e such that the value is m*2^e with 1 <= |m| < 2 (math.Logb). `8.0 logb` is 3.0.",
		Notes:       []string{"Requires: \"aql:math-util\" import"},
	})

	register(&Entry{
		Word:        "scalb",
		Summary:     "Scale by a power of two: x scalb n = x * 2^n.",
		Description: "`x scalb n` returns x * 2^n efficiently (math.Ldexp); n is truncated to an integer.",
		Notes:       []string{"Requires: \"aql:math-util\" import"},
	})

	register(&Entry{
		Word:        "fma",
		Summary:     "Fused multiply-add: fma a b c = a*b + c (single rounding).",
		Description: "`fma a b c` computes a*b + c with only one rounding step (math.FMA), more accurate than a separate mul then add. Use forward form so a*b is the product and c the addend.",
		Notes:       []string{"Requires: \"aql:math-util\" import"},
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
		Word:        "is-nan",
		Summary:     "Test whether a Float is NaN (not-a-number).",
		Description: "Returns true when the value is NaN. Use this to detect NaN, since `nan eq nan` is false by IEEE-754.",
		Notes:       []string{"Requires: \"aql:math-util\" import"},
	})

	register(&Entry{
		Word:        "is-inf",
		Summary:     "Test whether a Float is +inf or -inf.",
		Description: "Returns true when the value is positive or negative infinity.",
		Notes:       []string{"Requires: \"aql:math-util\" import"},
	})

	register(&Entry{
		Word:        "is-finite",
		Summary:     "Test whether a number is finite (neither inf nor NaN).",
		Description: "Returns true for any finite value. Integers are always finite.",
		Notes:       []string{"Requires: \"aql:math-util\" import"},
	})

	register(&Entry{
		Word:        "signbit",
		Summary:     "Test whether a number's sign bit is set (negative, incl. -0.0).",
		Description: "Returns true when the value is negative, including negative zero.",
		Notes:       []string{"Requires: \"aql:math-util\" import"},
	})

	register(&Entry{
		Word:    "remainder",
		Summary: "IEEE-754 remainder: a remainder b, rounding the quotient to nearest.",
		Description: "Returns a - n*b where n is a/b rounded to the nearest integer (ties to even). " +
			"Distinct from `mod`, which is the truncated remainder (fmod): `5.0 remainder 3.0` is -1.0 while `5.0 3.0 mod` is 2.0.",
		Notes: []string{"Requires: \"aql:math-util\" import"},
	})

	register(&Entry{
		Word:        "copysign",
		Summary:     "Combine the magnitude of one number with the sign of another.",
		Description: "`a copysign b` returns a value with the magnitude of a and the sign of b.",
		Notes:       []string{"Requires: \"aql:math-util\" import"},
	})

	register(&Entry{
		Word:        "nextafter",
		Summary:     "The next representable Float after a, toward b.",
		Description: "`a nextafter b` returns the adjacent float64 stepping from a toward b.",
		Notes:       []string{"Requires: \"aql:math-util\" import"},
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

func init() {
	register(&Entry{
		Word:        "with-decimal",
		Summary:     "Run a block under decimal precision / rounding overrides.",
		Description: "with-decimal {precision: n rounding: \"...\"} [body] evaluates the body with the given arbitrary-precision decimal context, so 0d-literal arithmetic rounds as specified.",
		Examples:    []string{`with-decimal {precision: 5} [0d1.0 div 0d3.0]   ;# => 0d0.33333`},
	})
}
