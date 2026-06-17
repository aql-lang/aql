package help

import (
	"io"
	"sort"
)

// ModuleInfo is one row of the built-in module catalog: the bare module id
// (the part after "aql:", e.g. "type-util") and its one-line summary.
type ModuleInfo struct {
	Name    string
	Summary string
}

// moduleCatalog is the catalog of compiled-in native modules and their
// one-line summaries. It lives here — rather than in the cmd or modules
// package — so every help surface that can't import the modules package (the
// `describe` language word in particular) can still render the module index.
// TestModuleCatalogMatchesModules (modules package) pins this list to
// modules.Names() so it cannot drift.
var moduleCatalog = []ModuleInfo{
	{"math-util", "Floating-point math: trig, logs, roots, constants."},
	{"array-util", "APL-style array vocabulary: shape, select, group, windows."},
	{"time-util", "Dates, durations, timezones, clocks, timers, and intervals."},
	{"matrix-util", "Tensors, matrices, and vectors with linear algebra."},
	{"bin-util", "Bitwise and byte-buffer helpers: masks, rotates, hashes."},
	{"type-util", "Type introspection and construction utilities."},
	{"vm", "Low-level virtual-machine primitives."},
	{"report", "Tabular reporting and formatting."},
	{"test", "Assertions and helpers for in-language tests."},
	{"rand", "Pseudo-random number generation."},
	{"query", "SQL-style query pipelines: from, where, join, group, order."},
	{"struct-util", "Structured-data utilities: merge, walk, transform, jsonify, …."},
	{"io", "I/O: read, write, stdin/stdout/stderr, printstr, trace."},
	{"net", "HTTP requests and API access: fetch, prepare, direct."},
	{"logic-util", "Derived boolean connectives: nand, nor, xnor, iff, implies."},
	{"string-util", "String manipulation: concat, split, trim, upper, lower, …."},
	{"minilang", "Embedded mini-languages behind the `mini` word: re (Go regexp), bf (brainfuck), gex (globs), register."},
	{"parselang", "Named parsers behind the `parse` word: register a parser, parse a source into an AST."},
}

// ModuleCatalog returns the built-in module catalog sorted by name. The
// returned slice is a fresh copy; callers may keep or mutate it freely.
func ModuleCatalog() []ModuleInfo {
	out := make([]ModuleInfo, len(moduleCatalog))
	copy(out, moduleCatalog)
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// ModuleSummary returns the one-line summary for a built-in module (keyed by
// its bare id, e.g. "type-util"), or "" when the module is unknown.
func ModuleSummary(name string) string {
	for _, m := range moduleCatalog {
		if m.Name == name {
			return m.Summary
		}
	}
	return ""
}

// WriteWordsByCategory writes the "words grouped by category" block: each
// category's name and summary, then its words wrapped under an indent. Shared
// by the CLI `aql describe` index and the REPL `describe` word so the two read
// identically.
func WriteWordsByCategory(w io.Writer) {
	for _, cat := range categories {
		writeLine(w, "  "+pad(cat.Name, 9)+" "+cat.Summary)
		writeWordGrid(w, cat.Words)
	}
}

// WriteModuleCatalog writes the module catalog block: each module id and its
// one-line summary, sorted by name.
func WriteModuleCatalog(w io.Writer) {
	for _, m := range ModuleCatalog() {
		writeLine(w, "  "+pad(m.Name, 12)+" "+m.Summary)
	}
}

// WriteCategory writes one category's header and its words, each with its
// one-line summary — the body of `describe <category>`.
func WriteCategory(w io.Writer, cat Category) {
	writeLine(w, cat.Name+" — "+cat.Summary)
	writeLine(w, "")
	writeLine(w, "Words:")
	for _, word := range cat.Words {
		summary := ""
		if e := Lookup(word); e != nil {
			summary = e.Summary
		}
		writeLine(w, "  "+pad(word, 12)+" "+summary)
	}
}

// writeWordGrid prints words wrapped under a 4-space indent at ~72 columns.
func writeWordGrid(w io.Writer, words []string) {
	const indent = "    "
	const width = 72
	line := indent
	for _, word := range words {
		if len(line) > len(indent) && len(line)+1+len(word) > width {
			writeLine(w, line)
			line = indent
		}
		if len(line) > len(indent) {
			line += " "
		}
		line += word
	}
	if len(line) > len(indent) {
		writeLine(w, line)
	}
}

// pad right-pads s to at least n runes with spaces (like %-Ns).
func pad(s string, n int) string {
	for len(s) < n {
		s += " "
	}
	return s
}

// writeLine writes s followed by a newline, ignoring write errors (these
// render to a REPL/CLI writer where a failed write is already fatal upstream).
func writeLine(w io.Writer, s string) {
	_, _ = io.WriteString(w, s+"\n")
}
