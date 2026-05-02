// Package help provides embedded help text for AQL built-in words.
//
//go:generate go run ../../../cmd/genhelp
package help

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

// Entry holds the help documentation for a single AQL word.
type Entry struct {
	Word        string // canonical word name
	Summary     string // one-line description
	Description string // brief multi-line explanation
	Notes       []string
}

// SigInfo holds the dynamically extracted signature data for formatting.
type SigInfo struct {
	Args    []string // type names per arg, e.g. ["Integer", "Integer"]
	Returns []string // inferred return type abbreviations
}

// FuncInfo carries everything needed for dynamic help rendering.
type FuncInfo struct {
	Name              string
	ForwardPrecedence bool
	Sigs              []SigInfo
	Entry             *Entry // static docs (may be nil)
}

// registry holds all help entries keyed by word name.
var registry = map[string]*Entry{}

// exampleResults holds pre-computed example results keyed by expression.
// Populated by go:generate via cmd/genhelp.
var exampleResults = map[string]string{}

// dynamicExampleResults holds example results generated at runtime for
// words registered after initial startup (e.g. native functions added
// via the API). Checked after the static map.
var dynamicExampleResults = map[string]string{}

// SetExampleResults replaces the pre-computed example results map.
// Called by the generated file or by test code.
func SetExampleResults(m map[string]string) {
	exampleResults = m
}

// GenerateDynamicExamples computes and stores example results for a word
// using the provided eval function. Called when new functions are
// registered after initial startup.
func GenerateDynamicExamples(info FuncInfo, eval func(string) (string, error)) {
	if eval == nil {
		return
	}
	for _, expr := range ExampleExprs(info) {
		if _, ok := exampleResults[expr]; ok {
			continue // already in static map
		}
		if _, ok := dynamicExampleResults[expr]; ok {
			continue
		}
		result, err := eval(expr)
		if err != nil || result == "" {
			continue
		}
		dynamicExampleResults[expr] = result
	}
}

// register adds an entry to the global help registry.
func register(e *Entry) {
	registry[e.Word] = e
}

// Lookup returns the help entry for a word, or nil if none exists.
func Lookup(word string) *Entry {
	return registry[word]
}

// Words returns all registered word names in no particular order.
func Words() []string {
	names := make([]string, 0, len(registry))
	for k := range registry {
		names = append(names, k)
	}
	return names
}

// isNonCommutative2Arg returns true if the word is a non-commutative
// binary operation (where arg order matters).
func isNonCommutative2Arg(info FuncInfo) bool {
	has2Arg := false
	for _, sig := range info.Sigs {
		if len(sig.Args) == 2 {
			has2Arg = true
			break
		}
	}
	if !has2Arg {
		return false
	}
	switch info.Name {
	case "sub", "div", "mod", "pow", "atan2",
		"lt", "gt", "lte", "gte", "implies":
		return true
	}
	return false
}

// typeAbbrev shortens a full type path to its leaf name.
func typeAbbrev(t string) string {
	parts := strings.Split(t, "/")
	return parts[len(parts)-1]
}

// Format renders a static help entry as a human-readable string.
// Used when dynamic registry data is not available (e.g. REPL /help).
func Format(e *Entry) string {
	var b strings.Builder
	b.WriteString(e.Word)
	b.WriteString(" — ")
	if e.Summary != "" {
		b.WriteString(e.Summary)
	} else {
		b.WriteString("<not described>")
	}
	b.WriteByte('\n')
	if e.Description != "" {
		b.WriteString("\nDescription:\n")
		writeWrapped(&b, e.Description, 70, "  ")
	}
	if len(e.Notes) > 0 {
		b.WriteString("\nNotes:\n")
		for _, n := range e.Notes {
			b.WriteString("  - ")
			b.WriteString(n)
			b.WriteByte('\n')
		}
	}
	return b.String()
}

