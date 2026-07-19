package modules

import (
	"fmt"
	"strings"
	"sync"

	eng "github.com/aql-lang/aql/eng/go"
	"github.com/aql-lang/aql/lang/go/native"
)

// The aql:parselang module — the ParseLang namespace of named parsers
// behind the core `parse` macro word. It is the sibling of aql:minilang
// (design/MINILANG.5.md): same macro mechanics, same registration story,
// a separate namespace. Where a mini-language PRODUCES A VALUE, a parser
// CONSUMES A SOURCE and yields whatever its language defines — typically an
// AST, but the return type is Any (could be a transduction, etc.).
//
// Every parser <name> is exported under the partitioned key `parse_<name>`
// and carries the STANDARD parser signature:
//
//	ParseLang.parse_<name> : [ source:String opts:Map ] [ Any ]
//
// sig[0] is the (already-resolved) source text, sig[1] the named options
// (`{}` when the caller gave none). The core `parse` word expands
// `parse <kind> <opts?> <source>` to `ParseLang.parse_<kind> <source> <opts>
// end` — note `source` is the required LAST surface argument (a String or a
// `{src:…}` Source map) while `opts` is the optional middle one. The first
// surface argument may instead BE the parser — a fn value (or a word bound
// to one) carrying the standard prefix — which `parse` calls directly with
// no kind lookup (the ParseLang-value form; see native_macro.go
// parseFnExpand).
//
// Out-of-band exports (no `parse_` prefix — never reachable via `parse`):
//
//	ParseLang.register  — install an AQL fn as a new parser
//	ParseLang.kinds     — list the registered parser-kind atoms
//	ParseLang.source    — resolve a source value (String | {src:…}) to a String
//
// Source resolution: a String passes through; a `{src:String}` map yields
// its src; a `{file:…}` map raises `parse_file_unsupported` (deferred in
// v1). Host-registered parsers (RegisterHostParser) get resolution for free
// — the framework resolves the source before the parser body runs. An
// AQL-registered parser receives the source as given and may call
// `ParseLang.source` to normalise it.

