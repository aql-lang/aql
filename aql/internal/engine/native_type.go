package engine

import (
	"fmt"
	"strconv"
	"strings"
)

// typeNatives covers the type-system words: record, table, object,
// make, type, untype, typeof, fulltypeof, is, guard, inspect, base,
// tor, tand, any, all, tany, tall, convert.
//
// `Resource` and `Entity` (the builtin object types) are NOT installed
// via NativeFunc — they are user-typed values pushed onto the type
// stack. `installResourceTypes` handles those during engine.Register.
var typeNatives = []NativeFunc{
	{
		Name:              "record",
		ForwardPrecedence: true,
		Signatures: []NativeSig{{
			Args:           []Type{TList},
			Handler:        recordHandler,
			Returns:        []Type{TRecord},
			RunInCheckMode: true,
		}},
	},
	{
		Name:              "table",
		ForwardPrecedence: true,
		Signatures: []NativeSig{{
			Args:           []Type{TAny},
			Handler:        tableHandler,
			Returns:        []Type{TTable},
			RunInCheckMode: true,
		}},
	},
	{
		Name:              "object",
		ForwardPrecedence: true,
		Signatures: []NativeSig{
			{
				Args:           []Type{TMap, TObject},
				Handler:        objectWithParentHandler,
				Returns:        []Type{TObjectType},
				RunInCheckMode: true,
			},
			{
				Args:           []Type{TMap},
				Handler:        objectHandler,
				Returns:        []Type{TObjectType},
				RunInCheckMode: true,
			},
		},
	},
	{
		Name:              "make",
		ForwardPrecedence: true,
		Signatures: []NativeSig{
			{Args: []Type{TScalarType, TMap, TAny}, Handler: makeScalarOptsHandler, ReturnsFn: ReturnsIdentity(0)},
			{Args: []Type{TObjectType, TMap}, Handler: makeObjHandler, ReturnsFn: ReturnsIdentity(0)},
			{Args: []Type{TArray, TList}, Handler: makeArrayHandler, Returns: []Type{TArray}},
			{Args: []Type{TScalarType, TAny}, Handler: makeScalarHandler, ReturnsFn: ReturnsIdentity(0)},
			{Args: []Type{TObject, TAny, TObject}, Handler: makeWithPrototype, Returns: []Type{TObject}},
			{Args: []Type{TAny, TAny, TMap}, Handler: makeWithOpts, Returns: []Type{TAny}},
			{Args: []Type{TAny, TAny}, Handler: makeHandler, Returns: []Type{TAny}},
		},
	},
	{
		Name:              "type",
		ForwardPrecedence: true,
		Signatures: []NativeSig{
			{
				Args:           []Type{TString, TAny},
				Handler:        typeHandler,
				Returns:        []Type{},
				RunInCheckMode: true,
			},
			{
				Args:           []Type{TAtom, TAny},
				QuoteArgs:      map[int]bool{0: true},
				Handler:        typeHandler,
				Returns:        []Type{},
				RunInCheckMode: true,
			},
		},
	},
	{
		Name:              "untype",
		ForwardPrecedence: true,
		Signatures: []NativeSig{
			{
				Args:           []Type{TString},
				Handler:        untypeHandler,
				Returns:        []Type{},
				RunInCheckMode: true,
			},
			{
				Args:           []Type{TAtom},
				QuoteArgs:      map[int]bool{0: true},
				Handler:        untypeHandler,
				Returns:        []Type{},
				RunInCheckMode: true,
			},
		},
	},
	{
		Name:              "typeof",
		ForwardPrecedence: true,
		Signatures: []NativeSig{{
			Args:    []Type{TAny},
			Handler: typeofHandler,
			Returns: []Type{TAtom},
		}},
	},
	{
		Name:              "fulltypeof",
		ForwardPrecedence: true,
		Signatures: []NativeSig{{
			Args:    []Type{TAny},
			Handler: fulltypeofHandler,
			Returns: []Type{TAtom},
		}},
	},
	{
		Name:              "is",
		ForwardPrecedence: true,
		Signatures: []NativeSig{{
			Args:       []Type{TAny, TAny},
			BarrierPos: 1,
			Handler:    isHandler,
			Returns:    []Type{TBoolean},
		}},
	},
	{
		Name:              "guard",
		ForwardPrecedence: true,
		Signatures: []NativeSig{{
			Args:       []Type{TAny, TBoolean},
			BarrierPos: 1,
			Handler:    guardHandler,
			Returns:    []Type{TAny},
		}},
	},
	{
		Name:              "inspect",
		ForwardPrecedence: true,
		Signatures: []NativeSig{
			{Args: []Type{TWord}, Handler: inspectWordHandler, Returns: []Type{TInspect}},
			{Args: []Type{TAtom}, Handler: inspectAtomHandler, Returns: []Type{TInspect}},
			{Args: []Type{TNode}, Handler: inspectTypeHandler, Returns: []Type{TInspect}},
			{Args: []Type{TScalar}, Handler: inspectTypeHandler, Returns: []Type{TInspect}},
		},
	},
	{
		Name:              "base",
		ForwardPrecedence: true,
		Signatures: []NativeSig{{
			Args:      []Type{TAny},
			Handler:   baseHandler,
			ReturnsFn: ReturnsIdentity(0),
		}},
	},
	// `tor` and `tand` moved to aqleng/go/core_boolean.go; installed
	// via aqleng.RegisterCoreTypeOps from register.go.
	{
		Name:              "any",
		ForwardPrecedence: true,
		Signatures: []NativeSig{
			{Args: []Type{TList}, Handler: anyHandler, Returns: []Type{TAny}},
		},
	},
	{
		Name:              "all",
		ForwardPrecedence: true,
		Signatures: []NativeSig{
			{Args: []Type{TList}, Handler: allHandler, Returns: []Type{TAny}},
		},
	},
	{
		Name:              "tany",
		ForwardPrecedence: true,
		Signatures: []NativeSig{
			{Args: []Type{TList}, Handler: tanyHandler, Returns: []Type{TAny}},
		},
	},
	{
		Name:              "tall",
		ForwardPrecedence: true,
		Signatures: []NativeSig{
			{Args: []Type{TList}, Handler: tallHandler, Returns: []Type{TAny}},
		},
	},
	{
		Name:              "convert",
		ForwardPrecedence: true,
		Signatures: []NativeSig{
			{
				Args:      []Type{TScalarType, TMap, TScalar},
				Patterns:  map[int]Value{1: convertOptsPattern()},
				Handler:   convert3Handler,
				ReturnsFn: ReturnsIdentity(0),
			},
			{
				Args:      []Type{TScalarType, TScalar},
				Handler:   convert2Handler,
				ReturnsFn: ReturnsIdentity(0),
			},
		},
	},
}

