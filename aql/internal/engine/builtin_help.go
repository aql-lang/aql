package engine

import (
	"fmt"

	"github.com/metsitaba/voxgig-exp/aql/internal/engine/help"
)

// BuildFuncInfo extracts dynamic signature data from the registry for a word.
func BuildFuncInfo(r *Registry, name string) *help.FuncInfo {
	fn := r.Lookup(name)
	if fn == nil {
		// Check if it's a simple def (not a function)
		if ds := r.DefStacks[name]; len(ds) > 0 {
			return &help.FuncInfo{
				Name:  name,
				Entry: help.Lookup(name),
			}
		}
		return nil
	}

	info := &help.FuncInfo{
		Name:              fn.Name,
		ForwardPrecedence: fn.ForwardPrecedence,
		Entry:             help.Lookup(name),
	}

	for _, sig := range fn.Signatures {
		if sig.Fallback {
			continue
		}
		si := help.SigInfo{}
		for _, t := range sig.Args {
			si.Args = append(si.Args, t.String())
		}
		// Infer return types from the handler by running with zero values
		// is not feasible, so we use the arg types as hints.
		// For now, derive returns from common patterns.
		si.Returns = inferReturns(fn.Name, sig)
		info.Sigs = append(info.Sigs, si)
	}

	return info
}

// inferReturns attempts to determine return types for a signature.
// This uses known patterns for builtin words.
func inferReturns(name string, sig Signature) []string {
	nArgs := len(sig.Args)

	// Known return type patterns for builtin words.
	switch {
	// Binary arithmetic: same type or promotion
	case nArgs == 2 && isArithWord(name):
		return inferArithReturns(name, sig)
	// Unary math
	case nArgs == 1 && isUnaryMathWord(name):
		return inferUnaryMathReturns(name, sig)
	// Comparison
	case nArgs == 2 && isCompareWord(name):
		return []string{"Boolean"}
	// Boolean ops
	case nArgs == 2 && isBoolWord(name):
		return []string{"Boolean"}
	case nArgs == 1 && name == "not":
		return []string{"Boolean"}
	// Stack ops return varying counts, skip
	default:
		return nil
	}
}

func isArithWord(name string) bool {
	switch name {
	case "add", "sub", "mul", "div", "mod", "min", "max", "pow",
		"atan2", "hypot":
		return true
	}
	return false
}

func isUnaryMathWord(name string) bool {
	switch name {
	case "abs", "negate", "sign", "ceil", "floor", "round", "trunc",
		"sqrt", "cbrt", "exp", "log", "log2", "log10",
		"sin", "cos", "tan", "asin", "acos", "atan":
		return true
	}
	return false
}

func isCompareWord(name string) bool {
	switch name {
	case "lt", "gt", "lte", "gte", "eq", "neq", "deq":
		return true
	}
	return false
}

func isBoolWord(name string) bool {
	switch name {
	case "and", "or", "xor", "nand", "implies":
		return true
	}
	return false
}

func inferArithReturns(name string, sig Signature) []string {
	if len(sig.Args) != 2 {
		return nil
	}
	a0 := sig.Args[0].String()
	a1 := sig.Args[1].String()

	// add's Scalar+Scalar → String
	if name == "add" && a0 == "Scalar" && a1 == "Scalar" {
		return []string{"Scalar/String"}
	}

	// Integer+Integer → Integer for most ops
	if a0 == "Scalar/Number/Integer" && a1 == "Scalar/Number/Integer" {
		return []string{"Scalar/Number/Integer"}
	}
	// Any decimal involvement → Decimal
	return []string{"Scalar/Number/Decimal"}
}

func inferUnaryMathReturns(name string, sig Signature) []string {
	a0 := sig.Args[0].String()
	switch name {
	case "abs", "negate":
		return []string{a0} // preserve input type
	case "sign":
		return []string{"Scalar/Number/Integer"}
	case "ceil", "floor", "round", "trunc":
		return []string{"Scalar/Number/Integer"}
	default:
		return []string{"Scalar/Number/Decimal"}
	}
}

func registerHelp(r *Registry) {
	// help: [] -> [] (print self-help)
	selfHandler := func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
		fmt.Fprintln(r.Output, "help — Show help for an AQL word.")
		fmt.Fprintln(r.Output, "")
		fmt.Fprintln(r.Output, "Usage:")
		fmt.Fprintln(r.Output, "  help              Show this message.")
		fmt.Fprintln(r.Output, "  <word> help       Show help for a word (e.g. add help).")
		fmt.Fprintln(r.Output, "  \"<name>\" help     Show help by string name (e.g. \"concat\" help).")
		return nil, nil
	}

	// help: [atom] -> [] or [string] -> []
	wordHandler := func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
		name := valToString(args[0])
		info := BuildFuncInfo(r, name)
		if info == nil {
			fmt.Fprintf(r.Output, "help: no help available for %q\n", name)
			return nil, nil
		}
		fmt.Fprint(r.Output, help.FormatDynamic(*info))
		return nil, nil
	}

	r.Register("help",
		Signature{Args: []Type{TString}, Handler: wordHandler},
		Signature{Args: []Type{TAtom}, Handler: wordHandler},
		Signature{
			Args:      []Type{TAtom},
			QuoteArgs: map[int]bool{0: true},
			Handler:   wordHandler,
		},
		Signature{Args: []Type{}, Handler: selfHandler},
	)
}
