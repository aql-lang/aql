package native

import (
	"fmt"

	"github.com/aql-lang/aql/eng/go"
)

// definitionNatives covers the binding / function-definition words:
// def, undef, var, fn, call, dblcall, args, __pa.
//
// Pure helpers used by these handlers (parseFnDef, parseFnParams,
// MatchFnSig, defName, defStackOnly, etc.) live alongside their
// callers in native_definition_fn.go and native_definition_helpers.go.
var definitionNatives = []NativeFunc{
	{
		Name:        "def",
		ForwardArgs: true,
		Signatures: []NativeSig{
			{
				// Typed-name binding: def name:*Type body. Sorts first
				// because TMap is more specific than TString / TAtom
				// at the same depth (higher inherent score).
				Args:           []*Type{TMap, TAny},
				NoEvalArgs:     map[int]bool{1: true},
				NoEvalMapArgs:  map[int]bool{0: true},
				Handler:        defTypedHandler,
				Returns:        []*Type{},
				RunInCheckMode: true,
			},
			{
				Args:           []*Type{TString, TAny},
				NoEvalArgs:     map[int]bool{1: true},
				Handler:        defHandler,
				Returns:        []*Type{},
				RunInCheckMode: true,
			},
			{
				Args:           []*Type{TAtom, TAny},
				QuoteArgs:      map[int]bool{0: true},
				NoEvalArgs:     map[int]bool{1: true},
				Handler:        defHandler,
				Returns:        []*Type{},
				RunInCheckMode: true,
			},
		},
	},
	{
		Name:        "undef",
		ForwardArgs: true,
		Signatures: []NativeSig{
			{
				Args:           []*Type{TString},
				Handler:        undefHandler,
				Returns:        []*Type{},
				RunInCheckMode: true,
			},
			{
				Args:           []*Type{TAtom},
				QuoteArgs:      map[int]bool{0: true},
				Handler:        undefHandler,
				Returns:        []*Type{},
				RunInCheckMode: true,
			},
			{
				Args:           []*Type{TString, TFnUndef},
				Handler:        undefFnHandler,
				Returns:        []*Type{},
				RunInCheckMode: true,
			},
			{
				Args:           []*Type{TAtom, TFnUndef},
				QuoteArgs:      map[int]bool{0: true},
				Handler:        undefFnHandler,
				Returns:        []*Type{},
				RunInCheckMode: true,
			},
		},
	},
	{
		Name:        "var",
		ForwardArgs: true,
		Signatures: []NativeSig{{
			Args:       []*Type{TList},
			NoEvalArgs: map[int]bool{0: true},
			Handler:    varHandler,
			Returns:    []*Type{TAny},
		}},
	},
	{
		Name:        "fn",
		ForwardArgs: true,
		Signatures: []NativeSig{{
			Args:           []*Type{TList},
			NoEvalArgs:     map[int]bool{0: true},
			Handler:        fnHandler,
			Returns:        []*Type{TFunction},
			RunInCheckMode: true,
		}},
	},
	{
		Name:        "fnsig",
		ForwardArgs: true,
		Signatures: []NativeSig{{
			Args:       []*Type{TList},
			NoEvalArgs: map[int]bool{0: true},
			Handler:    fnsigHandler,
			Returns:    []*Type{TFnUndef},
		}},
	},
	{
		Name:        "call",
		ForwardArgs: true,
		Signatures: []NativeSig{{
			Args:       []*Type{TList},
			NoEvalArgs: map[int]bool{0: true},
			Handler:    callHandler,
			Returns:    []*Type{TAny},
		}},
	},
	{
		Name:        "dblcall",
		ForwardArgs: true,
		Signatures: []NativeSig{{
			Args:       []*Type{TInteger, TList},
			NoEvalArgs: map[int]bool{1: true},
			Handler:    dblcallHandler,
			Returns:    []*Type{TAny},
		}},
	},
	{
		Name:        "args",
		ForwardArgs: true,
		Signatures: []NativeSig{{
			Handler: argsHandler,
			Returns: []*Type{TList},
		}},
	},
	{
		Name:        "__pa",
		ForwardArgs: true,
		Signatures: []NativeSig{{
			Handler: popArgsHandler,
			Returns: []*Type{},
		}},
	},
}

