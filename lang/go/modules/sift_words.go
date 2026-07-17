package modules

import (
	"github.com/aql-lang/aql/lang/go/native"
)

// sift_words.go — the seven Sift.* words and BuildSiftModule.

// BuildSiftModule creates the "aql:sift" native module: the Sift namespace of
// seven words, and (once per registry) the six family parse kinds + read
// formats. See design/SIFT.0.md §7, §8, §15.
func BuildSiftModule(parent *native.Registry) (native.ModuleDesc, error) {
	st := siftHostStateFor(parent)
	// Register the six families as parse kinds + read formats, once per
	// registry (re-import is idempotent). A family kind is the degenerate
	// preset {family:'<name>'} — all options come from the call site.
	st.mu.Lock()
	if !st.registered {
		st.registered = true
		st.mu.Unlock()
		for _, fam := range siftFamilies {
			spec := &siftSpec{name: fam, family: fam, opts: native.NewNone()}
			ps := ParseLangSpec{
				Name:    fam,
				Returns: []*native.Type{native.TAny},
				Handler: specHandler(spec),
				Pure:    true,
			}
			if err := RegisterHostParser(parent, ps); err != nil {
				return native.ModuleDesc{}, err
			}
			if native.HostFormats(parent) != nil {
				if err := native.RegisterFormat(parent, fam, &siftReadFormat{spec: spec, reg: parent}); err != nil {
					return native.ModuleDesc{}, err
				}
			}
			st.mu.Lock()
			st.specs[fam] = spec
			st.order = append(st.order, fam)
			st.mu.Unlock()
		}
	} else {
		st.mu.Unlock()
	}

	subReg, err := newDefaultRegistry()
	if err != nil {
		return native.ModuleDesc{}, err
	}
	exports := native.NewOrderedMap()

	// ---- define ---------------------------------------------------------
	subReg.RegisterNativeFunc(native.NativeFunc{
		Name: "sift-define",
		Signatures: []native.Signature{{
			Args:          []*native.Type{native.TAtom, native.TAny},
			QuoteArgs:     map[int]bool{0: true},
			Returns:       []*native.Type{},
			BarrierPos:    -1,
			CompileEffect: native.CompileStoresFn,
			Impl:          native.Go(siftDefineHandler(st)),
			ReturnsFn:     siftDefineReturns(st),
		}},
	})
	exports.Set("define", wrapMiniFnDef("sift-define", [][]native.FnParam{
		{{Type: native.TAtom, Quote: true}, {Type: native.TAny}},
	}, []*native.Type{}, map[int]bool{0: true}, subReg))

	// ---- parse ----------------------------------------------------------
	subReg.RegisterNativeFunc(native.NativeFunc{
		Name: "sift-parse",
		Signatures: []native.Signature{
			{Args: []*native.Type{native.TAny, native.TMap, native.TString}, Returns: []*native.Type{native.TAny}, BarrierPos: -1, Impl: native.Go(siftParseHandler(st))},
			{Args: []*native.Type{native.TAny, native.TString}, Returns: []*native.Type{native.TAny}, BarrierPos: -1, Impl: native.Go(siftParseHandler(st))},
		},
	})
	exports.Set("parse", wrapMiniFnDef("sift-parse", [][]native.FnParam{
		{{Type: native.TAny}, {Type: native.TMap}, {Type: native.TString}},
		{{Type: native.TAny}, {Type: native.TString}},
	}, []*native.Type{native.TAny}, nil, subReg))

	// ---- kinds ----------------------------------------------------------
	subReg.RegisterNativeFunc(native.NativeFunc{
		Name:       "sift-kinds",
		Signatures: []native.Signature{{Args: []*native.Type{}, Returns: []*native.Type{native.TList}, BarrierPos: -1, Impl: native.Go(siftKindsHandler(st))}},
	})
	exports.Set("kinds", wrapMiniFnDef("sift-kinds", [][]native.FnParam{{}}, []*native.Type{native.TList}, nil, subReg))

	// ---- families -------------------------------------------------------
	subReg.RegisterNativeFunc(native.NativeFunc{
		Name:       "sift-families",
		Signatures: []native.Signature{{Args: []*native.Type{}, Returns: []*native.Type{native.TList}, BarrierPos: -1, Impl: native.Go(siftFamiliesHandler)}},
	})
	exports.Set("families", wrapMiniFnDef("sift-families", [][]native.FnParam{{}}, []*native.Type{native.TList}, nil, subReg))

	// ---- spec -----------------------------------------------------------
	subReg.RegisterNativeFunc(native.NativeFunc{
		Name: "sift-spec",
		Signatures: []native.Signature{{
			Args: []*native.Type{native.TAtom}, QuoteArgs: map[int]bool{0: true},
			Returns: []*native.Type{native.TMap}, BarrierPos: -1, Impl: native.Go(siftSpecHandler(st)),
		}},
	})
	exports.Set("spec", wrapMiniFnDef("sift-spec", [][]native.FnParam{
		{{Type: native.TAtom, Quote: true}},
	}, []*native.Type{native.TMap}, map[int]bool{0: true}, subReg))

	// ---- detect ---------------------------------------------------------
	subReg.RegisterNativeFunc(native.NativeFunc{
		Name:       "sift-detect",
		Signatures: []native.Signature{{Args: []*native.Type{native.TAny}, Returns: []*native.Type{native.TAny}, BarrierPos: -1, Impl: native.Go(siftDetectHandler(st))}},
	})
	exports.Set("detect", wrapMiniFnDef("sift-detect", [][]native.FnParam{
		{{Type: native.TAny}},
	}, []*native.Type{native.TAny}, nil, subReg))

	// ---- check ----------------------------------------------------------
	subReg.RegisterNativeFunc(native.NativeFunc{
		Name:       "sift-check",
		Signatures: []native.Signature{{Args: []*native.Type{native.TAny}, Returns: []*native.Type{native.TList}, BarrierPos: -1, Impl: native.Go(siftCheckHandler)}},
	})
	exports.Set("check", wrapMiniFnDef("sift-check", [][]native.FnParam{
		{{Type: native.TAny}},
	}, []*native.Type{native.TList}, nil, subReg))

	return native.ModuleDesc{
		Src:     subReg,
		ID:      parent.Modules.NextID(),
		Exports: map[string]*native.OrderedMap{"Sift": exports},
	}, nil
}

