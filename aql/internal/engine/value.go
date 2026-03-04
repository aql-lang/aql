package engine

import (
	"fmt"
	"sort"
	"strings"
)

// OrderedMap is a map that preserves insertion order of keys.
type OrderedMap struct {
	keys []string
	vals map[string]Value
}

// NewOrderedMap creates an empty OrderedMap.
func NewOrderedMap() *OrderedMap {
	return &OrderedMap{vals: make(map[string]Value)}
}

// Set adds or updates a key-value pair, preserving insertion order.
func (m *OrderedMap) Set(key string, val Value) {
	if _, exists := m.vals[key]; !exists {
		m.keys = append(m.keys, key)
	}
	m.vals[key] = val
}

// Get retrieves a value by key.
func (m *OrderedMap) Get(key string) (Value, bool) {
	v, ok := m.vals[key]
	return v, ok
}

// Keys returns the keys in insertion order.
func (m *OrderedMap) Keys() []string {
	return m.keys
}

// SortedKeys returns the keys in sorted order (for deterministic comparison).
func (m *OrderedMap) SortedKeys() []string {
	sorted := make([]string, len(m.keys))
	copy(sorted, m.keys)
	sort.Strings(sorted)
	return sorted
}

// Len returns the number of entries.
func (m *OrderedMap) Len() int {
	return len(m.keys)
}

// ChildTypeInfo holds the child type constraint for a typed list or typed map.
// For example, [:string] constrains all list elements to be strings,
// and {:string} constrains all map values to be strings.
type ChildTypeInfo struct {
	Child Value
}

// RecordTypeInfo holds the field schema for a record type.
// Each field maps a name to a type-constraint Value (e.g. a type literal).
// A record type unifies with a concrete map if it has exactly the right keys
// and each value unifies with the corresponding field type.
type RecordTypeInfo struct {
	Fields *OrderedMap // field name → type-constraint Value
}

// TableTypeInfo holds the record schema for a table type.
// A table represents a list of record instances that all conform to the
// same record type.
type TableTypeInfo struct {
	Record RecordTypeInfo // the record type that each row must match
}

// FnParam describes one parameter in a function signature.
type FnParam struct {
	Name string // empty for unnamed positional parameters
	Type Type
}

// FnSig describes one overload of a function definition.
type FnSig struct {
	Params []FnParam
	Body   []Value
}

// FnDefInfo holds the parsed function specification for a def-defined function.
type FnDefInfo struct {
	Sigs []FnSig
}

// DisjunctInfo holds the alternatives for a disjunction (union) type.
// A disjunct unifies if any of its alternatives unifies with the target.
type DisjunctInfo struct {
	Alternatives []Value
}

// WordInfo carries the name and optional modifiers for a function reference.
type WordInfo struct {
	Name        string
	ArgCount    int  // -1 = unspecified
	ForcePrefix bool // lower/p
	ForceSuffix bool // lower/s
}

// ForwardInfo tracks suffix argument collection for a deferred function call.
type ForwardInfo struct {
	FuncName      string
	ExpectedArgs  int
	CollectedArgs int
	// FuncIndex records where the deferred function word sits in the stack.
	FuncIndex  int
	Precedence int        // copied from matched Signature
	Sig        *Signature // the matched signature, for direct execution on completion
}

// Value is a typed entry on the AQL stack.
type Value struct {
	VType Type
	Data  interface{}
}

// NewString creates a string value. Empty strings get type string/empty.
func NewString(s string) Value {
	if s == "" {
		return Value{VType: TStringEmpty, Data: s}
	}
	return Value{VType: TStringProper, Data: s}
}

// NewInteger creates a number/integer value.
func NewInteger(n int64) Value {
	return Value{VType: TInteger, Data: n}
}

// NewBoolean creates a boolean/true or boolean/false value.
func NewBoolean(b bool) Value {
	if b {
		return Value{VType: TBooleanTrue, Data: b}
	}
	return Value{VType: TBooleanFalse, Data: b}
}

// NewList creates a list value from a slice of Values.
func NewList(elems []Value) Value {
	return Value{VType: TList, Data: elems}
}

// NewTypedList creates a typed list value with a child type constraint.
// For example, NewTypedList(NewTypeLiteral(TString)) represents [:string].
func NewTypedList(child Value) Value {
	return Value{VType: TList, Data: ChildTypeInfo{Child: child}}
}

// NewMap creates a map value from an ordered map of string keys to Values.
func NewMap(entries *OrderedMap) Value {
	return Value{VType: TMap, Data: entries}
}

