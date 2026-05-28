package modules

import (
	"fmt"

	eng "github.com/aql-lang/aql/eng/go"
	"github.com/aql-lang/aql/lang/go/native"
)

// BuildTypeModule creates the "aql:type" native module — the second-
// tier type-operation vocabulary. The core type ops (refine, pathof,
// typeof, enum, is, teq, tpartial, guard, base, convert, tor, tand,
// tany, tall) are AQL built-ins. This module covers the rest.
//
// After import, words are accessed via dot notation: type.pick,
// type.exclude, type.lca, etc. The `t` prefix is dropped because the
// `type.` qualifier already disambiguates.
//
// Arg-order convention for module words:
//
//   - Surface form `a op b` (binary, swap) — `a` is on the stack
//     before the call, `b` is the forward arg. Each FnDef wrapper
//     uses a sig that requires the forward arg to come first
//     (BarrierPos=1 with the forward-typed param at sig[0]).
//   - In the FnDef body, execFnDefSig pushes wrapper args[0..N-1]
//     onto the stack in order — so the last-collected (the stack
//     operand) ends up on top. The inner native is therefore
//     written with args[0] = surface-LEFT (top of stack), args[1]
//     = surface-RIGHT (deeper).
//
// See lang/doc/design/TYPE-OPERATIONS.0.md.
func BuildTypeModule(parent *native.Registry) (native.ModuleDesc, error) {
	subReg, err := native.DefaultRegistry()
	if err != nil {
		return native.ModuleDesc{}, err
	}

	for _, n := range typeModuleNatives {
		subReg.RegisterNativeFunc(n)
	}

	exports := native.NewOrderedMap()

	// Unary Any -> Type.
	for _, name := range []string{"required", "parent", "root", "nominal"} {
		exports.Set(name, makeTypeUnaryFnDef(name, subReg, native.TType))
	}

	// Unary Any -> List (typed [:Type]).
	for _, name := range []string{"paramsof", "alts"} {
		exports.Set(name, makeTypeUnaryFnDef(name, subReg, native.TList))
	}

	// Unary Any -> Any.
	exports.Set("returnsof", makeTypeUnaryFnDef("returnsof", subReg, native.TAny))

	// Unary Any -> Integer.
	exports.Set("arityof", makeTypeUnaryFnDef("arityof", subReg, native.TInteger))

	// Binary Any Any -> Type (both args TAny — the wrapper accepts
	// either dispatch order; the handler validates). `merge` is also
	// a built-in word, so the inner native is registered as `_t_merge`
	// to avoid the dispatch path hitting the built-in via reg.Lookup.
	exports.Set("exclude", makeTypeBinaryAnyFnDef("exclude", "exclude", subReg, native.TType))
	exports.Set("extract", makeTypeBinaryAnyFnDef("extract", "extract", subReg, native.TType))
	exports.Set("merge", makeTypeBinaryAnyFnDef("_t_merge", "_t_merge", subReg, native.TType))
	exports.Set("lca", makeTypeBinaryAnyFnDef("lca", "lca", subReg, native.TType))

	// Any List -> Type (pick/omit). Surface form: `record op [list]`.
	// `pick` is a built-in stack op, so the inner is registered as
	// `_t_pick` to avoid the clash.
	exports.Set("pick", makeTypePickOmitFnDef("_t_pick", subReg))
	exports.Set("omit", makeTypePickOmitFnDef("omit", subReg))

	// Any Atom -> Type (brand). Surface form: `BaseType brand tag/q`.
	exports.Set("brand", makeTypeBrandFnDef("brand", subReg))

	modID := parent.Modules.NextID()
	desc := native.ModuleDesc{
		ID:      modID,
		Exports: map[string]*native.OrderedMap{"type": exports},
	}
	return desc, nil
}

// ---- FnDef wrapper helpers ----

