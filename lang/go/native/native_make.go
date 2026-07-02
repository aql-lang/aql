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
			{Args: []*Type{TIdeal, TMap}, TypeArgs: map[int]bool{0: true}, Impl: Go(eng.MakeObjHandler), ReturnsFn: ReturnsFreshInstance(0), BarrierPos: -1},
			// Node-family targets: make FlexMap/FlexList (mutable deep
			// copy) and make Map/List (deep immutable conversion — the
			// inverse). Structural type bodies that land in the Node
			// TypeArgs slot are deferred back to MakeHandler inside
			// the handler.
			{Args: []*Type{TNode, TAny}, TypeArgs: map[int]bool{0: true}, Impl: Go(eng.MakeNodeHandler), ReturnsFn: ReturnsFreshInstance(0), BarrierPos: -1},
			{Args: []*Type{TScalar, TAny}, TypeArgs: map[int]bool{0: true}, Impl: Go(eng.MakeScalarHandler), ReturnsFn: ReturnsFreshInstance(0), BarrierPos: -1},
			{Args: []*Type{TAny, TAny, TMap}, Impl: Go(eng.MakeWithOpts), Returns: []*Type{TAny}, BarrierPos: -1},
			{Args: []*Type{TAny, TAny}, Impl: Go(eng.MakeHandler), Returns: []*Type{TAny}, BarrierPos: -1},
		},
	},
}
