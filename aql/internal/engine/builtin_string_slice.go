package engine

func registerSlice(r *Registry) {
	// slice: [string, integer] -> [string]
	sliceHandler := func(args []Value) ([]Value, error) {
		return doSlice(args[0].AsString(), args[1].AsInteger(), -1, false,
			strOpts{unit: "code-unit"})
	}

	// slice: [string, integer, integer] -> [string]
	slice3Handler := func(args []Value) ([]Value, error) {
		return doSlice(args[0].AsString(), args[1].AsInteger(), args[2].AsInteger(), true,
			strOpts{unit: "code-unit"})
	}

	// slice: [string, integer, map] -> [string]
	sliceOptsHandler := func(args []Value) ([]Value, error) {
		opts := parseStrOpts(args[2])
		return doSlice(args[0].AsString(), args[1].AsInteger(), -1, false, opts)
	}

	// slice: [string, integer, integer, map] -> [string]
	slice4Handler := func(args []Value) ([]Value, error) {
		opts := parseStrOpts(args[3])
		return doSlice(args[0].AsString(), args[1].AsInteger(), args[2].AsInteger(), true, opts)
	}

	r.Register("slice",
		Signature{Args: []Type{TString, TInteger, TInteger, TMap}, Handler: slice4Handler},
		Signature{Args: []Type{TString, TInteger, TMap}, Handler: sliceOptsHandler},
		Signature{Args: []Type{TString, TInteger, TInteger}, Handler: slice3Handler},
		Signature{Args: []Type{TString, TInteger}, Handler: sliceHandler},
	)
}

func doSlice(input string, start, end int64, hasEnd bool, o strOpts) ([]Value, error) {
	if o.normForm != "" {
		input = applyNorm(input, o.normForm)
	}

	length := int64(strLen(input, o.unit))

	s := start
	e := length
	if hasEnd {
		e = end
	}

	if o.fromEnd {
		// Interpret as offsets from end
		s = length - s
		if hasEnd {
			e = length - end
		}
	}

	// Handle negative indices (Python-style)
	if s < 0 {
		s += length
	}
	if e < 0 {
		e += length
	}

	// Clamp
	if s < 0 {
		s = 0
	}
	if e > length {
		e = length
	}

	result := strSlice(input, int(s), int(e), o.unit)
	return []Value{NewString(result)}, nil
}