// installResourceTypes pushes the builtin Resource and Entity object
// types onto the type stack. Called once during engine.Register.
//
//   - Object/Resource has field kind:String
//   - Object/Resource/Entity inherits kind from Resource and adds
//     spec:String, entity:String
//
// These are registered via InstallDef so they get proper handler
// resolution and can be referenced by name in AQL code (e.g. make
// Entity {...}).
func installResourceTypes(r *Registry) {
	resourceFields := NewOrderedMap()
	resourceFields.Set("kind", NewTypeLiteral(TString))

	resourceInfo := ObjectTypeInfo{
		Fields: resourceFields,
		Parent: nil,
		ID:     FormatFixedTypeID("Object/Resource", BuiltinTypeIDs["Object/Resource"]),
	}

	InstallDef(r, "Resource", NewObjectType(resourceInfo))

	resourceVal, _ := r.TopOfDefStack("Resource")
	installedResource, _ := resourceVal.AsObjectType()

	entityFields := NewOrderedMap()
	entityFields.Set("spec", NewTypeLiteral(TString))
	entityFields.Set("entity", NewTypeLiteral(TString))

	entityInfo := ObjectTypeInfo{
		Fields: entityFields,
		Parent: &installedResource,
		ID:     FormatFixedTypeID("Object/Resource/Entity", BuiltinTypeIDs["Object/Resource/Entity"]),
	}

	InstallDef(r, "Entity", NewObjectType(entityInfo))
}

