package eng

// This file owns the canonical multi-sig fn parser. Both the bare
// aqleng `fn` (in core_words.go::registerCoreFn) and the production
// aql `def`/`fn` in lang/internal/engine/native_definition_fn.go call
// into ParseFnDef. Single source of truth — do NOT duplicate the
// triple-walking logic anywhere else.
//
// Public surface:
//
//   ParseFnDef(r, list)  (FnDefInfo, error)
//      — walks `list` in triples of [input-sig, output-sig, body],
//        building one FnSig per triple. Returns the assembled
//        FnDefInfo. An empty list yields an empty FnDefInfo.
//
//   OutputSigIsConcreteReturns(outputSig)  bool
//      — true iff every element of an output sig is a concrete value
//        (i.e. a return-by-value sig), not a type literal.
//
//   IsSigTypeValue(v)  bool
//      — true iff v looks like a type in a signature context
//        (type literal, type-name word, options/record/table/typed-
//        list/map etc.).
//
//   OutputSigValues(outputSig)  []Value
//      — extracts the concrete values from a return-by-value sig.

// ParseFnDef parses a function specification list into FnDefInfo.
// The list contains signature triples: [input-sig, output-sig, body],
// repeated as needed for multi-overload definitions:
//
//   def fact fn [
//     [n:Integer]            [Integer]  [n 1 addq]
//     [n:Integer m:Integer]  [Integer]  [n m mulq]
//   ]
//
// The list above contains 6 elements (= 2 triples), producing 2
// FnSigs. Each element of a triple may be abbreviated: a non-list
// value is treated as a single-element list (so `String` is
// equivalent to `[String]` for an output signature).
//
// The Registry argument is threaded through to ResolveSigType so
// type-named values inside param specs can resolve via the type
// stack and def stack.
func ParseFnDef(r *Registry, list []Value) (FnDefInfo, error) {
	var sigs []FnSig
	for i := 0; i+2 < len(list); i += 3 {
		inputSig := list[i]
		outputSig := list[i+1]
		body := list[i+2]

		if !inputSig.VType.Equal(TList) {
			inputSig = NewList([]Value{inputSig})
		}

		params, barrierPos, err := ParseFnParams(r, inputSig)
		if err != nil {
			return FnDefInfo{}, err
		}

		concreteReturns := OutputSigIsConcreteReturns(outputSig)

		var returns []Type
		if !concreteReturns {
			returns, err = ParseFnReturns(outputSig)
			if err != nil {
				return FnDefInfo{}, err
			}
		}

		var bodyElems []Value
		if body.VType.Equal(TList) && body.Data != nil {
			bodyElems = body.AsList().Slice()
		} else {
			bodyElems = []Value{body}
		}

		if concreteReturns {
			retVals := OutputSigValues(outputSig)
			if len(retVals) > 0 {
				bodyElems = append(bodyElems, NewWord("end"))
				bodyElems = append(bodyElems, retVals...)
				returns = make([]Type, len(retVals))
				for j := range retVals {
					returns[j] = TAny
				}
			}
		}

		sigs = append(sigs, FnSig{
			Params:     params,
			Returns:    returns,
			Body:       bodyElems,
			BarrierPos: barrierPos,
		})
	}
	return FnDefInfo{Sigs: sigs}, nil
}

// ParseFnUndefSpec parses a list of [input output] sig pairs (no body
// — even-length list) into a FnUndefInfo. Each pair becomes one
// FnSigSpec; the resulting FnUndefInfo represents a function-shape
// TYPE: every signature its inhabitants must satisfy.
//
// Used by the `fn` word when the input list has even length: outside
// `def`, `fn [[in] [out]]` is a TYPE, not a function value. Bind it
// via `def f:fn[[in][out]] some-impl` to assert that an existing
// function satisfies the shape.
//
// Mirrors the production aql parseFnUndefSpec in
// lang/internal/engine/native_definition_fn.go.
func ParseFnUndefSpec(r *Registry, list []Value) (FnUndefInfo, error) {
	var sigs []FnSigSpec
	for i := 0; i+1 < len(list); i += 2 {
		inputSig := list[i]
		outputSig := list[i+1]

		if !inputSig.VType.Equal(TList) {
			inputSig = NewList([]Value{inputSig})
		}

		params, _, err := ParseFnParams(r, inputSig)
		if err != nil {
			return FnUndefInfo{}, err
		}

		returns, err := ParseFnReturns(outputSig)
		if err != nil {
			return FnUndefInfo{}, err
		}

		sigs = append(sigs, FnSigSpec{
			Params:  params,
			Returns: returns,
		})
	}
	return FnUndefInfo{Sigs: sigs}, nil
}

// OutputSigIsConcreteReturns reports whether all values in the
// output signature are concrete (non-type) values — i.e. the sig
// is a return-by-value form (`[42 "ok"]`) rather than a return-by-
// type form (`[Integer String]`).
func OutputSigIsConcreteReturns(outputSig Value) bool {
	if outputSig.VType.Equal(TList) && outputSig.Data != nil {
		elems := outputSig.AsList()
		if elems.Len() == 0 {
			return false
		}
		for _, e := range elems.Slice() {
			if IsSigTypeValue(e) {
				return false
			}
		}
		return true
	}
	return !IsSigTypeValue(outputSig)
}

// IsSigTypeValue reports whether v looks like a type in a signature
// context — a type literal, a known type-name word, or a structural
// type (Options/Record/Table/TypedList/TypedMap/ObjectType).
func IsSigTypeValue(v Value) bool {
	if v.Data == nil && !v.VType.Equal(TNone) {
		return true
	}
	if v.IsOptionsType() || v.IsRecordType() || v.IsTypedList() ||
		v.IsTypedMap() || v.IsTableType() || v.IsObjectType() {
		return true
	}
	if v.IsWord() {
		_as0, _ := v.AsWord()
		name := _as0.Name
		if _, ok := TypeNameTable()[name]; ok {
			return true
		}
		if _, ok := ResolveTypePath(name); ok {
			return true
		}
		return false
	}
	if v.VType.Matches(TAtom) || v.VType.Matches(TString) {
		name, _ := v.AsString()
		if _, ok := TypeNameTable()[name]; ok {
			return true
		}
		if _, ok := ResolveTypePath(name); ok {
			return true
		}
		return false
	}
	return false
}

// OutputSigValues extracts the concrete values from a return-by-value
// output signature. For a list-form output sig, returns the elements;
// for a single-value form, wraps the value in a one-element slice.
func OutputSigValues(outputSig Value) []Value {
	if outputSig.VType.Equal(TList) && outputSig.Data != nil {
		elems := outputSig.AsList()
		result := elems.Slice()
		return result
	}
	return []Value{outputSig}
}
