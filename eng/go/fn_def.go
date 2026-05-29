package eng

// This file owns the canonical multi-sig fn parser. Both the bare
// aqleng `fn` (in core_words.go::registerCoreFn) and the production
// aql `def`/`fn` in lang/go/engine/native_definition_fn.go call
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
//   OutputSigIsConcreteReturns(r, outputSig)  bool
//      — true iff every element of an output sig is a concrete value
//        (i.e. a return-by-value sig), not a type literal.
//
//   IsSigTypeValue(r, v)  bool
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
//	def fact fn [
//	  [n:Integer]            [Integer]  [n 1 addq]
//	  [n:Integer m:Integer]  [Integer]  [n m mulq]
//	]
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

		if !inputSig.Parent.Equal(TList) {
			inputSig = NewList([]Value{inputSig})
		}

		params, barrierPos, err := ParseFnParams(r, inputSig)
		if err != nil {
			return FnDefInfo{}, err
		}

		concreteReturns := OutputSigIsConcreteReturns(r, outputSig)

		var returns []*Type
		if !concreteReturns {
			returns, err = ParseFnReturns(r, outputSig)
			if err != nil {
				return FnDefInfo{}, err
			}
		}

		var bodyElems []Value
		if body.Parent.Equal(TList) && body.Data != nil {
			_lst, _ := AsList(body)
			bodyElems = _lst.Slice()
		} else {
			bodyElems = []Value{body}
		}

		if concreteReturns {
			retVals := OutputSigValues(outputSig)
			if len(retVals) > 0 {
				bodyElems = append(bodyElems, NewEnd())
				bodyElems = append(bodyElems, retVals...)
				returns = make([]*Type, len(retVals))
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
// lang/go/engine/native_definition_fn.go.
func ParseFnUndefSpec(r *Registry, list []Value) (FnUndefInfo, error) {
	var sigs []FnSigSpec
	for i := 0; i+1 < len(list); i += 2 {
		inputSig := list[i]
		outputSig := list[i+1]

		if !inputSig.Parent.Equal(TList) {
			inputSig = NewList([]Value{inputSig})
		}

		params, _, err := ParseFnParams(r, inputSig)
		if err != nil {
			return FnUndefInfo{}, err
		}

		returns, err := ParseFnReturns(r, outputSig)
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
func OutputSigIsConcreteReturns(r *Registry, outputSig Value) bool {
	if outputSig.Parent.Equal(TList) && outputSig.Data != nil {
		elems, _ := AsList(outputSig)
		if elems.Len() == 0 {
			return false
		}
		for _, e := range elems.Slice() {
			if IsSigTypeValue(r, e) {
				return false
			}
		}
		return true
	}
	return !IsSigTypeValue(r, outputSig)
}

// IsSigTypeValue reports whether v looks like a type in a signature
// context — a type literal, a type-name word/atom/string, or a
// structural type (Options/Record/Table/TypedList/TypedMap/ObjectType).
//
// The registry is consulted so a USER-DEFINED type name (a capitalised
// `def Foo …`) is recognised, not just kernel builtins. Without it, a
// def'd type name in a fn output sig (`fn [[…] [BloomFilter] […]]`) was
// misclassified as a concrete return-by-value, which forced the static
// return type to `Any` and spliced the type literal onto the body
// stack — surfacing as a spurious "expected N return value(s)" error.
// A nil registry degrades to builtin-only recognition (the historical
// behaviour, kept for the registry-less callers).
func IsSigTypeValue(r *Registry, v Value) bool {
	if IsTypeLiteral(v) {
		return true
	}
	if IsOptionsType(v) || IsRecordType(v) || IsTypedList(v) ||
		IsTypedMap(v) || IsTableType(v) || IsObjectType(v) {
		return true
	}
	if IsWord(v) {
		_as0, _ := AsWord(v)
		return isSigTypeName(r, _as0.Name)
	}
	if v.Parent.Matches(TAtom) || v.Parent.Matches(TString) {
		name, _ := AsString(v)
		return isSigTypeName(r, name)
	}
	return false
}

// isSigTypeName reports whether name denotes a type in signature
// context: a kernel builtin name, a resolvable kernel type path, or —
// when a registry is available — an active user-defined type binding
// (`r.LookupTypeName`, the same authoritative TypeDef-backed lookup the
// sig-type resolver uses).
func isSigTypeName(r *Registry, name string) bool {
	if _, ok := TypeNameTable()[name]; ok {
		return true
	}
	if _, ok := ResolveTypePath(name); ok {
		return true
	}
	return r != nil && r.LookupTypeName(name) != nil
}

// OutputSigValues extracts the concrete values from a return-by-value
// output signature. For a list-form output sig, returns the elements;
// for a single-value form, wraps the value in a one-element slice.
func OutputSigValues(outputSig Value) []Value {
	if outputSig.Parent.Equal(TList) && outputSig.Data != nil {
		elems, _ := AsList(outputSig)
		result := elems.Slice()
		return result
	}
	return []Value{outputSig}
}