// BuildParseLangModule creates the "aql:parselang" native module. It ships
// the framework — register / kinds / source — and folds in any
// host-registered parsers; it carries no built-in parser kinds.
func BuildParseLangModule(parent *native.Registry) (native.ModuleDesc, error) {
	subReg, err := newDefaultRegistry()
	if err != nil {
		return native.ModuleDesc{}, err
	}
	exports := native.NewOrderedMap()
	// registerIdents tracks the source-call identity of each AQL-registered
	// parser so the runtime register handler is idempotent for the compiled
	// path's double execution (check-mode ReturnsFn install + VM re-run) while
	// a genuine user double-register still errors. See parseRegisterInstall.
	registerIdents := map[string]registerIdent{}

	// ---- out-of-band: register ----------------------------------------
	// ParseLang.register <name> <fn> installs an AQL function as the parser
	// `parse_<name>`. Every fn signature must start with the standard
	// prefix [source:(String|Any) opts:Map …]. Mirrors MiniLang.register.
	subReg.RegisterNativeFunc(native.NativeFunc{
		Name: "parselang-register",
		Signatures: []native.Signature{{
			Args:          []*native.Type{native.TAtom, native.TFunction},
			QuoteArgs:     map[int]bool{0: true},
			Returns:       []*native.Type{},
			BarrierPos:    -1,
			CompileEffect: native.CompileStoresFn, // stores the fn for interpreter-side dispatch
			Impl:          native.Go(parseRegisterHandler(exports, registerIdents)),
			// Check-mode install so `ParseLang.parse_<name>` is statically
			// resolvable; idempotent on the source-call identity so the compiled
			// program's runtime re-run does not raise parse_kind_exists.
			ReturnsFn: parseRegisterReturns(exports, registerIdents),
		}},
	})
	exports.Set("register", wrapMiniFnDef("parselang-register", [][]native.FnParam{
		{{Type: native.TAtom, Quote: true}, {Type: native.TFunction}},
	}, []*native.Type{}, map[int]bool{0: true}, subReg))

	// ---- out-of-band: kinds -------------------------------------------
	subReg.RegisterNativeFunc(native.NativeFunc{
		Name: "parselang-kinds",
		Signatures: []native.Signature{{
			Args:       []*native.Type{},
			Returns:    []*native.Type{native.TList},
			BarrierPos: -1,
			Impl:       native.Go(parseKindsHandler(exports)),
		}},
	})
	exports.Set("kinds", wrapMiniFnDef("parselang-kinds", [][]native.FnParam{{}},
		[]*native.Type{native.TList}, nil, subReg))

	// ---- out-of-band: deferred dispatch (compile-pass seam, NOT exported) ----
	// parselang-deferred-dispatch is the RUNTIME twin of the `parse` macro's
	// deferred-kind branch (Phase 6 M3, design/STAGE3-INLINING-DESIGN-ROUND.0.md
	// §6 Stage M3). An aql:parse kind (`Parse.register op g`) is installed only
	// at RUN time — the grammar is built by Parse builder words whose result is
	// not concrete under analysis — so the check pass cannot expand
	// `parse op 'src'` to the `ParseLang.parse_op` wrapper call it expands to
	// at run time. Instead the compile pass records ONE call to this word
	// (kind, source, opts), and at run time it resolves `parse_<kind>` against
	// the LIVE export map (which the compiled Parse.register call has by then
	// populated) and dispatches the registered parser through the same
	// callParseFn invoke the grammar's own callbacks use. A kind that is STILL
	// missing at run time (a runtime-conditional registration that did not
	// execute) raises the byte-identical parse_unknown_lang the `parse` macro
	// raises — the lookup is the dynamic proof, never a baked guess. The word
	// is NOT exported: the surface stays `parse <kind>`, and the check-only
	// deferred-kind table (native.MarkParseKindDeferred) is the only producer
	// of recorded calls. No check-pass state is installed into the export map
	// (the reverted register-ReturnsFn leak, aql-bytecode-stage3-inlining-plan
	// §"register-ReturnsFn attempt", stays closed).
	subReg.RegisterNativeFunc(native.NativeFunc{
		Name: "parselang-deferred-dispatch",
		Signatures: []native.Signature{{
			Args:       []*native.Type{native.TAtom, native.TAny, native.TMap},
			Returns:    []*native.Type{native.TAny},
			BarrierPos: -1,
			Impl:       native.Go(parseDeferredDispatchHandler(exports, subReg)),
		}},
	})
	if fn := subReg.Lookup("parselang-deferred-dispatch"); fn != nil && len(fn.Signatures) == 1 {
		native.InstallParseDeferredDispatch(parent, &fn.Signatures[0])
	}

	// ---- out-of-band: fn dispatch (compile-pass seam, NOT exported) ------
	// parselang-fn-dispatch is the RUNTIME twin of the `parse` macro's
	// ParseLang-VALUE form for a parser operand that is not concrete under
	// analysis (a computed parser, a Function-typed binding). The compile
	// pass records ONE call (fn, source, opts) — the fn operand's provenance
	// is its own producing event — and at run time this word dispatches the
	// LIVE fn through the same `<fn> <source> <opts> end` sequence
	// parseFnExpand splices, enforcing the same ParseLangFnSigWhy contract
	// with byte-identical errors. NOT exported: the surface stays
	// `parse <fn>`.
	subReg.RegisterNativeFunc(native.NativeFunc{
		Name: "parselang-fn-dispatch",
		Signatures: []native.Signature{{
			Args:       []*native.Type{native.TAny, native.TAny, native.TMap},
			Returns:    []*native.Type{native.TAny},
			BarrierPos: -1,
			Impl:       native.Go(parseFnDispatchHandler),
		}},
	})
	if fn := subReg.Lookup("parselang-fn-dispatch"); fn != nil && len(fn.Signatures) == 1 {
		native.InstallParseLangFnDispatch(parent, &fn.Signatures[0])
	}

	// ---- out-of-band: source ------------------------------------------
	// ParseLang.source <source> resolves a String or {src:…} Source map to
	// a String (so AQL parsers can opt into the same normalisation host
	// parsers get automatically).
	subReg.RegisterNativeFunc(native.NativeFunc{
		Name: "parselang-source",
		Signatures: []native.Signature{{
			Args:       []*native.Type{native.TAny},
			Returns:    []*native.Type{native.TString},
			BarrierPos: -1,
			Impl:       native.Go(parseSourceHandler),
		}},
	})
	exports.Set("source", wrapMiniFnDef("parselang-source", [][]native.FnParam{
		{{Type: native.TAny}},
	}, []*native.Type{native.TString}, nil, subReg))

	// ---- built-in parser kinds + host-registered parsers ----------------
	// The tabnas parser family (ini, json, jsonic, json5, jsonc, csv, toml,
	// yaml, xml, zon, markdown, feed) ships in the box, so `import
	// "aql:parselang"` then `parse <kind> '<text>'` works with no host
	// registration. Each gets source resolution (String or {src:…}) for
	// free, like every host parser. aontu (github.com/rjrodger/aontu, a
	// CUE-inspired unification config dialect with no Go port) ships as a
	// hand-written parser in native (see native/aontu.go) and installs the
	// same way. Host-registered parsers follow. One merged loop (built-ins
	// first, exactly the previous install order) keeps a single coverable
	// install-error arm — see design/TEST-SEAMS.10.md.
	//
	// create=true: record the live module even with no host parsers yet,
	// so a post-import RegisterHostParser injects directly (the loaded
	// cache would otherwise never rebuild). Mirrors the MiniLang fold.
	state := parseLangHostStateFor(parent, true)
	state.mu.Lock()
	builtins := append(tabnasParserSpecs(), aontuParserSpec())
	for _, spec := range append(builtins, state.specs...) {
		if err := installHostParser(exports, subReg, spec); err != nil {
			state.mu.Unlock()
			return native.ModuleDesc{}, err
		}
	}
	state.live = &builtParseLang{exports: exports, subReg: subReg}
	state.mu.Unlock()

	return native.ModuleDesc{
		Src:     subReg,
		ID:      parent.Modules.NextID(),
		Exports: map[string]*native.OrderedMap{"ParseLang": exports},
	}, nil
}

