package engine

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync/atomic"
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

// Delete removes a key-value pair. Returns true if the key existed.
func (m *OrderedMap) Delete(key string) bool {
	if _, exists := m.vals[key]; !exists {
		return false
	}
	delete(m.vals, key)
	for i, k := range m.keys {
		if k == key {
			m.keys = append(m.keys[:i], m.keys[i+1:]...)
			break
		}
	}
	return true
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
	Name    string // empty for unnamed positional parameters
	Type    Type
	Pattern *Value // optional: map/list pattern for structural matching
}

// FnSig describes one overload of a function definition.
type FnSig struct {
	Params  []FnParam
	Returns []Type // declared return types (nil = unchecked)
	Body    []Value
}

// FnDefInfo holds the parsed function specification for a def-defined function.
type FnDefInfo struct {
	Sigs []FnSig
}

// FnSigSpec describes a signature specification without a body, used for
// targeted undef of specific function signatures.
type FnSigSpec struct {
	Params  []FnParam
	Returns []Type
}

// FnUndefInfo holds signature specs for targeted undef of function signatures.
type FnUndefInfo struct {
	Sigs []FnSigSpec
}

// ReturnCheckInfo carries expected return types for fn-defined function validation.
type ReturnCheckInfo struct {
	FuncName string
	Returns  []Type
}

// DisjunctInfo holds the alternatives for a disjunction (union) type.
// A disjunct unifies if any of its alternatives unifies with the target.
type DisjunctInfo struct {
	Alternatives []Value
}

// markCounter is a global counter for generating unique mark IDs.
var markCounter atomic.Int64

// NextMarkID generates a unique mark ID.
func NextMarkID() string {
	n := markCounter.Add(1)
	return fmt.Sprintf("_m%d", n)
}

// MarkInfo identifies a mark on the stack. Marks are internal control-flow
// anchors placed by constructs like for-loops. Each mark has a unique ID
// so that a corresponding move can jump the pointer back to it.
// Body stores the original values between the mark and its paired move,
// enabling replay when the move fires.
type MarkInfo struct {
	ID   string  // unique identifier for this mark
	Body []Value // original content to replay (set by the move on first encounter)
}

// MoveInfo identifies a move on the stack. When the stack pointer reaches
// a move, it jumps back to the corresponding mark. The Reason field
// describes why the move exists (e.g. "for loop") and is used in error
// messages when the target mark cannot be found.
//
// Cont optionally carries for-loop continuation state. When set, stepMove
// uses it to drive multi-iteration loops: each firing advances the iterator,
// conditionally re-inserts mark+body+move for the next iteration, and
// accumulates results across iterations.
type MoveInfo struct {
	To     string   // ID of the target mark
	Reason string   // human-readable reason (for error messages)
	Cont   *ForCont // optional: for-loop iteration state
	IfCont *IfCont  // optional: if-statement continuation state
}

// ForCont holds the iteration state for a mark/move-driven for loop.
// It is carried by the MoveInfo and mutated across iterations.
type ForCont struct {
	Registry *Registry
	IterName string  // name of the iterator variable (e.g. "i")
	Current  int64   // current iteration value
	End      int64   // exclusive bound
	Step     int64   // increment per iteration
	Body     []Value // original body tokens (replayed each iteration)
	Results  []Value // accumulated results from completed iterations
}

// IfCont holds the continuation state for a mark/move-driven if statement.
// When the move fires, the condition result (between mark and move) is
// evaluated for truthiness to select the appropriate branch.
type IfCont struct {
	Then []Value // tokens to splice if condition is truthy
	Else []Value // tokens to splice if condition is falsy (nil for 2-arg if)
}

