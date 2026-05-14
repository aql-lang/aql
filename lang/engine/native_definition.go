package engine

import "fmt"

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
		return nil, fmt.Errorf("def %s: def names must not start with a capital letter (capitalised names are reserved for types)", name)
	}
	if err := ValidateWordName(name); err != nil {
		return nil, fmt.Errorf("def %s: %w", name, err)
	}
	if r.Types.Has(name) {
		return nil, fmt.Errorf("def %s: name clash — already a type", name)
	}
	InstallDef(r, name, body, stackOnly)
	r.Check.RecordDef(name, args[0].Pos)
	return nil, nil
}

func defTypedHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	nameMap := args[0].AsMap()
	if nameMap == nil || nameMap.Len() == 0 {
		return nil, fmt.Errorf("def: typed-name map must have exactly one key, got empty/non-concrete map")
	}
	if nameMap.Len() != 1 {
		return nil, fmt.Errorf("def: typed-name map must have exactly one key, got %d", nameMap.Len())
	}
	name := nameMap.Keys()[0]
	if IsCapitalisedName(name) {
		return nil, fmt.Errorf("def %s: def names must not start with a capital letter (capitalised names are reserved for types)", name)
	}
	if err := ValidateWordName(name); err != nil {
		return nil, fmt.Errorf("def %s: %w", name, err)
	}
	if r.Types.Has(name) {
		return nil, fmt.Errorf("def %s: name clash — already a type", name)
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
	if constraint.VType.Equal(TFnUndef) && body.IsAtom() {
		atomName, _ := body.AsAtom()
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
		InstallDef(r, name, out)
		r.Check.RecordDef(name, args[0].Pos)
		return nil, nil
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
	InstallDef(r, name, unified)
	r.Check.RecordDef(name, args[0].Pos)
	return nil, nil
}

// ---- undef ----

func undefHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	name := defName(args[0])
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

func varHandler(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
	list := args[0]
	if !list.VType.Equal(TList) {
		return nil, fmt.Errorf("var: argument must be a list")
	}
	if list.Data == nil {
		return nil, fmt.Errorf("var: argument must be a concrete list, got type literal")
	}
	elems := list.AsList()
	if elems.Len() == 0 {
		return nil, fmt.Errorf("var: empty list")
	}

	declVal := elems.Get(0)
	if !declVal.VType.Equal(TList) || declVal.Data == nil {
		return nil, fmt.Errorf("var: first element must be a list of variable declarations")
	}
	decls := declVal.AsList()
	body := elems.Slice()[1:]

	var result []Value
	var varNames []string

	for _, decl := range decls.Slice() {
		switch {
		case decl.IsWord():
			_as0, _ := decl.AsWord()
			name := _as0.Name
			varNames = append(varNames, name)
			result = append(result, NewWord("def"), NewWord(name), NewEnd())

		case decl.VType.Equal(TList) && decl.Data != nil:
			declElems := decl.AsList()
			if declElems.Len() < 2 {
				return nil, fmt.Errorf("var: declaration list must have name and value")
			}
			var name string
			if declElems.Get(0).IsWord() {
				_as1, _ := declElems.Get(0).AsWord()
				name = _as1.Name
			} else if declElems.Get(0).VType.Matches(TString) {
				name, _ = declElems.Get(0).AsString()
			} else {
				return nil, fmt.Errorf("var: declaration name must be a word or string")
			}
			varNames = append(varNames, name)
			result = append(result, NewWord("def"), NewWord(name))
			result = append(result, declElems.Slice()[1:]...)
			result = append(result, NewEnd())

		case decl.VType.Matches(TString):
			name, _ := decl.AsString()
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
		return nil, fmt.Errorf("fn: argument must be a list")
	}
	if list.Data == nil {
		return nil, fmt.Errorf("fn: argument must be a concrete list, got type literal")
	}
	elems := list.AsList().Slice()
	if len(elems) == 0 || len(elems)%3 != 0 {
		return nil, fmt.Errorf("fn: list length must be a non-zero multiple of 3 (input output body triples); use `fnsig` for the type-only form")
	}
	fnDef, err := parseFnDef(r, elems)
	if err != nil {
		return nil, err
	}
	return []Value{NewFunction(fnDef)}, nil
}

// ---- call ----

func callHandler(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
	body := args[0]

	if body.Data == nil {
		return nil, fmt.Errorf("call: argument must be a concrete list, got type literal")
	}
	if body.IsTypedList() || body.IsTableType() {
		return nil, fmt.Errorf("call: argument must be a plain list")
	}

	bodyElems := body.AsList()
	if bodyElems.Len() == 0 {
		return nil, nil
	}

	bodyCopy := bodyElems.Slice()
	return bodyCopy, nil
}

// ---- dblcall ----

func dblcallHandler(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
	n, _ := args[0].AsConcreteInteger()
	body := args[1]

	if body.Data == nil {
		return nil, fmt.Errorf("dblcall: callback must be a concrete list, got type literal")
	}
	if body.IsTypedList() || body.IsTableType() {
		return nil, fmt.Errorf("dblcall: callback must be a plain list")
	}

	doubled := NewInteger(n * 2)

	bodyElems := body.AsList()
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
	top, ok := r.Args.Top()
	if !ok {
		return nil, fmt.Errorf("args: not inside a function")
	}
	return []Value{top}, nil
}

func popArgsHandler(_ []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	r.Args.Pop()
	return nil, nil
}
