package modules

import (
	"encoding/hex"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"sync"

	eng "github.com/aql-lang/aql/eng/go"
	"github.com/aql-lang/aql/lang/go/native"
	tabnas "github.com/tabnas/parser/go"
)

// The aql:minilang module — the MiniLang namespace of embedded
// mini-languages behind the core `mini` macro word. See
// design/MINILANG.5.md.
//
// Every mini-language <name> is exported under the partitioned key
// `lang_<name>` and carries the STANDARD minilang signature:
//
//	MiniLang.lang_<name> : [ src:String opts:Map …inputs ] [ …outputs ]
//
// sig[0] is the minilang source text, sig[1] the named parameters
// (`{}` when the caller gave none), and any further positions are
// kind-declared inputs that a `mini` call site fills from the STACK
// (the expansion's trailing `end` stops forward collection). The core
// `mini` word expands `mini <kind> <src> <opts?>` to
// `MiniLang.lang_<kind> <src> <opts> end`; the desugared call is
// always equivalent.
//
// Out-of-band exports (no `lang_` prefix — never reachable via `mini`):
//
//	MiniLang.register  — install an AQL fn as a new mini-language
//	MiniLang.kinds     — list the registered kind atoms

// miniRePatterns memoizes compiled Go regexps keyed by pattern source,
// so a mini call in a loop compiles its pattern once per process.
var (
	miniRePatMu    sync.Mutex
	miniRePatterns = map[string]*regexp.Regexp{}
)

func miniCompiledPattern(src string) (*regexp.Regexp, error) {
	miniRePatMu.Lock()
	defer miniRePatMu.Unlock()
	if re, ok := miniRePatterns[src]; ok {
		return re, nil
	}
	re, err := regexp.Compile(src)
	if err != nil {
		return nil, err
	}
	miniRePatterns[src] = re
	return re, nil
}

