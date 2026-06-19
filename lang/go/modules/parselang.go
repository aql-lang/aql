package modules

import (
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"sort"
	"strings"
	"sync"

	eng "github.com/aql-lang/aql/eng/go"
	"github.com/aql-lang/aql/lang/go/native"
	tabnascsv "github.com/tabnas/csv/go"
	tabnasfeed "github.com/tabnas/feed/go"
	tabnasini "github.com/tabnas/ini/go"
	tabnasjson "github.com/tabnas/json/go"
	tabnasjson5 "github.com/tabnas/json5/go"
	tabnasjsonc "github.com/tabnas/jsonc/go"
	tabnasjsonic "github.com/tabnas/jsonic/go"
	tabnasmarkdown "github.com/tabnas/markdown/go"
	tabnastoml "github.com/tabnas/toml/go"
	tabnasxml "github.com/tabnas/xml/go"
	tabnasyaml "github.com/tabnas/yaml/go"
	tabnaszon "github.com/tabnas/zon/go"
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
			Args:          []*native.Type{native.TAtom, native.TFunction},
			QuoteArgs:     map[int]bool{0: true},
			Returns:       []*native.Type{},
			BarrierPos:    -1,
			CompileEffect: native.CompileStoresFn, // stores the fn for interpreter-side dispatch
			Handler:       parseRegisterHandler(exports),
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

// tabnasParser is the shape of a tabnas decoder: source text + the caller's
// `parse` opts → generic Go data (map[string]any / []any / scalars), or a
// parse error.
type tabnasParser func(src string, opts map[string]any) (any, error)

// tabnasParserSpecs returns the parser kinds backed by the tabnas parser
// family (github.com/tabnas/*). Every kind ships built-in — importing
// aql:parselang is enough to use `parse <kind> '<text>'`, no host
// registration needed. They share one any→Value conversion, so a parsed map
// becomes an AQL Map, a list an AQL List, and scalars their AQL counterparts.
//
// Shapes: the JSON family (json / jsonic / json5 / jsonc) plus yaml / zon
// yield whatever their top level denotes (Map, List or scalar); csv and
// markdown yield a List of rows / blocks; ini / toml / xml / feed yield a Map.
//
// The middle `parse <kind> <opts> <source>` opts map is forwarded to the
// jsonic-plugin kinds (json5 / jsonc / csv / xml / markdown / feed), whose
// option model IS a generic map — e.g. `parse csv {field:{separation:';'}}`.
// The standalone-Parse kinds (json / jsonic / yaml / toml / zon / ini) take
// typed (not map) options upstream, so they ignore opts for now.
func tabnasParserSpecs() []ParseLangSpec {
	// jsonicPlugin adapts a tabnas jsonic plugin (json5, csv, xml, …) — which
	// installs its grammar onto a parser instance from a map of options — into
	// a tabnasParser, merging the caller's opts over the kind's defaults.
	jsonicPlugin := func(defaults map[string]any, install func(*tabnasjsonic.Jsonic, map[string]any) error) tabnasParser {
		return func(src string, opts map[string]any) (any, error) {
			j := tabnasjsonic.Make()
			if err := install(j, mergeOpts(defaults, opts)); err != nil {
				return nil, err
			}
			return j.Parse(src)
		}
	}
	ignoreOpts := func(parse func(string) (any, error)) tabnasParser {
		return func(src string, _ map[string]any) (any, error) { return parse(src) }
	}
	defs := []struct {
		name    string
		returns *native.Type
		parse   tabnasParser
	}{
		// INI: [section] headers nest a Map; booleans decode to Booleans.
		{"ini", native.TMap, ignoreOpts(func(s string) (any, error) { return tabnasini.Parse(s) })},
		// JSON family — top level may be an object, array or scalar.
		{"json", native.TAny, ignoreOpts(func(s string) (any, error) { return tabnasjson.Parse(s) })},
		{"jsonic", native.TAny, ignoreOpts(func(s string) (any, error) { return tabnasjsonic.Parse(s) })},
		{"json5", native.TAny, jsonicPlugin(tabnasjson5.Defaults(), tabnasjson5.Json5)},
		{"jsonc", native.TAny, jsonicPlugin(nil, tabnasjsonc.Jsonc)},
		// CSV: a List of rows, each row a List of field strings.
		{"csv", native.TList, jsonicPlugin(nil, tabnascsv.Csv)},
		{"toml", native.TMap, ignoreOpts(func(s string) (any, error) { return tabnastoml.Parse(s) })},
		{"yaml", native.TAny, ignoreOpts(func(s string) (any, error) { return tabnasyaml.Parse(s) })},
		// XML: an element tree Map ({name, attributes, children, …}).
		{"xml", native.TMap, jsonicPlugin(nil, tabnasxml.Xml)},
		{"zon", native.TAny, ignoreOpts(func(s string) (any, error) { return tabnaszon.Parse(s) })},
		// Markdown: a List of blocks. object:false keeps rows as plain Lists
		// (the default ordered-map record shape has no exported accessors).
		{"markdown", native.TList, func(s string, opts map[string]any) (any, error) {
			j := tabnasjsonic.Make()
			if err := j.UseDefaults(tabnasmarkdown.Markdown, tabnasmarkdown.Defaults,
				mergeOpts(map[string]any{"object": false}, opts)); err != nil {
				return nil, err
			}
			return j.Parse(s)
		}},
		// Feed (RSS/Atom): normalised to a Map. The plugin returns a typed
		// AtomFeed struct, so a JSON round-trip flattens it to generic data.
		{"feed", native.TMap, func(s string, opts map[string]any) (any, error) {
			j := tabnasjsonic.Make()
			if err := tabnasfeed.Feed(j, mergeOpts(tabnasfeed.Defaults, opts)); err != nil {
				return nil, err
			}
			v, err := j.Parse(s)
			if err != nil {
				return nil, err
			}
			return jsonRoundTrip(v)
		}},
	}
	specs := make([]ParseLangSpec, len(defs))
	for i, d := range defs {
		specs[i] = ParseLangSpec{
			Name:    d.name,
			Returns: []*native.Type{d.returns},
			Handler: tabnasParseHandler(d.name, d.parse),
		}
	}
	return specs
}

// builtinParserKind reports whether name is one of the built-in tabnas parser
// kinds (json, csv, ini, …) — the names tabnasParserSpecs installs.
func builtinParserKind(name string) bool {
	for _, spec := range tabnasParserSpecs() {
		if spec.Name == name {
			return true
		}
	}
	return false
}

// tabnasParseHandler builds the parser native for one tabnas kind: it runs
// the decoder over the (already-resolved) source string and converts the
// result to an AQL Value. A decode failure raises [aql/parse_syntax_error].
func tabnasParseHandler(kind string, parse tabnasParser) native.Handler {
	target := "parse_" + kind
	return func(args []native.Value, _ map[string]native.Value, _ []native.Value, r *native.Registry) ([]native.Value, error) {
		src, err := args[0].AsConcreteString() // resolved by the framework
		if err != nil {
			return nil, r.AqlError("parse_error", kind+": src: "+err.Error(), target)
		}
		v, perr := parse(src, optsToMap(args[1]))
		if perr != nil {
			return nil, r.AqlErrorHint("parse_syntax_error",
				kind+": "+firstCleanLine(perr.Error()), target,
				"check that the source is well-formed "+kind)
		}
		return []native.Value{tabnasAnyToValue(v)}, nil
	}
}

// optsToMap converts the parser's opts argument (the middle `parse <kind>
// <opts> <source>` map, `{}` when the caller gave none) to a generic
// map[string]any the tabnas plugins understand. A non-Map / empty value
// yields nil, so a kind's defaults pass through mergeOpts untouched.
func optsToMap(v native.Value) map[string]any {
	if !v.Parent.ConformsTo(native.TMap) || !native.IsConcrete(v) {
		return nil
	}
	m, ok := native.ValueToAny(v).(map[string]any)
	if !ok {
		return nil
	}
	return m
}

// mergeOpts returns a fresh map of base overlaid with over — the caller's
// `parse` opts win over a kind's defaults, leaving both inputs untouched.
// Either may be nil.
func mergeOpts(base, over map[string]any) map[string]any {
	if len(base) == 0 && len(over) == 0 {
		return nil
	}
	out := make(map[string]any, len(base)+len(over))
	for k, v := range base {
		out[k] = v
	}
	for k, v := range over {
		out[k] = v
	}
	return out
}

// jsonRoundTrip flattens an arbitrary Go value (e.g. a typed feed struct) to
// generic data — map[string]any / []any / scalars — via encoding/json, so the
// shared converter can handle it uniformly.
func jsonRoundTrip(v any) (any, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	var out any
	if err := json.Unmarshal(b, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// ansiEscape matches the SGR colour escapes the tabnas parsers embed in their
// diagnostics. The pattern is a compile-time constant, so MustCompile is safe.
var ansiEscape = regexp.MustCompile("\x1b\\[[0-9;]*m")

// firstCleanLine returns the first line of a parser diagnostic with colour
// escapes stripped — the tabnas/jsonic parsers emit richly-formatted
// multi-line errors; the AQL error keeps just the headline.
func firstCleanLine(msg string) string {
	msg = ansiEscape.ReplaceAllString(msg, "")
	if i := strings.IndexByte(msg, '\n'); i >= 0 {
		msg = msg[:i]
	}
	return strings.TrimSpace(msg)
}

// tabnasMapToValue converts a decoded map to an AQL Map, ordering keys
// deterministically (the parsers hand back unordered Go maps).
func tabnasMapToValue(m map[string]any) native.Value {
	om := native.NewOrderedMap()
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		om.Set(k, tabnasAnyToValue(m[k]))
	}
	return native.NewMap(om)
}

// tabnasAnyToValue maps a single decoded value to an AQL Value. The parsers
// emit strings, booleans, numbers, nested maps and lists; an unrecognised
// shape degrades to its string form rather than erroring.
func tabnasAnyToValue(v any) native.Value {
	switch val := v.(type) {
	case nil:
		return native.NewTypeLiteral(native.TNone)
	case bool:
		return native.NewBoolean(val)
	case string:
		return native.NewString(val)
	case float64:
		// A whole-valued number stays an Integer, but ONLY when it fits in
		// int64 — an out-of-range float (e.g. 1e20 from a JSON/YAML source)
		// would convert implementation-dependently, so keep it a Float.
		if val == math.Trunc(val) && val >= math.MinInt64 && val < math.MaxInt64 {
			return native.NewInteger(int64(val))
		}
		return native.NewFloat(val)
	case int:
		return native.NewInteger(int64(val))
	case int64:
		return native.NewInteger(val)
	case []any:
		elems := make([]native.Value, len(val))
		for i, item := range val {
			elems[i] = tabnasAnyToValue(item)
		}
		return native.NewList(elems)
	case map[string]any:
		return tabnasMapToValue(val)
	default:
		return native.NewString(fmt.Sprintf("%v", val))
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
