package modules

import (
	"fmt"
	"strings"
	"sync"

	eng "github.com/aql-lang/aql/eng/go"
	"github.com/aql-lang/aql/lang/go/native"
)

// The aql:emitlang module — the EmitLang namespace of named emitters behind
// the core `emit` macro word. It is the symmetric inverse of aql:parselang:
// where a parser CONSUMES A SOURCE and yields a value, an emitter CONSUMES A
// VALUE and yields a string.
//
// Every emitter <name> is exported under the partitioned key `emit_<name>`
// and carries the STANDARD emitter signature:
//
//	EmitLang.emit_<name> : [ value:Any opts:Map ] [ String ]
//
// sig[0] is the value to render, sig[1] the named options (`{}` when the
// caller gave none). The core `emit` word expands `emit <kind> <opts?>
// <data>` to `EmitLang get emit_<kind> <data> <opts> end` — `data` is the
// required LAST surface argument while `opts` is the optional middle one.
//
// Out-of-band exports (no `emit_` prefix — never reachable via `emit`):
//
//	EmitLang.register  — install an AQL fn as a new emitter
//	EmitLang.kinds     — list the registered emitter-kind atoms
//
// `emit_auto` is the natural-format dispatcher: it resolves a value's natural
// emit kind (Map/List→json, Table→csv, Xml→xml) and delegates to it.

// BuildEmitLangModule creates the "aql:emitlang" native module. It ships the
// framework — register / kinds — folds in the built-in emit kinds from
// native.EmitKinds(), and folds in any host-registered emitters.
func BuildEmitLangModule(parent *native.Registry) (native.ModuleDesc, error) {
	subReg, err := newDefaultRegistry()
	if err != nil {
		return native.ModuleDesc{}, err
	}
	exports := native.NewOrderedMap()
	// registerIdents tracks the source-call identity of each AQL-registered
	// emitter so the runtime register handler is idempotent for the compiled
	// path's double execution (check-mode ReturnsFn install + VM re-run) while
	// a genuine user double-register still errors. Mirrors parselang-register.
	registerIdents := map[string]registerIdent{}

	// ---- out-of-band: register ----------------------------------------
	// EmitLang.register <name> <fn> installs an AQL function as the emitter
	// `emit_<name>`. Every fn signature must start with the standard prefix
	// [value:Any opts:Map …] and return a value.
	subReg.RegisterNativeFunc(native.NativeFunc{
		Name: "emitlang-register",
		Signatures: []native.Signature{{
			Args:          []*native.Type{native.TAtom, native.TFunction},
			QuoteArgs:     map[int]bool{0: true},
			Returns:       []*native.Type{},
			BarrierPos:    -1,
			CompileEffect: native.CompileStoresFn, // stores the fn for interpreter-side dispatch
			Impl:          native.Go(emitRegisterHandler(exports, registerIdents)),
			// Check-mode install so `emit <name>` is statically resolvable;
			// idempotent on the source-call identity so the compiled program's
			// runtime re-run does not raise emit_kind_exists. Mirrors
			// parseRegisterReturns.
			ReturnsFn: emitRegisterReturns(exports, registerIdents),
		}},
	})
	exports.Set("register", wrapMiniFnDef("emitlang-register", [][]native.FnParam{
		{{Type: native.TAtom, Quote: true}, {Type: native.TFunction}},
	}, []*native.Type{}, map[int]bool{0: true}, subReg))

	// ---- out-of-band: kinds -------------------------------------------
	subReg.RegisterNativeFunc(native.NativeFunc{
		Name: "emitlang-kinds",
		Signatures: []native.Signature{{
			Args:       []*native.Type{},
			Returns:    []*native.Type{native.TList},
			BarrierPos: -1,
			Impl:       native.Go(emitKindsHandler(exports)),
		}},
	})
	exports.Set("kinds", wrapMiniFnDef("emitlang-kinds", [][]native.FnParam{{}},
		[]*native.Type{native.TList}, nil, subReg))

	// ---- emit_auto: the natural-format dispatcher ---------------------
	// The opts slot carries the UNION Options schema of auto's natural-target
	// kinds (json / csv / xml), so an unknown opts key is a hard dispatch
	// rejection rather than a silently ignored option (G6) — see
	// installHostEmitter for the per-kind rule.
	subReg.RegisterNativeFunc(native.NativeFunc{
		Name: "emitlang-auto",
		Signatures: []native.Signature{{
			Args:       []*native.Type{native.TAny, native.TMap},
			Patterns:   map[int]native.Value{1: native.EmitAutoOptsSchema()},
			Returns:    []*native.Type{native.TString},
			BarrierPos: -1,
			Impl:       native.Go(emitAutoHandler),
		}},
	})
	exports.Set("emit_auto", wrapMiniFnDef("emitlang-auto", [][]native.FnParam{
		{{Type: native.TAny}, {Type: native.TMap}},
	}, []*native.Type{native.TString}, nil, subReg))

	// ---- built-in emit kinds + host-registered emitters -----------------
	// The canonical walk-based emitter family (json, jsonic, yaml, csv, tsv,
	// toml, ini, xml) ships in the box, so `import "aql:emitlang"` then
	// `emit <kind> <data>` works with no host registration; any
	// host-registered emitters follow. One merged loop (built-ins first,
	// exactly the previous install order) keeps a single coverable
	// install-error arm — see design/TEST-SEAMS.10.md.
	state := emitLangHostStateFor(parent, true)
	state.mu.Lock()
	for _, spec := range append(builtinEmitSpecs(), state.specs...) {
		if err := installHostEmitter(exports, subReg, spec); err != nil {
			state.mu.Unlock()
			return native.ModuleDesc{}, err
		}
	}
	state.live = &builtEmitLang{exports: exports, subReg: subReg}
	state.mu.Unlock()

	return native.ModuleDesc{
		Src:     subReg,
		ID:      parent.Modules.NextID(),
		Exports: map[string]*native.OrderedMap{"EmitLang": exports},
	}, nil
}

