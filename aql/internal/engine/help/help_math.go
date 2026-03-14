package help

func init() {
	register(&Entry{
		Word:    "add",
		Summary: "Add two numbers, or concatenate two scalars as strings.",
		Signatures: []string{
			"[integer integer] -> [integer]",
			"[decimal decimal] -> [decimal]",
			"[scalar scalar] -> [string]",
		},
		Description: "Adds two numeric values. When both are integers the result is an integer; " +
			"if either is a decimal the result is a decimal. For non-numeric scalars, " +
			"concatenates their string representations.",
		Examples: []string{
			`2 3 add          => 5`,
			`2.5 1.5 add      => 4`,
			`"a" "b" add      => "ab"`,
		},
	})

	register(&Entry{
		Word:    "sub",
		Summary: "Subtract the top value from the second value.",
		Signatures: []string{
			"[integer integer] -> [integer]",
			"[decimal decimal] -> [decimal]",
		},
		Description: "Subtracts: a b sub produces a - b.",
		Examples: []string{
			`5 3 sub      => 2`,
			`3.5 1.0 sub  => 2.5`,
		},
	})

	register(&Entry{
		Word:    "mul",
		Summary: "Multiply two numbers.",
		Signatures: []string{
			"[integer integer] -> [integer]",
			"[decimal decimal] -> [decimal]",
		},
		Description: "Multiplies two numeric values.",
		Examples: []string{
			`3 4 mul      => 12`,
			`2.5 2 mul    => 5`,
		},
	})

	register(&Entry{
		Word:    "div",
		Summary: "Divide the second value by the top value.",
		Signatures: []string{
			"[integer integer] -> [integer]",
			"[decimal decimal] -> [decimal]",
		},
		Description: "Divides: a b div produces a / b. Integer division truncates.",
		Examples: []string{
			`10 3 div     => 3`,
			`10.0 3 div   => 3.3333...`,
		},
		Notes: []string{
			"Division by zero produces an error.",
		},
	})

	register(&Entry{
		Word:    "mod",
		Summary: "Compute the remainder of integer division.",
		Signatures: []string{
			"[integer integer] -> [integer]",
			"[decimal decimal] -> [decimal]",
		},
		Description: "Computes: a b mod produces a % b.",
		Examples: []string{
			`10 3 mod     => 1`,
		},
	})

	register(&Entry{
		Word:    "abs",
		Summary: "Return the absolute value of a number.",
		Signatures: []string{
			"[integer] -> [integer]",
			"[decimal] -> [decimal]",
		},
		Description: "Returns the absolute (non-negative) value.",
		Examples: []string{
			`-5 abs       => 5`,
			`3 abs        => 3`,
		},
	})

	register(&Entry{
		Word:    "negate",
		Summary: "Negate a number (flip the sign).",
		Signatures: []string{
			"[integer] -> [integer]",
			"[decimal] -> [decimal]",
		},
		Description: "Returns -n for input n.",
		Examples: []string{
			`5 negate     => -5`,
			`-3 negate    => 3`,
		},
	})

	register(&Entry{
		Word:    "min",
		Summary: "Return the smaller of two numbers.",
		Signatures: []string{
			"[integer integer] -> [integer]",
			"[decimal decimal] -> [decimal]",
		},
		Description: "Returns the minimum of two values.",
		Examples: []string{`3 5 min => 3`},
	})

	register(&Entry{
		Word:    "max",
		Summary: "Return the larger of two numbers.",
		Signatures: []string{
			"[integer integer] -> [integer]",
			"[decimal decimal] -> [decimal]",
		},
		Description: "Returns the maximum of two values.",
		Examples: []string{`3 5 max => 5`},
	})

	register(&Entry{
		Word:    "pow",
		Summary: "Raise a number to a power.",
		Signatures: []string{
			"[integer integer] -> [integer]",
			"[decimal decimal] -> [decimal]",
		},
		Description: "Computes: a b pow produces a^b.",
		Examples: []string{`2 10 pow => 1024`},
	})

	register(&Entry{
		Word:    "sign",
		Summary: "Return the sign of a number (-1, 0, or 1).",
		Signatures: []string{
			"[integer] -> [integer]",
			"[decimal] -> [decimal]",
		},
		Description: "Returns -1 for negative, 0 for zero, 1 for positive.",
		Examples: []string{
			`-7 sign  => -1`,
			`0 sign   => 0`,
			`42 sign  => 1`,
		},
	})

	register(&Entry{
		Word:    "ceil",
		Summary: "Round a number up to the nearest integer.",
		Signatures: []string{"[integer] -> [decimal]", "[decimal] -> [decimal]"},
		Description: "Returns the smallest integer value greater than or equal to the input.",
		Examples: []string{`2.3 ceil => 3`},
	})

	register(&Entry{
		Word:    "floor",
		Summary: "Round a number down to the nearest integer.",
		Signatures: []string{"[integer] -> [decimal]", "[decimal] -> [decimal]"},
		Description: "Returns the largest integer value less than or equal to the input.",
		Examples: []string{`2.7 floor => 2`},
	})

	register(&Entry{
		Word:    "round",
		Summary: "Round a number to the nearest integer.",
		Signatures: []string{"[integer] -> [decimal]", "[decimal] -> [decimal]"},
		Description: "Rounds to the nearest integer; ties round to even (banker's rounding).",
		Examples: []string{`2.5 round => 2`, `3.5 round => 4`},
		Notes: []string{"Uses Go's math.RoundToEven."},
	})

	register(&Entry{
		Word:    "trunc",
		Summary: "Truncate a number toward zero.",
		Signatures: []string{"[integer] -> [decimal]", "[decimal] -> [decimal]"},
		Description: "Removes the fractional part, rounding toward zero.",
		Examples: []string{`2.9 trunc => 2`, `-2.9 trunc => -2`},
	})

	register(&Entry{
		Word:    "sqrt",
		Summary: "Compute the square root.",
		Signatures: []string{"[integer] -> [decimal]", "[decimal] -> [decimal]"},
		Description: "Returns the square root of the input.",
		Examples: []string{`9 sqrt => 3`},
	})

	register(&Entry{
		Word:    "cbrt",
		Summary: "Compute the cube root.",
		Signatures: []string{"[integer] -> [decimal]", "[decimal] -> [decimal]"},
		Description: "Returns the cube root of the input.",
		Examples: []string{`27 cbrt => 3`},
	})

	register(&Entry{
		Word:    "exp",
		Summary: "Compute e raised to a power.",
		Signatures: []string{"[integer] -> [decimal]", "[decimal] -> [decimal]"},
		Description: "Returns e^x where e is Euler's number.",
		Examples: []string{`1 exp => 2.71828...`},
	})

	register(&Entry{
		Word:    "log",
		Summary: "Compute the natural logarithm.",
		Signatures: []string{"[integer] -> [decimal]", "[decimal] -> [decimal]"},
		Description: "Returns the natural logarithm (base e) of the input.",
		Examples: []string{`1 log => 0`},
	})

	register(&Entry{
		Word:    "log2",
		Summary: "Compute the base-2 logarithm.",
		Signatures: []string{"[integer] -> [decimal]", "[decimal] -> [decimal]"},
		Description: "Returns the base-2 logarithm of the input.",
		Examples: []string{`8 log2 => 3`},
	})

	register(&Entry{
		Word:    "log10",
		Summary: "Compute the base-10 logarithm.",
		Signatures: []string{"[integer] -> [decimal]", "[decimal] -> [decimal]"},
		Description: "Returns the base-10 logarithm of the input.",
		Examples: []string{`100 log10 => 2`},
	})

	register(&Entry{
		Word:    "sin",
		Summary: "Compute the sine (input in radians).",
		Signatures: []string{"[integer] -> [decimal]", "[decimal] -> [decimal]"},
		Description: "Returns the sine of the input angle in radians.",
		Examples: []string{`0 sin => 0`},
	})

	register(&Entry{
		Word:    "cos",
		Summary: "Compute the cosine (input in radians).",
		Signatures: []string{"[integer] -> [decimal]", "[decimal] -> [decimal]"},
		Description: "Returns the cosine of the input angle in radians.",
		Examples: []string{`0 cos => 1`},
	})

	register(&Entry{
		Word:    "tan",
		Summary: "Compute the tangent (input in radians).",
		Signatures: []string{"[integer] -> [decimal]", "[decimal] -> [decimal]"},
		Description: "Returns the tangent of the input angle in radians.",
		Examples: []string{`0 tan => 0`},
	})

	register(&Entry{
		Word:    "asin",
		Summary: "Compute the arc sine (result in radians).",
		Signatures: []string{"[integer] -> [decimal]", "[decimal] -> [decimal]"},
		Description: "Returns the arc sine of the input. Input must be in [-1, 1].",
		Examples: []string{`0 asin => 0`},
	})

	register(&Entry{
		Word:    "acos",
		Summary: "Compute the arc cosine (result in radians).",
		Signatures: []string{"[integer] -> [decimal]", "[decimal] -> [decimal]"},
		Description: "Returns the arc cosine of the input. Input must be in [-1, 1].",
		Examples: []string{`1 acos => 0`},
	})

	register(&Entry{
		Word:    "atan",
		Summary: "Compute the arc tangent (result in radians).",
		Signatures: []string{"[integer] -> [decimal]", "[decimal] -> [decimal]"},
		Description: "Returns the arc tangent of the input.",
		Examples: []string{`0 atan => 0`},
	})

	register(&Entry{
		Word:    "atan2",
		Summary: "Compute the two-argument arc tangent.",
		Signatures: []string{"[number number] -> [decimal]"},
		Description: "Returns atan2(y, x): y x atan2. Handles quadrant correctly.",
		Examples: []string{`1 1 atan2 => 0.7853...`},
	})

	register(&Entry{
		Word:    "hypot",
		Summary: "Compute the hypotenuse length.",
		Signatures: []string{"[number number] -> [decimal]"},
		Description: "Returns sqrt(x² + y²) without overflow: x y hypot.",
		Examples: []string{`3 4 hypot => 5`},
	})

	register(&Entry{
		Word:    "math-pi",
		Summary: "Push the constant π onto the stack.",
		Signatures: []string{"[] -> [decimal]"},
		Description: "Pushes the mathematical constant pi (3.14159...).",
		Examples: []string{`math-pi => 3.141592653589793`},
	})

	register(&Entry{
		Word:    "math-e",
		Summary: "Push Euler's number e onto the stack.",
		Signatures: []string{"[] -> [decimal]"},
		Description: "Pushes the mathematical constant e (2.71828...).",
		Examples: []string{`math-e => 2.718281828459045`},
	})
}
