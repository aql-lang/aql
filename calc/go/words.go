package calc

import (
	"fmt"
	"io"
	"math"

	"github.com/aql-lang/aql/eng/go"
)

// RegisterWords installs the calculator's word vocabulary on r. The set is
// deliberately small and self-contained: every word is defined here, with
// no dependency on eng's optional core-word library. This proves that the
// eng kernel — Registry + dispatch + parser + signature matching — is
// usable as a host library by a module that brings its own words.
//
// out is where `print` and `show` write their output; pass os.Stdout for
// the CLI or a *bytes.Buffer in tests.
func RegisterWords(r *eng.Registry, out io.Writer) {
	registerArith(r)
	registerUnary(r)
	registerConstants(r)
	registerStackOps(r)
	registerDisplay(r, out)
}

// numHandler runs op on two numeric args (Integer or Decimal). If either
// arg is Decimal the result is Decimal; otherwise Integer division falls
// back to Decimal when op signals fractional output. div is the one
// exception: it always returns Decimal so 1 div 2 = 0.5 rather than 0.
func numHandler(op func(a, b float64) (float64, error), preferInt bool) eng.Handler {
	return func(args []eng.Value, _ map[string]eng.Value, _ []eng.Value, _ *eng.Registry) ([]eng.Value, error) {
		a, err := eng.AsNumber(args[0])
		if err != nil {
			return nil, err
		}
		b, err := eng.AsNumber(args[1])
		if err != nil {
			return nil, err
		}
		res, err := op(a, b)
		if err != nil {
			return nil, err
		}
		if preferInt &&
			args[0].Parent.Matches(eng.TInteger) &&
			args[1].Parent.Matches(eng.TInteger) &&
			res == math.Trunc(res) &&
			!math.IsInf(res, 0) &&
			!math.IsNaN(res) {
			return []eng.Value{eng.NewInteger(int64(res))}, nil
		}
		return []eng.Value{eng.NewDecimal(res)}, nil
	}
}

func registerArith(r *eng.Registry) {
	// Args are [a b] in surface order ("a sub b" => sig=[b,a]); to match
	// the engine convention every binary handler computes args[1] op args[0]
	// so the swap form reads naturally. See lang/CLAUDE.md "Non-commutative
	// two-arg sanity check" for the reasoning.
	bin := func(name string, op func(a, b float64) (float64, error), preferInt bool) {
		h := numHandler(op, preferInt)
		r.RegisterNativeFunc(eng.NativeFunc{
			Name:        name,
			ForwardArgs: true,
			Signatures: []eng.NativeSig{
				{Args: []*eng.Type{eng.TNumber, eng.TNumber}, Handler: h, Returns: []*eng.Type{eng.TNumber}},
			},
		})
	}
	bin("add", func(a, b float64) (float64, error) { return b + a, nil }, true)
	bin("sub", func(a, b float64) (float64, error) { return b - a, nil }, true)
	bin("mul", func(a, b float64) (float64, error) { return b * a, nil }, true)
	bin("div", func(a, b float64) (float64, error) {
		if a == 0 {
			return 0, fmt.Errorf("div: division by zero")
		}
		return b / a, nil
	}, false)
	bin("mod", func(a, b float64) (float64, error) {
		if a == 0 {
			return 0, fmt.Errorf("mod: modulo by zero")
		}
		return math.Mod(b, a), nil
	}, true)
	bin("pow", func(a, b float64) (float64, error) {
		return math.Pow(b, a), nil
	}, true)
}

func registerUnary(r *eng.Registry) {
	unary := func(name string, op func(float64) (float64, error), preferInt bool) {
		h := func(args []eng.Value, _ map[string]eng.Value, _ []eng.Value, _ *eng.Registry) ([]eng.Value, error) {
			x, err := eng.AsNumber(args[0])
			if err != nil {
				return nil, err
			}
			res, err := op(x)
			if err != nil {
				return nil, err
			}
			if preferInt && args[0].Parent.Matches(eng.TInteger) && res == math.Trunc(res) {
				return []eng.Value{eng.NewInteger(int64(res))}, nil
			}
			return []eng.Value{eng.NewDecimal(res)}, nil
		}
		r.RegisterNativeFunc(eng.NativeFunc{
			Name:        name,
			ForwardArgs: true,
			Signatures: []eng.NativeSig{
				{Args: []*eng.Type{eng.TNumber}, Handler: h, Returns: []*eng.Type{eng.TNumber}},
			},
		})
	}
	unary("neg", func(x float64) (float64, error) { return -x, nil }, true)
	unary("abs", func(x float64) (float64, error) { return math.Abs(x), nil }, true)
	unary("sqrt", func(x float64) (float64, error) {
		if x < 0 {
			return 0, fmt.Errorf("sqrt: negative input")
		}
		return math.Sqrt(x), nil
	}, false)
}