// ParseLang is the type of a parse_<lang> parser function — the Go
// implementation behind one registered `parse <kind>` word. It is the
// STANDARD parser signature (see the module comment above) in handler form:
// the framework resolves the source (String or {src:…}) BEFORE the function
// runs, so args[0] is the resolved source String and args[1] the opts Map
// (`{}` when the caller gave none); the ctx/stack slots are unused by the
// parse framing. The function returns the parse result — typically one
// value (an AST, a transduction, …) matching the declared Returns types —
// or a raised parse error.
//
// Every Go-implemented parser carries this type: the built-in kinds
// (tabnasParseHandler, aontuParserSpec), the read-format bridge
// (formatParseHandler), the Parse-builder bridge (parseGrammar.parseHandler),
// and host parsers supplied via ParseLangSpec.Handler.
type ParseLang func(args []native.Value, ctx map[string]native.Value, stack []native.Value, r *native.Registry) ([]native.Value, error)

// ParseLangSpec describes a Go-implemented parser for the host registration
// API (RegisterHostParser). The standard [source:String opts:Map] prefix is
// supplied automatically and the source is RESOLVED to a String before the
// handler runs, so a handler receives args[0]=source:String, args[1]=opts.
type ParseLangSpec struct {
	// Name is the parser-kind atom, unprefixed and lowercase ("calc", not
	// "parse_calc"). It is the token a caller writes after `parse`.
	Name string
	// Returns are the parser's output types (nil → [Any]).
	Returns []*native.Type
	// Handler implements the parser (a ParseLang). Required. Receives the
	// resolved source String and the opts Map.
	Handler ParseLang
	// Pure declares the handler a PURE function of (source, opts): no
	// registry mutation, no I/O, deterministic output. A pure parser's
	// result over a fully-concrete source + opts is folded at CHECK time
	// (pureParseFoldReturns), so a literal-source `parse <kind> '<text>'`
	// types as its actual parse result instead of dynamic(Any). Only the
	// built-in kinds set this — host parsers are unknown code and stay
	// unfolded unless the host opts in.
	Pure bool
}

// capParseLangHost is the registry capability slot holding host-registered
// parsers. Per-registry, like capMiniLangHost — no process-global table.
const capParseLangHost = "engine.parselang.host"

type parseLangHostState struct {
	mu    sync.Mutex
	specs []ParseLangSpec
	live  *builtParseLang
}

type builtParseLang struct {
	exports *native.OrderedMap
	subReg  *native.Registry
}

func parseLangHostStateFor(r *native.Registry, create bool) *parseLangHostState {
	if s, ok, _ := eng.Cap[*parseLangHostState](r, capParseLangHost); ok && s != nil {
		return s
	}
	if !create {
		return nil
	}
	s := &parseLangHostState{}
	_ = r.Capabilities.Set(capParseLangHost, s)
	return s
}