// ---- handlers --------------------------------------------------------------

func siftDefineInstall(st *siftHostState, args []native.Value, checkMode bool, r *native.Registry) error {
	name, err := args[0].AsConcreteAtom()
	if err != nil {
		return r.AqlError("sift_bad_name", "define: "+err.Error(), "sift")
	}
	if why := parseValidKindName(name); why != "" {
		return r.AqlError("sift_bad_name", "define: "+why, "sift")
	}
	spec, err := specFromValue(args[1], r)
	if err != nil {
		return err
	}
	return installKind(st, name, spec, registerIdentity(args[1]), checkMode, r)
}

func siftDefineHandler(st *siftHostState) native.Handler {
	return func(args []native.Value, _ map[string]native.Value, _ []native.Value, r *native.Registry) ([]native.Value, error) {
		if err := siftDefineInstall(st, args, r.Check.IsActive(), r); err != nil {
			return nil, err
		}
		return nil, nil
	}
}

func siftDefineReturns(st *siftHostState) native.ReturnsFunc {
	return func(args []native.Value, r *native.Registry) []native.Value {
		if len(args) < 2 || !native.IsConcrete(args[0]) {
			return nil
		}
		if _, isFn := native.FnDefFromValue(args[1]); !isFn && !native.IsConcrete(args[1]) {
			return nil
		}
		if err := siftDefineInstall(st, args, true, r); err != nil {
			surfaceRegisterCheckError(r, err, args[0])
		}
		return nil
	}
}

func siftParseHandler(st *siftHostState) native.Handler {
	return func(args []native.Value, _ map[string]native.Value, _ []native.Value, r *native.Registry) ([]native.Value, error) {
		var call native.Value = native.NewNone()
		var srcV native.Value
		if len(args) == 3 {
			call = args[1]
			srcV = args[2]
		} else {
			srcV = args[1]
		}
		src, err := resolveParseSource(srcV, r)
		if err != nil {
			return nil, err
		}
		spec, err := siftResolveSpec(st, args[0], r)
		if err != nil {
			return nil, err
		}
		v, err := runSpec(spec, src, call, r)
		if err != nil {
			return nil, err
		}
		return []native.Value{v}, nil
	}
}