// ---- def ----

func defHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	name := defName(args[0])
	stackOnly := defStackOnly(args[0])
	body := args[1]
	if IsCapitalisedName(name) {
		// `def` is the universal binder (lang/doc/design/TYPE-UNIFORM.0.md
		// Phase 2): a capitalised name is a TYPE binding. Delegate to
		// the kernel type installer — the same path the `type` word
		// uses — so object/predicate lattice-minting and all
		// type-installation validation happen in exactly one place.
		return nil, eng.InstallType(r, name, body)
	}
	if err := ValidateWordName(name); err != nil {
		return nil, fmt.Errorf("def %s: %w", name, err)
	}
	if r.Defs.IsType(name) {
		return nil, r.AqlError("def_error", fmt.Sprintf("def %s: name clash — already a type", name), "def")
	}
	InstallDef(r, name, body, stackOnly)
	r.Check.RecordDef(name, args[0].Pos)
	return nil, nil
}

func defTypedHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	nameMap, _ := AsMap(args[0])
	if nameMap == nil || nameMap.Len() == 0 {
		return nil, r.AqlError("def_error", "def: typed-name map must have exactly one key, got empty/non-concrete map", "def")
	}
	if nameMap.Len() != 1 {
		return nil, fmt.Errorf("def: typed-name map must have exactly one key, got %d", nameMap.Len())
	}
	name := nameMap.Keys()[0]
	if IsCapitalisedName(name) {
		return nil, r.AqlError("def_error", fmt.Sprintf("def %s: def names must not start with a capital letter (capitalised names are reserved for types)", name), "def")
	}
	if err := ValidateWordName(name); err != nil {
		return nil, fmt.Errorf("def %s: %w", name, err)
	}
	if r.Defs.IsType(name) {
		return nil, r.AqlError("def_error", fmt.Sprintf("def %s: name clash — already a type", name), "def")
	}
	constraint, _ := nameMap.Get(name)
	var typeName string
	constraint, typeName, _ = r.ResolveTypedNameValue(constraint)
	if !IsTypeBody(constraint) {
		return nil, fmt.Errorf("def %s: type annotation must be a type value, got %s", name, constraint.String())
	}
	describeType := func() string {
		if typeName != "" {
			return typeName
		}
		return constraint.String()
	}
	body := args[1]
	if constraint.VType.Equal(TFnUndef) && IsAtom(body) {
		atomName, _ := AsAtom(body)
		if top, ok := r.Defs.Top(atomName); ok {
			if top.VType.Equal(TFnDef) || top.VType.Equal(TFunction) {
				body = top
			}
		}
	}
	if constraint.VType.Equal(TFnDef) || constraint.VType.Equal(TFunction) {
		out, matched, err := r.RunPredicate(constraint, body)
		if err != nil {
			return nil, fmt.Errorf("def %s: predicate type %s: %w", name, describeType(), err)
		}
		if !matched {
			return nil, fmt.Errorf("def %s: value %s does not satisfy predicate type %s",
				name, body.String(), describeType())
		}
		// Rewrap with the predicate's *Type so dispatch keys off
		// the nominal name. The underlying Data is unchanged —
		// accessors (AsInteger, AsString, …) read the payload the
		// same way — but the VType change lets the LCA walk find
		// behaviors installed via `behave compare/q (fn
		// [[Positive Positive] …])` etc.
		//
		// Only fires when the predicate declares a concrete input
		// type (e.g. `fn [n:Integer …]`). Predicates with `Any`
		// input — the historical `fn [x:Any Any […]]` shape — are
		// pure validation gates: their *Type is parented at
		// TFnDef and rewrapping would break rendering and
		// downstream type tests (the value would print as
		// `Type/Function/Bbd({…})` rather than its underlying
		// scalar). The PredicateInputType check below mirrors the
		// InstallType decision so the two paths stay aligned.
		if typeName != "" && eng.PredicateInputType(constraint) != nil {
			if def := r.LookupTypeName(typeName); def != nil && def.Origin != eng.OriginBuiltin {
				out.VType = def
			}
		}
		InstallDef(r, name, out)
		r.Check.RecordDef(name, args[0].Pos)
		return nil, nil
	}

	// ObjectType constraint (`def x:Person {map}` where Person is
	// `type Person object {…}`): build a Person-typed ObjectInstance
	// from the body map via make-style construction. This closes the
	// "structural for validation, nominal for dispatch" gap for
	// object types — without this branch the value would have
	// VType=TMap and Person's registered behaviors would never
	// dispatch. The result carries VType=Person, satisfies the
	// `behave compare/q (fn [[Person Person] …])` dispatch path, and
	// supports `get`/`set` via the ObjectInstance signatures.
	//
	// Accepts both a raw Map (built via make) and an already-typed
	// ObjectInstance (passed through). Other body shapes fall
	// through to Unify and either succeed or surface a type error.
	if IsObjectType(constraint) {
		info, _ := AsObjectType(constraint)
		if body.VType.Equal(TMap) {
			result, err := eng.MakeObject(info, body, nil)
			if err != nil {
				return nil, fmt.Errorf("def %s: %w", name, err)
			}
			InstallDef(r, name, result[0])
			r.Check.RecordDef(name, args[0].Pos)
			return nil, nil
		}
		if IsObjectInstance(body) {
			oi, _ := AsObjectInstance(body)
			// Accept if the instance's nominal type matches the
			// declared one (covers `def x:Person make Person {…}`).
			if oi.TypeRef != nil && oi.TypeRef.ID == info.ID {
				InstallDef(r, name, body)
				r.Check.RecordDef(name, args[0].Pos)
				return nil, nil
			}
		}
	}
	if r.Check.IsActive() && constraint.IsDepScalar() {
		leaf := DependentLeafFromType(constraint.VType)
		if base, ok := DependentLeafBaseType(leaf); ok && body.VType.Matches(base) {
			InstallDef(r, name, body)
			r.Check.RecordDef(name, args[0].Pos)
			return nil, nil
		}
	}
	unified, ok := Unify(body, constraint)
	if !ok {
		if r.Check.IsActive() {
			r.Check.AddDiagnostic(CheckDiagnostic{
				Code: "type_error",
				Detail: fmt.Sprintf("def %s: value %s does not unify with declared type %s",
					name, body.String(), describeType()),
				Word: name,
				Row:  args[0].Pos.Row,
				Col:  args[0].Pos.Col,
			})
			InstallDef(r, name, NewCarrier(constraint.VType))
			r.Check.RecordDef(name, args[0].Pos)
			return nil, nil
		}
		return nil, fmt.Errorf("def %s: value %s does not unify with declared type %s",
			name, body.String(), describeType())
	}
	// FnUndef constraint (`def f:Mapper fn […]`): after Unify
	// confirms the function shape matches Mapper, rewrap the
	// VType so dispatch keys off Mapper rather than the generic
	// TFunction / TFnDef. Behaviors installed via
	// `behave compare/q (fn [[Mapper Mapper] …])` then dispatch on
	// f. Same rewrap pattern as predicate types — the payload
	// shape (FnDefInfo) is unchanged, accessors keep working, just
	// the dispatch identity flips.
	if constraint.VType.Equal(TFnUndef) && typeName != "" {
		if def := r.LookupTypeName(typeName); def != nil && def.Origin != eng.OriginBuiltin {
			unified.VType = def
		}
	}
	InstallDef(r, name, unified)
	r.Check.RecordDef(name, args[0].Pos)
	return nil, nil
}

