package modules

import (
	"fmt"

	"github.com/aql-lang/aql/lang/go/native"
)

// sift_module.go — the spec engine (validate a spec-map, compile it to a parse
// handler), the seven Sift words, and BuildSiftModule (which registers the six
// families and wires the namespace). See design/SIFT.0.md §6-§8.

// siftTopKeys is the closed set of spec-map top-level keys; anything else is a
// sift_spec_error (typos must surface, §6).
var siftTopKeys = map[string]bool{
	"family": true, "opts": true, "fields": true, "detect": true, "doc": true, "fn": true,
}

// specFromValue validates a spec-map Value into a siftSpec. A Function value is
// sugar for {family:'fn' fn:<fn>}.
func specFromValue(v native.Value, r *native.Registry) (*siftSpec, error) {
	if fn, ok := native.FnDefFromValue(v); ok {
		_ = fn
		if err := validateSiftFn(v, r); err != nil {
			return nil, err
		}
		return &siftSpec{family: famFn, fn: v, opts: native.NewNone()}, nil
	}
	m, err := native.RequireConcreteMap(v, "sift")
	if err != nil {
		return nil, r.AqlError("sift_spec_error", "spec must be a map or a function", "sift")
	}
	for _, k := range m.Keys() {
		if !siftTopKeys[k] {
			return nil, r.AqlError("sift_spec_error", "unknown spec key: "+k, "sift")
		}
	}
	spec := &siftSpec{opts: native.NewNone()}
	famV, ok := m.Get("family")
	if !ok {
		return nil, r.AqlError("sift_spec_error", "spec is missing 'family'", "sift")
	}
	spec.family = native.ValToString(famV)
	if spec.family != famFn && !isFamily(spec.family) {
		return nil, r.AqlError("sift_spec_error", "unknown family: "+spec.family, "sift")
	}
	if o, ok := m.Get("opts"); ok {
		if _, e := native.RequireConcreteMap(o, "sift"); e != nil {
			return nil, r.AqlError("sift_spec_error", "opts must be a map", "sift")
		}
		spec.opts = o
	}
	// fields sugar folds onto opts.fields (+ opts.types via the family reader).
	if fv, ok := m.Get("fields"); ok {
		src, _ := native.AsMap(spec.opts)
		if src != nil {
			if _, clash := src.Get("fields"); clash {
				return nil, r.AqlError("sift_spec_error", "fields given both top-level and in opts", "sift")
			}
		}
		om := cloneOrdered(src)
		om.Set("fields", fv)
		spec.opts = native.NewMap(om)
	}
	if spec.family == famFn {
		fnV, ok := m.Get("fn")
		if !ok {
			return nil, r.AqlError("sift_spec_error", "family:'fn' spec is missing 'fn'", "sift")
		}
		if err := validateSiftFn(fnV, r); err != nil {
			return nil, err
		}
		spec.fn = fnV
	}
	if err := readDetect(m, spec, r); err != nil {
		return nil, err
	}
	if dv, ok := m.Get("doc"); ok {
		spec.doc = native.ValToString(dv)
	}
	return spec, nil
}

func isFamily(f string) bool {
	for _, x := range siftFamilies {
		if x == f {
			return true
		}
	}
	return false
}

// validateSiftFn enforces the standard [source opts:Map …] signature prefix on
// a fn-form parser (the parseRegisterValidate contract).
func validateSiftFn(v native.Value, r *native.Registry) error {
	fnDef, ok := v.Data.(native.FnDefInfo)
	if !ok || len(fnDef.Signatures) == 0 {
		return r.AqlError("sift_spec_error", "fn: expected a function value", "sift")
	}
	for _, sig := range fnDef.Signatures {
		if len(sig.Params) < 2 {
			return r.AqlErrorHint("sift_spec_error",
				"fn: every signature must start [source:String opts:Map …]", "sift",
				"declare fn [[source:String opts:Map] [outputs] [body]]")
		}
		p0, p1 := sig.Params[0].Type, sig.Params[1].Type
		if p0 == nil || !(p0.ConformsTo(native.TString) || p0.Equal(native.TAny)) ||
			p1 == nil || !p1.ConformsTo(native.TMap) {
			return r.AqlErrorHint("sift_spec_error",
				"fn: every signature must start [source:String opts:Map …]", "sift",
				"declare fn [[source:String opts:Map] [outputs] [body]]")
		}
	}
	return nil
}