// RegisterHostParser installs a Go-implemented parser on reg — the embedder
// twin of the AQL `ParseLang.register` word. The parser becomes resolvable
// as `parse <Name> …` once the program imports "aql:parselang";
// registration may happen before OR after that import. The contract mirrors
// MiniLang: a lowercase kind name with no `parse_` prefix, a non-nil
// handler, no collision with an existing parser.
func RegisterHostParser(reg *native.Registry, spec ParseLangSpec) error {
	if why := parseValidKindName(spec.Name); why != "" {
		return fmt.Errorf("register parser: %s", why)
	}
	if spec.Handler == nil {
		return fmt.Errorf("register parser %q: handler must not be nil", spec.Name)
	}
	// Reject a collision with a built-in kind here, BEFORE import: state.live
	// is nil until aql:parselang is built, so the duplicate would otherwise
	// only surface as a delayed import failure when the built-ins are installed.
	if builtinParserKind(spec.Name) {
		return fmt.Errorf("register parser %q: already registered (built-in)", spec.Name)
	}
	state := parseLangHostStateFor(reg, true)
	state.mu.Lock()
	defer state.mu.Unlock()
	for _, s := range state.specs {
		if s.Name == spec.Name {
			return fmt.Errorf("register parser %q: already registered", spec.Name)
		}
	}
	if state.live != nil {
		if err := installHostParser(state.live.exports, state.live.subReg, spec); err != nil {
			return err
		}
	}
	state.specs = append(state.specs, spec)
	return nil
}

// RegisterFormatParser registers f as a read format (reachable from `read`
// by name and by each ext) AND as a `parse` kind of the same name (reachable
// from `parse <name>`). It is the single-call bridge for hosts that want a
// format available on BOTH surfaces from one definition. The parse side
// decodes via the format's Decode (a DecodeOpter format additionally honours
// `parse <name> <opts> <src>`).
func RegisterFormatParser(reg *native.Registry, name string, f native.Format, exts ...string) error {
	// Register the parse side FIRST: RegisterHostParser performs all the
	// fallible validation (name shape, built-in/duplicate collision) and the
	// parse-side install, so a rejected parser leaves `read`'s registries
	// untouched — the bridge is atomic. We pre-check the read-side
	// precondition (the formats capability) up front so the subsequent
	// native.RegisterFormat cannot fail after the parser is installed.
	if native.HostFormats(reg) == nil {
		return fmt.Errorf("register format parser %q: formats capability not installed", name)
	}
	if err := RegisterHostParser(reg, ParseLangSpec{
		Name:    name,
		Returns: []*native.Type{native.TAny},
		Handler: formatParseHandler(name, f),
	}); err != nil {
		return err
	}
	return native.RegisterFormat(reg, name, f, exts...)
}

// formatParseHandler adapts a read Format into a ParseLang: it decodes
// the resolved source (forwarding opts when the format is opts-aware) and
// returns the first decoded value.
func formatParseHandler(name string, f native.Format) ParseLang {
	target := "parse_" + name
	return func(args []native.Value, _ map[string]native.Value, _ []native.Value, r *native.Registry) ([]native.Value, error) {
		src, err := args[0].AsConcreteString() // resolved by the framework
		if err != nil {
			return nil, r.AqlError("parse_error", name+": src: "+err.Error(), target)
		}
		var out []native.Value
		var perr error
		if d, ok := f.(native.DecodeOpter); ok {
			out, perr = d.DecodeOpts(src, native.OptsToMap(args[1]))
		} else {
			out, perr = f.Decode(src)
		}
		if perr != nil {
			return nil, r.AqlErrorHint("parse_syntax_error",
				name+": "+native.FirstCleanLine(perr.Error()), target,
				"check that the source is well-formed "+name)
		}
		if len(out) == 0 {
			return []native.Value{native.NewTypeLiteral(native.TNone)}, nil
		}
		return []native.Value{out[0]}, nil
	}
}