// ---- undef ----

func undefHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	name := defName(args[0])
	if IsCapitalisedName(name) {
		// `undef` is the universal unbinder (the symmetric completion
		// of Phase 2's universal `def` — lang/doc/design/TYPE-UNIFORM.0.md):
		// a capitalised name is a TYPE binding, so pop it from the single
		// binding store and retire the minted lattice type.
		entry, ok := r.Defs.PopEntry(name)
		if !ok {
			return nil, r.AqlError("undef_error",
				fmt.Sprintf("undef %s: no such type binding", name), "undef")
		}
		if entry.TypeDef != nil {
			r.Types.Retire(entry.TypeDef)
		}
		return nil, nil
	}
	UninstallDef(r, name)
	return nil, nil
}

func undefFnHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	name := defName(args[0])
	undefInfo, ok := args[1].Data.(FnUndefInfo)
	if !ok {
		return nil, fmt.Errorf("undef: expected fn undef spec, got %s", args[1].String())
	}
	UninstallFnSigs(r, name, undefInfo)
	return nil, nil
}

// ---- var ----

func varHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	list := args[0]
	if !list.VType.Equal(TList) {
		return nil, r.AqlError("var_error", "var: argument must be a list", "var")
	}
	if list.Data == nil {
		return nil, r.AqlError("var_error", "var: argument must be a concrete list, got type literal", "var")
	}
	elems, _ := AsList(list)
	if elems.Len() == 0 {
		return nil, r.AqlError("var_error", "var: empty list", "var")
	}

	declVal := elems.Get(0)
	if !declVal.VType.Equal(TList) || declVal.Data == nil {
		return nil, r.AqlError("var_error", "var: first element must be a list of variable declarations", "var")
	}
	decls, _ := AsList(declVal)
	body := elems.Slice()[1:]

	var result []Value
	var varNames []string

	for _, decl := range decls.Slice() {
		switch {
		case IsWord(decl):
			_as0, _ := AsWord(decl)
			name := _as0.Name
			varNames = append(varNames, name)
			result = append(result, NewWord("def"), NewWord(name), NewEnd())

		case decl.VType.Equal(TList) && decl.Data != nil:
			declElems, _ := AsList(decl)
			if declElems.Len() < 2 {
				return nil, r.AqlError("var_error", "var: declaration list must have name and value", "var")
			}
			var name string
			if IsWord(declElems.Get(0)) {
				_as1, _ := AsWord(declElems.Get(0))
				name = _as1.Name
			} else if declElems.Get(0).VType.Matches(TString) {
				name, _ = AsString(declElems.Get(0))
			} else {
				return nil, r.AqlError("var_error", "var: declaration name must be a word or string", "var")
			}
			varNames = append(varNames, name)
			result = append(result, NewWord("def"), NewWord(name))
			result = append(result, declElems.Slice()[1:]...)
			result = append(result, NewEnd())

		case decl.VType.Matches(TString):
			name, _ := AsString(decl)
			varNames = append(varNames, name)
			result = append(result, NewWord("def"), NewWord(name), NewEnd())

		default:
			return nil, fmt.Errorf("var: invalid declaration: %s", decl.String())
		}
	}

	result = append(result, body...)

	for i := len(varNames) - 1; i >= 0; i-- {
		result = append(result, NewWord("undef"), NewWord(varNames[i]))
	}

	return result, nil
}