func makeTypeUnaryFnDef(wordName string, subReg *native.Registry, returnType *native.Type) native.Value {
	return native.NewFnDef(native.FnDefInfo{
		Name: wordName,
		Sigs: []native.FnSig{{
			Params:  []native.FnParam{{Type: native.TAny}},
			Returns: []*native.Type{returnType},
			Body:    []native.Value{native.NewWord(wordName)}, BarrierPos: -1,
		}},
		Registry: subReg,
	})
}

// makeTypeBinaryAnyFnDef takes both a Name (used for reg.Lookup) and
// a bodyWord (used to invoke the inner native). They differ when the
// export name clashes with a built-in word — see the `merge` case
// where the inner native is registered as `_t_merge`.
func makeTypeBinaryAnyFnDef(name, bodyWord string, subReg *native.Registry, returnType *native.Type) native.Value {
	return native.NewFnDef(native.FnDefInfo{
		Name: name,
		Sigs: []native.FnSig{{
			Params:  []native.FnParam{{Type: native.TAny}, {Type: native.TAny}},
			Returns: []*native.Type{returnType},
			Body:    []native.Value{native.NewWord(bodyWord)}, BarrierPos: -1,
		}},
		Registry: subReg,
	})
}

// makeTypePickOmitFnDef wraps pick/omit. Uses [TAny, TAny] BarrierPos=-1
// like the other binary wrappers so post-forward-collection dispatch
// matches uniformly. Handler validates types.
func makeTypePickOmitFnDef(wordName string, subReg *native.Registry) native.Value {
	return native.NewFnDef(native.FnDefInfo{
		Name: wordName,
		Sigs: []native.FnSig{{
			Params:  []native.FnParam{{Type: native.TAny}, {Type: native.TAny}},
			Returns: []*native.Type{native.TType},
			Body:    []native.Value{native.NewWord(wordName)}, BarrierPos: -1,
		}},
		Registry: subReg,
	})
}

// makeTypeBrandFnDef wraps brand. Same [TAny, TAny] BarrierPos=-1 shape
// as the other binary wrappers; handler validates the tag-atom and
// base-type arguments.
func makeTypeBrandFnDef(wordName string, subReg *native.Registry) native.Value {
	return native.NewFnDef(native.FnDefInfo{
		Name: wordName,
		Sigs: []native.FnSig{{
			Params:  []native.FnParam{{Type: native.TAny}, {Type: native.TAny}},
			Returns: []*native.Type{native.TType},
			Body:    []native.Value{native.NewWord(wordName)}, BarrierPos: -1,
		}},
		Registry: subReg,
	})
}

// ---- Common helpers ----

func typeBodyArg(v native.Value, opName string, r *native.Registry) (native.Value, error) {
	if !native.IsTypeBody(v) {
		return native.Value{}, r.AqlError("type_error",
			fmt.Sprintf("%s: argument must be a type, got %s", opName, v.String()), opName)
	}
	return v, nil
}

// altSubtypes reports whether alt is target itself or one of its
// subtypes. Used by type.exclude / type.extract to give the
// TypeScript-style `Exclude<T,U>` / `Extract<T,U>` semantics where
// removing/keeping `Number` from a disjunct affects every numeric
// subtype (Integer, Decimal). Walks the ancestry chain by `*Type.ID`
// so non-canonical pointers from `latticeNode` still match.
//
// Structural type bodies (record/disjunct/object) fall back to
// strict equality — set-algebra over those is reserved for future
// work.
func altSubtypes(alt, target native.Value) bool {
	if native.IsBareTypeNode(alt) && native.IsBareTypeNode(target) {
		aNode := &alt
		tNode := &target
		for d := aNode; d != nil; d = d.Parent {
			if d.ID != "" && tNode.ID != "" && d.ID == tNode.ID {
				return true
			}
			if d == tNode {
				return true
			}
		}
		return false
	}
	return native.ValuesEqual(alt, target)
}

// (typedTypeList helper removed: returning regular Lists from the
// module words renders cleanly; the [:Type] static-contract claim is
// documented but not enforced at the runtime value level.)