// FormatDynamic renders help for a word using live registry data.
func FormatDynamic(info FuncInfo) string {
	var b strings.Builder

	entry := info.Entry
	summary := "<not described>"
	if entry != nil && entry.Summary != "" {
		summary = entry.Summary
	}

	// Header
	b.WriteString(info.Name)
	b.WriteString(" — ")
	b.WriteString(summary)
	b.WriteByte('\n')

	// Precedence
	b.WriteByte('\n')
	if info.ForwardPrecedence {
		b.WriteString("Precedence: forward — looks ahead for arguments first.\n")
		writePrecedenceExamples(&b, info)
	} else {
		b.WriteString("Precedence: stack — arguments must be on the stack.\n")
		writePrecedenceExamplesStack(&b, info)
	}

	// Signatures
	b.WriteString("\nSignatures: (in match order)\n")
	writeSigs(&b, info.Sigs)

	// Description
	desc := "<not described>"
	if entry != nil && entry.Description != "" {
		desc = entry.Description
	}
	b.WriteString("\nDescription:\n")
	writeWrapped(&b, desc, 70, "  ")

	// Examples
	b.WriteString("\nExamples:\n")
	writeExamples(&b, info)

	// Notes
	if entry != nil && len(entry.Notes) > 0 {
		b.WriteString("\nNotes:\n")
		for _, n := range entry.Notes {
			b.WriteString("  - ")
			b.WriteString(n)
			b.WriteByte('\n')
		}
	}

	return b.String()
}

// writePrecedenceExamples shows all arg configurations for forward words.
// For 2-arg words: word x y <=> y word x <=> y x word
// This reflects outward-consumption: sig[0] is nearest to word on each side.
func writePrecedenceExamples(b *strings.Builder, info FuncInfo) {
	// Find longest signature (up to 3 args)
	maxArgs := 0
	for _, sig := range info.Sigs {
		if len(sig.Args) > maxArgs {
			maxArgs = len(sig.Args)
		}
	}
	if maxArgs > 3 {
		maxArgs = 3
	}
	if maxArgs == 0 {
		return
	}

	vars := []string{"x", "y", "z"}[:maxArgs]
	name := info.Name

	// Generate all configurations: 0 prefix to maxArgs prefix.
	// prefix=0: word x y z      (all forward)
	// prefix=1: z word x y      (1 on stack = sig[maxArgs-1], rest forward)
	// prefix=2: z y word x      (2 on stack, 1 forward)
	// prefix=3: z y x word      (all stack)
	//
	// Stack args appear outermost-first in source (deepest on stack = leftmost).
	// Forward args appear nearest-first after word.
	configs := make([]string, 0, maxArgs+1)
	for prefix := 0; prefix <= maxArgs; prefix++ {
		var parts []string
		// Stack args: the last `prefix` vars, in reverse order (deepest first)
		for i := maxArgs - 1; i >= maxArgs-prefix; i-- {
			parts = append(parts, vars[i])
		}
		parts = append(parts, name)
		// Forward args: the first (maxArgs-prefix) vars, in order
		for i := 0; i < maxArgs-prefix; i++ {
			parts = append(parts, vars[i])
		}
		configs = append(configs, strings.Join(parts, " "))
	}

	b.WriteString("  ")
	b.WriteString(strings.Join(configs, "  <=>  "))
	b.WriteByte('\n')
}

// writePrecedenceExamplesStack shows the stack-only pattern.
func writePrecedenceExamplesStack(b *strings.Builder, info FuncInfo) {
	maxArgs := 0
	for _, sig := range info.Sigs {
		if len(sig.Args) > maxArgs {
			maxArgs = len(sig.Args)
		}
	}
	if maxArgs > 3 {
		maxArgs = 3
	}
	if maxArgs == 0 {
		return
	}

	vars := []string{"x", "y", "z"}[:maxArgs]
	// Stack-only: all args must precede the word
	var parts []string
	for i := maxArgs - 1; i >= 0; i-- {
		parts = append(parts, vars[i])
	}
	parts = append(parts, info.Name)
	b.WriteString("  ")
	b.WriteString(strings.Join(parts, " "))
	b.WriteByte('\n')
}