// ---- record ----

func recordHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	list := args[0]
	if !list.VType.Equal(TList) {
		return nil, fmt.Errorf("record: argument must be a list")
	}
	if list.Data == nil {
		return nil, fmt.Errorf("record: argument must be a concrete list, got type literal")
	}
	elems := list.AsList()
	if elems.Len() == 0 {
		return nil, fmt.Errorf("record: list must have at least one field")
	}
	fields := NewOrderedMap()
	for _, elem := range elems.Slice() {
		if !elem.VType.Equal(TMap) {
			return nil, fmt.Errorf("record: each element must be a pair (map), got %s", elem.String())
		}
		m, ok := elem.Data.(*OrderedMap)
		if !ok {
			return nil, fmt.Errorf("record: each element must be a concrete pair, got %s", elem.String())
		}
		for _, key := range m.Keys() {
			val, _ := m.Get(key)
			val = ResolveFieldType(r, val)
			fields.Set(key, val)
		}
	}
	return []Value{NewRecordType(fields)}, nil
}

// ---- table ----

func tableHandler(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
	target := args[0]
	if !target.IsRecordType() {
		return nil, fmt.Errorf("table: argument must be a record type, got %s", target.String())
	}
	_as0, _ := target.AsRecordType()
	return []Value{NewTableType(_as0)}, nil
}

// ---- object ----

// parseObjectFields converts a map of field definitions into an
// OrderedMap of field name → type-constraint Value, resolving type
// references via r.
func parseObjectFields(fieldsMap *OrderedMap, r *Registry) *OrderedMap {
	fields := NewOrderedMap()
	for _, key := range fieldsMap.Keys() {
		val, _ := fieldsMap.Get(key)
		val = ResolveFieldType(r, val)
		fields.Set(key, val)
	}
	return fields
}

func objectHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	fieldsVal := args[0]
	if !fieldsVal.VType.Equal(TMap) {
		return nil, fmt.Errorf("object: argument must be a map of field definitions, got %s", fieldsVal.String())
	}
	m, ok := fieldsVal.Data.(*OrderedMap)
	if !ok {
		return nil, fmt.Errorf("object: argument must be a concrete map, got %s", fieldsVal.String())
	}
	fields := parseObjectFields(m, r)
	id := GenerateObjectTypeID()
	info := ObjectTypeInfo{
		Fields: fields,
		Parent: nil,
		ID:     id,
		Name:   "",
	}
	return []Value{NewObjectType(info)}, nil
}

func objectWithParentHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	fieldsVal := args[0]
	parentVal := args[1]

	if !fieldsVal.VType.Equal(TMap) {
		return nil, fmt.Errorf("object: first argument must be a map of field definitions, got %s", fieldsVal.String())
	}
	m, ok := fieldsVal.Data.(*OrderedMap)
	if !ok {
		return nil, fmt.Errorf("object: first argument must be a concrete map, got %s", fieldsVal.String())
	}

	if !parentVal.IsObjectType() {
		return nil, fmt.Errorf("object: parent must be an object type, got %s", parentVal.String())
	}
	parentInfo, _ := parentVal.AsObjectType()

	fields := parseObjectFields(m, r)

	parentAllFields := parentInfo.AllFields()
	for _, key := range fields.Keys() {
		childConstraint, _ := fields.Get(key)
		parentConstraint, exists := parentAllFields.Get(key)
		if !exists {
			continue
		}
		_, ok := Unify(parentConstraint, childConstraint)
		if !ok {
			return nil, fmt.Errorf("object: field %q in child type cannot expand parent type %s (child: %s, parent: %s)",
				key, parentInfo.Name, childConstraint.String(), parentConstraint.String())
		}
	}

	id := GenerateObjectTypeID()
	info := ObjectTypeInfo{
		Fields: fields,
		Parent: &parentInfo,
		ID:     id,
		Name:   "",
	}
	return []Value{NewObjectType(info)}, nil
}

