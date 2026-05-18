package specrunner

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/aql-lang/aql/eng/go"
)

// RegisterQFixtures installs the shared `…q` spec-runner fixtures
// (addq, subq, mulq, negq, concatq, describeq, tagq, factq, codeq,
// routeq, tripq, pairq, nilq, flexq, lengthq, firstq, plus the
// boundary-coverage tri*/quad*/quint*/hex*/sept* set) on r.
//
// These are NOT production words — they are dispatch / value / type-
// lattice probes. Both engspec (kernel-only setup) and langspec
// (production-language setup) install them so the moved tsv files
// continue to exercise the same dispatch shapes regardless of which
// runner picks them up.
func RegisterQFixtures(r *eng.Registry) {
	registerArith(r)
	registerStringProbe(r)
	registerDispatch(r)
	registerBarrierArity(r)
	registerListProbes(r)
}

func toFloat(v eng.Value) float64 {
	if v.VType.Matches(eng.TInteger) {
		n, _ := eng.AsInteger(v)
		return float64(n)
	}
	f, _ := eng.AsDecimal(v)
	return f
}

func numericBinary(intOp func(a, b int64) int64, floatOp func(a, b float64) float64) eng.Handler {
	return func(args []eng.Value, _ map[string]eng.Value, _ []eng.Value, _ *eng.Registry) ([]eng.Value, error) {
		if args[0].VType.Matches(eng.TInteger) && args[1].VType.Matches(eng.TInteger) {
			a, _ := eng.AsInteger(args[0])
			b, _ := eng.AsInteger(args[1])
			return []eng.Value{eng.NewInteger(intOp(a, b))}, nil
		}
		return []eng.Value{eng.NewDecimal(floatOp(toFloat(args[0]), toFloat(args[1])))}, nil
	}
}

func registerArith(r *eng.Registry) {
	numberPair := []*eng.Type{eng.TNumber, eng.TNumber}
	r.RegisterNativeFunc(eng.NativeFunc{
		Name: "addq", ForwardArgs: true,
		Signatures: []eng.NativeSig{{
			Args:    numberPair,
			Handler: numericBinary(func(a, b int64) int64 { return b + a }, func(a, b float64) float64 { return b + a }),
			Returns: []*eng.Type{eng.TNumber},
		}},
	})
	r.RegisterNativeFunc(eng.NativeFunc{
		Name: "subq", ForwardArgs: true,
		Signatures: []eng.NativeSig{{
			Args:    numberPair,
			Handler: numericBinary(func(a, b int64) int64 { return b - a }, func(a, b float64) float64 { return b - a }),
			Returns: []*eng.Type{eng.TNumber},
		}},
	})
	r.RegisterNativeFunc(eng.NativeFunc{
		Name: "mulq", ForwardArgs: true,
		Signatures: []eng.NativeSig{{
			Args:    numberPair,
			Handler: numericBinary(func(a, b int64) int64 { return b * a }, func(a, b float64) float64 { return b * a }),
			Returns: []*eng.Type{eng.TNumber},
		}},
	})
	r.RegisterNativeFunc(eng.NativeFunc{
		Name: "negq", ForwardArgs: true,
		Signatures: []eng.NativeSig{{
			Args: []*eng.Type{eng.TNumber}, BarrierPos: 1,
			Handler: func(args []eng.Value, _ map[string]eng.Value, _ []eng.Value, _ *eng.Registry) ([]eng.Value, error) {
				if args[0].VType.Matches(eng.TInteger) {
					n, _ := eng.AsInteger(args[0])
					return []eng.Value{eng.NewInteger(-n)}, nil
				}
				f, _ := eng.AsDecimal(args[0])
				return []eng.Value{eng.NewDecimal(-f)}, nil
			},
			Returns: []*eng.Type{eng.TNumber},
		}},
	})
}

func registerStringProbe(r *eng.Registry) {
	r.RegisterNativeFunc(eng.NativeFunc{
		Name: "concatq", ForwardArgs: true,
		Signatures: []eng.NativeSig{{
			Args: []*eng.Type{eng.TString, eng.TString},
			Handler: func(args []eng.Value, _ map[string]eng.Value, _ []eng.Value, _ *eng.Registry) ([]eng.Value, error) {
				a, _ := eng.AsString(args[0])
				b, _ := eng.AsString(args[1])
				return []eng.Value{eng.NewString(b + a)}, nil
			},
			Returns: []*eng.Type{eng.TString},
		}},
	})
}