func readDetect(m native.ReadMap, spec *siftSpec, r *native.Registry) error {
	dv, ok := m.Get("detect")
	if !ok {
		return nil
	}
	dm, err := native.RequireConcreteMap(dv, "sift")
	if err != nil {
		return r.AqlError("sift_spec_error", "detect must be a map", "sift")
	}
	if pv, ok := dm.Get("path"); ok {
		l, e := native.AsList(pv)
		if e != nil || l.IsNil() {
			return r.AqlError("sift_spec_error", "detect.path must be a list", "sift")
		}
		for i := 0; i < l.Len(); i++ {
			spec.pathPat = append(spec.pathPat, native.ValToString(l.Get(i)))
		}
	}
	if cv, ok := dm.Get("cmd"); ok {
		cm, e := native.RequireConcreteMap(cv, "sift")
		if e != nil {
			return r.AqlError("sift_spec_error", "detect.cmd must be a map", "sift")
		}
		if mv, ok := cm.Get("match"); ok {
			l, e := native.AsList(mv)
			if e != nil || l.IsNil() {
				return r.AqlError("sift_spec_error", "detect.cmd.match must be a list", "sift")
			}
			for i := 0; i < l.Len(); i++ {
				spec.cmdName = append(spec.cmdName, native.ValToString(l.Get(i)))
			}
		}
		if av, ok := cm.Get("argv"); ok {
			l, e := native.AsList(av)
			if e != nil || l.IsNil() {
				return r.AqlError("sift_spec_error", "detect.cmd.argv must be a list", "sift")
			}
			for i := 0; i < l.Len(); i++ {
				spec.cmdArgv = append(spec.cmdArgv, native.ValToString(l.Get(i)))
			}
		}
	}
	return nil
}

func cloneOrdered(m native.ReadMap) *native.OrderedMap {
	out := native.NewOrderedMap()
	if m == nil {
		return out
	}
	for _, k := range m.Keys() {
		v, _ := m.Get(k)
		out.Set(k, v)
	}
	return out
}

// mergeOpts overlays call-site opts on baked opts (call site wins).
func mergeOpts(baked, call native.Value) native.Value {
	bm, _ := native.AsMap(baked)
	cm, _ := native.AsMap(call)
	if bm == nil && cm == nil {
		return native.NewNone()
	}
	out := native.NewOrderedMap()
	if bm != nil {
		for _, k := range bm.Keys() {
			v, _ := bm.Get(k)
			out.Set(k, v)
		}
	}
	if cm != nil {
		for _, k := range cm.Keys() {
			v, _ := cm.Get(k)
			out.Set(k, v)
		}
	}
	return native.NewMap(out)
}

// runSpec parses src through spec with the given call-site opts.
func runSpec(spec *siftSpec, src string, call native.Value, r *native.Registry) (native.Value, error) {
	merged := mergeOpts(spec.opts, call)
	if spec.family == famFn {
		return callSiftFn(spec.fn, src, merged, r)
	}
	return siftParseFamily(spec.family, src, siftOpts{m: merged}, r)
}

// callSiftFn invokes an AQL [source opts] parser fn over the resolved source,
// mirroring the parselang deferred-dispatch invoke.
func callSiftFn(fn native.Value, src string, opts native.Value, r *native.Registry) (native.Value, error) {
	if m, _ := native.AsMap(opts); m == nil {
		opts = native.NewMap(native.NewOrderedMap()) // fn opts param is Map, never None
	}
	sub := native.NewTop(r)
	res, err := sub.Run([]native.Value{fn, native.NewString(src), opts, native.NewEnd()})
	if err != nil {
		return native.Value{}, err
	}
	if len(res) != 1 {
		return native.Value{}, r.AqlError("sift_parse_error",
			fmt.Sprintf("fn parser: expected one result, got %d", len(res)), "sift")
	}
	return res[0], nil
}

// specHandler adapts a compiled spec to a parselang Handler: args[0] is the
// resolved source String, args[1] the call-site opts Map.
func specHandler(spec *siftSpec) native.Handler {
	return func(args []native.Value, _ map[string]native.Value, _ []native.Value, r *native.Registry) ([]native.Value, error) {
		src, err := args[0].AsConcreteString()
		if err != nil {
			return nil, r.AqlError("sift_parse_error", "source: "+err.Error(), "sift")
		}
		v, err := runSpec(spec, src, args[1], r)
		if err != nil {
			return nil, err
		}
		return []native.Value{v}, nil
	}
}

// ---------------------------------------------------------------------------
// Registration: install a spec as a parselang kind (+ read format).
// ---------------------------------------------------------------------------