// writeSigs renders the signatures in column-aligned format.
func writeSigs(b *strings.Builder, sigs []SigInfo) {
	if len(sigs) == 0 {
		b.WriteString("  (none)\n")
		return
	}

	// Build formatted lines: [ [args...]  result ]
	type sigLine struct {
		args    string
		returns string
	}
	var lines []sigLine
	maxArgsLen := 0
	maxRetLen := 0
	for _, sig := range sigs {
		abbrevArgs := make([]string, len(sig.Args))
		for i, a := range sig.Args {
			abbrevArgs[i] = typeAbbrev(a)
		}
		argsStr := "[" + strings.Join(abbrevArgs, " ") + "]"

		abbrevRets := make([]string, len(sig.Returns))
		for i, r := range sig.Returns {
			abbrevRets[i] = typeAbbrev(r)
		}
		retStr := strings.Join(abbrevRets, " ")

		if len(argsStr) > maxArgsLen {
			maxArgsLen = len(argsStr)
		}
		if len(retStr) > maxRetLen {
			maxRetLen = len(retStr)
		}
		lines = append(lines, sigLine{args: argsStr, returns: retStr})
	}

	for _, l := range lines {
		b.WriteString("  [ ")
		b.WriteString(l.args)
		argPad := maxArgsLen - len(l.args) + 2
		for i := 0; i < argPad; i++ {
			b.WriteByte(' ')
		}
		b.WriteString(l.returns)
		retPad := maxRetLen - len(l.returns) + 1
		for i := 0; i < retPad; i++ {
			b.WriteByte(' ')
		}
		b.WriteString("]\n")
	}
}

// Standard example values by type leaf name.
var exampleValues = map[string][]string{
	"Integer": {"2", "3", "4", "5", "6", "7", "8", "9"},
	"Decimal": {"2.25", "3.5", "4.75", "5.25", "6.5", "7.75", "8.25", "9.5"},
	"Number":  {"2", "3.5", "4", "5.25", "6", "7.5", "8", "9.25"},
	"String":  {"'a'", "'b'", "'c'", "'d'", "'e'", "'f'"},
	"Scalar":  {"'a'", "'b'", "'c'", "'d'", "'e'", "'f'"},
	"Atom":    {"(quote a)", "(quote b)", "(quote c)", "(quote d)", "(quote e)", "(quote f)"},
	"Boolean": {"true", "false", "true", "false"},
	"List":    {"['a','b']", "['c','d']", "['e','f']"},
	"Map":     {"{a:1,b:2}", "{c:3,d:4}", "{e:5,f:6}"},
	"Any":     {"2", "3", "4", "5", "6", "7", "8", "9"},
	"Node":    {"{a:1}", "['x']", "{b:2}", "['y']"},
}

// exampleVal returns a standard example value for a type.
// counter is used to cycle through the available values.
func exampleVal(typeName string, counter *int) string {
	leaf := typeAbbrev(typeName)
	vals, ok := exampleValues[leaf]
	if !ok {
		vals = exampleValues["Any"]
	}
	idx := *counter % len(vals)
	*counter++
	return vals[idx]
}

// sigArgsSameType checks if all args in a sig are the same type.
func sigArgsSameType(sig SigInfo) bool {
	if len(sig.Args) < 2 {
		return false
	}
	for _, a := range sig.Args[1:] {
		if a != sig.Args[0] {
			return false
		}
	}
	return true
}

// ExampleExprs returns all example expressions that would be generated
// for a word, without computing results. Used by the generator to know
// which expressions to pre-compute.
func ExampleExprs(info FuncInfo) []string {
	var exprs []string
	configIdx := 0
	counters := map[string]int{}
	for _, sig := range info.Sigs {
		nArgs := len(sig.Args)
		if nArgs == 0 {
			continue
		}
		vals := make([]string, nArgs)
		for i, a := range sig.Args {
			leaf := typeAbbrev(a)
			c := counters[leaf]
			vals[i] = exampleVal(a, &c)
			counters[leaf] = c
		}
		var prefixes []int
		if !info.ForwardPrecedence {
			prefixes = []int{nArgs}
		} else if sigArgsSameType(sig) {
			for p := 0; p <= nArgs; p++ {
				prefixes = append(prefixes, p)
			}
		} else {
			prefixes = []int{configIdx % (nArgs + 1)}
		}
		for _, prefix := range prefixes {
			exprs = append(exprs, buildExampleExpr(info.Name, vals, prefix, nArgs))
		}
		configIdx++
	}
	return exprs
}