// NewTypedMap creates a typed map value with a child type constraint.
// For example, NewTypedMap(NewTypeLiteral(TString)) represents {:string}.
func NewTypedMap(child Value) Value {
	return Value{VType: TMap, Data: ChildTypeInfo{Child: child}}
}

// NewRecordType creates a record type value from a field schema.
// The fields map contains field names as keys and type-constraint Values as values.
// For example, record{x:number, y:number} constrains maps to have exactly
// keys x and y with number-typed values.
func NewRecordType(fields *OrderedMap) Value {
	return Value{VType: TMap, Data: RecordTypeInfo{Fields: fields}}
}

// NewTableType creates a table type value from a record type.
// A table type constrains a list so that each element is a map conforming
// to the given record schema.
func NewTableType(record RecordTypeInfo) Value {
	return Value{VType: TList, Data: TableTypeInfo{Record: record}}
}

// NewAtom creates an atom value from a bare unquoted word.
func NewAtom(name string) Value {
	return Value{VType: TAtom, Data: name}
}

// NewTypeLiteral creates a value representing a type itself (e.g. "number", "string").
// The Data is nil since type literals have no specific literal value.
func NewTypeLiteral(t Type) Value {
	return Value{VType: t, Data: nil}
}

// NewWord creates a word value (function reference) with no modifiers.
func NewWord(name string) Value {
	return Value{
		VType: TWord,
		Data:  WordInfo{Name: name, ArgCount: -1},
	}
}

// NewWordModified creates a word value with explicit modifiers.
func NewWordModified(name string, argCount int, forcePrefix, forceSuffix bool) Value {
	return Value{
		VType: TWord,
		Data: WordInfo{
			Name:        name,
			ArgCount:    argCount,
			ForcePrefix: forcePrefix,
			ForceSuffix: forceSuffix,
		},
	}
}

// NewForward creates a forward primitive value for suffix argument tracking.
func NewForward(info ForwardInfo) Value {
	return Value{VType: TForward, Data: info}
}

// NewOpenParen creates an open-paren marker value for sub-expression scoping.
func NewOpenParen() Value {
	return Value{VType: TOpenParen, Data: nil}
}

// NewFnDef creates a function definition value for storage on DefStacks.
func NewFnDef(info FnDefInfo) Value {
	return Value{VType: TFnDef, Data: info}
}

// NewDisjunct creates a disjunction type value from a list of alternatives.
func NewDisjunct(alternatives []Value) Value {
	return Value{VType: TDisjunct, Data: DisjunctInfo{Alternatives: alternatives}}
}

// IsWord reports whether this value is a word (function reference).
func (v Value) IsWord() bool {
	return v.VType.Equal(TWord)
}

// IsForward reports whether this value is a forward primitive.
func (v Value) IsForward() bool {
	return v.VType.Equal(TForward)
}

// IsBoolean reports whether this value is a boolean type.
func (v Value) IsBoolean() bool {
	return v.VType.Matches(TBoolean)
}

// IsOpenParen reports whether this value is an open-paren marker.
func (v Value) IsOpenParen() bool {
	return v.VType.Equal(TOpenParen)
}

// IsDisjunct reports whether this value is a disjunction type.
func (v Value) IsDisjunct() bool {
	_, ok := v.Data.(DisjunctInfo)
	return ok && v.VType.Equal(TDisjunct)
}

// AsDisjunct returns the DisjunctInfo, panics if not a disjunct.
func (v Value) AsDisjunct() DisjunctInfo {
	return v.Data.(DisjunctInfo)
}

// IsAtom reports whether this value is an atom.
func (v Value) IsAtom() bool {
	return v.VType.Equal(TAtom)
}

// AsAtom returns the string payload, panics if not an atom.
func (v Value) AsAtom() string {
	return v.Data.(string)
}

// IsTypedList reports whether this value is a typed list (has child type constraint).
func (v Value) IsTypedList() bool {
	_, ok := v.Data.(ChildTypeInfo)
	return ok && v.VType.Equal(TList)
}

// IsTypedMap reports whether this value is a typed map (has child type constraint).
func (v Value) IsTypedMap() bool {
	_, ok := v.Data.(ChildTypeInfo)
	return ok && v.VType.Equal(TMap)
}

// IsRecordType reports whether this value is a record type (map with field schema).
func (v Value) IsRecordType() bool {
	_, ok := v.Data.(RecordTypeInfo)
	return ok && v.VType.Equal(TMap)
}