// BuildMiniLangModule creates the "aql:minilang" native module.
func BuildMiniLangModule(parent *native.Registry) (native.ModuleDesc, error) {
	subReg, err := native.DefaultRegistry()
	if err != nil {
		return native.ModuleDesc{}, err
	}
	// Compiled carriers escape to the importer (the hook splices them
	// into the parent's tape), so the mint draws its ID from the
	// importing tree's counter.
	subReg.Types.AdoptSeqFrom(parent.Types)
	tMini := subReg.Types.MintType("MiniLangCompiled", native.TIdeal)
	exports := native.NewOrderedMap()

	// mintMiniFnType mints the NAMED member type for a filter kind's
	// partially-applied Function and exports it under the capitalized
	// kind name (`MiniLang.Re`, `MiniLang.Gex`, …). The member
	// predicate matches any Function whose FnDefInfo carries the
	// kind's MiniKind tag — Parent stays TFunction, so every existing
	// fn-value code path is untouched, while `is` and typed fn params
	// ([m:Rex] after `def Rex MiniLang.Re`) dispatch on the specific
	// kind. Per-import mint, like MiniLangCompiled above (and the
	// aql:io StreamKind precedent for exported member types). typeof
	// still reports Function — the member type is a constraint, the
	// same convention DepScalar types follow.
	mintMiniFnType := func(kind string) {
		name := strings.ToUpper(kind[:1]) + kind[1:]
		if _, exists := exports.Get(name); exists {
			return // idempotent (check-mode install + runtime re-run)
		}
		// MintTypeWithBehavior + a bare MemberBehavior rather than
		// MintMemberType: the auto parent gate would reject a
		// DEF-BOUND partial, which lives under Word/__FN (FnDef), not
		// Type/Function — function values have two lattice homes, and
		// the FnDefInfo-payload probe covers both.
		t := subReg.Types.MintTypeWithBehavior(name, native.TFunction,
			eng.MemberBehavior(func(v native.Value) bool {
				info, ok := v.Data.(native.FnDefInfo)
				return ok && info.MiniKind == kind
			}))
		exports.Set(name, native.NewTypeLiteral(t))
	}

	// Wrapper params are UNNAMED — the trivial-delegation short-circuit in
	// execFnDefLiteral requires Body=[Word(inner)] with all-unnamed Params
	// (named params would route through CallAQL name-binding and starve
	// the inner native). See lang/go/CLAUDE.md "Module FnDef Wrappers".
	stdPrefix := []native.FnParam{
		{Type: native.TString}, // src
		{Type: native.TMap},    // opts
	}

	// ---- kind: re — Go regular-expression match -----------------------
	// [src opts subject:String] → [Map] with the match structure
	// {ok ms fst lst n}; each match is {m i e g} (text, byte start/end,
	// capture groups — an unmatched group is none). opts: {limit:I}
	// caps the number of matches (default all).
	subReg.RegisterNativeFunc(native.NativeFunc{
		Name: "minilang-re",
		Signatures: []native.Signature{{
			Args:       []*native.Type{native.TString, native.TMap, native.TString},
			Returns:    []*native.Type{native.TMap},
			BarrierPos: -1,
			Impl:       native.Go(miniReHandler),
		}},
	})
	exports.Set("lang_re", wrapMiniFnDef("minilang-re", [][]native.FnParam{
		append(append([]native.FnParam{}, stdPrefix...), native.FnParam{Type: native.TString}),
	}, []*native.Type{native.TMap}, nil, subReg))

	// ---- compiled re: the `run-re` consumer + the expansion-time hook -----
	// `run-re` takes the precompiled carrier (sig position 0) + opts + the
	// stack subject; the `re` compile hook (registered on parent below)
	// compiles the pattern at the call site and splices a run-re call.
	subReg.RegisterNativeFunc(native.NativeFunc{
		Name: "minilang-run-re",
		Signatures: []native.Signature{{
			Args:       []*native.Type{tMini, native.TMap, native.TString},
			Returns:    []*native.Type{native.TMap},
			BarrierPos: -1,
			Impl:       native.Go(miniRunReHandler),
		}},
	})
	exports.Set("run-re", wrapMiniFnDef("minilang-run-re", [][]native.FnParam{
		{{Type: tMini}, {Type: native.TMap}, {Type: native.TString}},
	}, []*native.Type{native.TMap}, nil, subReg))
	native.RegisterMiniCompileGoHook(parent, "re", miniReCompileFor(tMini))
	mintMiniFnType("re")

	// ---- kind: bf — brainfuck ------------------------------------------
	// Filter form  [src opts input:String] → [String]: the stack value is
	// the `,` input stream. Generator form [src opts] → [String]: input
	// comes from opts.in (default ""). opts: {in:S, steps:I} — steps is
	// the execution budget (default 1e6; exceeding it raises rather than
	// hanging).
	subReg.RegisterNativeFunc(native.NativeFunc{
		Name: "minilang-bf",
		Signatures: []native.Signature{
			{
				Args:       []*native.Type{native.TString, native.TMap, native.TString},
				Returns:    []*native.Type{native.TString},
				BarrierPos: -1,
				Impl:       native.Go(miniBfHandler),
			},
			{
				Args:       []*native.Type{native.TString, native.TMap},
				Returns:    []*native.Type{native.TString},
				BarrierPos: -1,
				Impl:       native.Go(miniBfHandler),
			},
		},
	})
	exports.Set("lang_bf", wrapMiniFnDef("minilang-bf", [][]native.FnParam{
		append(append([]native.FnParam{}, stdPrefix...), native.FnParam{Type: native.TString}),
		stdPrefix,
	}, []*native.Type{native.TString}, nil, subReg))

	// ---- kind: gex — glob-expression selector -------------------------
	// [src opts subject:Any] → [Any]. gex is a SELECTOR (gex's `.on`), not
	// a match-extractor like `re`: a List subject is filtered to matching
	// elements, a Map to entries whose key matches, and a scalar returns
	// itself when it matches or None otherwise. The pattern is anchored
	// (whole-subject), with `**`/`*?` for a literal `*`/`?`. See gex.go.
	subReg.RegisterNativeFunc(native.NativeFunc{
		Name: "minilang-gex",
		Signatures: []native.Signature{{
			Args:       []*native.Type{native.TString, native.TMap, native.TAny},
			Returns:    []*native.Type{native.TAny},
			BarrierPos: -1,
			Impl:       native.Go(miniGexHandler),
		}},
	})
	exports.Set("lang_gex", wrapMiniFnDef("minilang-gex", [][]native.FnParam{
		append(append([]native.FnParam{}, stdPrefix...), native.FnParam{Type: native.TAny}),
	}, []*native.Type{native.TAny}, nil, subReg))
	mintMiniFnType("gex")

	// ---- kind: math — traditional maths formula evaluator --------------
	// [src opts] → [Number]. Evaluate a formula like `x*y-z^2` (operators
	// + - * / % ^, unary +/-, parens) whose variables are bound by the
	// named params (opts). Backed by the tabnas/expr Pratt parser; numeric
	// coercion follows AQL's integer/float domain rules. See minilang_math.go.
	subReg.RegisterNativeFunc(native.NativeFunc{
		Name: "minilang-math",
		Signatures: []native.Signature{{
			Args:       []*native.Type{native.TString, native.TMap},
			Returns:    []*native.Type{native.TNumber},
			BarrierPos: -1,
			Impl:       native.Go(miniMathHandler),
		}},
	})
	exports.Set("lang_math", wrapMiniFnDef("minilang-math", [][]native.FnParam{stdPrefix},
		[]*native.Type{native.TNumber}, nil, subReg))

	// ---- kind: hb — hex Bytes literal ----------------------------------
	// [src opts] → [Bytes]. `+hb/deadbeef/` (≡ mini hb 'deadbeef') decodes
	// an even-length hex string to a Bytes constant. Whitespace and `_` in
	// the source are ignored, so `+hb/de_ad_be_ef/` groups for readability.
	subReg.RegisterNativeFunc(native.NativeFunc{
		Name: "minilang-hb",
		Signatures: []native.Signature{{
			Args:       []*native.Type{native.TString, native.TMap},
			Returns:    []*native.Type{native.TBytes},
			BarrierPos: -1,
			Impl:       native.Go(miniHexBytesHandler),
		}},
	})
	exports.Set("lang_hb", wrapMiniFnDef("minilang-hb", [][]native.FnParam{stdPrefix},
		[]*native.Type{native.TBytes}, nil, subReg))

	// ---- kind: bb — binary Bytes literal -------------------------------
	// [src opts] → [Bytes]. `+bb/01001100/` decodes a string of 0/1 bits
	// (a multiple of 8, MSB-first per byte) to a Bytes constant. Whitespace
	// and `_` are ignored, so `+bb/01001100_11110000/` groups for clarity.
	subReg.RegisterNativeFunc(native.NativeFunc{
		Name: "minilang-bb",
		Signatures: []native.Signature{{
			Args:       []*native.Type{native.TString, native.TMap},
			Returns:    []*native.Type{native.TBytes},
			BarrierPos: -1,
			Impl:       native.Go(miniBinBytesHandler),
		}},
	})
	exports.Set("lang_bb", wrapMiniFnDef("minilang-bb", [][]native.FnParam{stdPrefix},
		[]*native.Type{native.TBytes}, nil, subReg))

	// ---- kind: micron (short form: m) — Micron literal ------------------
	// [src opts] → [Micron]. `+m:alice@example.com` (≡ mini m
	// 'alice@example.com') parses the source with the ONE merged tabnas
	// grammar (eng.MicronFromString): each builtin Micron leaf owns a
	// tabnas literal grammar and the (*Tabnas).Merge combination
	// dispatches on shape — Emailon, then Urlon, then Pathon — so the
	// literal returns the appropriate type. Pathon's grammar accepts any
	// whitespace-free source, so it is the catch-all: `+m:a/b` is a
	// Pathon, and a micron literal never fails to parse.
	// URL sources contain `:` and `/`, so pick a delimiter outside the
	// source — `+m|https://x.com/a` — or the first `:`/`/` closes the
	// literal early (the standard closed-form rule).
	subReg.RegisterNativeFunc(native.NativeFunc{
		Name: "minilang-micron",
		Signatures: []native.Signature{{
			Args:       []*native.Type{native.TString, native.TMap},
			Returns:    []*native.Type{native.TMicron},
			BarrierPos: -1,
			Impl:       native.Go(miniMicronHandlerFor(parent)),
		}},
	})
	micronFnDef := wrapMiniFnDef("minilang-micron", [][]native.FnParam{stdPrefix},
		[]*native.Type{native.TMicron}, nil, subReg)
	exports.Set("lang_micron", micronFnDef)
	exports.Set("lang_m", micronFnDef)

	// ---- out-of-band: micron — the OPT-IN user-Micron literal hook ------
	// `MiniLang.micron <Kind> '<pattern>' <fn>` registers a literal shape
	// for a USER-defined Micron kind: the anchored pattern gates the
	// shape, the fn ([String] → a Kind instance) constructs the value,
	// and the shape merges into the `+m` grammar BETWEEN the builtin
	// leaves and the Pathon catch-all (registration order) — Emailon and
	// Urlon keep their spans; anything the user shapes don't claim still
	// falls to Pathon. Registrations are per-registry and permanent,
	// like MiniLang.register kinds.
	subReg.RegisterNativeFunc(native.NativeFunc{
		Name: "minilang-micron-lit",
		Signatures: []native.Signature{{
			Args:          []*native.Type{native.TAny, native.TString, native.TFunction},
			Returns:       []*native.Type{},
			BarrierPos:    -1,
			CompileEffect: native.CompileStoresFn, // the fn is stored, not invoked on the tape
			Impl:          native.Go(miniMicronLitHandlerFor(parent)),
			// Check-mode hook: the kind/pattern/fn shape rules are
			// value-decidable — a dry pass flags them during analysis
			// (registry-state rules like the duplicate check stay
			// with the runtime handler).
			ReturnsFn: miniMicronLitReturns,
		}},
	})
	exports.Set("micron", wrapMiniFnDef("minilang-micron-lit", [][]native.FnParam{
		{{Type: native.TAny}, {Type: native.TString}, {Type: native.TFunction}},
	}, []*native.Type{}, nil, subReg))

	// ---- kind: jp — JSONPath query (github.com/ohler55/ojg) -------------
	// [src opts doc:Any] → [List]. Run a JSONPath query over the stack
	// subject — a Node (Map/List), Object, Array, Table or Record — and
	// return the matched nodes as a List. See minilang_query.go.
	subReg.RegisterNativeFunc(native.NativeFunc{
		Name: "minilang-jp",
		Signatures: []native.Signature{{
			Args:       []*native.Type{native.TString, native.TMap, native.TAny},
			Returns:    []*native.Type{native.TList},
			BarrierPos: -1,
			Impl:       native.Go(miniJsonPathHandler),
		}},
	})
	exports.Set("lang_jp", wrapMiniFnDef("minilang-jp", [][]native.FnParam{
		append(append([]native.FnParam{}, stdPrefix...), native.FnParam{Type: native.TAny}),
	}, []*native.Type{native.TList}, nil, subReg))
	mintMiniFnType("jp")

	// ---- kind: jq — jq filter (github.com/itchyny/gojq) ----------------
	// [src opts doc:Any] → [List]. Run a jq filter over the stack subject
	// (the same data shapes as jp) and return its output stream as a List.
	subReg.RegisterNativeFunc(native.NativeFunc{
		Name: "minilang-jq",
		Signatures: []native.Signature{{
			Args:       []*native.Type{native.TString, native.TMap, native.TAny},
			Returns:    []*native.Type{native.TList},
			BarrierPos: -1,
			Impl:       native.Go(miniJqHandler),
		}},
	})
	exports.Set("lang_jq", wrapMiniFnDef("minilang-jq", [][]native.FnParam{
		append(append([]native.FnParam{}, stdPrefix...), native.FnParam{Type: native.TAny}),
	}, []*native.Type{native.TList}, nil, subReg))
	mintMiniFnType("jq")

	// ---- kind: xp — XPath query (github.com/antchfx/xpath) -------------
	// [src opts doc:Xml] → [List]. Run an XPath expression over the stack
	// Node/Xml subject and return the result as a List — matched nodes for a
	// node-set (an element as its Node/Xml value, an attribute/text node as a
	// String), or a one-element list for a scalar count/string/boolean result.
	// See minilang_xpath.go.
	subReg.RegisterNativeFunc(native.NativeFunc{
		Name: "minilang-xp",
		Signatures: []native.Signature{{
			Args:       []*native.Type{native.TString, native.TMap, native.TXml},
			Returns:    []*native.Type{native.TList},
			BarrierPos: -1,
			Impl:       native.Go(miniXPathHandler),
		}},
	})
	exports.Set("lang_xp", wrapMiniFnDef("minilang-xp", [][]native.FnParam{
		append(append([]native.FnParam{}, stdPrefix...), native.FnParam{Type: native.TXml}),
	}, []*native.Type{native.TList}, nil, subReg))
	mintMiniFnType("xp")

	// ---- out-of-band: register -----------------------------------------
	// MiniLang.register <name> <fn> installs an AQL function as the
	// mini-language `lang_<name>`. The fn must carry the standard
	// signature prefix [String, Map, …] on every overload. Raw fn values
	// in export maps dispatch like words (lang/spec/fn-value.tsv), so the
	// value is stored as given; `register` exists to own the CONTRACT —
	// name and signature validation, collision checks.
	// registerIdents tracks the source-call identity of each AQL-registered
	// kind so the check-mode install (miniRegisterReturns) and the runtime
	// re-run (miniRegisterHandler) of the SAME `register` call are idempotent
	// rather than a mini_kind_exists collision — mirrors parselang-register.
	miniRegisterIdents := map[string]registerIdent{}
	subReg.RegisterNativeFunc(native.NativeFunc{
		Name: "minilang-register",
		Signatures: []native.Signature{{
			Args:          []*native.Type{native.TAtom, native.TFunction},
			QuoteArgs:     map[int]bool{0: true},
			Returns:       []*native.Type{},
			BarrierPos:    -1,
			CompileEffect: native.CompileStoresFn, // stores the fn for interpreter-side dispatch
			Impl:          native.Go(miniRegisterHandler(exports, miniRegisterIdents, mintMiniFnType)),
			// Check-mode install so a later `mini <name> …` resolves the
			// statically-registered kind (the fn is provided literally) instead
			// of flagging "no mini-language is registered" — the minilang twin
			// of parselang-register's check-mode ReturnsFn.
			ReturnsFn: miniRegisterReturns(exports, miniRegisterIdents, mintMiniFnType),
		}},
	})
	exports.Set("register", wrapMiniFnDef("minilang-register", [][]native.FnParam{
		{{Type: native.TAtom, Quote: true}, {Type: native.TFunction}},
	}, []*native.Type{}, map[int]bool{0: true}, subReg))

	// ---- out-of-band: kinds ----------------------------------------------
	// MiniLang.kinds → List of the registered kind atoms (lang_ stripped).
	subReg.RegisterNativeFunc(native.NativeFunc{
		Name: "minilang-kinds",
		Signatures: []native.Signature{{
			Args:       []*native.Type{},
			Returns:    []*native.Type{native.TList},
			BarrierPos: -1,
			Impl:       native.Go(miniKindsHandler(exports)),
		}},
	})
	exports.Set("kinds", wrapMiniFnDef("minilang-kinds", [][]native.FnParam{{}},
		[]*native.Type{native.TList}, nil, subReg))

	// ---- out-of-band: register-compiled (AQL compile hook) ---------------
	// MiniLang.register-compiled <name> <macro> installs an expansion-time
	// compiler (a macro template) as compile_<name>. The kind must already
	// have a transducer (lang_<name>) — the compiler is an optimization on it.
	// See design/MINILANG.5.md §13.
	subReg.RegisterNativeFunc(native.NativeFunc{
		Name: "minilang-register-compiled",
		Signatures: []native.Signature{{
			Args:          []*native.Type{native.TAtom, native.TFunction},
			QuoteArgs:     map[int]bool{0: true},
			Returns:       []*native.Type{},
			BarrierPos:    -1,
			CompileEffect: native.CompileStoresFn, // stores the fn for interpreter-side dispatch
			Impl:          native.Go(miniRegisterCompiledHandler(exports)),
		}},
	})
	exports.Set("register-compiled", wrapMiniFnDef("minilang-register-compiled", [][]native.FnParam{
		{{Type: native.TAtom, Quote: true}, {Type: native.TFunction}},
	}, []*native.Type{}, map[int]bool{0: true}, subReg))

	// ---- host-registered kinds (the Go embedder API) -------------------
	// Fold in every kind a host installed via RegisterHostMiniLang on the
	// importing registry, then record the live (exports, subReg) pair so a
	// LATER RegisterHostMiniLang injects directly into this already-built
	// module rather than waiting for a re-import that the loaded-module
	// cache would never trigger.
	// create=true: even with no host kinds yet, recording the live module
	// lets a post-import RegisterHostMiniLang inject directly (the loaded
	// cache would otherwise never rebuild).
	state := miniLangHostStateFor(parent, true)
	state.mu.Lock()
	for _, spec := range state.specs {
		if err := installHostMiniLang(exports, subReg, spec); err != nil {
			state.mu.Unlock()
			return native.ModuleDesc{}, err
		}
	}
	state.live = &builtMiniLang{exports: exports, subReg: subReg}
	state.mu.Unlock()

	return native.ModuleDesc{
		ID:      parent.Modules.NextID(),
		Exports: map[string]*native.OrderedMap{"MiniLang": exports},
	}, nil
}