// siftResolveSpec turns Sift.parse's first argument into a spec: a Map is an
// inline spec; a String/Atom names a registered kind.
func siftResolveSpec(st *siftHostState, v native.Value, r *native.Registry) (*siftSpec, error) {
	if v.Parent.Equal(native.TMap) && native.IsConcrete(v) {
		return specFromValue(v, r)
	}
	if v.Parent.ConformsTo(native.TString) || v.Parent.ConformsTo(native.TAtom) {
		name := native.ValToString(v)
		st.mu.Lock()
		spec, ok := st.specs[name]
		st.mu.Unlock()
		if !ok {
			return nil, r.AqlError("sift_unknown_kind", "no such kind: "+name, "sift")
		}
		return spec, nil
	}
	if _, ok := native.FnDefFromValue(v); ok {
		return specFromValue(v, r)
	}
	return nil, r.AqlError("sift_spec_error", "parse: first argument must be a spec map or a kind name", "sift")
}

func siftKindsHandler(st *siftHostState) native.Handler {
	return func(_ []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
		st.mu.Lock()
		defer st.mu.Unlock()
		elems := make([]native.Value, len(st.order))
		for i, n := range st.order {
			elems[i] = native.NewAtom(n)
		}
		return []native.Value{native.NewList(elems)}, nil
	}
}

func siftFamiliesHandler(_ []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
	elems := make([]native.Value, len(siftFamilies))
	for i, f := range siftFamilies {
		elems[i] = native.NewAtom(f)
	}
	return []native.Value{native.NewList(elems)}, nil
}

func siftSpecHandler(st *siftHostState) native.Handler {
	return func(args []native.Value, _ map[string]native.Value, _ []native.Value, r *native.Registry) ([]native.Value, error) {
		name, err := args[0].AsConcreteAtom()
		if err != nil {
			return nil, r.AqlError("sift_bad_name", "spec: "+err.Error(), "sift")
		}
		st.mu.Lock()
		spec, ok := st.specs[name]
		st.mu.Unlock()
		if !ok {
			return nil, r.AqlError("sift_unknown_kind", "no such kind: "+name, "sift")
		}
		return []native.Value{specToValue(spec)}, nil
	}
}

// specToValue renders a registered spec back to a Map (fn kinds elide fn:).
func specToValue(spec *siftSpec) native.Value {
	m := native.NewOrderedMap()
	m.Set("name", native.NewAtom(spec.name))
	m.Set("family", native.NewAtom(spec.family))
	if om, _ := native.AsMap(spec.opts); om != nil {
		m.Set("opts", spec.opts)
	}
	if spec.doc != "" {
		m.Set("doc", native.NewString(spec.doc))
	}
	return native.NewMap(m)
}

func siftDetectHandler(st *siftHostState) native.Handler {
	return func(args []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
		st.mu.Lock()
		defer st.mu.Unlock()
		// A List argument is an argv (match on argv0 basename); a String is a path.
		if l, err := native.AsList(args[0]); err == nil && !l.IsNil() && l.Len() > 0 {
			argv0 := native.ValToString(l.Get(0))
			base := argv0
			if i := lastSlash(argv0); i >= 0 {
				base = argv0[i+1:]
			}
			if kind, ok := st.cmdDetect[base]; ok {
				return []native.Value{native.NewAtom(kind)}, nil
			}
			return []native.Value{native.NewNone()}, nil
		}
		path := native.ValToString(args[0])
		for glob, kind := range st.pathDetect {
			if matchGlob(glob, path) {
				return []native.Value{native.NewAtom(kind)}, nil
			}
		}
		return []native.Value{native.NewNone()}, nil
	}
}

func lastSlash(s string) int {
	for i := len(s) - 1; i >= 0; i-- {
		if s[i] == '/' {
			return i
		}
	}
	return -1
}

// matchGlob matches the policy trailing-* glob dialect: a literal match, or a
// prefix match when the pattern ends with '*'.
func matchGlob(pattern, s string) bool {
	if len(pattern) > 0 && pattern[len(pattern)-1] == '*' {
		return len(s) >= len(pattern)-1 && s[:len(pattern)-1] == pattern[:len(pattern)-1]
	}
	return pattern == s
}

func siftCheckHandler(args []native.Value, _ map[string]native.Value, _ []native.Value, r *native.Registry) ([]native.Value, error) {
	issues := native.NewList([]native.Value{})
	// validate without registering; collect the one error (if any) as an issue.
	if _, err := specFromValue(args[0], r); err != nil {
		code, detail := "sift_spec_error", err.Error()
		if ae, ok := err.(*native.AqlError); ok {
			code, detail = ae.Code, ae.Detail
		}
		issue := native.NewOrderedMap()
		issue.Set("code", native.NewAtom(code))
		issue.Set("detail", native.NewString(detail))
		issues = native.NewList([]native.Value{native.NewMap(issue)})
	}
	return []native.Value{issues}, nil
}
