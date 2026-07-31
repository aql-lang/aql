package modules

func init() {
	registerDocs("boru:math-util", map[string]string{
		"abs":        "Absolute value (magnitude) of a number.",
		"acos":       "Arccosine, returning an angle in radians.",
		"asin":       "Arcsine, returning an angle in radians.",
		"atan":       "Arctangent, returning an angle in radians.",
		"atan2":      "Arctangent of y/x using the signs of both args.",
		"cbrt":       "Cube root of a number.",
		"ceil":       "Round up to the nearest integer.",
		"copysign":   "Magnitude of the first arg with the sign of the second.",
		"cos":        "Cosine of an angle given in radians.",
		"e":          "Euler's number, the constant e.",
		"exp":        "Exponential e raised to the given power.",
		"floor":      "Round down to the nearest integer.",
		"fma":        "Fused multiply-add a*b + c with a single rounding.",
		"hypot":      "Euclidean norm sqrt(x^2 + y^2) without overflow.",
		"is-finite":  "Predicate: true if the value is finite.",
		"is-inf":     "Predicate: true if the value is infinite.",
		"is-nan":     "Predicate: true if the value is NaN.",
		"log":        "Natural (base-e) logarithm.",
		"log10":      "Base-10 logarithm.",
		"log2":       "Base-2 logarithm.",
		"logb":       "Radix-2 exponent (binary exponent) of a number.",
		"max":        "Larger of two numbers, ignoring NaN.",
		"min":        "Smaller of two numbers, ignoring NaN.",
		"negate":     "Arithmetic negation of a number.",
		"nextafter":  "Next representable Float toward the second arg.",
		"pi":         "The constant pi.",
		"remainder":  "IEEE round-to-nearest remainder (distinct from mod).",
		"round":      "Round to nearest, ties away from zero.",
		"round-even": "Round to nearest, ties to even (banker's rounding).",
		"scalb":      "Scale by a power of two: x * 2^n.",
		"sign":       "Sign of a number: -1, 0, or 1.",
		"signbit":    "Predicate: true if the value's sign bit is set.",
		"sin":        "Sine of an angle given in radians.",
		"sqrt":       "Square root of a number.",
		"tan":        "Tangent of an angle given in radians.",
		"trunc":      "Truncate toward zero, dropping the fraction.",
	})

	// Almost all of this module reads correctly as generated permutations
	// (`MathUtil.sqrt 16.0`), so only the words with a real trap are
	// authored: the two rounding modes, the ones that need Floats, and the
	// swap-order binaries. Results are from verified
	// lang/spec/module-math.tsv rows.
	registerExamples("boru:math-util", map[string][]string{
		"round": {
			`MathUtil.round 4.6                               ;# 5`,
			`2.5 MathUtil.round                               ;# 3 — halves go away from zero`,
		},
		"round-even": {
			`2.5 MathUtil.round-even                          ;# 2`,
			`3.5 MathUtil.round-even                          ;# 4 — halves go to even; use this for statistics`,
		},
		"sqrt": {
			`MathUtil.sqrt 16.0                               ;# 4.0 — a FLOAT operand; 16 is an Integer`,
			`16.0 MathUtil.sqrt                               ;# 4.0`,
		},
		"is-nan": {
			`nan MathUtil.is-nan                              ;# true`,
			`5.0 MathUtil.is-nan                              ;# false — nan is never equal to itself, so ASK`,
		},
		"is-finite": {
			`inf MathUtil.is-finite                           ;# false`,
			`5 MathUtil.is-finite                             ;# true — Integers count as finite`,
		},
		"signbit": {
			`-3.0 MathUtil.signbit                            ;# true — the SIGN BIT, so -0.0 is true too`,
		},
		"remainder": {
			`7.0 MathUtil.remainder 3.0                       ;# 1.0`,
			`5.0 MathUtil.remainder 3.0                       ;# -1.0 — IEEE remainder, so it can be negative`,
		},
		"copysign":  {`3.0 MathUtil.copysign -1.0                       ;# -3.0 — magnitude of the first, sign of the second`},
		"scalb":     {`3.0 MathUtil.scalb 4                             ;# 48.0 — multiply by 2^n, exactly`},
		"logb":      {`8.0 MathUtil.logb                                ;# 3.0 — the binary exponent`},
		"nextafter": {`1.0 MathUtil.nextafter 2.0                       ;# 1.0000000000000002 — the very next Float`},
	})
}