// writeExamples generates column-aligned examples, one per signature,
// cycling through arg configurations. For 2-arg words, infix examples
// come first. Args appear descending left-to-right in source.
func writeExamples(b *strings.Builder, info FuncInfo) {
	if len(info.Sigs) == 0 {
		b.WriteString("  (no examples)\n")
		return
	}

	type exLine struct {
		expr   string
		result string
		prefix int // for sorting: infix (1) first
	}
	var examples []exLine
	maxExprLen := 0

	configIdx := 0

	// Per-type counters for cycling example values (descending)
	counters := map[string]int{}

	for _, sig := range info.Sigs {
		nArgs := len(sig.Args)
		if nArgs == 0 {
			continue
		}

		// Pick example values: vals[0] gets the smaller value (sig[0]),
		// vals[1] gets the larger (sig[1]). In infix form (vals[1] word vals[0]),
		// this gives descending left-to-right.
		vals := make([]string, nArgs)
		for i, a := range sig.Args {
			leaf := typeAbbrev(a)
			c := counters[leaf]
			vals[i] = exampleVal(a, &c)
			counters[leaf] = c
		}

		// Build expressions for each config.
		// Stack-only words: only all-prefix config.
		// Same-type args: show all prefix/forward configs.
		// Otherwise: one config, cycling.
		var prefixes []int
		if !info.ForwardPrecedence {
			prefixes = []int{nArgs} // all on stack
		} else if sigArgsSameType(sig) {
			for p := 0; p <= nArgs; p++ {
				prefixes = append(prefixes, p)
			}
		} else {
			prefixes = []int{configIdx % (nArgs + 1)}
		}

		for _, prefix := range prefixes {
			expr := buildExampleExpr(info.Name, vals, prefix, nArgs)
			result := evalExample(expr, info.Name, sig, vals)
			if len(expr) > maxExprLen {
				maxExprLen = len(expr)
			}
			examples = append(examples, exLine{expr: expr, result: result, prefix: prefix})
		}

		configIdx++
	}

	// For 2-arg words, sort so infix examples (prefix=1) come first
	// within each signature group. We do this by partitioning: infix first,
	// then others in original order.
	maxArgs := 0
	for _, sig := range info.Sigs {
		if len(sig.Args) > maxArgs {
			maxArgs = len(sig.Args)
		}
	}
	if maxArgs == 2 {
		var infix, others []exLine
		for _, ex := range examples {
			if ex.prefix == 1 {
				infix = append(infix, ex)
			} else {
				others = append(others, ex)
			}
		}
		examples = append(infix, others...)
	}

	nonComm := isNonCommutative2Arg(info)
	noteShown := false

	for _, ex := range examples {
		b.WriteString("  ")
		b.WriteString(ex.expr)
		padding := maxExprLen - len(ex.expr) + 3
		for i := 0; i < padding; i++ {
			b.WriteByte(' ')
		}
		b.WriteString(";# ")
		b.WriteString(ex.result)
		if nonComm && ex.prefix == 0 && !noteShown {
			b.WriteString("  NOTE: most significant argument is last")
			noteShown = true
		}
		b.WriteByte('\n')
	}
}

// buildSigArgs returns the sig args in signature order for a given config.
// Regardless of prefix/forward layout, the argument equivalence principle
// means all configs produce sig[0]=vals[0], sig[1]=vals[1], etc.
func buildSigArgs(vals []string, prefix, nArgs int) []string {
	result := make([]string, len(vals))
	copy(result, vals)
	return result
}

// buildExampleExpr constructs an expression with `prefix` args on stack
// and the rest forward. Mirrors the precedence layout:
//   prefix=0: word v0 v1 v2      (all forward)
//   prefix=1: v2 word v0 v1      (last on stack)
//   prefix=2: v2 v1 word v0      (last two on stack)
//   prefix=3: v2 v1 v0 word      (all on stack)
func buildExampleExpr(name string, vals []string, prefix, nArgs int) string {
	var parts []string
	// Stack args: last `prefix` vals, outermost (deepest) first
	for i := nArgs - 1; i >= nArgs-prefix; i-- {
		parts = append(parts, vals[i])
	}
	parts = append(parts, name)
	// Forward args: first (nArgs-prefix) vals, in order
	for i := 0; i < nArgs-prefix; i++ {
		parts = append(parts, vals[i])
	}
	return strings.Join(parts, " ")
}

