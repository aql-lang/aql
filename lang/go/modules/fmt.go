package modules

import (
	"github.com/aql-lang/aql/lang/go/formatter"
	"github.com/aql-lang/aql/lang/go/native"
)

// BuildFmtModule creates the "aql:fmt" native module: the source
// pretty-printer, exposed as a runtime AQL API under the "Fmt" namespace.
//
// After import, the words are accessed via dot notation:
//
//	import "aql:fmt"
//	Fmt.format "def   x    1"        # => "def x 1\n"  — canonical layout
//
// The word wraps the shared, dependency-free driver in lang/go/formatter
// (formatter.Format). The `aql fmt` CLI subcommand calls that SAME driver
// directly on file bytes, so the command line and the language share one
// implementation — a single source of formatting truth rather than two
// paths that can drift (see design/FMT-MODULE.md, "Shared driver").
//
// aql:fmt is a capability/DSL module, so its namespace is the plain "Fmt"
// (not the "*Util" form reserved for utility-helper libraries) — matching
// aql:io -> IO and aql:net -> Net (lang/go/CLAUDE.md, "Naming rule").
func BuildFmtModule(parent *native.Registry) (native.ModuleDesc, error) {
	// Surface a stylesheet-parse failure (the embedded fmt-rules.aql — the
	// rules are EXPRESSED IN AQL, see formatter/rules_aql.go) at module
	// construction rather than formatting with a zero table — the ADR-005
	// recorded-error pattern (native.TypeInitError precedent).
	if err := formatter.DefaultRulesInitError(); err != nil {
		return native.ModuleDesc{}, err
	}
	return buildDelegatingModule(parent, "aql:fmt", "Fmt", FmtNatives)
}

// FmtNatives is the NativeFunc slice for the aql:fmt module's
// Go-implemented words. Each wraps a function in lang/go/formatter so the
// language-level word and the CLI share one driver:
//
//   - format          — format an AQL source string (canonical rule table).
//   - format-markdown — reformat AQL inside ```aql fences and
//     <!-- aqlfmt --> … <!-- /aqlfmt --> regions of a
//     Markdown document, leaving the rest untouched.
//   - format-html     — reformat AQL inside <!-- aqlfmt --> regions of an
//     HTML document, leaving the rest untouched.
//   - rules           — the canonical layout rule table as AQL data.
//   - format-with     — format under a (partial) rule-table override.
//   - tree            — the layout CST as a $kind-tagged value tree.
//   - render          — the document algebra (see fmtdoc.go).
//   - kind / children — the rule-dispatch vocabulary (see fmtrule.go).
//
// Registered into the module's isolated sub-registry by BuildFmtModule;
// every signature is BarrierPos -1 (all-forward eligible), the rule module
// wrappers must follow so the swap form `src Fmt.format` still dispatches
// (lang/go/CLAUDE.md, "Module FnDef Wrappers").
var FmtNatives = append([]native.NativeFunc{
	stringFormatterNative("format", formatter.Format),
	stringFormatterNative("format-markdown", formatter.FormatMarkdown),
	stringFormatterNative("format-html", formatter.FormatHTML),
	renderDocNative(),
	fmtTreeNative(),
	fmtRulesNative(),
	fmtFormatWithNative(),
}, fmtRuleNatives...)

// stringFormatterNative builds a String -> String native named name whose
// handler applies fn to a concrete source string, rejecting a non-concrete
// argument (a type literal / dependent-type constraint) rather than
// panicking.
func stringFormatterNative(name string, fn func(string) string) native.NativeFunc {
	return native.NativeFunc{
		Name: name,
		Signatures: []native.Signature{
			{
				Args: []*native.Type{native.TString},
				Impl: native.Go(func(args []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
					src, err := args[0].AsConcreteString()
					if err != nil {
						return nil, err
					}
					return []native.Value{native.NewString(fn(src))}, nil
				}),
				Returns:    []*native.Type{native.TString},
				BarrierPos: -1,
			},
		},
	}
}