// ---- fn ----

// fnHandler always produces a Function value. The list must be a
// non-zero multiple of 3 (input/output/body triples). For the
// type-only / shape form (input/output pairs, no body) use the
// separate `fnsig` word — registered via eng.RegisterCoreFnSig
// from register.go.
func fnHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	list := args[0]
	if !list.VType.Equal(TList) {
		return nil, r.AqlError("fn_error", "fn: argument must be a list", "fn")
	}
	if list.Data == nil {
		return nil, r.AqlError("fn_error", "fn: argument must be a concrete list, got type literal", "fn")
	}
	_lst, _ := AsList(list)
	elems := _lst.Slice()
	if len(elems) == 0 || len(elems)%3 != 0 {
		return nil, r.AqlError("fn_error", "fn: list length must be a non-zero multiple of 3 (input output body triples); use `fnsig` for the type-only form", "fn")
	}
	fnDef, err := parseFnDef(r, elems)
	if err != nil {
		return nil, err
	}
	return []Value{NewFunction(fnDef)}, nil
}

// fnsigHandler — `fnsig [input output …]` produces a function-SHAPE
// type literal (FnUndef) from input/output sig pairs. The type-only
// counterpart to `fn` — same grammar, no body. The list length must
// be a non-zero multiple of 2 (each pair is one signature). The
// result is an FnUndef value usable as a type constraint, e.g.
// `def f:fnsig [[Integer] [String]] impl` asserts that `impl` is a
// function whose signatures cover the shape `Integer → String`.
//
// FnUndef is structural: any function value whose registered
// signatures satisfy every pair in the FnUndef matches. See
// eng/go/fnsig.go::FnUndefMatchesFnDef.
func fnsigHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	if args[0].Data == nil {
		return nil, &AqlError{
			Code:   "fnsig_invalid_spec",
			Detail: "fnsig: argument must be a concrete list",
		}
	}
	_lst, _ := AsList(args[0])
	spec := _lst.Slice()
	if len(spec) == 0 || len(spec)%2 != 0 {
		return nil, &AqlError{
			Code:   "fnsig_invalid_spec",
			Detail: "fnsig: list length must be a non-zero multiple of 2 (input output pairs); use `fn` for the with-body form",
		}
	}
	info, err := parseFnUndefSpec(r, spec)
	if err != nil {
		return nil, err
	}
	return []Value{NewFnUndef(info)}, nil
}