// MiniLangSpec describes a Go-implemented mini-language kind for the host
// registration API (RegisterHostMiniLang). The standard minilang prefix
// [src:String opts:Map] is supplied automatically — a host declares only
// the additional stack Inputs (zero or more, AFTER the prefix), the
// Returns, and the Handler. The handler receives args[0]=src,
// args[1]=opts, args[2..]=Inputs in order, exactly like a built-in kind.
type MiniLangSpec struct {
	// Name is the kind atom, unprefixed and lowercase ("iop", not
	// "lang_iop"). It is the token a caller writes after `mini`.
	Name string
	// Inputs are the kind's stack inputs AFTER the [src opts] prefix.
	// Nil/empty for a generator kind (inputs come from opts). Params
	// should be unnamed (only Type / Quote are read) so the wrapper keeps
	// the trivial-delegation dispatch short-circuit.
	Inputs []native.FnParam
	// Returns are the kind's output types (nil = no declared output).
	Returns []*native.Type
	// Handler implements the kind at runtime — the immediate transducer
	// `[src opts …inputs] → outputs`. Required: it is the semantic reference
	// and the dynamic-src fallback even when Compile is set.
	Handler native.Handler
	// Compile is an OPTIONAL expansion-time compiler. When set, a `mini
	// <Name> <src>` call runs it at the call site (whenever src is concrete)
	// and splices its tokens instead of the standard call — e.g. a precompiled
	// carrier + a consumer word. `mini` has no expansion cache, so the hook
	// re-runs per call: memoize the compile (as `re` does). A non-concrete
	// runtime src and check mode fall back to Handler.
	Compile native.MiniCompileHook
}

