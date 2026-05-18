package native

import "github.com/aql-lang/aql/eng/go"

// comparisonNatives is the consolidated set of comparison words —
// lt / gt / lte / gte / eq / neq / deq — plus the closed-interval
// DepScalar constructor `between`. Each comparison word also accepts
// a `Type N` form that builds a DepScalar refinement of the named
// scalar type (`Integer lt 10`, `Integer between 0 100`, …).
//
// Argument convention follows the b-op-a mirror rule:
//
//	a b lt     → args[0]=b args[1]=a → compare(a, b) → a < b
//	10 lt 3    → infix reading: 10 < 3 → false
//
// Algorithms (LtHandler / GtHandler / EqHandler / DeqHandler /
// CompareValues / MakeDepScalarSig / BetweenHandler) live in eng;
// this file owns the word names and dispatch wiring.
var comparisonNatives = []NativeFunc{
	{
		Name:        "lt",
		ForwardArgs: true,
		Signatures: []NativeSig{
			eng.MakeDepScalarSig("lt", eng.DepLT),
			{
				Args:    []*Type{TAny, TAny},
				Handler: eng.LtHandler,
				Returns: []*Type{TBoolean},
			},
		},
	},
	{
		Name:        "gt",
		ForwardArgs: true,
		Signatures: []NativeSig{
			eng.MakeDepScalarSig("gt", eng.DepGT),
			{
				Args:    []*Type{TAny, TAny},
				Handler: eng.GtHandler,
				Returns: []*Type{TBoolean},
			},
		},
	},
	{
		Name:        "lte",
		ForwardArgs: true,
		Signatures: []NativeSig{
			eng.MakeDepScalarSig("lte", eng.DepLTE),
			{
				Args:    []*Type{TAny, TAny},
				Handler: eng.LteHandler,
				Returns: []*Type{TBoolean},
			},
		},
	},
	{
		Name:        "gte",
		ForwardArgs: true,
		Signatures: []NativeSig{
			eng.MakeDepScalarSig("gte", eng.DepGTE),
			{
				Args:    []*Type{TAny, TAny},
				Handler: eng.GteHandler,
				Returns: []*Type{TBoolean},
			},
		},
	},
	{
		Name:        "between",
		ForwardArgs: true,
		Signatures: []NativeSig{{
			Args:           []*Type{TScalar, TScalar, TScalarType},
			Handler:        eng.BetweenHandler,
			Returns:        []*Type{TDependent},
			RunInCheckMode: true,
		}},
	},
	{
		Name:        "eq",
		ForwardArgs: true,
		Signatures: []NativeSig{{
			Args:    []*Type{TAny, TAny},
			Handler: eng.EqHandler,
			Returns: []*Type{TBoolean},
		}},
	},
	{
		Name:        "neq",
		ForwardArgs: true,
		Signatures: []NativeSig{{
			Args:    []*Type{TAny, TAny},
			Handler: eng.NeqHandler,
			Returns: []*Type{TBoolean},
		}},
	},
	{
		Name:        "deq",
		ForwardArgs: true,
		Signatures: []NativeSig{{
			Args:    []*Type{TAny, TAny},
			Handler: eng.DeqHandler,
			Returns: []*Type{TBoolean},
		}},
	},
}