func registerDispatch(r *eng.Registry) {
	r.RegisterNativeFunc(eng.NativeFunc{
		Name: "describeq", ForwardArgs: true,
		Signatures: []eng.NativeSig{
			{
				Args: []*eng.Type{eng.TInteger},
				Handler: func(args []eng.Value, _ map[string]eng.Value, _ []eng.Value, _ *eng.Registry) ([]eng.Value, error) {
					n, _ := eng.AsInteger(args[0])
					return []eng.Value{eng.NewString("int:" + strconv.FormatInt(n, 10))}, nil
				},
				Returns: []*eng.Type{eng.TString},
			},
			{
				Args: []*eng.Type{eng.TString},
				Handler: func(args []eng.Value, _ map[string]eng.Value, _ []eng.Value, _ *eng.Registry) ([]eng.Value, error) {
					s, _ := eng.AsString(args[0])
					return []eng.Value{eng.NewString("str:" + s)}, nil
				},
				Returns: []*eng.Type{eng.TString},
			},
		},
	})
	r.RegisterNativeFunc(eng.NativeFunc{
		Name: "tagq", ForwardArgs: true,
		Signatures: []eng.NativeSig{
			{Args: []*eng.Type{eng.TAny}, Handler: func(_ []eng.Value, _ map[string]eng.Value, _ []eng.Value, _ *eng.Registry) ([]eng.Value, error) {
				return []eng.Value{eng.NewString("any")}, nil
			}, Returns: []*eng.Type{eng.TString}},
			{Args: []*eng.Type{eng.TInteger}, Handler: func(_ []eng.Value, _ map[string]eng.Value, _ []eng.Value, _ *eng.Registry) ([]eng.Value, error) {
				return []eng.Value{eng.NewString("specific")}, nil
			}, Returns: []*eng.Type{eng.TString}},
		},
	})
	r.RegisterNativeFunc(eng.NativeFunc{
		Name: "factq", ForwardArgs: true,
		Signatures: []eng.NativeSig{
			{
				Args: []*eng.Type{eng.TInteger}, Patterns: map[int]eng.Value{0: eng.NewInteger(0)},
				Handler: func(_ []eng.Value, _ map[string]eng.Value, _ []eng.Value, _ *eng.Registry) ([]eng.Value, error) {
					return []eng.Value{eng.NewInteger(1)}, nil
				},
				Returns: []*eng.Type{eng.TInteger},
			},
			{
				Args: []*eng.Type{eng.TInteger},
				Handler: func(args []eng.Value, _ map[string]eng.Value, _ []eng.Value, _ *eng.Registry) ([]eng.Value, error) {
					n, _ := eng.AsInteger(args[0])
					return []eng.Value{eng.NewInteger(n)}, nil
				},
				Returns: []*eng.Type{eng.TInteger},
			},
		},
	})
	r.RegisterNativeFunc(eng.NativeFunc{
		Name: "codeq", ForwardArgs: true,
		Signatures: []eng.NativeSig{
			{
				Args: []*eng.Type{eng.TInteger}, Patterns: map[int]eng.Value{0: eng.NewInteger(99)},
				Handler: func(_ []eng.Value, _ map[string]eng.Value, _ []eng.Value, _ *eng.Registry) ([]eng.Value, error) {
					return []eng.Value{eng.NewString("ninety-nine")}, nil
				},
				Returns: []*eng.Type{eng.TString},
			},
			{
				Args: []*eng.Type{eng.TInteger},
				Handler: func(_ []eng.Value, _ map[string]eng.Value, _ []eng.Value, _ *eng.Registry) ([]eng.Value, error) {
					return []eng.Value{eng.NewString("general")}, nil
				},
				Returns: []*eng.Type{eng.TString},
			},
		},
	})
	r.RegisterNativeFunc(eng.NativeFunc{
		Name: "routeq", ForwardArgs: true,
		Signatures: []eng.NativeSig{
			{
				Args: []*eng.Type{eng.TString}, Patterns: map[int]eng.Value{0: eng.NewString("admin")},
				Handler: func(_ []eng.Value, _ map[string]eng.Value, _ []eng.Value, _ *eng.Registry) ([]eng.Value, error) {
					return []eng.Value{eng.NewString("matched-admin")}, nil
				},
				Returns: []*eng.Type{eng.TString},
			},
			{
				Args: []*eng.Type{eng.TString},
				Handler: func(_ []eng.Value, _ map[string]eng.Value, _ []eng.Value, _ *eng.Registry) ([]eng.Value, error) {
					return []eng.Value{eng.NewString("other")}, nil
				},
				Returns: []*eng.Type{eng.TString},
			},
		},
	})
}