// evalExample looks up a pre-computed result for an expression,
// checking the static map first, then the dynamic map, then falling
// back to static computation.
func evalExample(expr, name string, sig SigInfo, vals []string) string {
	if result, ok := exampleResults[expr]; ok {
		return result
	}
	if result, ok := dynamicExampleResults[expr]; ok {
		return result
	}
	sigArgs := buildSigArgs(vals, 0, len(vals))
	return computeExampleResult(name, sig, sigArgs)
}

// computeExampleResult computes a result string for an example.
// sigArgs are in signature order: sigArgs[0] = sig[0], sigArgs[1] = sig[1].
// For common operations we compute real results; otherwise use "...".
func computeExampleResult(name string, sig SigInfo, sigArgs []string) string {
	if len(sigArgs) == 2 {
		return computeBinaryResult(name, sig, sigArgs[0], sigArgs[1])
	}
	if len(sigArgs) == 1 {
		return computeUnaryResult(name, sig, sigArgs[0])
	}
	return "..."
}

func computeBinaryResult(name string, sig SigInfo, a, b string) string {
	// a = sig[0] (nearest arg), b = sig[1] (further arg).
	// Non-commutative ops compute args[1] op args[0] = b op a.
	aF, aOk := parseNum(a)
	bF, bOk := parseNum(b)

	if aOk && bOk {
		var result float64
		switch name {
		case "add":
			result = aF + bF
		case "sub":
			result = bF - aF
		case "mul":
			result = aF * bF
		case "div":
			if aF == 0 {
				return "error"
			}
			result = bF / aF
		case "mod":
			if aF == 0 {
				return "error"
			}
			result = math.Mod(bF, aF)
		case "pow":
			return "..."
		case "min":
			if aF < bF {
				result = aF
			} else {
				result = bF
			}
		case "max":
			if aF > bF {
				result = aF
			} else {
				result = bF
			}
		default:
			return "..."
		}
		return formatResult(result, sig)
	}

	// String concat for add: handler does args[1] + args[0] = b + a
	if name == "add" {
		aStr := strings.Trim(a, "'")
		bStr := strings.Trim(b, "'")
		return "'" + bStr + aStr + "'"
	}

	return "..."
}

// formatResult formats a numeric result to match the engine's output.
func formatResult(result float64, sig SigInfo) string {
	if len(sig.Returns) == 0 {
		return "..."
	}
	retLeaf := typeAbbrev(sig.Returns[0])
	if retLeaf == "Integer" {
		return fmt.Sprintf("%d", int64(result))
	}
	// Match engine: strconv.FormatFloat with 'f', -1, 64
	return strconv.FormatFloat(result, 'f', -1, 64)
}

func computeUnaryResult(name string, sig SigInfo, a string) string {
	aF, aOk := parseNum(a)
	if !aOk {
		return "..."
	}
	var result float64
	switch name {
	case "abs":
		if aF < 0 {
			result = -aF
		} else {
			result = aF
		}
	case "negate":
		result = -aF
	case "sign":
		if aF > 0 {
			return "1"
		} else if aF < 0 {
			return "-1"
		}
		return "0"
	default:
		return "..."
	}
	return formatResult(result, sig)
}

func parseNum(s string) (float64, bool) {
	s = strings.Trim(s, "'")
	var f float64
	n, err := fmt.Sscanf(s, "%f", &f)
	return f, err == nil && n == 1
}

// writeWrapped writes text wrapped at width with the given indent.
func writeWrapped(b *strings.Builder, text string, width int, indent string) {
	words := strings.Fields(text)
	lineLen := 0
	b.WriteString(indent)
	for i, w := range words {
		if i > 0 && lineLen+1+len(w) > width {
			b.WriteByte('\n')
			b.WriteString(indent)
			lineLen = 0
		} else if i > 0 {
			b.WriteByte(' ')
			lineLen++
		}
		b.WriteString(w)
		lineLen += len(w)
	}
	b.WriteByte('\n')
}