func fieldsOf(v native.Value) *native.OrderedMap {
	switch {
	case native.IsRecordType(v):
		rec, _ := native.AsRecordType(v)
		return rec.Fields
	case native.IsObjectType(v):
		info, _ := native.AsObjectType(v)
		return info.AllFields()
	}
	return nil
}

func constructRecordOrObjectLike(original native.Value, newFields *native.OrderedMap, r *native.Registry) native.Value {
	if native.IsRecordType(original) {
		return native.NewRecordType(newFields)
	}
	id := native.GenerateObjectTypeID()
	info := native.ObjectTypeInfo{
		Fields: newFields,
		Parent: nil,
		ID:     id,
	}
	def := r.Types.MintType(id, native.TObject)
	return native.NewObjectType(def, info)
}

func stripNoneFromField(t native.Value) native.Value {
	if !native.IsDisjunct(t) {
		return t
	}
	disj, _ := native.AsDisjunct(t)
	kept := make([]native.Value, 0, len(disj.Alternatives))
	for _, alt := range disj.Alternatives {
		if !native.IsNoneShape(alt) {
			kept = append(kept, alt)
		}
	}
	if len(kept) == 0 {
		return t
	}
	if len(kept) == 1 {
		return kept[0]
	}
	return native.NewDisjunct(kept)
}

func latticeNode(v native.Value) *native.Type {
	if native.IsBareTypeNode(v) {
		node := v
		return &node
	}
	return v.Parent
}

// fnSigs returns the signature list for a Function / FnDef / FnUndef value.
func fnSigs(v native.Value, opName string, r *native.Registry) ([]native.FnSig, error) {
	if fn, ok := v.Data.(native.FnDefInfo); ok {
		return fn.Sigs, nil
	}
	if fnu, ok := v.Data.(native.FnUndefInfo); ok {
		out := make([]native.FnSig, len(fnu.Sigs))
		for i, s := range fnu.Sigs {
			out[i] = native.FnSig{Params: s.Params, Returns: s.Returns}
		}
		return out, nil
	}
	return nil, r.AqlError("type_error",
		fmt.Sprintf("%s: expected a Function or FunctionSignature, got %s", opName, v.String()), opName)
}

// fieldNames decodes a list of field-name atoms/strings into []string.
func fieldNames(list native.Value, opName string, r *native.Registry) ([]string, error) {
	if !native.IsConcrete(list) {
		return nil, r.AqlError("type_error",
			opName+": expected a concrete list of field names", opName)
	}
	lst, _ := native.AsList(list)
	out := make([]string, 0, lst.Len())
	for i := 0; i < lst.Len(); i++ {
		e := lst.Get(i)
		switch {
		case native.IsAtom(e):
			a, _ := native.AsAtom(e)
			out = append(out, a)
		case e.Parent != nil && e.Parent.Equal(native.TString):
			s, _ := native.AsString(e)
			out = append(out, s)
		default:
			return nil, r.AqlError("type_error",
				fmt.Sprintf("%s: field-name element must be Atom or String, got %s", opName, e.String()), opName)
		}
	}
	return out, nil
}

// ---- Inner native handlers ----
//
// All binary handlers below read args using the standard swap-form
// convention: args[0] = forward arg (surface-RIGHT), args[1] = stack
// arg (surface-LEFT). For `target type.exclude what`, args[0]=what,
// args[1]=target.

