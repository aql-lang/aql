// Package describe implements `aql describe` — documentation for the
// AQL *language*: its built-in words, their categories, and loadable
// modules. The forms are:
//
//	aql describe                     a categorised guide to words and modules
//	aql describe <word>              full per-word docs (e.g. aql describe add)
//	aql describe <category>          the words in one category (e.g. math)
//	aql describe aql:<module>        a module and the words it exports
//	aql describe aql:<module>:<word> one exported word of a module
//
// A name it doesn't recognise is given one more chance: describe tries to
// load it as a module (native, installed, or file) before giving up.
// CLI/command help lives separately under `aql help`.
package describe

import (
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/aql-lang/aql/cmd/go/internal/command"
	parse "github.com/aql-lang/aql/eng/go/parser"
	"github.com/aql-lang/aql/lang/go/modules"
	"github.com/aql-lang/aql/lang/go/native"
	helppkg "github.com/aql-lang/aql/lang/go/native/help"
)

// moduleSummaries gives a one-line description for each built-in module.
// Keys are the bare module ids returned by modules.Names() (the part after
// "aql:"), so they must carry the "-util" suffix where the module has one.
var moduleSummaries = map[string]string{
	"math-util":   "Floating-point math: trig, logs, roots, constants.",
	"array-util":  "APL-style array vocabulary: shape, select, group, windows.",
	"time-util":   "Dates, durations, timezones, clocks, timers, and intervals.",
	"matrix-util": "Tensors, matrices, and vectors with linear algebra.",
	"decision":    "Decision tables and rule evaluation.",
	"bin-util":    "Bitwise and byte-buffer helpers: masks, rotates, hashes.",
	"type-util":   "Type introspection and construction utilities.",
	"vm":          "Low-level virtual-machine primitives.",
	"report":      "Tabular reporting and formatting.",
	"test":        "Assertions and helpers for in-language tests.",
	"rand":        "Pseudo-random number generation.",
	"query":       "SQL-style query pipelines: from, where, join, group, order.",
	"struct-util": "Structured-data utilities: merge, walk, transform, jsonify, ….",
	"io":          "I/O: read, write, stdin/stdout/stderr, printstr, trace.",
	"net":         "HTTP requests and API access: fetch, prepare, direct.",
	"logic-util":  "Derived boolean connectives: nand, nor, xnor, iff, implies.",
	"string-util": "String manipulation: concat, split, trim, upper, lower, ….",
}

type cmd struct{}

// New returns the describe subcommand.
func New() command.Command { return &cmd{} }

func (*cmd) Name() string     { return "describe" }
func (*cmd) Synopsis() string { return "document an AQL word, category, or module (or list them)" }
func (*cmd) Run(args []string, _ io.Reader, stdout, _ io.Writer) int {
	return Run(args, stdout)
}

// Run dispatches `aql describe [name]`. Precedence for a bare name (no colon):
// category first (so generic names like `math`/`type`/`string` show their
// group), then word, then dotted export, then a built-in module, and finally
// a best-effort module load. A name containing ':' is always a module path.
func Run(args []string, w io.Writer) int {
	if len(args) == 0 {
		writeIndex(w)
		return 0
	}

	name := args[0]

	// A colon marks a module reference: "aql:type-util" or, with a word,
	// "aql:type-util:foo" / "type-util:foo". No built-in word carries a colon,
	// so this is unambiguous and takes precedence over every bare-name form.
	if strings.Contains(name, ":") {
		return describeModulePath(w, name)
	}

	// A category groups related words. Checked before words so that browsing
	// names (math, type, string, stack, …) open the group; the only words that
	// share a category name are `help` (use `aql help`) and the niche `stack`
	// dump word, both better served by the group view here.
	if cat, ok := helppkg.LookupCategory(name); ok {
		writeCategory(w, cat)
		return 0
	}

	return describeName(w, name)
}

// describeName resolves a bare name to a word, a dotted module export, a
// built-in module, or — failing all of those — a module it tries to load.
func describeName(w io.Writer, name string) int {
	reg, err := newRegistry()
	if err != nil {
		fmt.Fprintf(w, "describe: cannot load registry: %s\n", err)
		return 1
	}

	// A registered word (or simple def) renders from live signature data.
	if info := native.BuildFuncInfo(reg, name); info != nil {
		fmt.Fprint(w, helppkg.FormatDynamic(*info))
		return 0
	}

	// A dotted name (ArrayUtil.indices) names a single module export.
	if strings.Contains(name, ".") {
		if info := qualifiedExportInfo(reg, name); info != nil {
			fmt.Fprint(w, helppkg.FormatDynamic(*info))
			return 0
		}
	}

	// A documented-but-unregistered word falls back to its static entry.
	if entry := helppkg.Lookup(name); entry != nil {
		fmt.Fprint(w, helppkg.Format(entry))
		return 0
	}

	// A bare built-in module name (math-util, struct-util, …).
	if isModule(name) {
		desc, derr := modules.Resolve(name, reg)
		if derr != nil {
			fmt.Fprintf(w, "describe: cannot load module %q: %s\n", name, derr)
			return 1
		}
		return renderModuleDesc(w, "aql:"+name, desc)
	}

	// Unknown: give it one more chance as a loadable module (installed or file).
	if desc, lerr := native.ResolveAnyModule(reg, name); lerr == nil {
		return renderModuleDesc(w, name, desc)
	}

	fmt.Fprintf(w, "describe: no description available for %q\n", name)
	fmt.Fprintln(w, "Run 'aql describe' to list available words and modules.")
	return 1
}