// EmitLangSpec describes a Go-implemented emitter for the host registration
// API (RegisterHostEmitter). The standard [value:Any opts:Map] prefix is
// supplied automatically; the handler receives args[0]=value, args[1]=opts.
type EmitLangSpec struct {
	// Name is the emitter-kind atom, unprefixed and lowercase ("json", not
	// "emit_json"). It is the token a caller writes after `emit`.
	Name string
	// Returns are the emitter's output types (nil → [String]).
	Returns []*native.Type
	// Handler implements the emitter. Required. Receives the value and opts.
	Handler native.Handler

	// optsSchema, when set (an Options type literal), rides the inner
	// native's opts slot as a dispatch Pattern: a CONCRETE opts map with a
	// key outside the schema is a hard signature rejection at check and run
	// time — the G6 options-typo fix (`emit json {prety:true}` must not
	// silently emit compact JSON). Unexported: only the BUILT-IN kinds
	// declare it (builtinEmitSpecs, from native.EmitOptsSchema — the exact
	// key set each Encode reads). Host- and AQL-registered emitters own
	// their key sets, so their opts stay an unchecked plain Map.
	optsSchema native.Value
}

// capEmitLangHost is the registry capability slot holding host-registered
// emitters. Per-registry, like capParseLangHost — no process-global table.
const capEmitLangHost = "engine.emitlang.host"

type emitLangHostState struct {
	mu    sync.Mutex
	specs []EmitLangSpec
	live  *builtEmitLang
}

type builtEmitLang struct {
	exports *native.OrderedMap
	subReg  *native.Registry
}

func emitLangHostStateFor(r *native.Registry, create bool) *emitLangHostState {
	if s, ok, _ := eng.Cap[*emitLangHostState](r, capEmitLangHost); ok && s != nil {
		return s
	}
	if !create {
		return nil
	}
	s := &emitLangHostState{}
	_ = r.Capabilities.Set(capEmitLangHost, s)
	return s
}

// RegisterHostEmitter installs a Go-implemented emitter on reg — the embedder
// twin of the AQL `EmitLang.register` word. The emitter becomes resolvable as
// `emit <Name> …` once the program imports "aql:emitlang"; registration may
// happen before OR after that import.
func RegisterHostEmitter(reg *native.Registry, spec EmitLangSpec) error {
	if why := emitValidKindName(spec.Name); why != "" {
		return fmt.Errorf("register emitter: %s", why)
	}
	if spec.Handler == nil {
		return fmt.Errorf("register emitter %q: handler must not be nil", spec.Name)
	}
	if builtinEmitKind(spec.Name) {
		return fmt.Errorf("register emitter %q: already registered (built-in)", spec.Name)
	}
	state := emitLangHostStateFor(reg, true)
	state.mu.Lock()
	defer state.mu.Unlock()
	for _, s := range state.specs {
		if s.Name == spec.Name {
			return fmt.Errorf("register emitter %q: already registered", spec.Name)
		}
	}
	if state.live != nil {
		if err := installHostEmitter(state.live.exports, state.live.subReg, spec); err != nil {
			return err
		}
	}
	state.specs = append(state.specs, spec)
	return nil
}

