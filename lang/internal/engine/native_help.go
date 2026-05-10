package engine

import (
	"io"
	"strings"

	"github.com/metsitaba/voxgig-exp/lang/internal/engine/help"
)

// EnableDynamicHelp sets up the OnRegisterHook so that functions
// registered after MarkReady() get their help examples computed
// dynamically. Call this after initial setup and ParseFunc are ready.
func EnableDynamicHelp(r *Registry) {
	r.OnRegisterHook = func(name string) {
		info := BuildFuncInfo(r, name)
		if info == nil {
			return
		}
		eval := makeDynamicEval(r)
		if eval == nil {
			return
		}
		help.GenerateDynamicExamples(*info, eval)
	}
}

// makeDynamicEval returns a function that parses and evaluates an AQL
// expression, returning the formatted result. Returns nil if ParseFunc
// is not set.
func makeDynamicEval(r *Registry) func(string) (string, error) {
	if r.ParseFunc == nil {
		return nil
	}
	return func(expr string) (string, error) {
		vals, err := r.ParseFunc(expr)
		if err != nil {
			return "", err
		}
		savedOut := r.Output
		r.Output = io.Discard
		defer func() { r.Output = savedOut }()

		eng := NewTop(r)
		result, err := eng.Run(vals)
		if err != nil {
			return "", err
		}
		var parts []string
		for _, v := range result {
			parts = append(parts, v.String())
		}
		return strings.Join(parts, " "), nil
	}
}

