package help

func init() {
	register(&Entry{
		Word:    "add",
		Summary: "Add two numbers, or concatenate two scalars as strings.",
		Description: "Adds two numeric values. When both are integers the result is an integer; " +
			"if either is a decimal the result is a decimal. For non-numeric scalars, " +
			"concatenates their string representations.",
	})

	register(&Entry{
		Word:    "sub",
		Summary: "Subtract the top value from the second value.",
		Description: "Subtracts: a b sub produces a - b.",
	})

	register(&Entry{
		Word:    "mul",
		Summary: "Multiply two numbers.",
		Description: "Multiplies two numeric values.",
	})

	register(&Entry{
		Word:    "div",
		Summary: "Divide the second value by the top value.",
		Description: "Divides: a b div produces a / b. Integer division truncates toward zero.",
		Notes: []string{
			"Division by zero produces an error.",
		},
	})

	register(&Entry{
		Word:    "mod",
		Summary: "Compute the remainder of integer division.",
		Description: "Computes: a b mod produces a %% b.",
	})

	register(&Entry{
		Word:    "abs",
		Summary: "Return the absolute value of a number.",
		Description: "Returns the absolute (non-negative) value.",
	})

	register(&Entry{
		Word:    "negate",
		Summary: "Negate a number (flip the sign).",
		Description: "Returns -n for input n.",
	})

	register(&Entry{
		Word:    "min",
		Summary: "Return the smaller of two numbers.",
		Description: "Returns the minimum of two values.",
	})

	register(&Entry{
		Word:    "max",
		Summary: "Return the larger of two numbers.",
		Description: "Returns the maximum of two values.",
	})

	register(&Entry{
		Word:    "pow",
		Summary: "Raise a number to a power.",
		Description: "Computes: a b pow produces a^b.",
		Notes: []string{
			"Negative exponents produce an error for integer pow.",
		},
	})

	register(&Entry{
		Word:    "sign",
		Summary: "Return the sign of a number (-1, 0, or 1).",
		Description: "Returns -1 for negative, 0 for zero, 1 for positive.",
	})

	register(&Entry{
		Word:    "ceil",
		Summary: "Round a decimal up to the nearest integer.",
		Description: "Returns the smallest integer value greater than or equal to the input.",
	})

	register(&Entry{
		Word:    "floor",
		Summary: "Round a decimal down to the nearest integer.",
		Description: "Returns the largest integer value less than or equal to the input.",
	})

	register(&Entry{
		Word:    "round",
		Summary: "Round a decimal to the nearest integer.",
		Description: "Rounds to the nearest integer; ties round away from zero.",
		Notes: []string{"Uses Go's math.Round: 0.5 rounds away from zero."},
	})

	register(&Entry{
		Word:    "trunc",
		Summary: "Truncate a decimal toward zero.",
		Description: "Removes the fractional part, rounding toward zero.",
	})

	register(&Entry{
		Word:    "sqrt",
		Summary: "Compute the square root.",
		Description: "Returns the square root of the input.",
	})

	register(&Entry{
		Word:    "cbrt",
		Summary: "Compute the cube root.",
		Description: "Returns the cube root of the input.",
	})

	register(&Entry{
		Word:    "exp",
		Summary: "Compute e raised to a power.",
		Description: "Returns e^x where e is Euler's number.",
	})

	register(&Entry{
		Word:    "log",
		Summary: "Compute the natural logarithm.",
		Description: "Returns the natural logarithm (base e) of the input.",
	})

	register(&Entry{
		Word:    "log2",
		Summary: "Compute the base-2 logarithm.",
		Description: "Returns the base-2 logarithm of the input.",
	})

	register(&Entry{
		Word:    "log10",
		Summary: "Compute the base-10 logarithm.",
		Description: "Returns the base-10 logarithm of the input.",
	})

	register(&Entry{
		Word:    "sin",
		Summary: "Compute the sine (input in radians).",
		Description: "Returns the sine of the input angle in radians.",
	})

	register(&Entry{
		Word:    "cos",
		Summary: "Compute the cosine (input in radians).",
		Description: "Returns the cosine of the input angle in radians.",
	})

	register(&Entry{
		Word:    "tan",
		Summary: "Compute the tangent (input in radians).",
		Description: "Returns the tangent of the input angle in radians.",
	})

	register(&Entry{
		Word:    "asin",
		Summary: "Compute the arc sine (result in radians).",
		Description: "Returns the arc sine of the input. Input must be in [-1, 1].",
	})

	register(&Entry{
		Word:    "acos",
		Summary: "Compute the arc cosine (result in radians).",
		Description: "Returns the arc cosine of the input. Input must be in [-1, 1].",
	})

	register(&Entry{
		Word:    "atan",
		Summary: "Compute the arc tangent (result in radians).",
		Description: "Returns the arc tangent of the input.",
	})

	register(&Entry{
		Word:    "atan2",
		Summary: "Compute the two-argument arc tangent.",
		Description: "Returns atan2(y, x): y x atan2. Handles quadrant correctly.",
	})

	register(&Entry{
		Word:    "hypot",
		Summary: "Compute the hypotenuse length.",
		Description: "Returns sqrt(x*x + y*y) without overflow: x y hypot.",
	})

	register(&Entry{
		Word:    "math-pi",
		Summary: "Push the constant pi onto the stack.",
		Description: "Pushes the mathematical constant pi (3.14159...). Stack-only.",
	})

	register(&Entry{
		Word:    "math-e",
		Summary: "Push Euler's number e onto the stack.",
		Description: "Pushes the mathematical constant e (2.71828...). Stack-only.",
	})
}