// capMiniLangHost is the registry capability slot holding host-registered
// mini-language kinds. Per-registry so each lang.New() instance owns its
// own set (no process-global table, no leak across instances).
const capMiniLangHost = "engine.minilang.host"

// miniLangHostState is the per-registry record of host-registered kinds.
// specs is the source of truth (folded into the module at build time);
// live points at the built module so a post-import registration can inject
// straight away.
type miniLangHostState struct {
	mu    sync.Mutex
	specs []MiniLangSpec
	live  *builtMiniLang
}

// builtMiniLang is the live (exports, subReg) pair captured once
// BuildMiniLangModule has run for a registry.
type builtMiniLang struct {
	exports *native.OrderedMap
	subReg  *native.Registry
}

// miniLangHostStateFor returns the host-minilang state on r, creating it
// when create is true. Stored on the registry's capability store so it is
// scoped to the instance and collected with it.
func miniLangHostStateFor(r *native.Registry, create bool) *miniLangHostState {
	if s, ok, _ := eng.Cap[*miniLangHostState](r, capMiniLangHost); ok && s != nil {
		return s
	}
	if !create {
		return nil
	}
	s := &miniLangHostState{}
	_ = r.Capabilities.Set(capMiniLangHost, s)
	return s
}

// RegisterHostMiniLang installs a Go-implemented mini-language kind on reg
// — the embedder twin of the AQL-level `MiniLang.register` word. The kind
// becomes resolvable as `mini <Name> …` once the program imports
// "aql:minilang"; registration may happen before OR after that import
// (a post-import call injects into the already-built module).
//
// It owns the same contract as `MiniLang.register`: a lowercase kind name
// with no `lang_` prefix, a non-nil handler, and the standard
// [src:String opts:Map …] signature prefix (guaranteed here by
// construction). A name that collides with an existing kind — built-in or
// host — is refused.
func RegisterHostMiniLang(reg *native.Registry, spec MiniLangSpec) error {
	if why := miniValidKindName(spec.Name); why != "" {
		return fmt.Errorf("register minilang: %s", why)
	}
	if spec.Handler == nil {
		return fmt.Errorf("register minilang %q: handler must not be nil", spec.Name)
	}
	if spec.Compile != nil {
		native.RegisterMiniCompileGoHook(reg, spec.Name, spec.Compile)
	}
	state := miniLangHostStateFor(reg, true)
	state.mu.Lock()
	defer state.mu.Unlock()
	for _, s := range state.specs {
		if s.Name == spec.Name {
			return fmt.Errorf("register minilang %q: already registered", spec.Name)
		}
	}
	// If the module is already built in this registry, install live (this
	// also catches a collision against a built-in kind such as `re`/`bf`).
	if state.live != nil {
		if err := installHostMiniLang(state.live.exports, state.live.subReg, spec); err != nil {
			return err
		}
	}
	state.specs = append(state.specs, spec)
	return nil
}

// installHostMiniLang registers spec's handler as an inner native in subReg
// and exports the standard trivial-delegation wrapper under lang_<name>.
// Mirrors the built-in kind path (wrapMiniFnDef) so a host kind dispatches
// identically. The [src opts] prefix is prepended here; host Inputs are
// copied type/quote-only to keep every wrapper param unnamed.
func installHostMiniLang(exports *native.OrderedMap, subReg *native.Registry, spec MiniLangSpec) error {
	key := "lang_" + spec.Name
	if _, exists := exports.Get(key); exists {
		return fmt.Errorf("register minilang %q: already registered", spec.Name)
	}
	params := make([]native.FnParam, 0, 2+len(spec.Inputs))
	params = append(params,
		native.FnParam{Type: native.TString},
		native.FnParam{Type: native.TMap})
	for _, in := range spec.Inputs {
		params = append(params, native.FnParam{Type: in.Type, Quote: in.Quote})
	}
	args := make([]*native.Type, len(params))
	for i, p := range params {
		args[i] = p.Type
	}
	inner := "minilang-host-" + spec.Name
	subReg.RegisterNativeFunc(native.NativeFunc{
		Name: inner,
		Signatures: []native.Signature{{
			Args:       args,
			Returns:    spec.Returns,
			BarrierPos: -1,
			Impl:       native.Go(spec.Handler),
		}},
	})
	exports.Set(key, wrapMiniFnDef(inner, [][]native.FnParam{params}, spec.Returns, nil, subReg))
	return nil
}

// wrapMiniFnDef builds the module FnDef wrapper for an inner native —
// the wrapRandFnDef pattern, generalised to multiple overloads. Every
// FnSig is a trivial delegation (Body=[Word(inner)]) with BarrierPos -1
// (CRITICAL — see lang/go/CLAUDE.md "Module FnDef Wrappers").
func wrapMiniFnDef(wordName string, overloads [][]native.FnParam, returns []*native.Type, quoteArgs map[int]bool, subReg *native.Registry) native.Value {
	sigs := make([]native.FnSig, 0, len(overloads))
	for _, params := range overloads {
		sigs = append(sigs, native.FnSig{
			Params:     params,
			Returns:    returns,
			Impl:       native.AQL([]native.Value{native.NewWord(wordName)}),
			BarrierPos: -1,
			QuoteArgs:  quoteArgs,
		})
	}
	return native.NewFnDef(native.FnDefInfo{
		Name:       wordName,
		Signatures: sigs,
		Registry:   subReg,
	})
}

// miniOptInt reads an Integer option from an opts map, returning def
// when the key is absent. A present-but-non-integer value is an error.
func miniOptInt(opts native.Value, key string, def int64) (int64, error) {
	m, _ := native.AsMap(opts)
	if m == nil {
		return def, nil
	}
	v, ok := m.Get(key)
	if !ok {
		return def, nil
	}
	n, err := v.AsConcreteInteger()
	if err != nil {
		return 0, fmt.Errorf("opts.%s: %w", key, err)
	}
	return n, nil
}