// installHostParser registers a shell native that resolves the source value
// to a String and then calls the host handler, and exports the standard
// trivial-delegation wrapper under parse_<name>. The dispatch source slot is
// TAny so a String OR a {src:…}/{file:…} Source map matches the signature.
func installHostParser(exports *native.OrderedMap, subReg *native.Registry, spec ParseLangSpec) error {
	key := "parse_" + spec.Name
	if _, exists := exports.Get(key); exists {
		return fmt.Errorf("register parser %q: already registered", spec.Name)
	}
	handler := spec.Handler
	shell := func(args []native.Value, named map[string]native.Value, stack []native.Value, r *native.Registry) ([]native.Value, error) {
		src, err := resolveParseSource(args[0], r)
		if err != nil {
			return nil, err
		}
		resolved := make([]native.Value, len(args))
		copy(resolved, args)
		resolved[0] = native.NewString(src)
		return handler(resolved, named, stack, r)
	}
	inner := "parselang-host-" + spec.Name
	sig := native.Signature{
		Args:       []*native.Type{native.TAny, native.TMap},
		Returns:    spec.Returns,
		BarrierPos: -1,
		Impl:       native.Go(shell),
	}
	if spec.Pure {
		sig.ReturnsFn = pureParseFoldReturns(spec.Returns, shell)
	}
	subReg.RegisterNativeFunc(native.NativeFunc{
		Name:       inner,
		Signatures: []native.Signature{sig},
	})
	params := []native.FnParam{{Type: native.TAny}, {Type: native.TMap}}
	exports.Set(key, wrapMiniFnDef(inner, [][]native.FnParam{params}, spec.Returns, nil, subReg))
	return nil
}

// pureParseFoldReturns is the check-mode ReturnsFn for a PURE parser kind:
// when the source and opts operands are fully concrete data, it runs the
// real handler (a pure function of its inputs) and surfaces the CONCRETE
// parse result, so a literal-source `parse json '{"a":1}'` types field
// reads precisely instead of ending dynamic(Any). shell is the
// source-RESOLVING wrapper installHostParser builds around the kind's
// ParseLang — NOT the bare ParseLang itself: at fold time args[0] may still
// be a literal {src:…} map (parseFoldableValue admits concrete maps), and
// the shell performs the same resolution the runtime path does. Anything non-concrete —
// a computed source, a carrier-bearing opts map — falls back to the
// declared-Returns dynamic carriers, exactly what declaredReturnCarriers
// would have produced (declared Any rides dynamic). A handler ERROR also
// falls back, silently: the raise is the runtime's job (and may be caught
// by an enclosing `do`), so the check pass must not flag it here.
func pureParseFoldReturns(returns []*native.Type, shell native.Handler) native.ReturnsFunc {
	return func(args []native.Value, r *native.Registry) []native.Value {
		fallback := make([]native.Value, len(returns))
		for i, t := range returns {
			c := native.NewCarrier(t)
			if t.Equal(native.TAny) {
				c.Dynamic = true
			}
			fallback[i] = c
		}
		if len(args) != 2 || !parseFoldableValue(args[0]) || !parseFoldableValue(args[1]) {
			return fallback
		}
		out, err := shell(args, nil, nil, r)
		if err != nil || len(out) != len(returns) {
			return fallback
		}
		return out
	}
}

// parseFoldableValue reports v is fully-concrete INERT DATA — a concrete
// scalar (String/Number/Boolean/Atom family, or none), or a concrete
// list/map whose members are recursively so. Only such a value may be fed
// to a pure parse handler at check time: a carrier or dynamic anywhere
// inside (e.g. a computed opts field that defaults during the fold but is
// real at run time) could make the folded result diverge from the runtime
// call, which would be an unsound commitment. Identity-bearing payloads
// (stores, class instances, fn values, timers) are NOT inert — their
// check-time state is not their runtime state — so they refuse the fold.
func parseFoldableValue(v native.Value) bool {
	if v.Carrier || v.Dynamic || v.Undefined || !native.IsConcrete(v) {
		return false
	}
	switch {
	case v.Parent.ConformsTo(native.TList):
		l, err := native.AsList(v)
		if err != nil || l.IsNil() {
			return false
		}
		for i := 0; i < l.Len(); i++ {
			if !parseFoldableValue(l.Get(i)) {
				return false
			}
		}
		return true
	case v.Parent.ConformsTo(native.TMap):
		m, err := native.AsMap(v)
		if err != nil || m == nil {
			return false
		}
		for _, k := range m.Keys() {
			e, _ := m.Get(k)
			if !parseFoldableValue(e) {
				return false
			}
		}
		return true
	default:
		return v.Parent.ConformsTo(native.TScalar) || v.Parent.Equal(native.TNone)
	}
}

