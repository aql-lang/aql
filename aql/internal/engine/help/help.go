// Package help provides embedded help text for AQL built-in words.
package help

import (
	"fmt"
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

	// Generate all configurations: 0 prefix to maxArgs prefix
	configs := make([]string, 0, maxArgs+1)
	for prefix := 0; prefix <= maxArgs; prefix++ {
		var parts []string
		// prefix args (reversed: deepest first on stack)
		for i := prefix - 1; i >= 0; i-- {
			parts = append(parts, vars[i])
		}
		parts = append(parts, name)
		// forward args
		for i := prefix; i < maxArgs; i++ {
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
	"Decimal": {"2.1", "3.2", "4.3", "5.4", "6.5", "7.6", "8.7", "9.8"},
	"Number":  {"2", "3.1", "4", "5.1", "6", "7.1", "8", "9.1"},
	"String":  {"'a'", "'b'", "'c'", "'d'", "'e'", "'f'"},
	"Scalar":  {"'a'", "'b'", "'c'", "'d'", "'e'", "'f'"},
	"Atom":    {"a", "b", "c", "d", "e", "f"},
	"Boolean": {"true", "false", "true", "false"},
	"List":    {"['a','b']", "['c','d']", "['e','f']"},
	"Map":     {"{a:1,b:2}", "{c:3,d:4}", "{e:5,f:6}"},
	"Any":     {"2", "'a'", "true", "['x']"},
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

// writeExamples generates column-aligned examples, one per signature,
// cycling through arg configurations.
func writeExamples(b *strings.Builder, info FuncInfo) {
	if len(info.Sigs) == 0 {
		b.WriteString("  (no examples)\n")
		return
	}

	type exLine struct {
		expr   string
		result string
	}
	var examples []exLine
	maxExprLen := 0

	configIdx := 0 // cycles through configurations across sigs

	// Per-type counters for cycling example values
	counters := map[string]int{}

	for _, sig := range info.Sigs {
		nArgs := len(sig.Args)
		if nArgs == 0 {
			continue
		}

		// Pick example values for this signature
		vals := make([]string, nArgs)
		for i, a := range sig.Args {
			leaf := typeAbbrev(a)
			c := counters[leaf]
			vals[i] = exampleVal(a, &c)
			counters[leaf] = c
		}

		// Compute expected result
		result := computeExampleResult(info.Name, sig, vals)

		// Show all configs when both args are the same depth-1 type,
		// otherwise show one config and cycle.
		if sigArgsSameType(sig) {
			for prefix := 0; prefix <= nArgs; prefix++ {
				expr := buildExampleExpr(info.Name, vals, prefix, nArgs)
				if len(expr) > maxExprLen {
					maxExprLen = len(expr)
				}
				examples = append(examples, exLine{expr: expr, result: result})
			}
		} else {
			prefix := configIdx % (nArgs + 1)
			expr := buildExampleExpr(info.Name, vals, prefix, nArgs)
			if len(expr) > maxExprLen {
				maxExprLen = len(expr)
			}
			examples = append(examples, exLine{expr: expr, result: result})
		}

		configIdx++
	}

	for _, ex := range examples {
		b.WriteString("  ")
		b.WriteString(ex.expr)
		padding := maxExprLen - len(ex.expr) + 3
		for i := 0; i < padding; i++ {
			b.WriteByte(' ')
		}
		b.WriteString("=> ")
		b.WriteString(ex.result)
		b.WriteByte('\n')
	}
}

// buildExampleExpr constructs an expression with `prefix` args before the word
// and the rest after.
func buildExampleExpr(name string, vals []string, prefix, nArgs int) string {
	var parts []string
	// prefix args (reversed: innermost first on stack = outermost in source)
	for i := prefix - 1; i >= 0; i-- {
		parts = append(parts, vals[i])
	}
	parts = append(parts, name)
	// forward args
	for i := prefix; i < nArgs; i++ {
		parts = append(parts, vals[i])
	}
	return strings.Join(parts, " ")
}

// computeExampleResult computes a result string for an example.
// For common operations we compute real results; otherwise use "...".
func computeExampleResult(name string, sig SigInfo, vals []string) string {
	// We can compute results for arithmetic operations
	if len(vals) == 2 {
		return computeBinaryResult(name, sig, vals[0], vals[1])
	}
	if len(vals) == 1 {
		return computeUnaryResult(name, sig, vals[0])
	}
	return "..."
}

func computeBinaryResult(name string, sig SigInfo, a, b string) string {
	// Try to parse as numbers
	aF, aOk := parseNum(a)
	bF, bOk := parseNum(b)

	if aOk && bOk {
		var result float64
		switch name {
		case "add":
			result = aF + bF
		case "sub":
			result = aF - bF
		case "mul":
			result = aF * bF
		case "div":
			if bF == 0 {
				return "error"
			}
			result = aF / bF
		case "mod":
			if bF == 0 {
				return "error"
			}
			result = float64(int64(aF) % int64(bF))
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

	// String concat for add
	if name == "add" {
		aStr := strings.Trim(a, "'")
		bStr := strings.Trim(b, "'")
		return "'" + aStr + bStr + "'"
	}

	return "..."
}

// formatResult formats a numeric result based on the return type.
func formatResult(result float64, sig SigInfo) string {
	if len(sig.Returns) == 0 {
		return "..."
	}
	retLeaf := typeAbbrev(sig.Returns[0])
	if retLeaf == "Integer" {
		return fmt.Sprintf("%d", int64(result))
	}
	// Use fixed precision to avoid floating point noise,
	// then trim trailing zeros.
	s := fmt.Sprintf("%.10f", result)
	// Trim trailing zeros after decimal point
	if strings.Contains(s, ".") {
		s = strings.TrimRight(s, "0")
		s = strings.TrimRight(s, ".")
	}
	// Ensure at least one decimal place for decimal types
	if !strings.Contains(s, ".") {
		s += ".0"
	}
	return s
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
