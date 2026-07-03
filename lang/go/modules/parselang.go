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
// `{src:…}` Source map) while `opts` is the optional middle one.
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
	subReg, err := native.DefaultRegistry()
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

	// ---- built-in parser kinds ----------------------------------------
	// The tabnas parser family (ini, json, jsonic, json5, jsonc, csv, toml,
	// yaml, xml, zon, markdown, feed) ships in the box, so `import
	// "aql:parselang"` then `parse <kind> '<text>'` works with no host
	// registration. Each gets source resolution (String or {src:…}) for
	// free, like every host parser.
	for _, spec := range tabnasParserSpecs() {
		if err := installHostParser(exports, subReg, spec); err != nil {
			return native.ModuleDesc{}, err
		}
	}

	// aontu (github.com/rjrodger/aontu): a CUE-inspired unification config
	// dialect with no Go port, so it ships as a hand-written parser in
	// native (see native/aontu.go) rather than via the tabnas family. It is
	// built in the same way — importing aql:parselang is enough.
	if err := installHostParser(exports, subReg, aontuParserSpec()); err != nil {
		return native.ModuleDesc{}, err
	}

	// ---- host-registered parsers --------------------------------------
	// create=true: record the live module even with no host parsers yet,
	// so a post-import RegisterHostParser injects directly (the loaded
	// cache would otherwise never rebuild). Mirrors the MiniLang fold.
	state := parseLangHostStateFor(parent, true)
	state.mu.Lock()
	for _, spec := range state.specs {
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
	// Handler implements the parser. Required. Receives the resolved source
	// String and the opts Map.
	Handler native.Handler
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

// formatParseHandler adapts a read Format into a parse handler: it decodes
// the resolved source (forwarding opts when the format is opts-aware) and
// returns the first decoded value.
func formatParseHandler(name string, f native.Format) native.Handler {
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
	subReg.RegisterNativeFunc(native.NativeFunc{
		Name: inner,
		Signatures: []native.Signature{{
			Args:       []*native.Type{native.TAny, native.TMap},
			Returns:    spec.Returns,
			BarrierPos: -1,
			Impl:       native.Go(shell),
		}},
	})
	params := []native.FnParam{{Type: native.TAny}, {Type: native.TMap}}
	exports.Set(key, wrapMiniFnDef(inner, [][]native.FnParam{params}, spec.Returns, nil, subReg))
	return nil
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
	for _, sig := range fnDef.Signatures {
		if len(sig.Params) < 2 {
			return "", r.AqlErrorHint("parse_bad_signature",
				"register: every signature must start [source:String opts:Map …]",
				"register",
				"declare the fn as fn [[source:String opts:Map] [outputs] [body]]")
		}
		p0 := sig.Params[0].Type
		if p0 == nil || !(p0.ConformsTo(native.TString) || p0.Equal(native.TAny)) ||
			sig.Params[1].Type == nil || !sig.Params[1].Type.ConformsTo(native.TMap) {
			return "", r.AqlErrorHint("parse_bad_signature",
				"register: every signature must start [source:String opts:Map …]",
				"register",
				"declare the fn as fn [[source:String opts:Map] [outputs] [body]]")
		}
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

// tabnasParseHandler builds the parser native for one tabnas kind: it runs
// the decoder over the (already-resolved) source string and converts the
// result to an AQL Value. A decode failure raises [aql/parse_syntax_error].
func tabnasParseHandler(kind string, parse native.TabnasParser, convert func(any) native.Value) native.Handler {
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
