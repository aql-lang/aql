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
		Name:          "lt",
		CompileEffect: CompileScalarFold,

		Signatures: []Signature{
			eng.MakeDepScalarSig("lt", eng.DepLT),
			{
				Args:      []*Type{TAny, TAny},
				Impl:      Go(eng.LtHandler),
				Returns:   []*Type{TBoolean},
				ReturnsFn: eng.OrderingReturnsFn(eng.LtHandler, TBoolean), BarrierPos: -1,
			},
		},
	},
	{
		Name:          "gt",
		CompileEffect: CompileScalarFold,

		Signatures: []Signature{
			eng.MakeDepScalarSig("gt", eng.DepGT),
			{
				Args:      []*Type{TAny, TAny},
				Impl:      Go(eng.GtHandler),
				Returns:   []*Type{TBoolean},
				ReturnsFn: eng.OrderingReturnsFn(eng.GtHandler, TBoolean), BarrierPos: -1,
			},
		},
	},
	{
		Name:          "lte",
		CompileEffect: CompileScalarFold,

		Signatures: []Signature{
			eng.MakeDepScalarSig("lte", eng.DepLTE),
			{
				Args:      []*Type{TAny, TAny},
				Impl:      Go(eng.LteHandler),
				Returns:   []*Type{TBoolean},
				ReturnsFn: eng.OrderingReturnsFn(eng.LteHandler, TBoolean), BarrierPos: -1,
			},
		},
	},
	{
		Name:          "gte",
		CompileEffect: CompileScalarFold,

		Signatures: []Signature{
			eng.MakeDepScalarSig("gte", eng.DepGTE),
			{
				Args:      []*Type{TAny, TAny},
				Impl:      Go(eng.GteHandler),
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
		Name:          "cmp",
		CompileEffect: CompileScalarFold,

		Signatures: []Signature{{
			Args:      []*Type{TAny, TAny},
			Impl:      Go(eng.CmpHandler),
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
		CompileEffect: CompileIslandPure | CompileScalarFold,

		Signatures: []Signature{{
			Args:    []*Type{TAny, TAny},
			Impl:    Go(eng.TcmpHandler),
			Returns: []*Type{TInteger}, BarrierPos: -1,
			CompileEffect: CompileReadsFn, // type-algebra reads fn-value types, never invokes
		}},
	},
	{
		Name: "between",

		Signatures: []Signature{{
			Args:       []*Type{TScalar, TScalar, TScalar},
			TypeArgs:   map[int]bool{2: true},
			Impl:       Go(eng.BetweenHandler, RunInCheck()),
			Returns:    []*Type{TScalar},
			BarrierPos: -1,
		}},
	},
	{
		Name:          "eq",
		CompileEffect: CompileScalarFold,

		Signatures: []Signature{{
			Args:    []*Type{TAny, TAny},
			Impl:    Go(eng.EqHandler),
			Returns: []*Type{TBoolean}, BarrierPos: -1,
		}},
	},
	{
		Name:          "neq",
		CompileEffect: CompileScalarFold,

		Signatures: []Signature{{
			Args:    []*Type{TAny, TAny},
			Impl:    Go(eng.NeqHandler),
			Returns: []*Type{TBoolean}, BarrierPos: -1,
		}},
	},
	{
		Name:          "deq",
		CompileEffect: CompileScalarFold,

		Signatures: []Signature{{
			Args:    []*Type{TAny, TAny},
			Impl:    Go(eng.DeqHandler),
			Returns: []*Type{TBoolean}, BarrierPos: -1,
		}},
	},
}
