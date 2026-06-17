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

	// ---- out-of-band: register ----------------------------------------
	// ParseLang.register <name> <fn> installs an AQL function as the parser
	// `parse_<name>`. Every fn signature must start with the standard
	// prefix [source:(String|Any) opts:Map …]. Mirrors MiniLang.register.
	subReg.RegisterNativeFunc(native.NativeFunc{
		Name: "parselang-register",
		Signatures: []native.NativeSig{{
			Args:       []*native.Type{native.TAtom, native.TFunction},
			QuoteArgs:  map[int]bool{0: true},
			Returns:    []*native.Type{},
			BarrierPos: -1,
			Handler:    parseRegisterHandler(exports),
		}},
	})
	exports.Set("register", wrapMiniFnDef("parselang-register", [][]native.FnParam{
		{{Type: native.TAtom, Quote: true}, {Type: native.TFunction}},
	}, []*native.Type{}, map[int]bool{0: true}, subReg))

	// ---- out-of-band: kinds -------------------------------------------
	subReg.RegisterNativeFunc(native.NativeFunc{
		Name: "parselang-kinds",
		Signatures: []native.NativeSig{{
			Args:       []*native.Type{},
			Returns:    []*native.Type{native.TList},
			BarrierPos: -1,
			Handler:    parseKindsHandler(exports),
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
		Signatures: []native.NativeSig{{
			Args:       []*native.Type{native.TAny},
			Returns:    []*native.Type{native.TString},
			BarrierPos: -1,
			Handler:    parseSourceHandler,
		}},
	})
	exports.Set("source", wrapMiniFnDef("parselang-source", [][]native.FnParam{
		{{Type: native.TAny}},
	}, []*native.Type{native.TString}, nil, subReg))

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
		Signatures: []native.NativeSig{{
			Args:       []*native.Type{native.TAny, native.TMap},
			Returns:    spec.Returns,
			BarrierPos: -1,
			Handler:    shell,
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

// parseRegisterHandler validates and installs an AQL fn as parse_<name>.
// Mirrors miniRegisterHandler; the source param may be String or Any.
func parseRegisterHandler(exports *native.OrderedMap) native.Handler {
	return func(args []native.Value, _ map[string]native.Value, _ []native.Value, r *native.Registry) ([]native.Value, error) {
		name, err := args[0].AsConcreteAtom()
		if err != nil {
			return nil, r.AqlError("parse_bad_name", fmt.Sprintf("register: %v", err), "register")
		}
		if why := parseValidKindName(name); why != "" {
			return nil, r.AqlError("parse_bad_name", "register: "+why, "register")
		}
		key := "parse_" + name
		if _, exists := exports.Get(key); exists {
			return nil, r.AqlError("parse_kind_exists",
				fmt.Sprintf("register: parser %q is already registered", name), "register")
		}
		fnDef, ok := args[1].Data.(native.FnDefInfo)
		if !ok || len(fnDef.Signatures) == 0 {
			return nil, r.AqlError("parse_bad_signature", "register: expected a function value", "register")
		}
		for _, sig := range fnDef.Signatures {
			if len(sig.Params) < 2 {
				return nil, r.AqlErrorHint("parse_bad_signature",
					"register: every signature must start [source:String opts:Map …]",
					"register",
					"declare the fn as fn [[source:String opts:Map] [outputs] [body]]")
			}
			p0 := sig.Params[0].Type
			if p0 == nil || !(p0.ConformsTo(native.TString) || p0.Equal(native.TAny)) ||
				sig.Params[1].Type == nil || !sig.Params[1].Type.ConformsTo(native.TMap) {
				return nil, r.AqlErrorHint("parse_bad_signature",
					"register: every signature must start [source:String opts:Map …]",
					"register",
					"declare the fn as fn [[source:String opts:Map] [outputs] [body]]")
			}
		}
		exports.Set(key, args[1])
		return nil, nil
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