func registerBarrierArity(r *eng.Registry) {
	r.RegisterNativeFunc(eng.NativeFunc{
		Name: "tripq", ForwardArgs: true,
		Signatures: []eng.NativeSig{{
			Args: []*eng.Type{eng.TInteger, eng.TInteger, eng.TInteger},
			Handler: func(args []eng.Value, _ map[string]eng.Value, _ []eng.Value, _ *eng.Registry) ([]eng.Value, error) {
				a, _ := eng.AsInteger(args[0])
				b, _ := eng.AsInteger(args[1])
				c, _ := eng.AsInteger(args[2])
				return []eng.Value{eng.NewString(fmt.Sprintf("%d,%d,%d", a, b, c))}, nil
			},
			Returns: []*eng.Type{eng.TString},
		}},
	})
	r.RegisterNativeFunc(eng.NativeFunc{
		Name: "pairq", ForwardArgs: true,
		Signatures: []eng.NativeSig{{
			Args:       []*eng.Type{eng.TInteger, eng.TInteger},
			BarrierPos: 1,
			Handler: func(args []eng.Value, _ map[string]eng.Value, _ []eng.Value, _ *eng.Registry) ([]eng.Value, error) {
				a, _ := eng.AsInteger(args[0])
				b, _ := eng.AsInteger(args[1])
				return []eng.Value{eng.NewString(fmt.Sprintf("%d:%d", a, b))}, nil
			},
			Returns: []*eng.Type{eng.TString},
		}},
	})
	r.RegisterNativeFunc(eng.NativeFunc{
		Name: "nilq",
		Signatures: []eng.NativeSig{{
			Args: []*eng.Type{},
			Handler: func(_ []eng.Value, _ map[string]eng.Value, _ []eng.Value, _ *eng.Registry) ([]eng.Value, error) {
				return []eng.Value{eng.NewString("nil")}, nil
			},
			Returns: []*eng.Type{eng.TString},
		}},
	})
	r.RegisterNativeFunc(eng.NativeFunc{
		Name: "flexq", ForwardArgs: true,
		Signatures: []eng.NativeSig{
			{
				Args: []*eng.Type{eng.TInteger},
				Handler: func(args []eng.Value, _ map[string]eng.Value, _ []eng.Value, _ *eng.Registry) ([]eng.Value, error) {
					a, _ := eng.AsInteger(args[0])
					return []eng.Value{eng.NewString(fmt.Sprintf("one:%d", a))}, nil
				},
				Returns: []*eng.Type{eng.TString},
			},
			{
				Args: []*eng.Type{eng.TInteger, eng.TInteger},
				Handler: func(args []eng.Value, _ map[string]eng.Value, _ []eng.Value, _ *eng.Registry) ([]eng.Value, error) {
					a, _ := eng.AsInteger(args[0])
					b, _ := eng.AsInteger(args[1])
					return []eng.Value{eng.NewString(fmt.Sprintf("two:%d,%d", a, b))}, nil
				},
				Returns: []*eng.Type{eng.TString},
			},
		},
	})

	intArgsFmt := func(args []eng.Value, _ map[string]eng.Value, _ []eng.Value, _ *eng.Registry) ([]eng.Value, error) {
		parts := make([]string, len(args))
		for i, a := range args {
			n, _ := eng.AsInteger(a)
			parts[i] = strconv.FormatInt(n, 10)
		}
		return []eng.Value{eng.NewString(strings.Join(parts, ","))}, nil
	}
	intArity := func(name string, n, barrier int) {
		args := make([]*eng.Type, n)
		for i := range args {
			args[i] = eng.TInteger
		}
		r.RegisterNativeFunc(eng.NativeFunc{
			Name: name, ForwardArgs: true,
			Signatures: []eng.NativeSig{{
				Args: args, BarrierPos: barrier,
				Handler: intArgsFmt,
				Returns: []*eng.Type{eng.TString},
			}},
		})
	}
	intArity("tri1q", 3, 1)
	intArity("tri2q", 3, 2)
	intArity("quad1q", 4, 1)
	intArity("quadq", 4, 2)
	intArity("quad3q", 4, 3)
	intArity("quintq", 5, 3)
	intArity("hexq", 6, 3)
	intArity("septq", 7, 4)
}

func registerListProbes(r *eng.Registry) {
	r.RegisterNativeFunc(eng.NativeFunc{
		Name: "lengthq", ForwardArgs: true,
		Signatures: []eng.NativeSig{{
			Args: []*eng.Type{eng.TList},
			Handler: func(args []eng.Value, _ map[string]eng.Value, _ []eng.Value, _ *eng.Registry) ([]eng.Value, error) {
				lst, _ := eng.AsList(args[0])
				return []eng.Value{eng.NewInteger(int64(lst.Len()))}, nil
			},
			Returns: []*eng.Type{eng.TInteger},
		}},
	})
	r.RegisterNativeFunc(eng.NativeFunc{
		Name: "firstq", ForwardArgs: true,
		Signatures: []eng.NativeSig{{
			Args: []*eng.Type{eng.TList},
			Handler: func(args []eng.Value, _ map[string]eng.Value, _ []eng.Value, _ *eng.Registry) ([]eng.Value, error) {
				lst, _ := eng.AsList(args[0])
				if lst.Len() == 0 {
					return []eng.Value{eng.NewNone()}, nil
				}
				return []eng.Value{lst.Get(0)}, nil
			},
			Returns: []*eng.Type{eng.TAny},
		}},
	})
}
