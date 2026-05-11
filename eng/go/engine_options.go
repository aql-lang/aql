package eng

import (
	"fmt"
	"sort"
)

// EngineOptions configures NewWithOptions. Pass-by-value; the zero
// value of any field means "use the default" — see DefaultEngineOptions
// for the default values. This convention lets callers write partial
// options literally:
//
//	e, err := NewWithOptions(r, EngineOptions{MaxSteps: 5000})
//
// Words is left at the default ("*", install all core words). Or:
//
//	e, err := NewWithOptions(r, EngineOptions{Words: []string{"def", "fn", "dup", "drop"}})
//
// MaxSteps is left at the default (22222), Words is restricted.
//
// To force "no core words at all", set Words to a non-nil empty slice
// (`[]string{}`). A nil Words is treated as "use the default", not
// "register nothing" — the standard Go nil-vs-empty distinction.
type EngineOptions struct {
	// MaxSteps is the upper bound on the number of dispatch steps
	// the engine performs in a single Run. 0 means "use the default"
	// (DefaultEngineOptions().MaxSteps). A negative value disables
	// the limit (effectively unlimited).
	MaxSteps int

	// Words is a whitelist of core-word names to register on the
	// supplied Registry. The special name "*" means "install every
	// core word"; if "*" appears anywhere in the slice, all core
	// words are installed (other entries in that slice are ignored).
	// An unknown name returns an error from NewWithOptions.
	//
	// nil means "use the default" (= ["*"]). An empty non-nil slice
	// means "register no core words at all" — the caller is
	// expected to populate the registry by other means.
	Words []string
}

// DefaultEngineOptions returns the default EngineOptions: top-level
// step limit, all core words installed.
//
// Two intended uses:
//
//   - As a starting point for callers who want to override only a
//     few fields:
//     opts := DefaultEngineOptions()
//     opts.MaxSteps = 5000
//     e, err := NewWithOptions(r, opts)
//
//   - As a documentation pin: code that reads a partial EngineOptions
//     and fills the gaps from this function makes the default-fallback
//     contract explicit at the call site.
//
// Recommended pattern for partial options
// ---------------------------------------
// NewWithOptions itself fills any zero-value fields in the supplied
// EngineOptions from DefaultEngineOptions, so callers who only set
// the fields they care about get the defaults for the rest "for
// free". Two ways to write a partial-options call:
//
//	e, err := NewWithOptions(r, EngineOptions{MaxSteps: 5000})
//
//	opts := DefaultEngineOptions()
//	opts.Words = []string{"def", "fn"}
//	e, err := NewWithOptions(r, opts)
//
// Both are idiomatic. The struct-literal form is more compact for
// one-or-two-field overrides; the DefaultEngineOptions-then-mutate
// form is clearer when the caller wants to communicate "I want all
// the defaults, with these specific tweaks."
//
// We deliberately do NOT provide With-style functional options
// (WithMaxSteps, WithWords, …): the field-set is small and stable,
// the struct-literal form already covers the partial-override case,
// and the With-functions add an indirection without buying anything.
func DefaultEngineOptions() EngineOptions {
	return EngineOptions{
		MaxSteps: 22222,
		Words:    []string{"*"},
	}
}

// NewWithOptions constructs an Engine on the given Registry, applying
// the supplied EngineOptions. Any zero-value field in opts is filled
// from DefaultEngineOptions.
//
// When opts.Words contains "*", every core word listed in
// CoreWordNames is installed on r. Otherwise opts.Words is treated
// as an explicit whitelist — only the named core words are installed,
// in the order given. An unknown name (one that doesn't appear in
// CoreWordNames and isn't "*") returns an *AqlError with code
// "unknown_core_word".
//
// NewWithOptions MUTATES the supplied Registry by registering the
// requested core words. If the caller has already registered words
// with the same names, they will be re-registered (per
// upsertFnDef's append-signatures semantics). Pass a fresh Registry
// from NewRegistry() if you want a clean slate.
func NewWithOptions(registry *Registry, opts EngineOptions) (*Engine, error) {
	defaults := DefaultEngineOptions()
	if opts.MaxSteps == 0 {
		opts.MaxSteps = defaults.MaxSteps
	}
	if opts.Words == nil {
		opts.Words = defaults.Words
	}

	if err := installCoreWordsByWhitelist(registry, opts.Words); err != nil {
		return nil, err
	}

	return &Engine{registry: registry, stepLimit: opts.MaxSteps}, nil
}

// CoreWordNames returns the canonical list of core-word names known
// to NewWithOptions. The returned slice is sorted alphabetically and
// is a fresh copy — callers are free to mutate it.
func CoreWordNames() []string {
	names := make([]string, 0, len(coreWordRegister))
	for name := range coreWordRegister {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// installCoreWordsByWhitelist registers the named core words on r.
// "*" anywhere in the slice means "all core words" and short-
// circuits the per-name loop.
func installCoreWordsByWhitelist(r *Registry, words []string) error {
	for _, w := range words {
		if w == "*" {
			RegisterCoreWords(r)
			return nil
		}
	}
	// Validate first so a bad name in the middle of the list doesn't
	// leave the registry partially populated.
	for _, w := range words {
		if _, ok := coreWordRegister[w]; !ok {
			known := CoreWordNames()
			return &AqlError{
				Code: "unknown_core_word",
				Detail: fmt.Sprintf(
					"EngineOptions.Words contains unknown core word: %q (known core words: %v)",
					w, known,
				),
			}
		}
	}
	for _, w := range words {
		coreWordRegister[w](r)
	}
	return nil
}

// coreWordRegister maps each core-word name to the function that
// installs JUST that word on a Registry. Used by
// installCoreWordsByWhitelist for per-name selection.
//
// The map is populated in init() so each entry references its
// per-word installer in core_words.go / core_boolean.go.
var coreWordRegister map[string]func(*Registry)

func init() {
	coreWordRegister = map[string]func(*Registry){
		"def":   registerCoreDef,
		"fn":    registerCoreFn,
		"quote": registerCoreQuote,
		"args":  registerCoreArgs,
		"dup":   registerCoreDup,
		"swap":  registerCoreSwap,
		"drop":  registerCoreDrop,
		"over":  registerCoreOver,
		"rot":   registerCoreRot,
		"nip":   registerCoreNip,
		"tuck":  registerCoreTuck,
		"dup2":  registerCoreDup2,
		"swap2": registerCoreSwap2,
		"drop2": registerCoreDrop2,
		"over2": registerCoreOver2,
		"not":   registerCoreNot,
		"and":   registerCoreAnd,
		"or":    registerCoreOr,
		"tor":   registerCoreTor,
		"tand":  registerCoreTand,
	}
}
