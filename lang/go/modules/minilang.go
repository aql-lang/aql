package modules

import (
	"fmt"
	"regexp"
	"strings"
	"sync"

	eng "github.com/aql-lang/aql/eng/go"
	"github.com/aql-lang/aql/lang/go/native"
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
	if miniTypeInitErr != nil {
		return native.ModuleDesc{}, miniTypeInitErr
	}
	subReg, err := native.DefaultRegistry()
	if err != nil {
		return native.ModuleDesc{}, err
	}
	exports := native.NewOrderedMap()

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
		Signatures: []native.NativeSig{{
			Args:       []*native.Type{native.TString, native.TMap, native.TString},
			Returns:    []*native.Type{native.TMap},
			BarrierPos: -1,
			Handler:    miniReHandler,
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
		Signatures: []native.NativeSig{{
			Args:       []*native.Type{TMiniCompiled, native.TMap, native.TString},
			Returns:    []*native.Type{native.TMap},
			BarrierPos: -1,
			Handler:    miniRunReHandler,
		}},
	})
	exports.Set("run-re", wrapMiniFnDef("minilang-run-re", [][]native.FnParam{
		{{Type: TMiniCompiled}, {Type: native.TMap}, {Type: native.TString}},
	}, []*native.Type{native.TMap}, nil, subReg))
	native.RegisterMiniCompileGoHook(parent, "re", miniReCompile)

	// ---- kind: bf — brainfuck ------------------------------------------
	// Filter form  [src opts input:String] → [String]: the stack value is
	// the `,` input stream. Generator form [src opts] → [String]: input
	// comes from opts.in (default ""). opts: {in:S, steps:I} — steps is
	// the execution budget (default 1e6; exceeding it raises rather than
	// hanging).
	subReg.RegisterNativeFunc(native.NativeFunc{
		Name: "minilang-bf",
		Signatures: []native.NativeSig{
			{
				Args:       []*native.Type{native.TString, native.TMap, native.TString},
				Returns:    []*native.Type{native.TString},
				BarrierPos: -1,
				Handler:    miniBfHandler,
			},
			{
				Args:       []*native.Type{native.TString, native.TMap},
				Returns:    []*native.Type{native.TString},
				BarrierPos: -1,
				Handler:    miniBfHandler,
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
		Signatures: []native.NativeSig{{
			Args:       []*native.Type{native.TString, native.TMap, native.TAny},
			Returns:    []*native.Type{native.TAny},
			BarrierPos: -1,
			Handler:    miniGexHandler,
		}},
	})
	exports.Set("lang_gex", wrapMiniFnDef("minilang-gex", [][]native.FnParam{
		append(append([]native.FnParam{}, stdPrefix...), native.FnParam{Type: native.TAny}),
	}, []*native.Type{native.TAny}, nil, subReg))

	// ---- kind: m — traditional maths formula evaluator -----------------
	// [src opts] → [Number]. Evaluate a formula like `x*y-z^2` (operators
	// + - * / % ^, unary +/-, parens) whose variables are bound by the
	// named params (opts). Backed by the tabnas/expr Pratt parser; numeric
	// coercion follows AQL's integer/float domain rules. See minilang_math.go.
	subReg.RegisterNativeFunc(native.NativeFunc{
		Name: "minilang-m",
		Signatures: []native.NativeSig{{
			Args:       []*native.Type{native.TString, native.TMap},
			Returns:    []*native.Type{native.TNumber},
			BarrierPos: -1,
			Handler:    miniMathHandler,
		}},
	})
	exports.Set("lang_m", wrapMiniFnDef("minilang-m", [][]native.FnParam{stdPrefix},
		[]*native.Type{native.TNumber}, nil, subReg))

	// ---- kind: jp — JSONPath query (github.com/ohler55/ojg) -------------
	// [src opts doc:Any] → [List]. Run a JSONPath query over the stack
	// subject — a Node (Map/List), Object, Array, Table or Record — and
	// return the matched nodes as a List. See minilang_query.go.
	subReg.RegisterNativeFunc(native.NativeFunc{
		Name: "minilang-jp",
		Signatures: []native.NativeSig{{
			Args:       []*native.Type{native.TString, native.TMap, native.TAny},
			Returns:    []*native.Type{native.TList},
			BarrierPos: -1,
			Handler:    miniJsonPathHandler,
		}},
	})
	exports.Set("lang_jp", wrapMiniFnDef("minilang-jp", [][]native.FnParam{
		append(append([]native.FnParam{}, stdPrefix...), native.FnParam{Type: native.TAny}),
	}, []*native.Type{native.TList}, nil, subReg))

	// ---- kind: jq — jq filter (github.com/itchyny/gojq) ----------------
	// [src opts doc:Any] → [List]. Run a jq filter over the stack subject
	// (the same data shapes as jp) and return its output stream as a List.
	subReg.RegisterNativeFunc(native.NativeFunc{
		Name: "minilang-jq",
		Signatures: []native.NativeSig{{
			Args:       []*native.Type{native.TString, native.TMap, native.TAny},
			Returns:    []*native.Type{native.TList},
			BarrierPos: -1,
			Handler:    miniJqHandler,
		}},
	})
	exports.Set("lang_jq", wrapMiniFnDef("minilang-jq", [][]native.FnParam{
		append(append([]native.FnParam{}, stdPrefix...), native.FnParam{Type: native.TAny}),
	}, []*native.Type{native.TList}, nil, subReg))

	// ---- kind: xp — XPath query (github.com/antchfx/xpath) -------------
	// [src opts doc:Xml] → [List]. Run an XPath expression over the stack
	// Node/Xml subject and return the result as a List — matched nodes for a
	// node-set (an element as its Node/Xml value, an attribute/text node as a
	// String), or a one-element list for a scalar count/string/boolean result.
	// See minilang_xpath.go.
	subReg.RegisterNativeFunc(native.NativeFunc{
		Name: "minilang-xp",
		Signatures: []native.NativeSig{{
			Args:       []*native.Type{native.TString, native.TMap, native.TXml},
			Returns:    []*native.Type{native.TList},
			BarrierPos: -1,
			Handler:    miniXPathHandler,
		}},
	})
	exports.Set("lang_xp", wrapMiniFnDef("minilang-xp", [][]native.FnParam{
		append(append([]native.FnParam{}, stdPrefix...), native.FnParam{Type: native.TXml}),
	}, []*native.Type{native.TList}, nil, subReg))

	// ---- out-of-band: register -----------------------------------------
	// MiniLang.register <name> <fn> installs an AQL function as the
	// mini-language `lang_<name>`. The fn must carry the standard
	// signature prefix [String, Map, …] on every overload. Raw fn values
	// in export maps dispatch like words (lang/spec/fn-value.tsv), so the
	// value is stored as given; `register` exists to own the CONTRACT —
	// name and signature validation, collision checks.
	subReg.RegisterNativeFunc(native.NativeFunc{
		Name: "minilang-register",
		Signatures: []native.NativeSig{{
			Args:          []*native.Type{native.TAtom, native.TFunction},
			QuoteArgs:     map[int]bool{0: true},
			Returns:       []*native.Type{},
			BarrierPos:    -1,
			CompileEffect: native.CompileStoresFn, // stores the fn for interpreter-side dispatch
			Handler:       miniRegisterHandler(exports),
		}},
	})
	exports.Set("register", wrapMiniFnDef("minilang-register", [][]native.FnParam{
		{{Type: native.TAtom, Quote: true}, {Type: native.TFunction}},
	}, []*native.Type{}, map[int]bool{0: true}, subReg))

	// ---- out-of-band: kinds ----------------------------------------------
	// MiniLang.kinds → List of the registered kind atoms (lang_ stripped).
	subReg.RegisterNativeFunc(native.NativeFunc{
		Name: "minilang-kinds",
		Signatures: []native.NativeSig{{
			Args:       []*native.Type{},
			Returns:    []*native.Type{native.TList},
			BarrierPos: -1,
			Handler:    miniKindsHandler(exports),
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
		Signatures: []native.NativeSig{{
			Args:          []*native.Type{native.TAtom, native.TFunction},
			QuoteArgs:     map[int]bool{0: true},
			Returns:       []*native.Type{},
			BarrierPos:    -1,
			CompileEffect: native.CompileStoresFn, // stores the fn for interpreter-side dispatch
			Handler:       miniRegisterCompiledHandler(exports),
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
		Signatures: []native.NativeSig{{
			Args:       args,
			Returns:    spec.Returns,
			BarrierPos: -1,
			Handler:    spec.Handler,
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
			Body:       []native.Value{native.NewWord(wordName)},
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

// miniTypeInitErr records a type-registration failure (ADR-005: recorded, not
// panicked) for BuildMiniLangModule to surface.
var miniTypeInitErr error

// TMiniCompiled is the inert carrier a compile hook splices in place of the
// DSL source: it wraps the kind's precompiled artifact (for `re`, a
// *regexp.Regexp) in an ExtensionPayload the kernel never inspects.
var TMiniCompiled = registerMiniCompiledType()

func registerMiniCompiledType() *native.Type {
	t, err := eng.Builtin.RegisterExternalBuiltin("Ideal/MiniLangCompiled", 5003, nil)
	if err != nil {
		miniTypeInitErr = fmt.Errorf("minilang: register Ideal/MiniLangCompiled: %w", err)
	}
	return t
}

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
func miniReCompile(src string, opts native.Value, _ *native.Registry) ([]native.Value, error) {
	re, cerr := miniCompiledPattern(src)
	if cerr != nil {
		return []native.Value{
			native.NewWord("MiniLang"), native.NewWord("get"), native.NewWord("lang_re"),
			native.NewString(src), opts, native.NewEnd(),
		}, nil
	}
	carrier := eng.NewExtension(TMiniCompiled, re)
	return []native.Value{
		native.NewWord("MiniLang"), native.NewWord("get"), native.NewWord("run-re"),
		carrier, opts, native.NewEnd(),
	}, nil
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
func miniRegisterHandler(exports *native.OrderedMap) native.Handler {
	return func(args []native.Value, _ map[string]native.Value, _ []native.Value, r *native.Registry) ([]native.Value, error) {
		name, err := args[0].AsConcreteAtom()
		if err != nil {
			return nil, r.AqlError("mini_bad_name", fmt.Sprintf("register: %v", err), "register")
		}
		if why := miniValidKindName(name); why != "" {
			return nil, r.AqlError("mini_bad_name", "register: "+why, "register")
		}
		key := "lang_" + name
		if _, exists := exports.Get(key); exists {
			return nil, r.AqlError("mini_kind_exists",
				fmt.Sprintf("register: minilang %q is already registered", name), "register")
		}
		fnDef, ok := args[1].Data.(native.FnDefInfo)
		if !ok || len(fnDef.Signatures) == 0 {
			return nil, r.AqlError("mini_bad_signature", "register: expected a function value", "register")
		}
		for _, sig := range fnDef.Signatures {
			if len(sig.Params) < 2 ||
				sig.Params[0].Type == nil || !sig.Params[0].Type.ConformsTo(native.TString) ||
				sig.Params[1].Type == nil || !sig.Params[1].Type.ConformsTo(native.TMap) {
				return nil, r.AqlErrorHint("mini_bad_signature",
					"register: every signature must start with the standard prefix [src:String opts:Map …]",
					"register",
					"declare the fn as fn [[src:String opts:Map …inputs] [outputs] [body]]")
			}
		}
		exports.Set(key, args[1])
		return nil, nil
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