// AsRecordType returns the RecordTypeInfo, panics if not a record type.
func (v Value) AsRecordType() RecordTypeInfo {
	return v.Data.(RecordTypeInfo)
}

// IsTableType reports whether this value is a table type (list with record schema).
func (v Value) IsTableType() bool {
	_, ok := v.Data.(TableTypeInfo)
	return ok && v.VType.Equal(TList)
}

// AsTableType returns the TableTypeInfo, panics if not a table type.
func (v Value) AsTableType() TableTypeInfo {
	return v.Data.(TableTypeInfo)
}

// AsChildType returns the ChildTypeInfo, panics if not a typed list or typed map.
func (v Value) AsChildType() ChildTypeInfo {
	return v.Data.(ChildTypeInfo)
}

// AsWord returns the WordInfo, panics if not a word.
func (v Value) AsWord() WordInfo {
	return v.Data.(WordInfo)
}

// AsForward returns the ForwardInfo, panics if not a forward.
func (v Value) AsForward() ForwardInfo {
	return v.Data.(ForwardInfo)
}

// AsString returns the string payload, panics if not a string type.
func (v Value) AsString() string {
	return v.Data.(string)
}

// AsInteger returns the int64 payload, panics if not an integer type.
func (v Value) AsInteger() int64 {
	return v.Data.(int64)
}

// AsBoolean returns the bool payload, panics if not a boolean type.
func (v Value) AsBoolean() bool {
	return v.Data.(bool)
}

// AsList returns the []Value payload, panics if not a list type.
func (v Value) AsList() []Value {
	return v.Data.([]Value)
}

// AsMap returns the OrderedMap payload, panics if not a map type.
func (v Value) AsMap() *OrderedMap {
	return v.Data.(*OrderedMap)
}

// String returns a human-readable representation.
func (v Value) String() string {
	switch {
	case v.IsWord():
		w := v.AsWord()
		return fmt.Sprintf("word(%s)", w.Name)
	case v.IsForward():
		f := v.AsForward()
		return fmt.Sprintf("forward(%s,%d/%d)", f.FuncName, f.CollectedArgs, f.ExpectedArgs)
	case v.IsOpenParen():
		return "("
	case v.Data == nil:
		// Type literal with no specific value (e.g. "number", "string").
		return v.VType.String()
	case v.VType.Matches(TString):
		return fmt.Sprintf("'%s'", v.Data)
	case v.VType.Equal(TAtom):
		return v.Data.(string)
	case v.VType.Matches(TInteger):
		return fmt.Sprintf("%d", v.Data)
	case v.VType.Matches(TBoolean):
		if v.AsBoolean() {
			return "true"
		}
		return "false"
	case v.VType.Equal(TList):
		if tt, ok := v.Data.(TableTypeInfo); ok {
			parts := make([]string, 0, tt.Record.Fields.Len())
			for _, k := range tt.Record.Fields.Keys() {
				val, _ := tt.Record.Fields.Get(k)
				parts = append(parts, k+":"+val.String())
			}
			return "table{" + strings.Join(parts, ",") + "}"
		}
		if ct, ok := v.Data.(ChildTypeInfo); ok {
			return "[:" + ct.Child.String() + "]"
		}
		elems := v.AsList()
		parts := make([]string, len(elems))
		for i, e := range elems {
			parts[i] = e.String()
		}
		return "[" + strings.Join(parts, ",") + "]"
	case v.IsDisjunct():
		di := v.AsDisjunct()
		parts := make([]string, len(di.Alternatives))
		for i, alt := range di.Alternatives {
			parts[i] = alt.String()
		}
		return strings.Join(parts, "|")
	case v.VType.Equal(TMap):
		if ct, ok := v.Data.(ChildTypeInfo); ok {
			return "{:" + ct.Child.String() + "}"
		}
		if rt, ok := v.Data.(RecordTypeInfo); ok {
			parts := make([]string, 0, rt.Fields.Len())
			for _, k := range rt.Fields.Keys() {
				val, _ := rt.Fields.Get(k)
				parts = append(parts, k+":"+val.String())
			}
			return "record{" + strings.Join(parts, ",") + "}"
		}
		m := v.AsMap()
		parts := make([]string, 0, m.Len())
		for _, k := range m.Keys() {
			val, _ := m.Get(k)
			parts = append(parts, k+":"+val.String())
		}
		return "{" + strings.Join(parts, ",") + "}"
	default:
		return fmt.Sprintf("%v(%v)", v.VType, v.Data)
	}
}