// installHostEmitter registers a shell native that calls the host handler and
// exports the standard trivial-delegation wrapper under emit_<name>.
func installHostEmitter(exports *native.OrderedMap, subReg *native.Registry, spec EmitLangSpec) error {
	key := "emit_" + spec.Name
	if _, exists := exports.Get(key); exists {
		return fmt.Errorf("register emitter %q: already registered", spec.Name)
	}
	returns := spec.Returns
	if returns == nil {
		returns = []*native.Type{native.TString}
	}
	inner := "emitlang-host-" + spec.Name
	sig := native.Signature{
		Args:       []*native.Type{native.TAny, native.TMap},
		Returns:    returns,
		BarrierPos: -1,
		Impl:       native.Go(spec.Handler),
	}
	// A built-in kind's known opts keys ride as an Options-schema Pattern so
	// dispatch rejects a typo'd key outright (see EmitLangSpec.optsSchema).
	if native.IsOptionsType(spec.optsSchema) {
		sig.Patterns = map[int]native.Value{1: spec.optsSchema}
	}
	subReg.RegisterNativeFunc(native.NativeFunc{
		Name:       inner,
		Signatures: []native.Signature{sig},
	})
	params := []native.FnParam{{Type: native.TAny}, {Type: native.TMap}}
	exports.Set(key, wrapMiniFnDef(inner, [][]native.FnParam{params}, returns, nil, subReg))
	return nil
}

// builtinEmitSpecs returns the emitter kinds backed by the canonical
// walk-based emitter core (native.EmitKinds()). The slice order mirrors
// EmitKinds() and is pinned by EmitLang.kinds (lang/spec/module-emitlang.tsv).
func builtinEmitSpecs() []EmitLangSpec {
	kinds := native.EmitKinds()
	specs := make([]EmitLangSpec, len(kinds))
	for i, k := range kinds {
		schema, _ := native.EmitOptsSchema(k.Name)
		specs[i] = EmitLangSpec{
			Name:       k.Name,
			Returns:    []*native.Type{native.TString},
			Handler:    emitKindHandler(k.Name, k.Encode),
			optsSchema: schema,
		}
	}
	return specs
}

// builtinEmitKind reports whether name is one of the built-in emit kinds.
func builtinEmitKind(name string) bool {
	for _, spec := range builtinEmitSpecs() {
		if spec.Name == name {
			return true
		}
	}
	return false
}

// emitKindHandler builds the emitter native for one kind: it runs the kind's
// Encode over the value and opts, mapping an emit error to its AQL code.
func emitKindHandler(kind string, encode func(native.Value, map[string]any) (string, error)) native.Handler {
	target := "emit_" + kind
	return func(args []native.Value, _ map[string]native.Value, _ []native.Value, r *native.Registry) ([]native.Value, error) {
		s, err := encode(args[0], native.OptsToMap(args[1]))
		if err != nil {
			if code := native.EmitErrorCode(err); code != "" {
				return nil, r.AqlError(code, err.Error(), target)
			}
			return nil, r.AqlError("emit_error", kind+": "+err.Error(), target)
		}
		return []native.Value{native.NewString(s)}, nil
	}
}

// emitAutoHandler resolves a value's natural emit kind and delegates to that
// kind's encoder. No natural target → emit_no_natural (with a hint listing
// the built-in kinds).
func emitAutoHandler(args []native.Value, _ map[string]native.Value, _ []native.Value, r *native.Registry) ([]native.Value, error) {
	name, ok := native.NaturalEmitKind(args[0])
	if !ok {
		return nil, r.AqlErrorHint("emit_no_natural",
			"emit: this value has no natural emit format", "emit_auto",
			"name a kind explicitly: emit <kind> <data>. Kinds: "+native.EmitKindNames())
	}
	for _, k := range native.EmitKinds() {
		if k.Name == name {
			s, err := k.Encode(args[0], native.OptsToMap(args[1]))
			if err != nil {
				if code := native.EmitErrorCode(err); code != "" {
					return nil, r.AqlError(code, err.Error(), "emit_auto")
				}
				return nil, r.AqlError("emit_error", name+": "+err.Error(), "emit_auto")
			}
			return []native.Value{native.NewString(s)}, nil
		}
	}
	return nil, r.AqlError("emit_no_natural",
		"emit: no encoder for natural kind "+name, "emit_auto")
}

// emitValidKindName reports why an emitter-kind name is unusable, or "".
func emitValidKindName(name string) string {
	if name == "" {
		return "emitter name must not be empty"
	}
	if strings.HasPrefix(name, "emit_") {
		return "emitter name must not carry the emit_ prefix (register adds it)"
	}
	if name[0] >= 'A' && name[0] <= 'Z' {
		return "emitter name must be lowercase (capitalised names are types)"
	}
	return ""
}

