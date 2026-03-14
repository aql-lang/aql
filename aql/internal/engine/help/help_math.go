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
			`2 3 add              => 5`,
			`2.5 1.5 add          => 4`,
			`"a" "b" add          => 'ab'`,
			`10 -3 add            => 7`,
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
			`5 3 sub              => 2`,
			`10.5 3.0 sub         => 7.5`,
			`1 5 sub              => -4`,
			`0 0 sub              => 0`,
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
			`3 4 mul              => 12`,
			`2.5 4.0 mul          => 10`,
			`0 100 mul            => 0`,
			`-3 -4 mul            => 12`,
		},
	})

	register(&Entry{
		Word:    "div",
		Summary: "Divide the second value by the top value.",
		Signatures: []string{
			"[integer integer] -> [integer]",
			"[decimal decimal] -> [decimal]",
		},
		Description: "Divides: a b div produces a / b. Integer division truncates toward zero.",
		Examples: []string{
			`10 3 div             => 3`,
			`7 2 div              => 3`,
			`7.0 2.0 div          => 3.5`,
			`-7 2 div             => -3`,
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
		Description: "Computes: a b mod produces a %% b.",
		Examples: []string{
			`10 3 mod             => 1`,
			`7 2 mod              => 1`,
			`8 4 mod              => 0`,
			`-7 3 mod             => -1`,
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
			`-5 abs               => 5`,
			`3 abs                => 3`,
			`0 abs                => 0`,
			`-2.5 abs             => 2.5`,
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
			`5 negate             => -5`,
			`-3 negate            => 3`,
			`0 negate             => 0`,
			`2.5 negate           => -2.5`,
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
		Examples: []string{
			`3 5 min              => 3`,
			`5 3 min              => 3`,
			`-1 1 min             => -1`,
			`7 7 min              => 7`,
		},
	})

	register(&Entry{
		Word:    "max",
		Summary: "Return the larger of two numbers.",
		Signatures: []string{
			"[integer integer] -> [integer]",
			"[decimal decimal] -> [decimal]",
		},
		Description: "Returns the maximum of two values.",
		Examples: []string{
			`3 5 max              => 5`,
			`5 3 max              => 5`,
			`-1 1 max             => 1`,
			`7 7 max              => 7`,
		},
	})

	register(&Entry{
		Word:    "pow",
		Summary: "Raise a number to a power.",
		Signatures: []string{
			"[integer integer] -> [integer]",
			"[decimal decimal] -> [decimal]",
		},
		Description: "Computes: a b pow produces a^b.",
		Examples: []string{
			`2 10 pow             => 1024`,
			`3 3 pow              => 27`,
			`5 0 pow              => 1`,
			`10 2 pow             => 100`,
		},
		Notes: []string{
			"Negative exponents produce an error for integer pow.",
		},
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
			`-7 sign              => -1`,
			`0 sign               => 0`,
			`42 sign              => 1`,
			`-2.5 sign            => -1`,
		},
	})

	register(&Entry{
		Word:    "ceil",
		Summary: "Round a decimal up to the nearest integer.",
		Signatures: []string{"[decimal] -> [integer]"},
		Description: "Returns the smallest integer value greater than or equal to the input.",
		Examples: []string{
			`2.3 ceil             => 3`,
			`2.7 ceil             => 3`,
			`-2.3 ceil            => -2`,
			`-2.7 ceil            => -2`,
		},
	})

	register(&Entry{
		Word:    "floor",
		Summary: "Round a decimal down to the nearest integer.",
		Signatures: []string{"[decimal] -> [integer]"},
		Description: "Returns the largest integer value less than or equal to the input.",
		Examples: []string{
			`2.7 floor            => 2`,
			`2.3 floor            => 2`,
			`-2.3 floor           => -3`,
			`-2.7 floor           => -3`,
		},
	})

	register(&Entry{
		Word:    "round",
		Summary: "Round a decimal to the nearest integer.",
		Signatures: []string{"[decimal] -> [integer]"},
		Description: "Rounds to the nearest integer; ties round away from zero.",
		Examples: []string{
			`2.7 round            => 3`,
			`2.3 round            => 2`,
			`2.5 round            => 3`,
			`-2.5 round           => -3`,
		},
		Notes: []string{"Uses Go's math.Round: 0.5 rounds away from zero."},
	})

	register(&Entry{
		Word:    "trunc",
		Summary: "Truncate a decimal toward zero.",
		Signatures: []string{"[decimal] -> [integer]"},
		Description: "Removes the fractional part, rounding toward zero.",
		Examples: []string{
			`2.9 trunc            => 2`,
			`-2.9 trunc           => -2`,
			`0.5 trunc            => 0`,
			`-0.5 trunc           => 0`,
		},
	})

	register(&Entry{
		Word:    "sqrt",
		Summary: "Compute the square root.",
		Signatures: []string{"[integer] -> [decimal]", "[decimal] -> [decimal]"},
		Description: "Returns the square root of the input.",
		Examples: []string{
			`9 sqrt               => 3`,
			`4 sqrt               => 2`,
			`2 sqrt               => 1.4142135623730951`,
			`0 sqrt               => 0`,
		},
	})

	register(&Entry{
		Word:    "cbrt",
		Summary: "Compute the cube root.",
		Signatures: []string{"[integer] -> [decimal]", "[decimal] -> [decimal]"},
		Description: "Returns the cube root of the input.",
		Examples: []string{
			`27 cbrt              => 3`,
			`8 cbrt               => 2`,
			`1 cbrt               => 1`,
			`0 cbrt               => 0`,
		},
	})

	register(&Entry{
		Word:    "exp",
		Summary: "Compute e raised to a power.",
		Signatures: []string{"[integer] -> [decimal]", "[decimal] -> [decimal]"},
		Description: "Returns e^x where e is Euler's number.",
		Examples: []string{
			`0 exp                => 1`,
			`1 exp                => 2.718281828459045`,
			`2 exp                => 7.38905609893065`,
			`-1 exp               => 0.36787944117144233`,
		},
	})

	register(&Entry{
		Word:    "log",
		Summary: "Compute the natural logarithm.",
		Signatures: []string{"[integer] -> [decimal]", "[decimal] -> [decimal]"},
		Description: "Returns the natural logarithm (base e) of the input.",
		Examples: []string{
			`1 log                => 0`,
			`math-e log           => 1`,
			`10 log               => 2.302585092994046`,
			`100 log              => 4.605170185988092`,
		},
	})

	register(&Entry{
		Word:    "log2",
		Summary: "Compute the base-2 logarithm.",
		Signatures: []string{"[integer] -> [decimal]", "[decimal] -> [decimal]"},
		Description: "Returns the base-2 logarithm of the input.",
		Examples: []string{
			`8 log2               => 3`,
			`1 log2               => 0`,
			`1024 log2            => 10`,
			`2 log2               => 1`,
		},
	})

	register(&Entry{
		Word:    "log10",
		Summary: "Compute the base-10 logarithm.",
		Signatures: []string{"[integer] -> [decimal]", "[decimal] -> [decimal]"},
		Description: "Returns the base-10 logarithm of the input.",
		Examples: []string{
			`100 log10            => 2`,
			`1000 log10           => 3`,
			`1 log10              => 0`,
			`10 log10             => 1`,
		},
	})

	register(&Entry{
		Word:    "sin",
		Summary: "Compute the sine (input in radians).",
		Signatures: []string{"[integer] -> [decimal]", "[decimal] -> [decimal]"},
		Description: "Returns the sine of the input angle in radians.",
		Examples: []string{
			`0 sin                => 0`,
			`math-pi 2.0 div sin  => 1`,
			`1 sin                => 0.8414709848078965`,
			`-1 sin               => -0.8414709848078965`,
		},
	})

	register(&Entry{
		Word:    "cos",
		Summary: "Compute the cosine (input in radians).",
		Signatures: []string{"[integer] -> [decimal]", "[decimal] -> [decimal]"},
		Description: "Returns the cosine of the input angle in radians.",
		Examples: []string{
			`0 cos                => 1`,
			`math-pi cos          => -1`,
			`1 cos                => 0.5403023058681398`,
			`-1 cos               => 0.5403023058681398`,
		},
	})

	register(&Entry{
		Word:    "tan",
		Summary: "Compute the tangent (input in radians).",
		Signatures: []string{"[integer] -> [decimal]", "[decimal] -> [decimal]"},
		Description: "Returns the tangent of the input angle in radians.",
		Examples: []string{
			`0 tan                => 0`,
			`math-pi 4.0 div tan  => 1`,
			`1 tan                => 1.557407724654902`,
			`-1 tan               => -1.557407724654902`,
		},
	})

	register(&Entry{
		Word:    "asin",
		Summary: "Compute the arc sine (result in radians).",
		Signatures: []string{"[integer] -> [decimal]", "[decimal] -> [decimal]"},
		Description: "Returns the arc sine of the input. Input must be in [-1, 1].",
		Examples: []string{
			`0 asin               => 0`,
			`1 asin               => 1.5707963267948966`,
			`-1 asin              => -1.5707963267948966`,
			`0.5 asin             => 0.5235987755982989`,
		},
	})

	register(&Entry{
		Word:    "acos",
		Summary: "Compute the arc cosine (result in radians).",
		Signatures: []string{"[integer] -> [decimal]", "[decimal] -> [decimal]"},
		Description: "Returns the arc cosine of the input. Input must be in [-1, 1].",
		Examples: []string{
			`1 acos               => 0`,
			`0 acos               => 1.5707963267948966`,
			`-1 acos              => 3.141592653589793`,
			`0.5 acos             => 1.0471975511965976`,
		},
	})

	register(&Entry{
		Word:    "atan",
		Summary: "Compute the arc tangent (result in radians).",
		Signatures: []string{"[integer] -> [decimal]", "[decimal] -> [decimal]"},
		Description: "Returns the arc tangent of the input.",
		Examples: []string{
			`0 atan               => 0`,
			`1 atan               => 0.7853981633974483`,
			`-1 atan              => -0.7853981633974483`,
			`100 atan             => 1.5607966601082313`,
		},
	})

	register(&Entry{
		Word:    "atan2",
		Summary: "Compute the two-argument arc tangent.",
		Signatures: []string{"[number number] -> [decimal]"},
		Description: "Returns atan2(y, x): y x atan2. Handles quadrant correctly.",
		Examples: []string{
			`1 1 atan2            => 0.7853981633974483`,
			`1 0 atan2            => 1.5707963267948966`,
			`0 1 atan2            => 0`,
			`-1 -1 atan2          => -2.356194490192345`,
		},
	})

	register(&Entry{
		Word:    "hypot",
		Summary: "Compute the hypotenuse length.",
		Signatures: []string{"[number number] -> [decimal]"},
		Description: "Returns sqrt(x*x + y*y) without overflow: x y hypot.",
		Examples: []string{
			`3 4 hypot            => 5`,
			`5 12 hypot           => 13`,
			`1 1 hypot            => 1.4142135623730951`,
			`0 5 hypot            => 5`,
		},
	})

	register(&Entry{
		Word:    "math-pi",
		Summary: "Push the constant pi onto the stack.",
		Signatures: []string{"[] -> [decimal]"},
		Description: "Pushes the mathematical constant pi (3.14159...). Prefix-only.",
		Examples: []string{
			`math-pi              => 3.141592653589793`,
			`math-pi 2 mul        => 6.283185307179586`,
			`math-pi 2.0 div      => 1.5707963267948966`,
			`math-pi math-pi mul  => 9.869604401089358`,
		},
	})

	register(&Entry{
		Word:    "math-e",
		Summary: "Push Euler's number e onto the stack.",
		Signatures: []string{"[] -> [decimal]"},
		Description: "Pushes the mathematical constant e (2.71828...). Prefix-only.",
		Examples: []string{
			`math-e               => 2.718281828459045`,
			`math-e 2.0 div       => 1.3591409142295225`,
			`math-e log           => 1`,
			`math-e math-e mul    => 7.3890560989306495`,
		},
	})
}
