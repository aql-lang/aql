package native

import "github.com/aql-lang/aql/eng/go"

// makeNatives installs the `make` word — the universal constructor
// for typed values (scalars, class instances, records, tables).
//
//	make T data       — build a T-typed value from data; T may be a
//	                    Scalar type, a ClassType, a RecordType, a
//	                    TableType, or any subtype thereof.
//	make T data opts  — same with an options map (currently the
//	                    `use_base:true` flag for classes/records).
//
// The algorithm primitives (MakeHandler, MakeScalarHandler,
// MakeScalarOptsHandler, MakeObjHandler, MakeWithOpts, plus
// MakeObject / MakeConvert / MakeFieldValue / ResolveFieldType) live
// in eng/go/core_make.go; this file owns the word name, signature
// shape, and dispatch wiring.
// makeObjReturns is the class/Resource make sig's check-mode result:
// ReturnsFreshInstance(0) plus the schema-decidable construction validation
// (eng.CheckMakeConstruction — unknown / missing / mistyped fields on a
// concrete construction map, with the byte-identical runtime messages).
func makeObjReturns() ReturnsFunc {
	fresh := ReturnsFreshInstance(0)
	return func(args []Value, r *Registry) []Value {
		if len(args) >= 2 {
			eng.CheckMakeConstruction(r, args[0], args[1], args[0].Pos)
		}
		return fresh(args, r)
	}
}

// makeScalarReturns is the scalar make sig's check-mode result:
// ReturnsFreshInstance(0) plus the Micron construction validation
// (eng.CheckMicronConstruction — string parsing, unknown / missing /
// mistyped fields, the abstract root — on a fully-concrete source,
// with the byte-identical runtime messages). Micron targets dispatch
// through the scalar sig (the family roots under Scalar), so this is
// their CheckMakeConstruction analogue.
func makeScalarReturns() ReturnsFunc {
	fresh := ReturnsFreshInstance(0)
	return func(args []Value, r *Registry) []Value {
		if len(args) >= 2 {
			eng.CheckMicronConstruction(r, args[0], args[1], args[0].Pos)
		}
		return fresh(args, r)
	}
}

var makeNatives = []NativeFunc{
	{
		Name:          "make",
		CompileEffect: CompileIslandPure,

		Signatures: []Signature{
			// make is an IMPURE constructor: each call mints a fresh
			// instance at run time (NewValueRaw), so its check-mode result
			// carrier must also be fresh-identity per call — ReturnsIdentity
			// would return the SAME type-literal value (one Value.ID), which
			// collapses two `make P {}` operands onto one in the bytecode
			// lowerer's per-value provenance. See ReturnsFreshInstance.
			{Args: []*Type{TScalar, TMap, TAny}, TypeArgs: map[int]bool{0: true}, Impl: Go(eng.MakeScalarOptsHandler), ReturnsFn: ReturnsFreshInstance(0), BarrierPos: -1},
			{Args: []*Type{TIdeal, TMap}, TypeArgs: map[int]bool{0: true}, Impl: Go(eng.MakeObjHandler), ReturnsFn: makeObjReturns(), BarrierPos: -1},
			// Node-family targets: make FlexMap/FlexList (mutable deep
			// copy) and make Map/List (deep immutable conversion — the
			// inverse). Structural type bodies that land in the Node
			// TypeArgs slot are deferred back to MakeHandler inside
			// the handler.
			{Args: []*Type{TNode, TAny}, TypeArgs: map[int]bool{0: true}, Impl: Go(eng.MakeNodeHandler), ReturnsFn: ReturnsFreshInstance(0), BarrierPos: -1},
			{Args: []*Type{TScalar, TAny}, TypeArgs: map[int]bool{0: true}, Impl: Go(eng.MakeScalarHandler), ReturnsFn: makeScalarReturns(), BarrierPos: -1},
			{Args: []*Type{TAny, TAny, TMap}, Impl: Go(eng.MakeWithOpts), Returns: []*Type{TAny}, BarrierPos: -1},
			{Args: []*Type{TAny, TAny}, Impl: Go(eng.MakeHandler), Returns: []*Type{TAny}, BarrierPos: -1},
		},
	},
}