// emitRegisterValidate runs the name + signature validation `register`
// enforces, returning the validated key ("emit_<name>") or an error. Shared by
// the runtime handler and the check-mode ReturnsFn so both apply the exact
// same contract.
func emitRegisterValidate(args []native.Value, r *native.Registry) (string, error) {
	name, err := args[0].AsConcreteAtom()
	if err != nil {
		return "", r.AqlError("emit_bad_name", fmt.Sprintf("register: %v", err), "register")
	}
	if why := emitValidKindName(name); why != "" {
		return "", r.AqlError("emit_bad_name", "register: "+why, "register")
	}
	fnDef, ok := args[1].Data.(native.FnDefInfo)
	if !ok || len(fnDef.Signatures) == 0 {
		return "", r.AqlError("emit_bad_signature", "register: expected a function value", "register")
	}
	for _, sig := range fnDef.Signatures {
		if len(sig.Params) < 2 {
			return "", r.AqlErrorHint("emit_bad_signature",
				"register: every signature must start [value:Any opts:Map …] and return a value",
				"register",
				"declare the fn as fn [[value:Any opts:Map] [String] [body]]")
		}
		p0 := sig.Params[0].Type
		if p0 == nil || !p0.Equal(native.TAny) {
			return "", r.AqlErrorHint("emit_bad_signature",
				"register: the first parameter must be value:Any so the emitter accepts every value",
				"register",
				"declare the fn as fn [[value:Any opts:Map] [String] [body]]")
		}
		p1 := sig.Params[1].Type
		if p1 == nil || !p1.ConformsTo(native.TMap) {
			return "", r.AqlErrorHint("emit_bad_signature",
				"register: every signature must start [value:Any opts:Map …] and return a String",
				"register",
				"declare the fn as fn [[value:Any opts:Map] [String] [body]]")
		}
		if len(sig.Returns) == 0 || sig.Returns[0] == nil || !sig.Returns[0].ConformsTo(native.TString) {
			return "", r.AqlErrorHint("emit_bad_signature",
				"register: every signature must return a String (emit is value→string)",
				"register",
				"declare the fn as fn [[value:Any opts:Map] [String] [body]]")
		}
	}
	return "emit_" + name, nil
}

// emitRegisterInstall validates and installs an AQL fn as emit_<name>. It is
// the single install body shared by the runtime handler and the check-mode
// ReturnsFn; the collision / idempotency rule lives in registerCollisionInstall
// (shared with ParseLang / MiniLang).
func emitRegisterInstall(exports *native.OrderedMap, idents map[string]registerIdent, args []native.Value, r *native.Registry) error {
	key, err := emitRegisterValidate(args, r)
	if err != nil {
		return err
	}
	return registerCollisionInstall(exports, idents, key, args[1], r.Check.IsActive(), func() error {
		return r.AqlError("emit_kind_exists",
			fmt.Sprintf("register: emitter %q is already registered", strings.TrimPrefix(key, "emit_")), "register")
	})
}

// emitRegisterHandler validates and installs an AQL fn as emit_<name>.
// Mirrors parseRegisterHandler inverted: the standard prefix is
// [value:Any opts:Map …] and every signature must return a value.
func emitRegisterHandler(exports *native.OrderedMap, idents map[string]registerIdent) native.Handler {
	return func(args []native.Value, _ map[string]native.Value, _ []native.Value, r *native.Registry) ([]native.Value, error) {
		if err := emitRegisterInstall(exports, idents, args, r); err != nil {
			return nil, err
		}
		return nil, nil
	}
}

// emitRegisterReturns is the check-mode counterpart of emitRegisterHandler: it
// performs the SAME install so a dynamically-registered emitter
// (`EmitLang.register up …`) is statically resolvable as `emit up` during the
// check pass. Idempotent on the registering source-call identity, so the
// runtime handler re-running the identical `register` in the compiled program
// is a no-op rather than an emit_kind_exists error. A non-concrete name or fn
// value leaves the export unresolved. Mirrors parseRegisterReturns.
func emitRegisterReturns(exports *native.OrderedMap, idents map[string]registerIdent) native.ReturnsFunc {
	return func(args []native.Value, r *native.Registry) []native.Value {
		if len(args) < 2 || !native.IsConcrete(args[0]) {
			return nil
		}
		if _, ok := args[1].Data.(native.FnDefInfo); !ok {
			return nil
		}
		if err := emitRegisterInstall(exports, idents, args, r); err != nil {
			surfaceRegisterCheckError(r, err, args[0])
		}
		return nil
	}
}

// emitKindsHandler lists the registered emitter-kind atoms (emit_ stripped),
// in registration order. emit_auto is excluded — it is the natural-format
// dispatcher, not a named kind.
func emitKindsHandler(exports *native.OrderedMap) native.Handler {
	return func(_ []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
		var kinds []native.Value
		for _, k := range exports.Keys() {
			if k == "emit_auto" {
				continue
			}
			if strings.HasPrefix(k, "emit_") {
				kinds = append(kinds, native.NewAtom(strings.TrimPrefix(k, "emit_")))
			}
		}
		return []native.Value{native.NewList(kinds)}, nil
	}
}