// miniOptString reads a String option, "" default.
func miniOptString(opts native.Value, key string) (string, error) {
	m, _ := native.AsMap(opts)
	if m == nil {
		return "", nil
	}
	v, ok := m.Get(key)
	if !ok {
		return "", nil
	}
	s, err := v.AsConcreteString()
	if err != nil {
		return "", fmt.Errorf("opts.%s: %w", key, err)
	}
	return s, nil
}

// miniDropGrouping strips ASCII whitespace and underscores so a binary/hex
// source can be grouped for readability (`+hb/de_ad_be_ef/`, `+bb/0100_1100/`).
func miniDropGrouping(s string) string {
	return strings.Map(func(c rune) rune {
		switch c {
		case ' ', '\t', '\n', '\r', '_':
			return -1
		}
		return c
	}, s)
}

// capMicronLits is the registry capability slot holding the user-Micron
// literal registrations (MiniLang.micron) and their memoized merged
// grammar. Scoped to the registry instance, like capMiniLangHost.
const capMicronLits = "engine.minilang.micron-literals"

// micronLitState is the per-registry record of user-Micron literal
// shapes. specs is the source of truth in registration order; grammar
// is the memoized eng.MicronGrammarWith merge (nil = rebuild). r and
// firstErr bridge one Parse call: the grammar actions dispatch the
// registered AQL fns re-entrantly through r, and a builder failure is
// recorded here so the handler can raise it LOUDLY (a registered shape
// that matched has claimed the span — the eng-side action would
// otherwise fall back to Pathon). Single-threaded under mu.
type micronLitState struct {
	mu       sync.Mutex
	specs    []eng.MicronLiteralSpec
	kinds    map[string]bool
	grammar  *tabnas.Tabnas
	r        *native.Registry
	firstErr error
}

// micronLitStateFor returns the user-Micron literal state on r,
// creating it when create is true.
func micronLitStateFor(r *native.Registry, create bool) *micronLitState {
	if s, ok, _ := eng.Cap[*micronLitState](r, capMicronLits); ok && s != nil {
		return s
	}
	if !create {
		return nil
	}
	s := &micronLitState{kinds: map[string]bool{}}
	_ = r.Capabilities.Set(capMicronLits, s)
	return s
}

// miniMicronLitHandlerFor builds the `MiniLang.micron` handler —
// args[0]=the user Micron kind, args[1]=the shape pattern (a Go
// regexp, auto-anchored to the whole span), args[2]=the builder fn
// ([String] → a Kind instance). Registrations key on the IMPORTING
// registry (captured at module build, like Parse.register).
// miniMicronLitValidate is the shared kind/pattern/fn validation for
// the runtime handler and the check-mode dry pass. In lenient mode a
// non-concrete value is skipped, never flagged. Returns the resolved
// kind and compiled (anchored) pattern; either may be nil in lenient
// mode when the corresponding arg was a carrier.
func miniMicronLitValidate(args []native.Value, r *native.Registry, lenient bool) (*native.Type, *regexp.Regexp, native.Value, error) {
	kindV := args[0]
	var kind *native.Type
	switch {
	case eng.IsMicronType(kindV):
		info, _ := eng.AsMicronType(kindV)
		kind = info.Type
	case native.IsBareTypeNode(kindV):
		kind = native.CanonicalType(r, &kindV)
	}
	if kind == nil || !kind.ConformsTo(native.TMicron) {
		if lenient && !native.IsConcrete(kindV) && !native.IsBareTypeNode(kindV) {
			kind = nil // a carrier — the runtime handler decides
		} else {
			return nil, nil, native.Value{}, r.AqlError("micron_literal",
				fmt.Sprintf("MiniLang.micron: expected a Micron kind, got %s", kindV.String()), "micron")
		}
	}
	if kind != nil && kind.Origin != eng.OriginUserDef {
		return nil, nil, native.Value{}, r.AqlErrorHint("micron_literal",
			fmt.Sprintf("MiniLang.micron: %s is a builtin Micron — its literal shape is fixed", kind.Name),
			"micron",
			"register shapes for USER kinds only (def Nameon refine Micron {…})")
	}
	var re *regexp.Regexp
	if !lenient || native.IsConcrete(args[1]) {
		pat, err := args[1].AsConcreteString()
		if err != nil {
			return nil, nil, native.Value{}, r.AqlError("micron_literal",
				fmt.Sprintf("MiniLang.micron: pattern: %v", err), "micron")
		}
		var rerr error
		re, rerr = miniCompiledPattern(`\A(?:` + pat + `)\z`)
		if rerr != nil {
			return nil, nil, native.Value{}, r.AqlError("micron_literal",
				fmt.Sprintf("MiniLang.micron: invalid pattern %q: %v", pat, rerr), "micron")
		}
	}
	fn := args[2]
	if !isCallableValue(fn) && !(lenient && !native.IsConcrete(fn)) {
		return nil, nil, native.Value{}, r.AqlError("micron_literal",
			fmt.Sprintf("MiniLang.micron: the builder must be a Function, got %s", fn.String()), "micron")
	}
	return kind, re, fn, nil
}

// miniMicronLitReturns is MiniLang.micron's check-mode hook: the
// kind/pattern/fn shape rules are value-decidable, so a lenient dry
// pass flags them during analysis with the byte-identical runtime
// message. Registry-state rules (the duplicate-kind check) stay with
// the runtime handler.
func miniMicronLitReturns(args []native.Value, r *native.Registry) []native.Value {
	if r != nil && r.Check.IsActive() && len(args) == 3 {
		if _, _, _, err := miniMicronLitValidate(args, r, true); err != nil {
			code, detail := "micron_literal", err.Error()
			var ae *eng.AqlError
			if errors.As(err, &ae) {
				code, detail = ae.Code, ae.Detail
			}
			eng.CheckAddUniqueDiagnostic(r, code, detail, "micron", args[0].Pos)
		}
	}
	return []native.Value{}
}

func miniMicronLitHandlerFor(parent *native.Registry) native.Handler {
	return func(args []native.Value, _ map[string]native.Value, _ []native.Value, r *native.Registry) ([]native.Value, error) {
		kind, re, fn, err := miniMicronLitValidate(args, parent, false)
		if err != nil {
			return nil, err
		}

		st := micronLitStateFor(parent, true)
		st.mu.Lock()
		defer st.mu.Unlock()
		if st.kinds[kind.Name] {
			return nil, r.AqlError("micron_literal",
				fmt.Sprintf("MiniLang.micron: a literal shape for %s is already registered", kind.Name), "micron")
		}
		st.kinds[kind.Name] = true
		kindT := kind
		st.specs = append(st.specs, eng.MicronLiteralSpec{
			Tag:     kind.Name,
			Token:   fmt.Sprintf("#U%d%s", len(st.specs), strings.ToUpper(kind.Name)),
			Pattern: re,
			Build: func(s string) (native.Value, error) {
				out, berr := callParseFn(st.r, fn, []native.Value{native.NewString(s)})
				if berr == nil && (len(out) != 1 || !out[0].Parent.ConformsTo(kindT)) {
					berr = fmt.Errorf("the %s builder must return one %s instance", kindT.Name, kindT.Name)
				}
				if berr != nil {
					st.firstErr = fmt.Errorf("%s literal %q: %v", kindT.Name, s, berr)
					return native.Value{}, berr
				}
				return out[0], nil
			},
		})
		st.grammar = nil // rebuild on next parse
		return nil, nil
	}
}