// describeModulePath handles a name containing ':' — a module, or a single
// word within one. The module is resolved as a built-in when known, otherwise
// loaded on demand.
func describeModulePath(w io.Writer, name string) int {
	reg, err := newRegistry()
	if err != nil {
		fmt.Fprintf(w, "describe: cannot load registry: %s\n", err)
		return 1
	}

	modRef, word := splitModulePath(name)
	desc, err := resolveModule(reg, modRef)
	if err != nil {
		fmt.Fprintf(w, "describe: cannot load module %q: %s\n", modRef, err)
		fmt.Fprintln(w, "Run 'aql describe' to list available words and modules.")
		return 1
	}

	if word == "" {
		return renderModuleDesc(w, modRef, desc)
	}

	if info := exportInfoFromDesc(desc, word); info != nil {
		fmt.Fprint(w, helppkg.FormatDynamic(*info))
		return 0
	}

	fmt.Fprintf(w, "describe: module %s has no exported word %q\n", modRef, word)
	if names := exportWordNames(desc); len(names) > 0 {
		fmt.Fprintf(w, "Exported words: %s\n", strings.Join(names, " "))
	}
	return 1
}

// splitModulePath separates a colon-form argument into a module reference and
// an optional word. "aql:type-util:foo" → ("aql:type-util", "foo");
// "aql:type-util" → ("aql:type-util", ""); "type-util:foo" → ("type-util",
// "foo"). The "aql:" prefix is preserved on the module reference so the
// resolver routes it as a native import.
func splitModulePath(name string) (modRef, word string) {
	hadAQL := strings.HasPrefix(name, "aql:")
	rest := strings.TrimPrefix(name, "aql:")
	short := rest
	if i := strings.IndexByte(rest, ':'); i >= 0 {
		short, word = rest[:i], rest[i+1:]
	}
	if hadAQL {
		return "aql:" + short, word
	}
	return short, word
}

// resolveModule resolves a module reference to its descriptor, using the
// built-in native path when the module is compiled in and a best-effort load
// otherwise ("attempt to load a module if unknown").
func resolveModule(reg *native.Registry, modRef string) (native.ModuleDesc, error) {
	if short := strings.TrimPrefix(modRef, "aql:"); isModule(short) {
		return modules.Resolve(short, reg)
	}
	return native.ResolveAnyModule(reg, modRef)
}

// writeIndex prints the categorised guide: every word grouped by category,
// then the loadable modules, then how to drill into each.
func writeIndex(w io.Writer) {
	fmt.Fprintln(w, "AQL language reference — built-in words by category, and loadable modules.")
	fmt.Fprintln(w)

	fmt.Fprintln(w, "Words:")
	for _, cat := range helppkg.Categories() {
		fmt.Fprintf(w, "  %-9s %s\n", cat.Name, cat.Summary)
		writeWordGrid(w, cat.Words)
	}

	mods := modules.Names()
	sort.Strings(mods)
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Modules (import with \"aql:<name>\" import):")
	for _, m := range mods {
		fmt.Fprintf(w, "  %-12s %s\n", m, moduleSummaries[m])
	}

	fmt.Fprintln(w)
	fmt.Fprintln(w, "Drill in:")
	fmt.Fprintln(w, "  aql describe <word>                e.g. aql describe add")
	fmt.Fprintln(w, "  aql describe <category>            e.g. aql describe math")
	fmt.Fprintln(w, "  aql describe aql:<module>          e.g. aql describe aql:type-util")
	fmt.Fprintln(w, "  aql describe aql:<module>:<word>   e.g. aql describe aql:type-util:tpartial")
	fmt.Fprintln(w, "Docs: "+helppkg.ReferenceURL)
}

// writeWordGrid prints a category's words wrapped under a 4-space indent.
func writeWordGrid(w io.Writer, words []string) {
	const indent = "    "
	const width = 72
	line := indent
	for _, word := range words {
		if len(line) > len(indent) && len(line)+1+len(word) > width {
			fmt.Fprintln(w, line)
			line = indent
		}
		if len(line) > len(indent) {
			line += " "
		}
		line += word
	}
	if len(line) > len(indent) {
		fmt.Fprintln(w, line)
	}
}

