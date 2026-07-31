package eng

// This file owns the canonical multi-sig fn parser. Both the bare
// borueng `fn` (in core_words.go::registerCoreFn) and the production
// boru `def`/`fn` in lang/go/engine/native_definition_fn.go call
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
		var returnPatterns []*Value
		if !concreteReturns {
			returns, returnPatterns, err = ParseFnReturns(r, outputSig)
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

		decl := DeclSite{Pos: outputSig.Pos()}
		if r != nil {
			decl.Source = r.Source
			decl.File = r.BaseFile
		}

		sigs = append(sigs, FnSig{
			Params:         params,
			Returns:        returns,
			ReturnPatterns: returnPatterns,
			Decl:           decl,
			Impl:           BORU(bodyElems),
			BarrierPos:     barrierPos,
			QuoteArgs:      QuoteArgsFromParams(params),
		})
	}
	return FnDefInfo{Signatures: sigs}, nil
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
// Mirrors the production boru parseFnUndefSpec in
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

		returns, _, err := ParseFnReturns(r, outputSig)
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
		IsTypedMap(v) || IsTableType(v) || IsClassType(v) {
		return true
	}
	// A Disjunct is a TYPE too — `def IS (Integer tor String)` then
	// `fn [[…] [IS] […]]`. When the name has already been evaluated to
	// its body (a Disjunct VALUE rather than the Word), reading it as a
	// concrete return-by-value forces the static return type to Any and
	// splices the value onto the body stack — the same spurious
	// "expected N return value(s)" the registry lookup above was added
	// to stop, one value-shape further along.
	if IsDisjunct(v) {
		return true
	}
	if IsWord(v) {
		_as0, _ := AsWord(v)
		return isSigTypeName(r, _as0.Name)
	}
	if v.Parent.ConformsTo(TAtom) || v.Parent.ConformsTo(TString) {
		name, _ := AsString(v)
		return isSigTypeName(r, name)
	}
	// A DOTTED type reference (`MatrixUtil.Matrix` — a Reach) reaching a type
	// literal through bound module exports: recognise it as a TYPE, exactly
	// as dottedParamType resolves it for a param slot. Without this an output
	// sig `[Pkg.Type]` was mis-read as a concrete return-by-value — the
	// resolved type literal got spliced onto the body and surfaced as a
	// spurious "expected N return value(s), got N+1" (a MatrixUtil tensor
	// return type, the stats cov-matrix false positive). The Word-name case
	// above already covers a plain `def`'d type name; this is its dotted twin.
	if IsReach(v) {
		if _, ok := dottedParamType(r, v); ok {
			return true
		}
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