// miniMicronHandlerFor builds the micron/m transducer — args[0]=src,
// args[1]=opts (none defined). With no MiniLang.micron registrations
// it parses with the builtin merged grammar (eng.MicronFromString);
// otherwise with the per-registry merge that splices the user shapes
// between the builtin leaves and the Pathon catch-all.
func miniMicronHandlerFor(parent *native.Registry) native.Handler {
	return func(args []native.Value, _ map[string]native.Value, _ []native.Value, r *native.Registry) ([]native.Value, error) {
		src, err := args[0].AsConcreteString()
		if err != nil {
			return nil, r.AqlError("mini_parse_error", fmt.Sprintf("micron: src: %v", err), "lang_micron")
		}
		st := micronLitStateFor(parent, false)
		if st == nil {
			return miniMicronBuiltin(src, r)
		}
		st.mu.Lock()
		defer st.mu.Unlock()
		if len(st.specs) == 0 {
			return miniMicronBuiltin(src, r)
		}
		if src == "" {
			return miniMicronBuiltin(src, r) // the empty relative path, as eng handles it
		}
		if st.grammar == nil {
			g, gerr := eng.MicronGrammarWith(st.specs...)
			if gerr != nil {
				return nil, r.AqlError("mini_parse_error", fmt.Sprintf("micron: grammar merge: %v", gerr), "lang_micron")
			}
			st.grammar = g
		}
		st.r, st.firstErr = r, nil
		node, perr := st.grammar.Parse(src)
		st.r = nil
		if st.firstErr != nil {
			return nil, r.AqlError("mini_parse_error", fmt.Sprintf("micron: %v", st.firstErr), "lang_micron")
		}
		if perr != nil {
			return nil, r.AqlError("mini_parse_error",
				fmt.Sprintf("micron: cannot parse literal %q: %v", src, native.FirstCleanLine(perr.Error())), "lang_micron")
		}
		v, ok := node.(native.Value)
		if !ok {
			return nil, r.AqlError("mini_parse_error",
				fmt.Sprintf("micron: literal %q did not produce a value", src), "lang_micron")
		}
		return []native.Value{v}, nil
	}
}

// miniMicronBuiltin is the no-registrations fast path: the builtin
// merged grammar via eng.MicronFromString.
func miniMicronBuiltin(src string, r *native.Registry) ([]native.Value, error) {
	v, merr := eng.MicronFromString(src)
	if merr != nil {
		return nil, r.AqlError("mini_parse_error", fmt.Sprintf("micron: %v", merr), "lang_micron")
	}
	return []native.Value{v}, nil
}

// miniHexBytesHandler — args[0]=src, args[1]=opts. Decodes an even-length
// hex string to Bytes. The value is built via eng.FromNative (the
// []byte→Bytes bridge), which runs here at runtime where the bridge is live.
func miniHexBytesHandler(args []native.Value, _ map[string]native.Value, _ []native.Value, r *native.Registry) ([]native.Value, error) {
	src, err := args[0].AsConcreteString()
	if err != nil {
		return nil, r.AqlError("mini_parse_error", fmt.Sprintf("hb: src: %v", err), "lang_hb")
	}
	b, derr := hex.DecodeString(miniDropGrouping(src))
	if derr != nil {
		return nil, r.AqlErrorHint("mini_parse_error", fmt.Sprintf("hb: %v", derr), "lang_hb",
			"use an even number of hex digits, e.g. +hb/deadbeef/")
	}
	return []native.Value{eng.FromNative(b)}, nil
}

// miniBinBytesHandler — args[0]=src, args[1]=opts. Decodes a string of 0/1
// bits (a multiple of 8, MSB-first per byte) to Bytes.
func miniBinBytesHandler(args []native.Value, _ map[string]native.Value, _ []native.Value, r *native.Registry) ([]native.Value, error) {
	src, err := args[0].AsConcreteString()
	if err != nil {
		return nil, r.AqlError("mini_parse_error", fmt.Sprintf("bb: src: %v", err), "lang_bb")
	}
	bits := miniDropGrouping(src)
	if len(bits)%8 != 0 {
		return nil, r.AqlErrorHint("mini_parse_error",
			fmt.Sprintf("bb: bit count %d is not a multiple of 8", len(bits)), "lang_bb",
			"pad to whole bytes, e.g. +bb/01001100/")
	}
	out := make([]byte, len(bits)/8)
	for i := range out {
		var v byte
		for j := 0; j < 8; j++ {
			v <<= 1
			switch bits[i*8+j] {
			case '1':
				v |= 1
			case '0':
			default:
				return nil, r.AqlErrorHint("mini_parse_error",
					fmt.Sprintf("bb: %q is not a binary digit", string(bits[i*8+j])), "lang_bb",
					"use only 0 and 1, e.g. +bb/01001100/")
			}
		}
		out[i] = v
	}
	return []native.Value{eng.FromNative(out)}, nil
}

// miniReHandler — args[0]=src, args[1]=opts, args[2]=subject.
func miniReHandler(args []native.Value, _ map[string]native.Value, _ []native.Value, r *native.Registry) ([]native.Value, error) {
	src, err := args[0].AsConcreteString()
	if err != nil {
		return nil, r.AqlError("mini_error", fmt.Sprintf("re: src: %v", err), "lang_re")
	}
	subject, err := args[2].AsConcreteString()
	if err != nil {
		return nil, r.AqlError("mini_error", fmt.Sprintf("re: subject: %v", err), "lang_re")
	}
	limit, err := miniOptInt(args[1], "limit", -1)
	if err != nil {
		return nil, r.AqlError("mini_error", fmt.Sprintf("re: %v", err), "lang_re")
	}
	re, cerr := miniCompiledPattern(src)
	if cerr != nil {
		return nil, r.AqlErrorHint("mini_parse_error",
			fmt.Sprintf("re: %v", cerr), "lang_re",
			"fix the pattern — note backslashes in '…'/\"…\" strings need doubling ('\\\\d'); backtick strings are backslash-safe")
	}
	return []native.Value{reMatchResult(re, subject, limit)}, nil
}

// reMatchResult builds the standard re match structure {ok ms fst lst n} —
// shared by the runtime kind (lang_re, compiles per call via the memo) and the
// compiled consumer (run-re, gets a precompiled regexp from the carrier).
func reMatchResult(re *regexp.Regexp, subject string, limit int64) native.Value {
	idxs := re.FindAllStringSubmatchIndex(subject, int(limit))
	matches := make([]native.Value, 0, len(idxs))
	for _, ix := range idxs {
		mm := native.NewOrderedMap()
		mm.Set("m", native.NewString(subject[ix[0]:ix[1]]))
		mm.Set("i", native.NewInteger(int64(ix[0])))
		mm.Set("e", native.NewInteger(int64(ix[1])))
		groups := make([]native.Value, 0, len(ix)/2-1)
		for g := 1; g < len(ix)/2; g++ {
			s, e := ix[2*g], ix[2*g+1]
			if s < 0 {
				groups = append(groups, native.NewNone())
			} else {
				groups = append(groups, native.NewString(subject[s:e]))
			}
		}
		mm.Set("g", native.NewList(groups))
		matches = append(matches, native.NewMap(mm))
	}

	out := native.NewOrderedMap()
	out.Set("ok", native.NewBoolean(len(matches) > 0))
	out.Set("ms", native.NewList(matches))
	if len(matches) > 0 {
		out.Set("fst", matches[0])
		out.Set("lst", matches[len(matches)-1])
	}
	out.Set("n", native.NewInteger(int64(len(matches))))
	return native.NewMap(out)
}