var typeModuleNatives = []native.NativeFunc{
	// --- exclude (set difference) ---
	{
		Name: "exclude",

		Signatures: []native.NativeSig{{
			Args: []*native.Type{native.TAny, native.TAny},
			Handler: func(args []native.Value, _ map[string]native.Value, _ []native.Value, r *native.Registry) ([]native.Value, error) {
				what, err := typeBodyArg(args[0], "type.exclude", r)
				if err != nil {
					return nil, err
				}
				target, err := typeBodyArg(args[1], "type.exclude", r)
				if err != nil {
					return nil, err
				}
				targetAlts := native.FlattenDisjunctAlts(target)
				removeSet := native.FlattenDisjunctAlts(what)
				kept := make([]native.Value, 0, len(targetAlts))
				for _, alt := range targetAlts {
					drop := false
					for _, rm := range removeSet {
						// Drop alt if it equals or is a subtype of any
						// remove-set member — matches TypeScript's
						// `Exclude<T,U>` "remove subtypes of U" semantic.
						if altSubtypes(alt, rm) {
							drop = true
							break
						}
					}
					if !drop {
						kept = append(kept, alt)
					}
				}
				if len(kept) == 0 {
					return []native.Value{native.NewTypeLiteral(native.TNever)}, nil
				}
				if len(kept) == 1 {
					return []native.Value{kept[0]}, nil
				}
				return []native.Value{native.NewDisjunct(kept)}, nil
			},
			Returns: []*native.Type{native.TType}, BarrierPos: -1,
		}},
	},

	// --- extract (set intersection) ---
	{
		Name: "extract",

		Signatures: []native.NativeSig{{
			Args: []*native.Type{native.TAny, native.TAny},
			Handler: func(args []native.Value, _ map[string]native.Value, _ []native.Value, r *native.Registry) ([]native.Value, error) {
				what, err := typeBodyArg(args[0], "type.extract", r)
				if err != nil {
					return nil, err
				}
				target, err := typeBodyArg(args[1], "type.extract", r)
				if err != nil {
					return nil, err
				}
				targetAlts := native.FlattenDisjunctAlts(target)
				keepSet := native.FlattenDisjunctAlts(what)
				kept := make([]native.Value, 0, len(targetAlts))
				for _, alt := range targetAlts {
					for _, k := range keepSet {
						// Keep alt if it equals or is a subtype of any
						// keep-set member — matches TypeScript's
						// `Extract<T,U>` "keep subtypes of U" semantic.
						if altSubtypes(alt, k) {
							kept = append(kept, alt)
							break
						}
					}
				}
				if len(kept) == 0 {
					return []native.Value{native.NewTypeLiteral(native.TNever)}, nil
				}
				if len(kept) == 1 {
					return []native.Value{kept[0]}, nil
				}
				return []native.Value{native.NewDisjunct(kept)}, nil
			},
			Returns: []*native.Type{native.TType}, BarrierPos: -1,
		}},
	},

	// --- required (strip None from every field) ---
	{
		Name: "required",

		Signatures: []native.NativeSig{{
			Args: []*native.Type{native.TAny},
			Handler: func(args []native.Value, _ map[string]native.Value, _ []native.Value, r *native.Registry) ([]native.Value, error) {
				t := args[0]
				fields := fieldsOf(t)
				if fields == nil {
					return nil, r.AqlError("type_error",
						fmt.Sprintf("type.required: argument must be a Record or Object type, got %s", t.String()), "required")
				}
				newFields := native.NewOrderedMap()
				for _, k := range fields.Keys() {
					ft, _ := fields.Get(k)
					newFields.Set(k, stripNoneFromField(ft))
				}
				return []native.Value{constructRecordOrObjectLike(t, newFields, r)}, nil
			},
			Returns: []*native.Type{native.TType}, BarrierPos: -1,
		}},
	},

	// --- pick (retain only named fields) ---
	// args[0] = list of field names (forward); args[1] = record/object (stack).
	{
		Name: "_t_pick",

		Signatures: []native.NativeSig{{
			Args: []*native.Type{native.TAny, native.TAny},
			Handler: func(args []native.Value, _ map[string]native.Value, _ []native.Value, r *native.Registry) ([]native.Value, error) {
				namesList := args[0]
				t := args[1]
				fields := fieldsOf(t)
				if fields == nil {
					return nil, r.AqlError("type_error",
						fmt.Sprintf("type.pick: first argument must be a Record or Object type, got %s", t.String()), "pick")
				}
				names, err := fieldNames(namesList, "type.pick", r)
				if err != nil {
					return nil, err
				}
				wanted := map[string]bool{}
				for _, n := range names {
					wanted[n] = true
				}
				newFields := native.NewOrderedMap()
				for _, k := range fields.Keys() {
					if wanted[k] {
						v, _ := fields.Get(k)
						newFields.Set(k, v)
					}
				}
				return []native.Value{constructRecordOrObjectLike(t, newFields, r)}, nil
			},
			Returns: []*native.Type{native.TType}, BarrierPos: -1,
		}},
	},

	// --- omit (drop named fields) ---
	// args[0] = list of field names (forward); args[1] = record/object (stack).
	{
		Name: "omit",

		Signatures: []native.NativeSig{{
			Args: []*native.Type{native.TAny, native.TAny},
			Handler: func(args []native.Value, _ map[string]native.Value, _ []native.Value, r *native.Registry) ([]native.Value, error) {
				namesList := args[0]
				t := args[1]
				fields := fieldsOf(t)
				if fields == nil {
					return nil, r.AqlError("type_error",
						fmt.Sprintf("type.omit: first argument must be a Record or Object type, got %s", t.String()), "omit")
				}
				names, err := fieldNames(namesList, "type.omit", r)
				if err != nil {
					return nil, err
				}
				dropped := map[string]bool{}
				for _, n := range names {
					dropped[n] = true
				}
				newFields := native.NewOrderedMap()
				for _, k := range fields.Keys() {
					if !dropped[k] {
						v, _ := fields.Get(k)
						newFields.Set(k, v)
					}
				}
				return []native.Value{constructRecordOrObjectLike(t, newFields, r)}, nil
			},
			Returns: []*native.Type{native.TType}, BarrierPos: -1,
		}},
	},

	// --- merge (combine two records/objects, unify on overlap) ---
	// `a merge b`: args[0]=b (forward), args[1]=a (stack). Result is
	// constructed as a's shape (Record stays Record, Object stays Object).
	{
		Name: "_t_merge",

		Signatures: []native.NativeSig{{
			Args: []*native.Type{native.TAny, native.TAny},
			Handler: func(args []native.Value, _ map[string]native.Value, _ []native.Value, r *native.Registry) ([]native.Value, error) {
				b := args[0]
				a := args[1]
				af := fieldsOf(a)
				bf := fieldsOf(b)
				if af == nil || bf == nil {
					return nil, r.AqlError("type_error",
						fmt.Sprintf("type.merge: both arguments must be Record or Object types, got %s and %s", a.String(), b.String()), "merge")
				}
				newFields := native.NewOrderedMap()
				for _, k := range af.Keys() {
					v, _ := af.Get(k)
					newFields.Set(k, v)
				}
				for _, k := range bf.Keys() {
					vb, _ := bf.Get(k)
					if existing, ok := newFields.Get(k); ok {
						unified, uok := eng.Unify(existing, vb)
						if !uok {
							return nil, r.AqlError("type_error",
								fmt.Sprintf("type.merge: field %q cannot unify (%s vs %s)", k, existing.String(), vb.String()), "merge")
						}
						newFields.Set(k, unified)
					} else {
						newFields.Set(k, vb)
					}
				}
				return []native.Value{constructRecordOrObjectLike(a, newFields, r)}, nil
			},
			Returns: []*native.Type{native.TType}, BarrierPos: -1,
		}},
	},

	// --- paramsof (function parameter types) ---
	{
		Name: "paramsof",

		Signatures: []native.NativeSig{{
			Args: []*native.Type{native.TAny},
			Handler: func(args []native.Value, _ map[string]native.Value, _ []native.Value, r *native.Registry) ([]native.Value, error) {
				fn := args[0]
				sigs, err := fnSigs(fn, "type.paramsof", r)
				if err != nil {
					return nil, err
				}
				if len(sigs) == 0 {
					return []native.Value{native.NewList(nil)}, nil
				}
				params := sigs[0].Params
				elems := make([]native.Value, 0, len(params))
				for _, p := range params {
					if p.Type == nil {
						elems = append(elems, native.NewTypeLiteral(native.TAny))
					} else {
						elems = append(elems, native.NewTypeLiteral(p.Type))
					}
				}
				return []native.Value{native.NewList(elems)}, nil
			},
			Returns: []*native.Type{native.TList}, BarrierPos: -1,
		}},
	},

	// --- returnsof (function return type) ---
	{
		Name: "returnsof",

		Signatures: []native.NativeSig{{
			Args: []*native.Type{native.TAny},
			Handler: func(args []native.Value, _ map[string]native.Value, _ []native.Value, r *native.Registry) ([]native.Value, error) {
				fn := args[0]
				sigs, err := fnSigs(fn, "type.returnsof", r)
				if err != nil {
					return nil, err
				}
				if len(sigs) == 0 || len(sigs[0].Returns) == 0 {
					return []native.Value{native.NewTypeLiteral(native.TAny)}, nil
				}
				returns := sigs[0].Returns
				if len(returns) == 1 {
					return []native.Value{native.NewTypeLiteral(returns[0])}, nil
				}
				elems := make([]native.Value, len(returns))
				for i, rt := range returns {
					elems[i] = native.NewTypeLiteral(rt)
				}
				return []native.Value{native.NewList(elems)}, nil
			},
			Returns: []*native.Type{native.TAny}, BarrierPos: -1,
		}},
	},

	// --- arityof (number of required, non-optional params) ---
	{
		Name: "arityof",

		Signatures: []native.NativeSig{{
			Args: []*native.Type{native.TAny},
			Handler: func(args []native.Value, _ map[string]native.Value, _ []native.Value, r *native.Registry) ([]native.Value, error) {
				fn := args[0]
				sigs, err := fnSigs(fn, "type.arityof", r)
				if err != nil {
					return nil, err
				}
				if len(sigs) == 0 {
					return []native.Value{native.NewInteger(0)}, nil
				}
				n := int64(0)
				for _, p := range sigs[0].Params {
					if !p.Optional {
						n++
					}
				}
				return []native.Value{native.NewInteger(n)}, nil
			},
			Returns: []*native.Type{native.TInteger}, BarrierPos: -1,
		}},
	},

	// --- parent (direct lattice parent) ---
	{
		Name: "parent",

		Signatures: []native.NativeSig{{
			Args: []*native.Type{native.TAny},
			Handler: func(args []native.Value, _ map[string]native.Value, _ []native.Value, r *native.Registry) ([]native.Value, error) {
				t, err := typeBodyArg(args[0], "type.parent", r)
				if err != nil {
					return nil, err
				}
				node := latticeNode(t)
				if node == nil || node.Parent == nil {
					return []native.Value{native.NewTypeLiteral(native.TAny)}, nil
				}
				return []native.Value{native.NewTypeLiteral(node.Parent)}, nil
			},
			Returns: []*native.Type{native.TType}, BarrierPos: -1,
		}},
	},

	// --- root (top of the branch) ---
	{
		Name: "root",

		Signatures: []native.NativeSig{{
			Args: []*native.Type{native.TAny},
			Handler: func(args []native.Value, _ map[string]native.Value, _ []native.Value, r *native.Registry) ([]native.Value, error) {
				t, err := typeBodyArg(args[0], "type.root", r)
				if err != nil {
					return nil, err
				}
				node := latticeNode(t)
				if node == nil {
					return []native.Value{native.NewTypeLiteral(native.TAny)}, nil
				}
				for node.Parent != nil && !node.Parent.Equal(native.TAny) {
					node = node.Parent
				}
				return []native.Value{native.NewTypeLiteral(node)}, nil
			},
			Returns: []*native.Type{native.TType}, BarrierPos: -1,
		}},
	},

	// --- lca (least common ancestor of two types) ---
	{
		Name: "lca",

		Signatures: []native.NativeSig{{
			Args: []*native.Type{native.TAny, native.TAny},
			Handler: func(args []native.Value, _ map[string]native.Value, _ []native.Value, r *native.Registry) ([]native.Value, error) {
				a, err := typeBodyArg(args[0], "type.lca", r)
				if err != nil {
					return nil, err
				}
				b, err := typeBodyArg(args[1], "type.lca", r)
				if err != nil {
					return nil, err
				}
				// eng.CommonAncestorType uses pointer identity, but
				// latticeNode returns &v of a stack-local Value copy —
				// pointers don't match canonical kernel nodes. Walk by
				// ID instead.
				aNode := latticeNode(a)
				bNode := latticeNode(b)
				seen := map[string]*native.Type{}
				for d := aNode; d != nil; d = d.Parent {
					if d.ID != "" {
						seen[d.ID] = d
					}
				}
				for d := bNode; d != nil; d = d.Parent {
					if d.ID != "" {
						if hit, ok := seen[d.ID]; ok {
							return []native.Value{native.NewTypeLiteral(hit)}, nil
						}
					}
				}
				return []native.Value{native.NewTypeLiteral(native.TAny)}, nil
			},
			Returns: []*native.Type{native.TType}, BarrierPos: -1,
		}},
	},

	// --- alts (disjunct alternatives) ---
	{
		Name: "alts",

		Signatures: []native.NativeSig{{
			Args: []*native.Type{native.TAny},
			Handler: func(args []native.Value, _ map[string]native.Value, _ []native.Value, r *native.Registry) ([]native.Value, error) {
				t, err := typeBodyArg(args[0], "type.alts", r)
				if err != nil {
					return nil, err
				}
				alts := native.FlattenDisjunctAlts(t)
				return []native.Value{native.NewList(alts)}, nil
			},
			Returns: []*native.Type{native.TList}, BarrierPos: -1,
		}},
	},

	// --- nominal (alias for refine 1-arg) ---
	{
		Name: "nominal",

		Signatures: []native.NativeSig{{
			Args: []*native.Type{native.TAny},
			Handler: func(args []native.Value, _ map[string]native.Value, _ []native.Value, r *native.Registry) ([]native.Value, error) {
				t, err := typeBodyArg(args[0], "type.nominal", r)
				if err != nil {
					return nil, err
				}
				base := latticeNode(t)
				if base == nil {
					return nil, r.AqlError("type_error",
						"type.nominal: argument must be a lattice-resident type", "nominal")
				}
				anon := r.Types.MintRefinePrefab(native.CanonicalType(r, base))
				return []native.Value{native.NewTypeLiteral(anon)}, nil
			},
			Returns: []*native.Type{native.TType}, BarrierPos: -1,
		}},
	},

	// --- brand (nominal subtype with a tag atom) ---
	// `BaseType brand tag/q`: args[0]=tag (forward), args[1]=BaseType (stack).
	{
		Name: "brand",

		Signatures: []native.NativeSig{{
			Args: []*native.Type{native.TAny, native.TAny},
			Handler: func(args []native.Value, _ map[string]native.Value, _ []native.Value, r *native.Registry) ([]native.Value, error) {
				tag, err := args[0].AsConcreteAtom()
				if err != nil {
					return nil, err
				}
				t, err := typeBodyArg(args[1], "type.brand", r)
				if err != nil {
					return nil, err
				}
				base := latticeNode(t)
				if base == nil {
					return nil, r.AqlError("type_error",
						"type.brand: base must be a lattice-resident type", "brand")
				}
				anon := r.Types.MintRefinePrefab(native.CanonicalType(r, base))
				anon.Name = "brand:" + tag
				return []native.Value{native.NewTypeLiteral(anon)}, nil
			},
			Returns: []*native.Type{native.TType}, BarrierPos: -1,
		}},
	},
}