// writeCategory prints one category and the words it contains, each with its
// one-line summary.
func writeCategory(w io.Writer, cat helppkg.Category) {
	fmt.Fprintf(w, "%s — %s\n", cat.Name, cat.Summary)
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Words:")
	for _, word := range cat.Words {
		summary := ""
		if entry := helppkg.Lookup(word); entry != nil {
			summary = entry.Summary
		}
		fmt.Fprintf(w, "  %-12s %s\n", word, summary)
	}
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Use 'aql describe <word>' for full docs (signatures, examples) on any of these.")
	fmt.Fprintln(w, "Docs: "+helppkg.ReferenceURL)
}

// renderModuleDesc prints a module's summary (when built-in) and the words it
// exports, plus how to import and drill into them.
func renderModuleDesc(w io.Writer, ref string, desc native.ModuleDesc) int {
	if summary := moduleSummaries[strings.TrimPrefix(ref, "aql:")]; summary != "" {
		fmt.Fprintf(w, "%s — %s\n", ref, summary)
	} else {
		fmt.Fprintf(w, "%s\n", ref)
	}
	fmt.Fprintln(w)

	exportNames := make([]string, 0, len(desc.Exports))
	for k := range desc.Exports {
		exportNames = append(exportNames, k)
	}
	sort.Strings(exportNames)

	fmt.Fprintln(w, "Words (call as <export>.<word>):")
	for _, export := range exportNames {
		words := desc.Exports[export].Keys()
		sort.Strings(words)
		for _, word := range words {
			fmt.Fprintf(w, "  %s.%s\n", export, word)
		}
	}

	fmt.Fprintln(w)
	fmt.Fprintf(w, "Import with \"%s\" import, then call e.g. <export>.<word>.\n", ref)
	fmt.Fprintf(w, "Describe one word with 'aql describe %s:<word>'.\n", ref)
	fmt.Fprintln(w, "Docs: "+helppkg.ReferenceURL)
	return 0
}

// exportInfoFromDesc resolves a single word across a module descriptor's export
// namespaces and builds its help.FuncInfo (doc + provenance + signatures).
// Returns nil when no namespace exports a function by that name. Namespaces are
// walked in sorted order for a deterministic result.
func exportInfoFromDesc(desc native.ModuleDesc, word string) *helppkg.FuncInfo {
	nss := make([]string, 0, len(desc.Exports))
	for ns := range desc.Exports {
		nss = append(nss, ns)
	}
	sort.Strings(nss)
	for _, ns := range nss {
		om := desc.Exports[ns]
		if om == nil {
			continue
		}
		v, ok := om.Get(word)
		if !ok {
			continue
		}
		fn, ok := native.FnDefFromValue(v)
		if !ok {
			continue
		}
		return native.FnDefFuncInfo(ns+"."+word, fn)
	}
	return nil
}

// exportWordNames returns every exported word in a descriptor (dotted with its
// namespace), sorted — used to hint after a missing-word error.
func exportWordNames(desc native.ModuleDesc) []string {
	var out []string
	for ns, om := range desc.Exports {
		if om == nil {
			continue
		}
		for _, word := range om.Keys() {
			out = append(out, ns+"."+word)
		}
	}
	sort.Strings(out)
	return out
}

// qualifiedExportInfo resolves a dotted "Namespace.word" export across the
// native modules and builds its help.FuncInfo. Returns nil when the namespace
// or word is unknown. Unlike the runtime `describe`, the CLI has nothing
// imported, so it consults the module registry directly.
func qualifiedExportInfo(reg *native.Registry, name string) *helppkg.FuncInfo {
	dot := strings.IndexByte(name, '.')
	if dot <= 0 || dot >= len(name)-1 {
		return nil
	}
	ns, word := name[:dot], name[dot+1:]

	for _, m := range modules.Names() {
		desc, derr := modules.Resolve(m, reg)
		if derr != nil {
			continue
		}
		exports, ok := desc.Exports[ns]
		if !ok {
			continue
		}
		ev, ok := exports.Get(word)
		if !ok {
			return nil // namespace matched but no such word
		}
		fn, ok := native.FnDefFromValue(ev)
		if !ok {
			return nil
		}
		return native.FnDefFuncInfo(name, fn)
	}
	return nil
}

// newRegistry builds a registry wired for module resolution: the parser is
// installed (file modules need it) and the native-module resolver is enabled
// so "aql:<name>" references load.
func newRegistry() (*native.Registry, error) {
	reg, err := native.DefaultRegistry()
	if err != nil {
		return nil, err
	}
	reg.SetParseFunc(parse.Parse)
	modules.InstallResolver(reg)
	return reg, nil
}

func isModule(name string) bool {
	for _, m := range modules.Names() {
		if m == name {
			return true
		}
	}
	return false
}