// ---- type / untype ----

// validateAndInstallType validates a type-name/body pair and pushes
// the body onto r's type stack. Used by typeHandler.
func validateAndInstallType(r *Registry, name string, body Value) error {
	if !IsTypeBody(body) {
		return fmt.Errorf("type: body must be a type value (record, disjunct, type literal, typed list, or typed map), got %s", body.String())
	}
	if !IsCapitalisedName(name) {
		return fmt.Errorf("type %s: type names must start with a capital letter", name)
	}
	if !r.HasType(name) {
		if err := ValidateTypeNameParts(name, r.KnownTypeParts); err != nil {
			return err
		}
	}
	if r.Lookup(name) != nil {
		return fmt.Errorf("type %s: name clash — already a registered function", name)
	}
	if r.HasDef(name) {
		return fmt.Errorf("type %s: name clash — already a def'd value", name)
	}
	if body.IsObjectType() {
		info, _ := body.AsObjectType()
		if info.Parent != nil {
			info.Name = info.Parent.Name + "/" + name
		} else {
			info.Name = "Object/" + name
		}
		for _, p := range strings.Split(info.Name, "/") {
			r.KnownTypeParts[p] = true
		}
		body = NewObjectType(info)
	}
	r.PushType(name, body)
	for _, p := range strings.Split(name, "/") {
		r.KnownTypeParts[p] = true
	}
	return nil
}

func typeHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	name := defName(args[0])
	body := args[1]
	if err := validateAndInstallType(r, name, body); err != nil {
		return nil, err
	}
	return nil, nil
}

func untypeHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	name := defName(args[0])
	if !IsCapitalisedName(name) {
		return nil, fmt.Errorf("untype %s: type names must start with a capital letter", name)
	}
	if !r.PopType(name) {
		return nil, fmt.Errorf("untype %s: no such type binding", name)
	}
	return nil, nil
}

// ---- typeof / fulltypeof ----

func typeofHandler(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
	v := args[0]
	parts := v.VType.Parts
	if v.Data == nil && !v.VType.Matches(TWord) {
		parts = MetatypeFor(v.VType).Parts
	}
	if len(parts) > 0 {
		last := parts[len(parts)-1]
		if len(last) > 0 && last[0] >= '0' && last[0] <= '9' {
			parts = parts[:len(parts)-1]
		}
		if len(last) > 1 && last[0] == '-' && last[1] >= '0' && last[1] <= '9' {
			parts = parts[:len(parts)-1]
		}
	}
	result := parts[0]
	if len(parts) > 1 {
		result = parts[1]
	}
	return []Value{NewAtom(result)}, nil
}

func fulltypeofHandler(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
	v := args[0]
	parts := v.VType.Parts
	if v.Data == nil && !v.VType.Matches(TWord) {
		parts = MetatypeFor(v.VType).Parts
	}
	if len(parts) > 0 {
		last := parts[len(parts)-1]
		if len(last) > 0 && last[0] >= '0' && last[0] <= '9' {
			parts = parts[:len(parts)-1]
		}
		if len(last) > 1 && last[0] == '-' && last[1] >= '0' && last[1] <= '9' {
			parts = parts[:len(parts)-1]
		}
	}
	return []Value{NewAtom(strings.Join(parts, "/"))}, nil
}

// ---- is ----

func isHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	a, b := args[1], args[0]
	if b.VType.Equal(TFnUndef) && a.IsAtom() {
		name, _ := a.AsAtom()
		if top, ok := r.TopOfDefStack(name); ok {
			if top.VType.Equal(TFnDef) || top.VType.Equal(TFunction) {
				a = top
			}
		}
	}
	if b.VType.Equal(TFnDef) || b.VType.Equal(TFunction) {
		_, matched, err := r.RunPredicate(b, a)
		if err != nil {
			return []Value{NewBoolean(false)}, nil
		}
		return []Value{NewBoolean(matched)}, nil
	}
	if b.Data == nil && IsMetaType(b.VType) && a.Data == nil {
		aMeta := MetatypeFor(a.VType)
		return []Value{NewBoolean(aMeta.Matches(b.VType))}, nil
	}
	unified, ok := Unify(a, b)
	if !ok {
		return []Value{NewBoolean(false)}, nil
	}
	resolved := ResolveWordsDeep(a)
	if !unified.VType.Equal(resolved.VType) {
		return []Value{NewBoolean(false)}, nil
	}
	if !ValuesEqual(unified, resolved) {
		return []Value{NewBoolean(false)}, nil
	}
	return []Value{NewBoolean(true)}, nil
}