// BuildFuncInfo extracts dynamic signature data from the registry for a word.
func BuildFuncInfo(r *Registry, name string) *help.FuncInfo {
	fn := r.Lookup(name)
	if fn == nil {
		// Check if it's a simple def (not a function)
		if r.HasDef(name) {
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
		si := help.SigInfo{BarrierPos: sig.BarrierPos}
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
// Uses known patterns for builtin words.
func inferReturns(name string, sig Signature) []string {
	nArgs := len(sig.Args)

	// Exact overrides first (word → return types per sig shape).
	if ret := inferExact(name, sig); ret != nil {
		return ret
	}

	// Category-based inference.
	switch {
	case nArgs == 2 && isArithWord(name):
		return inferArithReturns(name, sig)
	case nArgs == 1 && isUnaryMathWord(name):
		return inferUnaryMathReturns(name, sig)
	case nArgs == 2 && isCompareWord(name):
		return []string{"Scalar/Boolean"}
	case nArgs == 2 && isBoolWord(name):
		return []string{"Scalar/Boolean"}
	case nArgs == 1 && name == "not":
		return []string{"Scalar/Boolean"}
	}
	return nil
}

// inferExact handles words with specific, known return types.
func inferExact(name string, sig Signature) []string {
	nArgs := len(sig.Args)
	switch name {
	// String ops
	case "upper", "lower":
		return []string{"Scalar/String"}
	case "concat":
		return []string{"Scalar/String"}
	case "split":
		return []string{"Node/List"}
	case "trim", "changecase", "normalize", "escape", "repeat", "pad", "replace":
		return []string{"Scalar/String"}
	case "contains":
		return []string{"Scalar/Boolean"}
	case "indexof":
		return []string{"Scalar/Number/Integer"}
	case "match":
		return []string{"Node/Map"}
	case "slice":
		if nArgs > 0 {
			last := sig.Args[nArgs-1].String()
			if last == "Node/List" {
				return []string{"Node/List"}
			}
		}
		return []string{"Scalar/String"}

	// Type ops
	case "typeof", "fulltypeof":
		return []string{"Scalar/Atom"}
	case "is":
		return []string{"Scalar/Boolean"}
	case "inspect":
		return []string{"Node/Map"}
	case "convert":
		return []string{"Scalar"}
	case "base":
		return []string{"Any"}
	case "record":
		return []string{"Object/Record"}
	case "table":
		return []string{"Object/Table"}
	case "object":
		return []string{"Object"}
	case "make":
		return []string{"Any"}

	// Storage
	case "set", "context-set":
		return nil // no return
	case "get", "context-get":
		return []string{"Any"}

	// Definition
	case "def", "undef", "type":
		return nil
	case "fn":
		return []string{"Word/Function"}
	case "call":
		return []string{"Any"}
	case "args":
		return []string{"Node/List"}
	case "var":
		return []string{"Any"}

	// Control flow
	case "do":
		return []string{"Any"}
	case "if":
		return []string{"Any"}
	case "for":
		return []string{"Any"}
	case "quote":
		return []string{"Any"}
	case "error":
		return []string{"Any"}

	// Accessors
	case "getr", "!.":
		return []string{"Any"}

	// I/O
	case "print", "printstr":
		return nil
	case "read":
		return []string{"Any"}
	case "write":
		return []string{"Scalar/String"}
	case "trace":
		return []string{"Any"}
	case "stdin", "stdout", "stderr":
		return []string{"Scalar/String"}

	// Module
	case "module":
		return []string{"Word/__MD"}
	case "import":
		return nil

	// Unify
	case "unify":
		return []string{"Scalar/String", "Scalar/Boolean"}

	// or/and short-circuit, returning the winning operand. The
	// [Boolean, Boolean] sig keeps the result narrowed to Boolean;
	// the [Any, Any] coerce sig returns the operand value as-is.
	case "or", "and":
		if nArgs == 2 && sig.Args[0].String() == "Any" {
			return []string{"Any"}
		}
		return []string{"Scalar/Boolean"}

	// Type union: tor builds a disjunct from any two values.
	case "tor":
		return []string{"Any"}

	// Type conjunction: tand merges/unifies two values.
	case "tand":
		return []string{"Any"}

	// List quantifiers: any/all return the winning element value;
	// tany/tall return a folded disjunct or merged value.
	case "any", "all", "tany", "tall":
		return []string{"Any"}

	// Help
	case "help":
		return nil

	// Constants
	case "math-pi", "math-e":
		return []string{"Scalar/Number/Decimal"}

	// Stack ops
	case "depth":
		return []string{"Scalar/Number/Integer"}
	case "stack":
		return []string{"Node/List"}
	case "dup":
		return []string{"Any", "Any"}
	case "swap":
		return []string{"Any", "Any"}
	case "drop":
		return nil
	case "over":
		return []string{"Any", "Any", "Any"}
	case "rot":
		return []string{"Any", "Any", "Any"}
	case "nip":
		return []string{"Any"}
	case "tuck":
		return []string{"Any", "Any", "Any"}
	case "dup2":
		return []string{"Any", "Any", "Any", "Any"}
	case "swap2":
		return []string{"Any", "Any", "Any", "Any"}
	case "drop2":
		return nil
	case "over2":
		return []string{"Any", "Any", "Any", "Any", "Any", "Any"}
	case "pick", "roll":
		return []string{"Any"}
	case "break", "continue":
		return nil
	}
	return nil
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
	case "and", "or", "xor", "nand", "nor", "iff", "xnor", "implies":
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

	if name == "add" && a0 == "Scalar" && a1 == "Scalar" {
		return []string{"Scalar/String"}
	}
	if a0 == "Scalar/Number/Integer" && a1 == "Scalar/Number/Integer" {
		return []string{"Scalar/Number/Integer"}
	}
	return []string{"Scalar/Number/Decimal"}
}

func inferUnaryMathReturns(name string, sig Signature) []string {
	a0 := sig.Args[0].String()
	switch name {
	case "abs", "negate":
		return []string{a0}
	case "sign":
		return []string{"Scalar/Number/Integer"}
	case "ceil", "floor", "round", "trunc":
		return []string{"Scalar/Number/Integer"}
	default:
		return []string{"Scalar/Number/Decimal"}
	}
}