// ModuleDesc describes a module: its generated ID and named exports.
// Each export call adds a named entry mapping export name → export map.
type ModuleDesc struct {
	ID      string                    // generated internal identifier
	Exports map[string]*OrderedMap    // export name → export map (name → value)
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
	PrefixArgs    int // number of sig args already consumed from the prefix (stack)
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

// NewInteger creates a number/integer value with a literal type.
// The literal value is encoded in the type path (e.g., number/integer/5),
// making it a subtype of number/integer. This enables pattern matching
// on specific values in function signatures.
func NewInteger(n int64) Value {
	// Format always starts with "Number/Integer/" — cannot fail.
	t, _ := NewType(fmt.Sprintf("Number/Integer/%d", n))
	return Value{VType: t, Data: n}
}

// NewDecimal creates a number/decimal value with a float64 payload.
func NewDecimal(f float64) Value {
	return Value{VType: TDecimal, Data: f}
}

// NewBoolean creates a boolean value. The boolean payload (true/false) is the
// value; there are no Boolean/True or Boolean/False sub-types.
func NewBoolean(b bool) Value {
	return Value{VType: TBoolean, Data: b}
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

// NewMark creates a mark value with the given unique ID and the body to
// replay when the corresponding move fires. The body should contain
// the original values between the mark and its paired move.
func NewMark(id string, body ...Value) Value {
	b := make([]Value, len(body))
	copy(b, body)
	return Value{VType: TMark, Data: MarkInfo{ID: id, Body: b}}
}

// NewMove creates a move value targeting the mark with the given ID.
// The reason string describes why this move exists (used in error messages).
func NewMove(to string, reason string) Value {
	return Value{VType: TMove, Data: MoveInfo{To: to, Reason: reason}}
}

// NewMoveCont creates a move value with for-loop continuation state.
func NewMoveCont(to, reason string, cont *ForCont) Value {
	return Value{VType: TMove, Data: MoveInfo{To: to, Reason: reason, Cont: cont}}
}

// NewMoveIf creates a move value with if-statement continuation state.
func NewMoveIf(to, reason string, ifCont *IfCont) Value {
	return Value{VType: TMove, Data: MoveInfo{To: to, Reason: reason, IfCont: ifCont}}
}

// NewFnDef creates a function definition value for storage on DefStacks.
func NewFnDef(info FnDefInfo) Value {
	return Value{VType: TFnDef, Data: info}
}

// NewFunction creates a function reference value. The underlying data is a
// FnDefInfo, but the type is TFunction so it can be matched by function-typed
// parameters and passed to other functions without being called.
func NewFunction(info FnDefInfo) Value {
	return Value{VType: TFunction, Data: info}
}

// NewFnUndef creates a function undef spec value for targeted signature removal.
func NewFnUndef(info FnUndefInfo) Value {
	return Value{VType: TFnUndef, Data: info}
}

// NewReturnCheck creates a return-check marker for fn return type validation.
func NewReturnCheck(info ReturnCheckInfo) Value {
	return Value{VType: TReturnCheck, Data: info}
}

// NewDisjunct creates a disjunction type value from a list of alternatives.
func NewDisjunct(alternatives []Value) Value {
	return Value{VType: TDisjunct, Data: DisjunctInfo{Alternatives: alternatives}}
}

// NewModule creates a module descriptor value.
func NewModule(desc ModuleDesc) Value {
	return Value{VType: TModule, Data: desc}
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

// IsMark reports whether this value is a mark.
func (v Value) IsMark() bool {
	return v.VType.Equal(TMark)
}

// AsMark returns the MarkInfo, panics if not a mark.
func (v Value) AsMark() MarkInfo {
	return v.Data.(MarkInfo)
}

// IsMove reports whether this value is a move.
func (v Value) IsMove() bool {
	return v.VType.Equal(TMove)
}

// AsMove returns the MoveInfo, panics if not a move.
func (v Value) AsMove() MoveInfo {
	return v.Data.(MoveInfo)
}

// IsReturnCheck reports whether this value is a return-check marker.
func (v Value) IsReturnCheck() bool {
	return v.VType.Equal(TReturnCheck)
}

// AsReturnCheck returns the ReturnCheckInfo, panics if not a return-check.
func (v Value) AsReturnCheck() ReturnCheckInfo {
	return v.Data.(ReturnCheckInfo)
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

// IsModule reports whether this value is a module descriptor.
func (v Value) IsModule() bool {
	return v.VType.Equal(TModule)
}

// AsModule returns the ModuleDesc, panics if not a module.
func (v Value) AsModule() ModuleDesc {
	return v.Data.(ModuleDesc)
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
	if v.VType.Equal(TList) {
		if _, ok := v.Data.(TableTypeInfo); ok {
			return true
		}
		if _, ok := v.Data.(TableData); ok {
			return true
		}
		if _, ok := v.Data.(QueryBuilder); ok {
			return true
		}
	}
	return false
}

// AsTableType returns the TableTypeInfo, panics if not a table type.
func (v Value) AsTableType() TableTypeInfo {
	if td, ok := v.Data.(TableData); ok {
		return TableTypeInfo{Record: td.Record}
	}
	if qb, ok := v.Data.(QueryBuilder); ok {
		return TableTypeInfo{Record: qb.Source.Record}
	}
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

// AsDecimal returns the float64 payload, panics if not a decimal type.
func (v Value) AsDecimal() float64 {
	return v.Data.(float64)
}

// AsNumber returns the numeric value as float64 regardless of whether it is
// an integer or decimal.
func (v Value) AsNumber() float64 {
	if v.VType.Matches(TDecimal) {
		return v.AsDecimal()
	}
	return float64(v.AsInteger())
}

// AsBoolean returns the bool payload, panics if not a boolean type.
func (v Value) AsBoolean() bool {
	return v.Data.(bool)
}

// AsList returns the []Value payload, panics if not a list type.
// Also works for TableData and QueryBuilder, returning the rows.
// For QueryBuilder, this triggers materialization.
func (v Value) AsList() []Value {
	if td, ok := v.Data.(TableData); ok {
		return td.Rows
	}
	if qb, ok := v.Data.(QueryBuilder); ok {
		td, err := qb.Materialize()
		if err != nil {
			return nil
		}
		return td.Rows
	}
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
	case v.IsMark():
		return fmt.Sprintf("mark(%s)", v.AsMark().ID)
	case v.IsMove():
		m := v.AsMove()
		return fmt.Sprintf("move(%s,%s)", m.To, m.Reason)
	case v.IsReturnCheck():
		rc := v.AsReturnCheck()
		return fmt.Sprintf("returncheck(%s)", rc.FuncName)
	case v.IsModule():
		md := v.AsModule()
		return fmt.Sprintf("module(%s)", md.ID)
	case v.Data == nil:
		// Type literal with no specific value (e.g. "number", "string").
		return v.VType.String()
	case v.VType.Matches(TString):
		return fmt.Sprintf("'%s'", v.Data)
	case v.VType.Equal(TAtom):
		return v.Data.(string)
	case v.VType.Matches(TDecimal):
		return strconv.FormatFloat(v.AsDecimal(), 'f', -1, 64)
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
		if td, ok := v.Data.(TableData); ok {
			parts := make([]string, 0, td.Record.Fields.Len())
			for _, k := range td.Record.Fields.Keys() {
				val, _ := td.Record.Fields.Get(k)
				parts = append(parts, k+":"+val.String())
			}
			rowParts := make([]string, len(td.Rows))
			for i, row := range td.Rows {
				rowParts[i] = row.String()
			}
			return "table{" + strings.Join(parts, ",") + "}[" + strings.Join(rowParts, ",") + "]"
		}
		if qb, ok := v.Data.(QueryBuilder); ok {
			td, err := qb.Materialize()
			if err != nil {
				return "query(error:" + err.Error() + ")"
			}
			v2 := Value{VType: TList, Data: td}
			return v2.String()
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
	case v.VType.Matches(TMap):
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