// ---- compiled re: the carrier type + the `run-re` consumer + the hook ----

// The MiniLangCompiled carrier type — the inert value a compile hook
// splices in place of the DSL source, wrapping the kind's precompiled
// artifact (for `re`, a *regexp.Regexp) in an ExtensionPayload the
// kernel never inspects — is a per-import module mint (former global
// FixedID 5003, retired): BuildMiniLangModule mints it and threads it
// to the run-re consumer and the `re` compile hook. See
// MintTemporalModuleTypes / MintTensorTypes for the pattern.

// miniRunReHandler — args[0]=compiled carrier, args[1]=opts, args[2]=subject.
// The compiled consumer for `re`: the pattern was compiled at the call site by
// miniReCompile, so this just matches.
func miniRunReHandler(args []native.Value, _ map[string]native.Value, _ []native.Value, r *native.Registry) ([]native.Value, error) {
	ep, ok := args[0].Data.(eng.ExtensionPayload)
	if !ok {
		return nil, r.AqlError("mini_error", "run-re: not a compiled pattern", "run-re")
	}
	re, ok := ep.Body.(*regexp.Regexp)
	if !ok {
		return nil, r.AqlError("mini_error", "run-re: not a compiled pattern", "run-re")
	}
	subject, err := args[2].AsConcreteString()
	if err != nil {
		return nil, r.AqlError("mini_error", fmt.Sprintf("re: subject: %v", err), "run-re")
	}
	limit, err := miniOptInt(args[1], "limit", -1)
	if err != nil {
		return nil, r.AqlError("mini_error", fmt.Sprintf("re: %v", err), "run-re")
	}
	return []native.Value{reMatchResult(re, subject, limit)}, nil
}

// miniReCompile is the `re` compile hook: it compiles the pattern at the call
// site and splices `MiniLang get run-re <compiled> <opts> end` — the inert
// precompiled carrier plus the consumer word, so the per-call cost is just the
// match (the compile is memoized by miniCompiledPattern).
//
// On a MALFORMED pattern it defers to the standard `lang_re` call rather than
// raising here: the carrier can't be built, and deferring keeps the error
// byte-identical to the transducer — which is what compiled mode runs (the
// bytecode recorder, in check mode, never takes the compile-hook path), so
// compiled/interpreted parity holds. Valid patterns still get the carrier.
func miniReCompileFor(tMini *native.Type) func(string, native.Value, *native.Registry) ([]native.Value, error) {
	return func(src string, opts native.Value, _ *native.Registry) ([]native.Value, error) {
		re, cerr := miniCompiledPattern(src)
		if cerr != nil {
			return []native.Value{
				native.NewWord("MiniLang"), native.NewWord("dot"), native.NewWord("lang_re"),
				native.NewString(src), opts, native.NewEnd(),
			}, nil
		}
		carrier := eng.NewExtension(tMini, re)
		return []native.Value{
			native.NewWord("MiniLang"), native.NewWord("dot"), native.NewWord("run-re"),
			carrier, opts, native.NewEnd(),
		}, nil
	}
}

// miniBfDefaultSteps is the default brainfuck execution budget — loud
// failure instead of a hang on a runaway program.
const miniBfDefaultSteps = 1_000_000

// miniBfHandler — args[0]=src, args[1]=opts, optional args[2]=input.
// Input precedence: stack input (3-arg form) > opts.in > "".
func miniBfHandler(args []native.Value, _ map[string]native.Value, _ []native.Value, r *native.Registry) ([]native.Value, error) {
	src, err := args[0].AsConcreteString()
	if err != nil {
		return nil, r.AqlError("mini_error", fmt.Sprintf("bf: src: %v", err), "lang_bf")
	}
	input := ""
	if len(args) == 3 {
		input, err = args[2].AsConcreteString()
		if err != nil {
			return nil, r.AqlError("mini_error", fmt.Sprintf("bf: input: %v", err), "lang_bf")
		}
	} else {
		input, err = miniOptString(args[1], "in")
		if err != nil {
			return nil, r.AqlError("mini_error", fmt.Sprintf("bf: %v", err), "lang_bf")
		}
	}
	steps, err := miniOptInt(args[1], "steps", miniBfDefaultSteps)
	if err != nil {
		return nil, r.AqlError("mini_error", fmt.Sprintf("bf: %v", err), "lang_bf")
	}

	out, bferr := runBrainfuck(src, input, steps)
	if bferr != nil {
		code := "mini_eval_error"
		hint := "the program exceeded its limits — raise opts {steps:N} or fix the loop"
		if bferr.parse {
			code = "mini_parse_error"
			hint = "balance the [ ] brackets"
		}
		return nil, r.AqlErrorHint(code, fmt.Sprintf("bf: %s", bferr.msg), "lang_bf", hint)
	}
	return []native.Value{native.NewString(out)}, nil
}

type bfError struct {
	msg   string
	parse bool
}

// runBrainfuck interprets the eight-command language over a 30000-cell
// byte tape (cells wrap mod 256; the pointer does NOT wrap — leaving the
// tape is an error). `,` reads from input, yielding 0 at EOF. maxSteps
// bounds executed instructions.
func runBrainfuck(prog, input string, maxSteps int64) (string, *bfError) {
	// Precompute the bracket jump table.
	jump := make(map[int]int)
	var stack []int
	for pc := 0; pc < len(prog); pc++ {
		switch prog[pc] {
		case '[':
			stack = append(stack, pc)
		case ']':
			if len(stack) == 0 {
				return "", &bfError{msg: fmt.Sprintf("unbalanced ']' at offset %d", pc), parse: true}
			}
			open := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			jump[open] = pc
			jump[pc] = open
		}
	}
	if len(stack) > 0 {
		return "", &bfError{msg: fmt.Sprintf("unbalanced '[' at offset %d", stack[len(stack)-1]), parse: true}
	}

	cells := make([]byte, 30000)
	ptr, in := 0, 0
	var out strings.Builder
	var steps int64
	for pc := 0; pc < len(prog); pc++ {
		steps++
		if steps > maxSteps {
			return "", &bfError{msg: fmt.Sprintf("step budget %d exceeded", maxSteps)}
		}
		switch prog[pc] {
		case '+':
			cells[ptr]++
		case '-':
			cells[ptr]--
		case '>':
			ptr++
			if ptr >= len(cells) {
				return "", &bfError{msg: fmt.Sprintf("pointer out of range at offset %d", pc)}
			}
		case '<':
			ptr--
			if ptr < 0 {
				return "", &bfError{msg: fmt.Sprintf("pointer out of range at offset %d", pc)}
			}
		case '.':
			out.WriteByte(cells[ptr])
		case ',':
			if in < len(input) {
				cells[ptr] = input[in]
				in++
			} else {
				cells[ptr] = 0
			}
		case '[':
			if cells[ptr] == 0 {
				pc = jump[pc]
			}
		case ']':
			if cells[ptr] != 0 {
				pc = jump[pc]
			}
		}
	}
	return out.String(), nil
}

