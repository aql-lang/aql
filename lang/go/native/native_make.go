package native

import "github.com/aql-lang/aql/eng/go"

// makeNatives installs the `make` word — the universal constructor
// for typed values (scalars, objects, records, paths, arrays).
//
//	make T data       — build a T-typed value from data; T may be a
//	                    Scalar type, an ObjectType, a RecordType, an
//	                    Array type, or any subtype thereof.
//	make T data opts  — same with an options map (currently the
//	                    `use_base:true` flag for objects/records).
//	make T data Proto — for an Object target: build the instance with
//	                    Proto's field values as the starting point.
//
// The algorithm primitives (MakeHandler, MakeScalarHandler,
// MakeScalarOptsHandler, MakeObjHandler, MakeArrayHandler,
// MakeWithPrototype, MakeWithOpts, plus MakeObject / MakeConvert /
// MakeFieldValue / ResolveFieldType) live in eng/go/core_make.go;
// this file owns the word name, signature shape, and dispatch
// wiring.
var makeNatives = []NativeFunc{
	{
		Name:        "make",
		ForwardArgs: true,
		Signatures: []NativeSig{
			{Args: []*Type{TScalar, TMap, TAny}, TypeArgs: map[int]bool{0: true}, Handler: eng.MakeScalarOptsHandler, ReturnsFn: ReturnsIdentity(0), BarrierPos: -1},
			{Args: []*Type{TIdeal, TMap}, TypeArgs: map[int]bool{0: true}, Handler: eng.MakeObjHandler, ReturnsFn: ReturnsIdentity(0), BarrierPos: -1},
			{Args: []*Type{TArray, TList}, Handler: eng.MakeArrayHandler, Returns: []*Type{TArray}, BarrierPos: -1},
			{Args: []*Type{TScalar, TAny}, TypeArgs: map[int]bool{0: true}, Handler: eng.MakeScalarHandler, ReturnsFn: ReturnsIdentity(0), BarrierPos: -1},
			{Args: []*Type{TObject, TAny, TObject}, Handler: eng.MakeWithPrototype, Returns: []*Type{TObject}, BarrierPos: -1},
			{Args: []*Type{TAny, TAny, TMap}, Handler: eng.MakeWithOpts, Returns: []*Type{TAny}, BarrierPos: -1},
			{Args: []*Type{TAny, TAny}, Handler: eng.MakeHandler, Returns: []*Type{TAny}, BarrierPos: -1},
		},
	},
}