// resolveParseSource normalises a `parse` source argument to its String
// contents: a String passes through; a `{src:String}` map yields its src; a
// `{file:…}` map raises `parse_file_unsupported` (deferred in v1); anything
// else is a clear error.
func resolveParseSource(v native.Value, r *native.Registry) (string, error) {
	if v.Parent.ConformsTo(native.TString) && native.IsConcrete(v) {
		return v.AsConcreteString()
	}
	if v.Parent.Equal(native.TMap) && native.IsConcrete(v) {
		m, _ := native.AsMap(v)
		if m == nil {
			return "", r.AqlError("parse_bad_source", "parse: source map is empty", "parse")
		}
		if _, ok := m.Get("file"); ok {
			return "", r.AqlErrorHint("parse_file_unsupported",
				"parse: {file:…} source is not yet supported", "parse",
				"use an inline string or {src:'…'} for now")
		}
		if s, ok := native.MapFieldString(m, "src"); ok {
			return s, nil
		}
		return "", r.AqlErrorHint("parse_bad_source",
			"parse: source map must have a 'src' String field", "parse",
			"write {src:'…'} or pass the source string directly")
	}
	return "", r.AqlErrorHint("parse_bad_source",
		"parse: source must be a String or a {src:…} map", "parse",
		"e.g. parse calc 'x + y'  or  parse calc {src:'x + y'}")
}

// parseValidKindName reports why a parser-kind name is unusable, or "".
// Mirrors miniValidKindName but rejects the parse_ prefix.
func parseValidKindName(name string) string {
	if name == "" {
		return "parser name must not be empty"
	}
	if strings.HasPrefix(name, "parse_") {
		return "parser name must not carry the parse_ prefix (register adds it)"
	}
	if name[0] >= 'A' && name[0] <= 'Z' {
		return "parser name must be lowercase (capitalised names are types)"
	}
	return ""
}

// parseSourceHandler is the ParseLang.source word: resolve a source value to
// its String.
func parseSourceHandler(args []native.Value, _ map[string]native.Value, _ []native.Value, r *native.Registry) ([]native.Value, error) {
	s, err := resolveParseSource(args[0], r)
	if err != nil {
		return nil, err
	}
	return []native.Value{native.NewString(s)}, nil
}

// parseRegisterValidate runs the name + signature validation `register`
// enforces, returning the validated key ("parse_<name>") or an error. Shared
// by the runtime handler and the check-mode ReturnsFn so both apply the exact
// same contract.
func parseRegisterValidate(args []native.Value, r *native.Registry) (string, error) {
	name, err := args[0].AsConcreteAtom()
	if err != nil {
		return "", r.AqlError("parse_bad_name", fmt.Sprintf("register: %v", err), "register")
	}
	if why := parseValidKindName(name); why != "" {
		return "", r.AqlError("parse_bad_name", "register: "+why, "register")
	}
	fnDef, ok := args[1].Data.(native.FnDefInfo)
	if !ok || len(fnDef.Signatures) == 0 {
		return "", r.AqlError("parse_bad_signature", "register: expected a function value", "register")
	}
	// The standard-prefix contract is shared with the `parse` macro's
	// fn-operand form (native.ParseLangFnSigWhy) so both surfaces enforce
	// byte-identical requirements.
	if why := native.ParseLangFnSigWhy(fnDef); why != "" {
		return "", r.AqlErrorHint("parse_bad_signature", "register: "+why, "register",
			"declare the fn as fn [[source:String opts:Map] [outputs] [body]]")
	}
	return "parse_" + name, nil
}

// parseRegisterInstall validates and installs an AQL fn as parse_<name>,
// tracking the registering source-call identity in idents. It is the single
// install body shared by the runtime handler and the check-mode ReturnsFn; the
// collision / idempotency rule lives in registerCollisionInstall (shared with
// MiniLang / EmitLang).
func parseRegisterInstall(exports *native.OrderedMap, idents map[string]registerIdent, args []native.Value, r *native.Registry) error {
	key, err := parseRegisterValidate(args, r)
	if err != nil {
		return err
	}
	return registerCollisionInstall(exports, idents, key, args[1], r.Check.IsActive(), func() error {
		return r.AqlError("parse_kind_exists",
			fmt.Sprintf("register: parser %q is already registered", strings.TrimPrefix(key, "parse_")), "register")
	})
}

// parseRegisterHandler validates and installs an AQL fn as parse_<name>.
// Mirrors miniRegisterHandler; the source param may be String or Any.
func parseRegisterHandler(exports *native.OrderedMap, idents map[string]registerIdent) native.Handler {
	return func(args []native.Value, _ map[string]native.Value, _ []native.Value, r *native.Registry) ([]native.Value, error) {
		if err := parseRegisterInstall(exports, idents, args, r); err != nil {
			return nil, err
		}
		return nil, nil
	}
}

