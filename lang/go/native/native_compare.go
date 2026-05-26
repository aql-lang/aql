package native

import "github.com/aql-lang/aql/eng/go"

// comparisonNatives is the consolidated set of comparison words —
// lt / gt / lte / gte / cmp / eq / neq / deq — plus the closed-
// interval DepScalar constructor `between`. The ordering words
// lt / gt / lte / gte also accept a `Type N` form that builds a
// DepScalar refinement of the named scalar type (`Integer lt 10`,
// `Integer between 0 100`, …).
//
// Argument convention follows the b-op-a mirror rule:
//
//	a b lt     → args[0]=b args[1]=a → compare(a, b) → a < b
//	10 lt 3    → infix reading: 10 < 3 → false
//	a b cmp    → -1 / 0 / 1 for a sorting before / with / after b
//
// Algorithms (LtHandler / GtHandler / CmpHandler / EqHandler /
// DeqHandler / CompareValues / MakeDepScalarSig / BetweenHandler)
// live in eng; this file owns the word names and dispatch wiring.
var comparisonNatives = []NativeFunc{
	{
		Name: "lt",

		Signatures: []NativeSig{
			eng.MakeDepScalarSig("lt", eng.DepLT),
			{
				Args:    []*Type{TAny, TAny},
				Handler: eng.LtHandler,
				Returns: []*Type{TBoolean}, BarrierPos: -1,
			},
		},
	},
	{
		Name: "gt",

		Signatures: []NativeSig{
			eng.MakeDepScalarSig("gt", eng.DepGT),
			{
				Args:    []*Type{TAny, TAny},
				Handler: eng.GtHandler,
				Returns: []*Type{TBoolean}, BarrierPos: -1,
			},
		},
	},
	{
		Name: "lte",

		Signatures: []NativeSig{
			eng.MakeDepScalarSig("lte", eng.DepLTE),
			{
				Args:    []*Type{TAny, TAny},
				Handler: eng.LteHandler,
				Returns: []*Type{TBoolean}, BarrierPos: -1,
			},
		},
	},
	{
		Name: "gte",

		Signatures: []NativeSig{
			eng.MakeDepScalarSig("gte", eng.DepGTE),
			{
				Args:    []*Type{TAny, TAny},
				Handler: eng.GteHandler,
				Returns: []*Type{TBoolean}, BarrierPos: -1,
			},
		},
	},
	{
		// cmp is the three-way comparison: `a b cmp` yields the
		// Integer -1, 0, or 1 — the raw ordering CompareValues
		// computes for lt / gt / sort, surfaced as a value.
		Name: "cmp",

		Signatures: []NativeSig{{
			Args:    []*Type{TAny, TAny},
			Handler: eng.CmpHandler,
			Returns: []*Type{TInteger}, BarrierPos: -1,
		}},
	},
	{
		Name: "between",

		Signatures: []NativeSig{{
			Args:           []*Type{TScalar, TScalar, TScalar},
			TypeArgs:       map[int]bool{2: true},
			Handler:        eng.BetweenHandler,
			Returns:        []*Type{TScalar},
			RunInCheckMode: true, BarrierPos: -1,
		}},
	},
	{
		Name: "eq",

		Signatures: []NativeSig{{
			Args:    []*Type{TAny, TAny},
			Handler: eng.EqHandler,
			Returns: []*Type{TBoolean}, BarrierPos: -1,
		}},
	},
	{
		Name: "neq",

		Signatures: []NativeSig{{
			Args:    []*Type{TAny, TAny},
			Handler: eng.NeqHandler,
			Returns: []*Type{TBoolean}, BarrierPos: -1,
		}},
	},
	{
		Name: "deq",

		Signatures: []NativeSig{{
			Args:    []*Type{TAny, TAny},
			Handler: eng.DeqHandler,
			Returns: []*Type{TBoolean}, BarrierPos: -1,
		}},
	},
}