// ---- guard ----

func guardHandler(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
	val := args[0]
	cond, err := args[1].AsConcreteBoolean()
	if err != nil {
		return nil, fmt.Errorf("guard: condition must be Boolean, got %s", args[1].VType.String())
	}
	if cond {
		return []Value{val}, nil
	}
	return []Value{NewTypeLiteral(TNone)}, nil
}

// ---- inspect ----

func inspectWordHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	_as0, _ := args[0].AsWord()
	name := _as0.Name

	if tv, ok := r.TopOfTypeStack(name); ok {
		return []Value{buildTypeInspection(name, tv)}, nil
	}
	if top, ok := r.TopOfDefStack(name); ok {
		if IsTypeBody(top) && !top.VType.Equal(TFnDef) && !top.VType.Equal(TFunction) {
			return []Value{buildTypeInspection(name, top)}, nil
		}
	}

	return []Value{buildInspection(r, name)}, nil
}

func inspectAtomHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	name, _ := args[0].AsConcreteAtom()
	if tv, ok := r.TopOfTypeStack(name); ok {
		return []Value{buildTypeInspection(name, tv)}, nil
	}
	if top, ok := r.TopOfDefStack(name); ok {
		if IsTypeBody(top) {
			return []Value{buildTypeInspection(name, top)}, nil
		}
	}
	return []Value{buildInspection(r, name)}, nil
}

func inspectTypeHandler(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
	return []Value{buildTypeInspection("", args[0])}, nil
}

// ---- base ----

func baseHandler(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
	v := args[0]
	t := v.VType
	result, err := BaseValue(t)
	if err != nil {
		return nil, err
	}
	return []Value{result}, nil
}

// torHandler, torReturnsFn, tandHandler: moved to aqleng/go/core_boolean.go.
// tand's `tall` reduction (below) calls aqleng.TandValues directly.

// ---- any / all / tany / tall ----

func anyHandler(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
	if !IsConcrete(args[0]) {
		return []Value{NewBoolean(false)}, nil
	}
	list := args[0].AsList()
	n := list.Len()
	if n == 0 {
		return []Value{NewBoolean(false)}, nil
	}
	var last Value
	for i := 0; i < n; i++ {
		v := list.Get(i)
		if CoerceBoolean(v) {
			return []Value{v}, nil
		}
		last = v
	}
	return []Value{last}, nil
}

func allHandler(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
	if !IsConcrete(args[0]) {
		return []Value{NewBoolean(true)}, nil
	}
	list := args[0].AsList()
	n := list.Len()
	if n == 0 {
		return []Value{NewBoolean(true)}, nil
	}
	var last Value
	for i := 0; i < n; i++ {
		v := list.Get(i)
		if !CoerceBoolean(v) {
			return []Value{v}, nil
		}
		last = v
	}
	return []Value{last}, nil
}

func tanyHandler(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
	if !IsConcrete(args[0]) {
		return nil, fmt.Errorf("tany: expected a concrete list")
	}
	list := args[0].AsList()
	n := list.Len()
	if n == 0 {
		return []Value{NewTypeLiteral(TNever)}, nil
	}
	if n == 1 {
		return []Value{list.Get(0)}, nil
	}
	var alts []Value
	for i := 0; i < n; i++ {
		alts = append(alts, FlattenDisjunctAlts(list.Get(i))...)
	}
	simplified := SimplifyDisjunctAlts(alts)
	if len(simplified) == 0 {
		return []Value{NewTypeLiteral(TNever)}, nil
	}
	if len(simplified) == 1 {
		return []Value{simplified[0]}, nil
	}
	return []Value{NewDisjunct(simplified)}, nil
}