// parseRegisterReturns is the check-mode counterpart of parseRegisterHandler:
// it performs the SAME install so a dynamically-registered parser
// (`ParseLang.register calc …`) is statically resolvable as
// `ParseLang.parse_calc` during the check pass. The install is idempotent on
// the registering source-call identity (parseRegisterInstall), so the runtime
// handler re-running the identical `register` in the compiled program is a
// no-op rather than a `parse_kind_exists` error. A non-concrete name or fn
// value leaves the export unresolved (the downstream `get` refuses, as before).
func parseRegisterReturns(exports *native.OrderedMap, idents map[string]registerIdent) native.ReturnsFunc {
	return func(args []native.Value, r *native.Registry) []native.Value {
		if len(args) < 2 || !native.IsConcrete(args[0]) {
			return nil
		}
		if _, ok := args[1].Data.(native.FnDefInfo); !ok {
			return nil
		}
		if err := parseRegisterInstall(exports, idents, args, r); err != nil {
			surfaceRegisterCheckError(r, err, args[0])
		}
		return nil
	}
}

// parseDeferredDispatchHandler builds the runtime resolver behind the
// compiled `parse <kind>` deferred dispatch (see the registration comment in
// BuildParseLangModule). args are [kind:Atom source:Any opts:Map]. The
// resolution mirrors the `parse` macro's own runtime behaviour exactly: a
// registered kind REPLAYS the macro's expansion tail — the resolved parser
// value followed by `source opts end`, stepped in a sub-engine over the
// calling registry, which is the token sequence the interpreter itself
// re-steps after `ParseLang dot parse_<kind>` resolves — and a missing kind
// raises the parse macro's byte-identical parse_unknown_lang (code, detail,
// hint). The export-map read at run time IS the dynamic registration proof:
// a runtime-conditional Parse.register that did not execute leaves the kind
// missing here exactly as it does for the interpreter's expansion.
func parseDeferredDispatchHandler(exports *native.OrderedMap, _ *native.Registry) native.Handler {
	return func(args []native.Value, _ map[string]native.Value, _ []native.Value, r *native.Registry) ([]native.Value, error) {
		kind, err := args[0].AsConcreteAtom()
		if err != nil {
			return nil, r.AqlErrorHint("parse_error",
				"parse: the kind must be a literal name", "parse",
				"write the kind as a bare word: parse calc 'x + y'")
		}
		v, ok := exports.Get("parse_" + kind)
		if !ok {
			return nil, r.AqlErrorHint("parse_unknown_lang",
				fmt.Sprintf("parse: no parser %q is registered", kind), "parse",
				`import "aql:parselang" first; register parsers with ParseLang.register; ParseLang.kinds lists what is loaded`)
		}
		sub := native.NewTop(r)
		res, err := sub.Run([]native.Value{v, args[1], args[2], native.NewEnd()})
		if err != nil {
			return nil, err
		}
		if len(res) != 1 {
			// Defensive: every Parse.register kind declares Returns [Any] and
			// its handler yields exactly one value; a drifted registration must
			// fail loudly rather than corrupt the operand stack.
			return nil, r.AqlError("internal_error",
				fmt.Sprintf("parse %s: expected one result, got %d", kind, len(res)), "parse")
		}
		return res, nil
	}
}

// parseFnDispatchHandler is the runtime resolver behind the compiled
// `parse <fn>` value-form dispatch for a non-concrete parser operand (see
// the registration comment in BuildParseLangModule). args are
// [fn source opts]. It enforces the contract parseFnExpand enforces at
// expansion time — the operand must be a Function carrying the standard
// [source opts] prefix (native.ParseLangFnSigWhy) — with byte-identical
// errors, then replays the value-form expansion tail: the fn value followed
// by `source opts end`, stepped in a sub-engine over the calling registry.
func parseFnDispatchHandler(args []native.Value, _ map[string]native.Value, _ []native.Value, r *native.Registry) ([]native.Value, error) {
	fnDef, ok := args[0].Data.(native.FnDefInfo)
	if !ok {
		return nil, r.AqlErrorHint("parse_error",
			"parse: the parser is not a usable function value", "parse",
			"pass a parser fn: parse (fn [[source:String opts:Map] [Any] [...]]) 'x'")
	}
	if why := native.ParseLangFnSigWhy(fnDef); why != "" {
		return nil, r.AqlErrorHint("parse_bad_signature",
			"parse: "+why, "parse",
			"declare the fn as fn [[source:String opts:Map] [outputs] [body]]")
	}
	sub := native.NewTop(r)
	res, err := sub.Run([]native.Value{args[0], args[1], args[2], native.NewEnd()})
	if err != nil {
		return nil, err
	}
	if len(res) != 1 {
		// Defensive: a conforming parser yields exactly one value; a drifted
		// fn must fail loudly rather than corrupt the operand stack.
		return nil, r.AqlError("internal_error",
			fmt.Sprintf("parse fn: expected one result, got %d", len(res)), "parse")
	}
	return res, nil
}