// miniValidKindName reports why a kind name is unusable, or "".
func miniValidKindName(name string) string {
	if name == "" {
		return "kind name must not be empty"
	}
	if strings.HasPrefix(name, "lang_") {
		return "kind name must not carry the lang_ prefix (register adds it)"
	}
	if name[0] >= 'A' && name[0] <= 'Z' {
		return "kind name must be lowercase (capitalised names are types)"
	}
	return ""
}

// miniRegisterHandler validates and installs an AQL fn as lang_<name>.
// The handler closes over the module's export map; the stored value is
// the raw Function, which dispatches like a word post the fn-value fix.
func miniRegisterHandler(exports *native.OrderedMap, idents map[string]registerIdent, mint func(kind string)) native.Handler {
	return func(args []native.Value, _ map[string]native.Value, _ []native.Value, r *native.Registry) ([]native.Value, error) {
		if err := miniRegisterInstall(exports, idents, args, r, mint); err != nil {
			return nil, err
		}
		return nil, nil
	}
}

// miniRegisterInstall validates the name/signature contract and installs the
// fn as lang_<name>. Shared by the runtime handler and the check-mode
// ReturnsFn so both apply the exact same contract — mirrors
// parseRegisterInstall; the collision / idempotency rule lives in
// registerCollisionInstall (shared with ParseLang / EmitLang).
func miniRegisterInstall(exports *native.OrderedMap, idents map[string]registerIdent, args []native.Value, r *native.Registry, mint func(kind string)) error {
	name, err := args[0].AsConcreteAtom()
	if err != nil {
		return r.AqlError("mini_bad_name", fmt.Sprintf("register: %v", err), "register")
	}
	if why := miniValidKindName(name); why != "" {
		return r.AqlError("mini_bad_name", "register: "+why, "register")
	}
	fnDef, ok := args[1].Data.(native.FnDefInfo)
	if !ok || len(fnDef.Signatures) == 0 {
		return r.AqlError("mini_bad_signature", "register: expected a function value", "register")
	}
	filterShaped := true
	for _, sig := range fnDef.Signatures {
		if len(sig.Params) < 2 ||
			sig.Params[0].Type == nil || !sig.Params[0].Type.ConformsTo(native.TString) ||
			sig.Params[1].Type == nil || !sig.Params[1].Type.ConformsTo(native.TMap) {
			return r.AqlErrorHint("mini_bad_signature",
				"register: every signature must start with the standard prefix [src:String opts:Map …]",
				"register",
				"declare the fn as fn [[src:String opts:Map …inputs] [outputs] [body]]")
		}
		if len(sig.Params) != 3 {
			filterShaped = false
		}
	}
	key := "lang_" + name
	if err := registerCollisionInstall(exports, idents, key, args[1], r.Check.IsActive(), func() error {
		return r.AqlError("mini_kind_exists",
			fmt.Sprintf("register: minilang %q is already registered", name), "register")
	}); err != nil {
		return err
	}
	// A FILTER-shaped kind (every sig exactly [src opts subject]) expands
	// to a MiniKind-tagged partial, so it gets the same named member type
	// the builtin filter kinds have (`MiniLang.Poly` for kind `poly`) —
	// usable in `is` and typed fn params. Other shapes never produce a
	// tagged partial, so a type would be uninhabited: skip the mint.
	if filterShaped && mint != nil {
		mint(name)
	}
	return nil
}

// miniRegisterReturns is the check-mode counterpart of miniRegisterHandler: it
// performs the SAME install so a dynamically-registered kind
// (`MiniLang.register poly …`) is statically resolvable by a later
// `mini poly …` during the check pass. Idempotent on the source-call identity,
// so the compiled program's runtime re-run is a no-op rather than
// mini_kind_exists. A non-concrete name or fn value leaves the kind
// unregistered (the downstream `mini` degrades as before).
func miniRegisterReturns(exports *native.OrderedMap, idents map[string]registerIdent, mint func(kind string)) native.ReturnsFunc {
	return func(args []native.Value, r *native.Registry) []native.Value {
		// PURE-CHECK ONLY. Under a REAL compile (CompileCheck) the kind must NOT
		// be installed here: a kind that ALSO has a `register-compiled` macro
		// hook (module-minilang §L80) lowers `mini <name>` through the hook, and
		// pre-installing the transducer makes the compiled path diverge from the
		// interpreter (it ran the transducer instead of the hook). Leaving the
		// compile pass un-installed keeps that row on its prior fallback path;
		// pure check still installs so `mini <name>` resolves and the row clears.
		if r == nil || r.Check.Compiling {
			return nil
		}
		if len(args) < 2 || !native.IsConcrete(args[0]) {
			return nil
		}
		if _, ok := args[1].Data.(native.FnDefInfo); !ok {
			return nil
		}
		if err := miniRegisterInstall(exports, idents, args, r, mint); err != nil {
			surfaceRegisterCheckError(r, err, args[0])
		}
		return nil
	}
}

// miniRegisterCompiledHandler installs an AQL compile hook (a macro) as
// compile_<name>. The kind must already have a transducer (lang_<name>) — the
// compiler is an optional optimization layered on it, and the transducer is
// the semantic reference / check-mode target / non-concrete-src fallback.
// args[0]=name (atom), args[1]=macro.
func miniRegisterCompiledHandler(exports *native.OrderedMap) native.Handler {
	return func(args []native.Value, _ map[string]native.Value, _ []native.Value, r *native.Registry) ([]native.Value, error) {
		name, err := args[0].AsConcreteAtom()
		if err != nil {
			return nil, r.AqlError("mini_bad_name", fmt.Sprintf("register-compiled: %v", err), "register-compiled")
		}
		if why := miniValidKindName(name); why != "" {
			return nil, r.AqlError("mini_bad_name", "register-compiled: "+why, "register-compiled")
		}
		if _, ok := exports.Get("lang_" + name); !ok {
			return nil, r.AqlErrorHint("mini_no_transducer",
				fmt.Sprintf("register-compiled: no mini-language %q to compile", name),
				"register-compiled",
				"register its transducer first: MiniLang.register "+name+" <fn>")
		}
		fnDef, ok := args[1].Data.(native.FnDefInfo)
		if !ok || !fnDef.Macro || len(fnDef.Signatures) == 0 {
			return nil, r.AqlErrorHint("mini_bad_compiler",
				"register-compiled: the compiler must be a macro", "register-compiled",
				"MiniLang.register-compiled "+name+" (macro [[src opts] [ quote [ … ] ]])")
		}
		exports.Set("compile_"+name, args[1])
		return nil, nil
	}
}

// miniKindsHandler lists the registered kind atoms (lang_ stripped), in
// registration order.
func miniKindsHandler(exports *native.OrderedMap) native.Handler {
	return func(_ []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
		var kinds []native.Value
		for _, k := range exports.Keys() {
			if strings.HasPrefix(k, "lang_") {
				kinds = append(kinds, native.NewAtom(strings.TrimPrefix(k, "lang_")))
			}
		}
		return []native.Value{native.NewList(kinds)}, nil
	}
}
