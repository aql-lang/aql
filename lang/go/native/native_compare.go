package native

import "github.com/aql-lang/aql/eng/go"

// comparisonNatives is the consolidated set of comparison words —
// lt / gt / lte / gte / cmp / tcmp / eq / neq / deq — plus the closed-
// interval DepScalar constructor `between`. The ordering words
// lt / gt / lte / gte also accept a `Type N` form that builds a
// DepScalar refinement of the named scalar type (`Integer lt 10`,
// `Integer between 0 100`, …).
//
// The ordering words (cmp / lt / lte / gt / gte) are FAMILY-RESTRICTED:
// they compare only same-type values, or values a shared same-family
// comparer can handle (Integer-vs-Float, two Dates, …). A cross-family
// pair (Integer-vs-String, List-vs-Map) raises [aql/incomparable]. The
// total order over ALL values — what sort and the collection words use —
// is surfaced by `tcmp`, which compares anything. Equality words
// (eq / neq / deq) stay total: cross-type compares as not-equal, never
// an error.
//
// Argument convention follows the b-op-a mirror rule:
//
//	a b lt     → args[0]=b args[1]=a → compare(a, b) → a < b
//	10 lt 3    → infix reading: 10 < 3 → false
//	a b cmp    → -1 / 0 / 1 for a sorting before / with / after b
//	a b tcmp   → same, across any types (unrestricted total order)
//
// Algorithms (LtHandler / GtHandler / CmpHandler / TcmpHandler /
// EqHandler / DeqHandler / CompareValues / MakeDepScalarSig /
// BetweenHandler) live in eng; this file owns the word names and
// dispatch wiring.
var comparisonNatives = []NativeFunc{
	{
		Name: "lt",

		Signatures: []NativeSig{
			eng.MakeDepScalarSig("lt", eng.DepLT),
			{
				Args:      []*Type{TAny, TAny},
				Handler:   eng.LtHandler,
				Returns:   []*Type{TBoolean},
				ReturnsFn: eng.OrderingReturnsFn(eng.LtHandler, TBoolean), BarrierPos: -1,
			},
		},
	},
	{
		Name: "gt",

		Signatures: []NativeSig{
			eng.MakeDepScalarSig("gt", eng.DepGT),
			{
				Args:      []*Type{TAny, TAny},
				Handler:   eng.GtHandler,
				Returns:   []*Type{TBoolean},
				ReturnsFn: eng.OrderingReturnsFn(eng.GtHandler, TBoolean), BarrierPos: -1,
			},
		},
	},
	{
		Name: "lte",

		Signatures: []NativeSig{
			eng.MakeDepScalarSig("lte", eng.DepLTE),
			{
				Args:      []*Type{TAny, TAny},
				Handler:   eng.LteHandler,
				Returns:   []*Type{TBoolean},
				ReturnsFn: eng.OrderingReturnsFn(eng.LteHandler, TBoolean), BarrierPos: -1,
			},
		},
	},
	{
		Name: "gte",

		Signatures: []NativeSig{
			eng.MakeDepScalarSig("gte", eng.DepGTE),
			{
				Args:      []*Type{TAny, TAny},
				Handler:   eng.GteHandler,
				Returns:   []*Type{TBoolean},
				ReturnsFn: eng.OrderingReturnsFn(eng.GteHandler, TBoolean), BarrierPos: -1,
			},
		},
	},
	{
		// cmp is the three-way comparison: `a b cmp` yields the
		// Integer -1, 0, or 1 for a sorting before / with / after b.
		// Family-restricted — cross-family operands raise
		// [aql/incomparable]; use tcmp for a cross-type total order.
		Name: "cmp",

		Signatures: []NativeSig{{
			Args:      []*Type{TAny, TAny},
			Handler:   eng.CmpHandler,
			Returns:   []*Type{TInteger},
			ReturnsFn: eng.OrderingReturnsFn(eng.CmpHandler, TInteger), BarrierPos: -1,
		}},
	},
	{
		// tcmp is cmp without the family restriction: `a b tcmp`
		// yields -1 / 0 / 1 for ANY two values, via the unified
		// lattice total order (the order sort and the collection
		// words use). Reach for it when you want cross-type ordering
		// that cmp refuses, e.g. `1 tcmp "a"`.
		Name:          "tcmp",
		CompileEffect: CompileIslandPure,

		Signatures: []NativeSig{{
			Args:    []*Type{TAny, TAny},
			Handler: eng.TcmpHandler,
			Returns: []*Type{TInteger}, BarrierPos: -1,
			CompileEffect: CompileReadsFn, // type-algebra reads fn-value types, never invokes
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