// installKind registers spec under name as a `parse <name>` kind and, when the
// formats capability is present, a `read {fmt:'<name>'}` format. It records the
// spec in sift state (introspection + detection). Idempotent for the compiled
// path's check+VM double execution via the identity ledger; a genuine
// re-definition raises sift_kind_exists.
func installKind(st *siftHostState, name string, spec *siftSpec, ident string, checkMode bool, r *native.Registry) error {
	st.mu.Lock()
	defer st.mu.Unlock()
	if existing, dup := st.specs[name]; dup {
		if existing.installIdent != "" && ident != "::" && existing.installIdent == ident {
			if checkMode {
				return nil
			}
			if existing.fromCheck {
				existing.fromCheck = false
				return nil
			}
		}
		return r.AqlError("sift_kind_exists", "parser "+name+" is already registered", "sift")
	}
	if builtinParserKind(name) {
		return r.AqlError("sift_kind_exists", "parser "+name+" collides with a built-in kind", "sift")
	}
	if err := checkDetectConflicts(st, name, spec, r); err != nil {
		return err
	}
	spec.name = name
	spec.installIdent = ident
	spec.fromCheck = checkMode
	ps := ParseLangSpec{
		Name:    name,
		Returns: []*native.Type{native.TAny},
		Handler: specHandler(spec),
		Pure:    spec.family != famFn,
	}
	if err := RegisterHostParser(r, ps); err != nil {
		return r.AqlError("sift_kind_exists", err.Error(), "sift")
	}
	if native.HostFormats(r) != nil {
		_ = native.RegisterFormat(r, name, &siftReadFormat{spec: spec, reg: r})
	}
	st.specs[name] = spec
	st.order = append(st.order, name)
	for _, p := range spec.pathPat {
		st.pathDetect[p] = name
	}
	for _, c := range spec.cmdName {
		st.cmdDetect[c] = name
	}
	return nil
}

func checkDetectConflicts(st *siftHostState, name string, spec *siftSpec, r *native.Registry) error {
	for _, p := range spec.pathPat {
		if other, ok := st.pathDetect[p]; ok && other != name {
			return r.AqlError("sift_detect_conflict", fmt.Sprintf("path %q already claimed by %q", p, other), "sift")
		}
	}
	for _, c := range spec.cmdName {
		if other, ok := st.cmdDetect[c]; ok && other != name {
			return r.AqlError("sift_detect_conflict", fmt.Sprintf("command %q already claimed by %q", c, other), "sift")
		}
	}
	return nil
}

// siftReadFormat bridges a compiled spec to the read Format interface.
type siftReadFormat struct {
	spec *siftSpec
	reg  *native.Registry
}

func (f *siftReadFormat) Decode(content string) ([]native.Value, error) {
	return f.DecodeOpts(content, nil)
}

func (f *siftReadFormat) DecodeOpts(content string, opts map[string]any) ([]native.Value, error) {
	v, err := runSpec(f.spec, content, anyMapToValue(opts), f.reg)
	if err != nil {
		return nil, err
	}
	return []native.Value{v}, nil
}

func (f *siftReadFormat) Encode(native.Value) (string, error) {
	return "", fmt.Errorf("sift format %q is read-only", f.spec.name)
}

func (f *siftReadFormat) ReadOnly() bool { return true }

// anyMapToValue converts read's map[string]any opts to an AQL Map (best-effort
// for the read path; enum values arrive as strings, adequate for the families).
func anyMapToValue(m map[string]any) native.Value {
	if len(m) == 0 {
		return native.NewNone()
	}
	om := native.NewOrderedMap()
	for k, v := range m {
		om.Set(k, anyToValue(v))
	}
	return native.NewMap(om)
}

func anyToValue(x any) native.Value {
	switch v := x.(type) {
	case nil:
		return native.NewNone()
	case string:
		return native.NewString(v)
	case bool:
		return native.NewBoolean(v)
	case int:
		return native.NewInteger(int64(v))
	case int64:
		return native.NewInteger(v)
	case float64:
		if v == float64(int64(v)) {
			return native.NewInteger(int64(v))
		}
		return native.NewFloat(v)
	case []any:
		elems := make([]native.Value, len(v))
		for i, e := range v {
			elems[i] = anyToValue(e)
		}
		return native.NewList(elems)
	case map[string]any:
		return anyMapToValue(v)
	default:
		return native.NewString(fmt.Sprintf("%v", v))
	}
}