func tallHandler(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
	if !IsConcrete(args[0]) {
		return nil, fmt.Errorf("tall: expected a concrete list")
	}
	list := args[0].AsList()
	n := list.Len()
	if n == 0 {
		return []Value{NewTypeLiteral(TAny)}, nil
	}
	acc := list.Get(0)
	for i := 1; i < n; i++ {
		acc = TandValues(acc, list.Get(i))
	}
	return []Value{acc}, nil
}

// ---- convert ----

// convertOptsPattern returns the Options pattern for the 3-arg
// `convert` variant: {base?: String|None}.
func convertOptsPattern() Value {
	baseOpts := NewOrderedMap()
	baseOpts.Set("base", NewDisjunct([]Value{NewTypeLiteral(TString), NewTypeLiteral(TNone)}))
	return NewOptionsType(baseOpts)
}

// convertTo performs the actual scalar-type conversion.
func convertTo(src Value, targetType Type, base string) (Value, error) {
	switch {
	case targetType.Matches(TString):
		if base == "" {
			return NewString(ValToString(src)), nil
		}
		if !src.VType.Matches(TInteger) {
			return Value{}, fmt.Errorf("convert: base %q only supported for integer to string", base)
		}
		n, _ := src.AsInteger()
		var s string
		switch base {
		case "hex":
			s = strconv.FormatInt(n, 16)
		case "HEX":
			s = strings.ToUpper(strconv.FormatInt(n, 16))
		case "bin":
			s = strconv.FormatInt(n, 2)
		case "oct":
			s = strconv.FormatInt(n, 8)
		default:
			return Value{}, fmt.Errorf("convert: unknown base %q", base)
		}
		return NewString(s), nil

	case targetType.Matches(TDecimal):
		text := ValToString(src)
		f, err := strconv.ParseFloat(text, 64)
		if err != nil {
			return Value{}, fmt.Errorf("convert: cannot convert %q to decimal", text)
		}
		return NewDecimal(f), nil

	case targetType.Matches(TNumber) || targetType.Matches(TInteger):
		text := ValToString(src)
		if base == "" {
			n, err := strconv.ParseInt(text, 10, 64)
			if err != nil {
				return Value{}, fmt.Errorf("convert: cannot convert %q to number", text)
			}
			return NewInteger(n), nil
		}
		var numBase int
		switch base {
		case "hex":
			numBase = 16
		case "bin":
			numBase = 2
		case "oct":
			numBase = 8
		default:
			return Value{}, fmt.Errorf("convert: unknown base %q", base)
		}
		n, err := strconv.ParseInt(text, numBase, 64)
		if err != nil {
			return Value{}, fmt.Errorf("convert: cannot convert %q to number (base %d)", text, numBase)
		}
		return NewInteger(n), nil

	case targetType.Matches(TBoolean):
		return NewBoolean(CoerceBoolean(src)), nil

	case targetType.Equal(TAtom):
		return NewAtom(ValToString(src)), nil

	default:
		return Value{}, fmt.Errorf("convert: unsupported target type %s", targetType)
	}
}

func convert2Handler(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
	targetType := args[0]
	src := args[1]
	if targetType.Data != nil {
		return nil, fmt.Errorf("convert: first argument must be a type literal, got %s", targetType.VType)
	}
	result, err := convertTo(src, targetType.VType, "")
	if err != nil {
		return nil, err
	}
	return []Value{result}, nil
}

func convert3Handler(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
	targetType := args[0]
	opts := args[1]
	src := args[2]
	if targetType.Data != nil {
		return nil, fmt.Errorf("convert: first argument must be a type literal, got %s", targetType.VType)
	}

	base := ""
	if opts.Data != nil {
		m := opts.AsMap()
		if m != nil {
			if bv, ok := m.Get("base"); ok {
				base = ValToString(bv)
			}
		}
	}

	result, err := convertTo(src, targetType.VType, base)
	if err != nil {
		return nil, err
	}
	return []Value{result}, nil
}