// ---- call ----

func callHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	body := args[0]

	if body.Data == nil {
		return nil, r.AqlError("call_error", "call: argument must be a concrete list, got type literal", "call")
	}
	if IsTypedList(body) || IsTableType(body) {
		return nil, r.AqlError("call_error", "call: argument must be a plain list", "call")
	}

	bodyElems, _ := AsList(body)
	if bodyElems.Len() == 0 {
		return nil, nil
	}

	bodyCopy := bodyElems.Slice()
	return bodyCopy, nil
}

// ---- dblcall ----

func dblcallHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	n, _ := args[0].AsConcreteInteger()
	body := args[1]

	if body.Data == nil {
		return nil, r.AqlError("dblcall_error", "dblcall: callback must be a concrete list, got type literal", "dblcall")
	}
	if IsTypedList(body) || IsTableType(body) {
		return nil, r.AqlError("dblcall_error", "dblcall: callback must be a plain list", "dblcall")
	}

	doubled := NewInteger(n * 2)

	bodyElems, _ := AsList(body)
	if bodyElems.Len() == 0 {
		return []Value{doubled}, nil
	}

	tokens := make([]Value, 0, bodyElems.Len()+3)
	tokens = append(tokens, NewOpenParen())
	tokens = append(tokens, doubled)
	bodyCopy := bodyElems.Slice()
	tokens = append(tokens, bodyCopy...)
	tokens = append(tokens, NewCloseParen())
	return tokens, nil
}

// ---- args / __pa ----

func argsHandler(_ []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	top, ok, err := r.Args.Top()
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, r.AqlError("args_error", "args: not inside a function", "args")
	}
	return []Value{top}, nil
}

func popArgsHandler(_ []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	if _, err := r.Args.Pop(); err != nil {
		return nil, err
	}
	return nil, nil
}