// tabnasParserSpecs returns the parser kinds backed by the tabnas parser
// family, built from the shared native.TabnasKinds() core (which read also
// consumes — see lang/go/native/tabnas.go). Every kind ships built-in —
// importing aql:parselang is enough to use `parse <kind> '<text>'`, no host
// registration needed.
//
// The slice order mirrors native.TabnasKinds() and is pinned by
// ParseLang.kinds (lang/spec/module-parselang.tsv §3).
func tabnasParserSpecs() []ParseLangSpec {
	kinds := native.TabnasKinds()
	specs := make([]ParseLangSpec, len(kinds))
	for i, k := range kinds {
		specs[i] = ParseLangSpec{
			Name:    k.Name,
			Returns: []*native.Type{k.Returns},
			Handler: tabnasParseHandler(k.Name, k.Parse, k.Convert),
			// The tabnas decoders are pure functions of (source, opts) —
			// no registry state, no I/O — so a literal source folds at
			// check time.
			Pure: true,
		}
	}
	return specs
}

// builtinParserKind reports whether name is one of the built-in parser kinds
// (the tabnas family json/csv/ini/… plus the hand-written aontu) — the names
// BuildParseLangModule installs before any host registration.
func builtinParserKind(name string) bool {
	if name == "aontu" {
		return true
	}
	for _, spec := range tabnasParserSpecs() {
		if spec.Name == name {
			return true
		}
	}
	return false
}

// aontuParserSpec is the built-in aontu parse kind. Its handler decodes the
// resolved source via native.AontuParse and converts the generic result
// (map[string]any / []any / scalars) to an AQL Node of Maps and Lists; a
// decode or unification failure raises [aql/parse_syntax_error].
func aontuParserSpec() ParseLangSpec {
	return ParseLangSpec{
		Name:    "aontu",
		Returns: []*native.Type{native.TAny},
		// AontuParse is a pure decode + unification over the source text —
		// same fold contract as the tabnas family.
		Pure: true,
		Handler: func(args []native.Value, _ map[string]native.Value, _ []native.Value, r *native.Registry) ([]native.Value, error) {
			src, err := args[0].AsConcreteString() // resolved by the framework
			if err != nil {
				return nil, r.AqlError("parse_error", "aontu: src: "+err.Error(), "parse_aontu")
			}
			v, perr := native.AontuParse(src, native.OptsToMap(args[1]))
			if perr != nil {
				return nil, r.AqlErrorHint("parse_syntax_error",
					native.FirstCleanLine(perr.Error()), "parse_aontu",
					"check that the source is well-formed aontu")
			}
			return []native.Value{native.AnyToValue(v)}, nil
		},
	}
}

// tabnasParseHandler builds the ParseLang for one tabnas kind: it runs
// the decoder over the (already-resolved) source string and converts the
// result to an AQL Value. A decode failure raises [aql/parse_syntax_error].
func tabnasParseHandler(kind string, parse native.TabnasParser, convert func(any) native.Value) ParseLang {
	target := "parse_" + kind
	return func(args []native.Value, _ map[string]native.Value, _ []native.Value, r *native.Registry) ([]native.Value, error) {
		src, err := args[0].AsConcreteString() // resolved by the framework
		if err != nil {
			return nil, r.AqlError("parse_error", kind+": src: "+err.Error(), target)
		}
		v, perr := parse(src, native.OptsToMap(args[1]))
		if perr != nil {
			return nil, r.AqlErrorHint("parse_syntax_error",
				kind+": "+native.FirstCleanLine(perr.Error()), target,
				"check that the source is well-formed "+kind)
		}
		return []native.Value{convert(v)}, nil
	}
}

// parseKindsHandler lists the registered parser-kind atoms (parse_ stripped),
// in registration order.
func parseKindsHandler(exports *native.OrderedMap) native.Handler {
	return func(_ []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
		var kinds []native.Value
		for _, k := range exports.Keys() {
			if strings.HasPrefix(k, "parse_") {
				kinds = append(kinds, native.NewAtom(strings.TrimPrefix(k, "parse_")))
			}
		}
		return []native.Value{native.NewList(kinds)}, nil
	}
}
