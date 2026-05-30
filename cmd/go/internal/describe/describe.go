// Package describe implements `aql describe` — documentation for the
// AQL *language*: its built-in words and loadable modules. With no
// argument it lists every word and module; with a word name it prints
// full per-word docs; with a module name it lists that module's
// exported words. CLI/command help lives separately under `aql help`.
package describe

import (
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/aql-lang/aql/cmd/go/internal/command"
	"github.com/aql-lang/aql/lang/go/modules"
	"github.com/aql-lang/aql/lang/go/native"
	helppkg "github.com/aql-lang/aql/lang/go/native/help"
)

// moduleSummaries gives a one-line description for each built-in module.
// Keyed by the bare module name (the part after "aql:").
var moduleSummaries = map[string]string{
	"math":      "Floating-point math: trig, logs, roots, constants.",
	"array":     "Numeric array construction and element-wise operations.",
	"time":      "Clocks, timers, and intervals.",
	"matrix":    "Tensors, matrices, and vectors with linear algebra.",
	"decision":  "Decision tables and rule evaluation.",
	"solardemo": "Demonstration module backed by an HTTP fixture.",
	"bin":       "Binary encoding and byte-buffer helpers.",
	"type":      "Type introspection and construction utilities.",
	"vm":        "Low-level virtual-machine primitives.",
	"report":    "Tabular reporting and formatting.",
	"test":      "Assertions and helpers for in-language tests.",
	"rand":      "Pseudo-random number generation.",
}

type cmd struct{}

// New returns the describe subcommand.
func New() command.Command { return &cmd{} }

func (*cmd) Name() string     { return "describe" }
func (*cmd) Synopsis() string { return "document an AQL word or module (or list them)" }
func (*cmd) Run(args []string, _ io.Reader, stdout, _ io.Writer) int {
	return Run(args, stdout)
}

// Run handles `aql describe`, `aql describe <word>`, and
// `aql describe <module>`.
func Run(args []string, w io.Writer) int {
	if len(args) == 0 {
		writeIndex(w)
		return 0
	}

	name := args[0]

	// A module name (with or without the "aql:" prefix) describes the
	// module and its exported words.
	if mod := strings.TrimPrefix(name, "aql:"); isModule(mod) {
		return describeModule(w, mod)
	}

	reg, err := native.DefaultRegistry()
	if err == nil {
		if info := native.BuildFuncInfo(reg, name); info != nil {
			fmt.Fprint(w, helppkg.FormatDynamic(*info))
			return 0
		}
	}

	// Fall back to the static entry for documented-but-unregistered words.
	if entry := helppkg.Lookup(name); entry != nil {
		fmt.Fprint(w, helppkg.Format(entry))
		return 0
	}

	fmt.Fprintf(w, "describe: no description available for %q\n", name)
	fmt.Fprintln(w, "Run 'aql describe' to list available words and modules.")
	return 1
}

// writeIndex lists every documented word and every built-in module.
func writeIndex(w io.Writer) {
	words := helppkg.Words()
	sort.Strings(words)

	fmt.Fprintln(w, "Words:")
	for _, word := range words {
		entry := helppkg.Lookup(word)
		fmt.Fprintf(w, "  %-16s %s\n", word, entry.Summary)
	}

	mods := modules.Names()
	sort.Strings(mods)
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Modules (import with \"aql:<name>\" import):")
	for _, m := range mods {
		fmt.Fprintf(w, "  %-16s %s\n", m, moduleSummaries[m])
	}

	fmt.Fprintln(w)
	fmt.Fprintln(w, "Use 'aql describe <word>' or 'aql describe <module>' for detail.")
	fmt.Fprintln(w, "Docs: "+helppkg.ReferenceURL)
}

// describeModule prints a module's summary and the words it exports.
func describeModule(w io.Writer, name string) int {
	fmt.Fprintf(w, "aql:%s — %s\n", name, moduleSummaries[name])
	fmt.Fprintln(w)

	reg, err := native.DefaultRegistry()
	if err != nil {
		fmt.Fprintf(w, "describe: cannot load registry: %s\n", err)
		return 1
	}
	desc, err := modules.Resolve(name, reg)
	if err != nil {
		fmt.Fprintf(w, "describe: cannot load module %q: %s\n", name, err)
		return 1
	}

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
	fmt.Fprintf(w, "Import with \"aql:%s\" import, then call e.g. %s.<word>.\n", name, name)
	fmt.Fprintln(w, "Docs: "+helppkg.ReferenceURL)
	return 0
}

func isModule(name string) bool {
	for _, m := range modules.Names() {
		if m == name {
			return true
		}
	}
	return false
}