func registerConstants(r *eng.Registry) {
	push := func(name string, v eng.Value) {
		r.RegisterNativeFunc(eng.NativeFunc{
			Name: name,
			Signatures: []eng.NativeSig{{
				Handler: func(_ []eng.Value, _ map[string]eng.Value, _ []eng.Value, _ *eng.Registry) ([]eng.Value, error) {
					return []eng.Value{v}, nil
				},
				Returns: []*eng.Type{eng.TNumber},
			}},
		})
	}
	push("pi", eng.NewDecimal(math.Pi))
	push("e", eng.NewDecimal(math.E))
}

func registerStackOps(r *eng.Registry) {
	// dup / drop / swap / over operate on the full stack via FullStack
	// signatures. Calc keeps its stack vocabulary small — the production
	// language layer in lang/ has the full Forth-style set.
	full := func(name string, n int, fn func(stk []eng.Value) ([]eng.Value, error)) {
		r.RegisterNativeFunc(eng.NativeFunc{
			Name: name,
			Signatures: []eng.NativeSig{{
				FullStack: true,
				Handler: func(_ []eng.Value, _ map[string]eng.Value, stk []eng.Value, _ *eng.Registry) ([]eng.Value, error) {
					if len(stk) < n {
						return nil, fmt.Errorf("%s: needs %d items, stack has %d", name, n, len(stk))
					}
					return fn(stk)
				},
				Returns: []*eng.Type{},
			}},
		})
	}
	full("dup", 1, func(stk []eng.Value) ([]eng.Value, error) {
		return append(append([]eng.Value{}, stk...), stk[len(stk)-1]), nil
	})
	full("drop", 1, func(stk []eng.Value) ([]eng.Value, error) {
		return append([]eng.Value{}, stk[:len(stk)-1]...), nil
	})
	full("swap", 2, func(stk []eng.Value) ([]eng.Value, error) {
		out := append([]eng.Value{}, stk...)
		i := len(out) - 1
		out[i], out[i-1] = out[i-1], out[i]
		return out, nil
	})
	full("over", 2, func(stk []eng.Value) ([]eng.Value, error) {
		return append(append([]eng.Value{}, stk...), stk[len(stk)-2]), nil
	})
	full("clear", 0, func(_ []eng.Value) ([]eng.Value, error) {
		return []eng.Value{}, nil
	})
	r.RegisterNativeFunc(eng.NativeFunc{
		Name: "depth",
		Signatures: []eng.NativeSig{{
			FullStack: true,
			Handler: func(_ []eng.Value, _ map[string]eng.Value, stk []eng.Value, _ *eng.Registry) ([]eng.Value, error) {
				out := append([]eng.Value{}, stk...)
				return append(out, eng.NewInteger(int64(len(stk)))), nil
			},
			Returns: []*eng.Type{eng.TInteger},
		}},
	})
}

func registerDisplay(r *eng.Registry, out io.Writer) {
	if out == nil {
		out = io.Discard
	}
	// print — consume the top value and write its String() representation
	// followed by a newline. Returns nothing.
	r.RegisterNativeFunc(eng.NativeFunc{
		Name: "print",
		Signatures: []eng.NativeSig{{
			Args: []*eng.Type{eng.TAny},
			Handler: func(args []eng.Value, _ map[string]eng.Value, _ []eng.Value, _ *eng.Registry) ([]eng.Value, error) {
				fmt.Fprintln(out, args[0].String())
				return nil, nil
			},
			Returns: []*eng.Type{},
		}},
	})
	// show — write the full stack without consuming it. The output is one
	// space-separated line followed by a newline, matching the REPL's
	// end-of-line printout for non-interactive inspection.
	r.RegisterNativeFunc(eng.NativeFunc{
		Name: "show",
		Signatures: []eng.NativeSig{{
			FullStack: true,
			Handler: func(_ []eng.Value, _ map[string]eng.Value, stk []eng.Value, _ *eng.Registry) ([]eng.Value, error) {
				parts := make([]string, len(stk))
				for i, v := range stk {
					parts[i] = v.String()
				}
				if len(parts) == 0 {
					fmt.Fprintln(out, "(empty)")
				} else {
					for i, p := range parts {
						if i > 0 {
							fmt.Fprint(out, " ")
						}
						fmt.Fprint(out, p)
					}
					fmt.Fprintln(out)
				}
				return append([]eng.Value{}, stk...), nil
			},
			Returns: []*eng.Type{},
		}},
	})
}
