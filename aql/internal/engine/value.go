package engine

import (
	"encoding/hex"
	"fmt"
	"math/rand/v2"
	"sort"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
)

// ReadList is a read-only view of a list of Values.
// Node list values expose this via AsList(). To mutate, use AsMutableList().
type ReadList struct {
	elems []Value
}

// Get returns the element at index i. Panics if out of bounds.
func (l ReadList) Get(i int) Value {
	return l.elems[i]
}

// Len returns the number of elements.
func (l ReadList) Len() int {
	return len(l.elems)
}

// Slice returns a copy of the underlying slice.
func (l ReadList) Slice() []Value {
	out := make([]Value, len(l.elems))
	copy(out, l.elems)
	return out
}

// IsNil reports whether this ReadList has no backing data.
func (l ReadList) IsNil() bool {
	return l.elems == nil
}

// ReadMap is a read-only view of an ordered key-value map.
// Node values (Map, Options) expose this interface via AsMap().
// To mutate, use AsMutableMap() which is only valid for Object instances.
type ReadMap interface {
	Get(key string) (Value, bool)
	Keys() []string
	SortedKeys() []string
	Len() int
}

// OrderedMap is a map that preserves insertion order of keys.
type OrderedMap struct {
	keys     []string
	vals     map[string]Value
	Implicit bool              // true when created from implicit pair syntax (e.g., [x:Integer])
	Meta     map[string]any    // optional metadata for parser/engine communication
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

// OptionsTypeInfo holds the field schema for an options type.
// Each field maps a name to a default value or type constraint.
// Concrete values serve as defaults when the key is absent during unification;
// type literals require the caller to provide a value.
type OptionsTypeInfo struct {
	Fields *OrderedMap // field name → default value or type constraint
}

// TableTypeInfo holds the record schema for a table type.
// A table represents a list of record instances that all conform to the
// same record type.
type TableTypeInfo struct {
	Record RecordTypeInfo // the record type that each row must match
}

// FnParam describes one parameter in a function signature.
type FnParam struct {
	Name     string // empty for unnamed positional parameters
	Type     Type
	Pattern  *Value // optional: map/list pattern for structural matching
	Optional bool   // true if this param was marked optional via ?
}

// FnSig describes one overload of a function definition.
type FnSig struct {
	Params  []FnParam
	Returns []Type // declared return types (nil = unchecked)
	Body    []Value
}

// FnDefInfo holds the parsed function specification for a def-defined function.
// If Registry is non-nil, the function was defined in a module and should
// execute in that registry's context (closure semantics).
type FnDefInfo struct {
	Sigs     []FnSig
	Registry *Registry
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
	FuncName     string
	Returns      []Type
	UnnamedCount int // number of unnamed params pushed onto the stack before the body
}

// DisjunctInfo holds the alternatives for a disjunction (union) type.
// A disjunct unifies if any of its alternatives unifies with the target.
type DisjunctInfo struct {
	Alternatives []Value
}

// ObjectTypeInfo holds the type definition for an object type.
// Object types form an inheritance hierarchy analogous to class inheritance.
// For example, Object/Foo has parent Object, Object/Foo/Bar has parent Foo.
// Fields are the type's own fields (not including inherited ones).
// Parent points to the parent object type (nil for direct children of Object root).
// ID is a unique internal identifier: "T_" followed by 12 lowercase hex characters.
// Name is the full type path (e.g. "Object/Foo/Bar"), set when the type is
// registered via def.
type ObjectTypeInfo struct {
	Fields *OrderedMap      // own fields (field name → type-constraint Value)
	Parent *ObjectTypeInfo  // parent object type (nil if direct child of Object)
	ID     string           // unique internal ID: "T_" + 12 hex chars
	Name   string           // full type path (e.g. "Object/Foo/Bar")
}

// AllFields returns all fields including inherited ones. Parent fields come
// first, followed by the type's own fields. Own fields override inherited
// fields with the same name.
func (o *ObjectTypeInfo) AllFields() *OrderedMap {
	result := NewOrderedMap()
	if o.Parent != nil {
		parentFields := o.Parent.AllFields()
		for _, k := range parentFields.Keys() {
			v, _ := parentFields.Get(k)
			result.Set(k, v)
		}
	}
	for _, k := range o.Fields.Keys() {
		v, _ := o.Fields.Get(k)
		result.Set(k, v)
	}
	return result
}

// ObjectInstanceInfo holds a concrete instance of an object type.
// TypeRef points back to the ObjectTypeInfo that created this instance.
// Fields holds the type's own resolved field values.
// Prototype points to the parent instance (like JavaScript prototypes):
// field lookups fall through to the prototype chain for inherited fields.
type ObjectInstanceInfo struct {
	TypeRef   *ObjectTypeInfo    // the object type this is an instance of
	Fields    *OrderedMap        // own field name → resolved Value
	Prototype *ObjectInstanceInfo // parent instance (nil if root type)
}

// GetField returns a field value by searching own fields first, then walking
// the prototype chain. Returns the value and true if found, zero and false otherwise.
func (oi ObjectInstanceInfo) GetField(name string) (Value, bool) {
	if v, ok := oi.Fields.Get(name); ok {
		return v, true
	}
	if oi.Prototype != nil {
		return oi.Prototype.GetField(name)
	}
	return Value{}, false
}

// AllFields returns all fields including those from the prototype chain.
// Prototype fields come first, own fields override.
func (oi ObjectInstanceInfo) AllFields() *OrderedMap {
	result := NewOrderedMap()
	if oi.Prototype != nil {
		proto := oi.Prototype.AllFields()
		for _, k := range proto.Keys() {
			v, _ := proto.Get(k)
			result.Set(k, v)
		}
	}
	for _, k := range oi.Fields.Keys() {
		v, _ := oi.Fields.Get(k)
		result.Set(k, v)
	}
	return result
}

// StoreInstanceInfo is a copy-on-write key-value store (Object/Store).
// Unlike regular Object instances which have typed fields, Store instances
// hold arbitrary key-value pairs. Key resolution walks the prototype chain,
// enabling scope-like lookup when contexts are nested.
//
// Copy-on-write: the AQL `set` word creates a new Store layer (prototype =
// old Store) instead of mutating in place. If this Store is nested inside
// a parent Store, the parent is COW'd too, propagating up to the ctxStack.
type StoreInstanceInfo struct {
	TypeName  string              // full type path, e.g. "Object/Store" or "Object/Store/System"
	Data      map[string]Value    // own key-value pairs (COW layer)
	Prototype *StoreInstanceInfo  // prototype chain for key lookup / COW base
	Parent    *StoreInstanceInfo  // containing Store (for COW propagation), nil if root
	ParentKey string              // key in Parent that references this Store
}

// Get looks up a key in this store, walking the prototype chain if not found.
func (si *StoreInstanceInfo) Get(key string) (Value, bool) {
	if v, ok := si.Data[key]; ok {
		return v, true
	}
	if si.Prototype != nil {
		return si.Prototype.Get(key)
	}
	return Value{}, false
}

// Set stores a key-value pair directly (for internal/init use only).
// AQL code should use the set word which does COW via cowSet.
func (si *StoreInstanceInfo) Set(key string, val Value) {
	si.Data[key] = val
	// Track parent relationship for nested Stores.
	if childStore, ok := val.Data.(*StoreInstanceInfo); ok {
		childStore.Parent = si
		childStore.ParentKey = key
	}
}

// ArrayInstanceInfo is a mutable ordered array (Object/Array).
// Unlike immutable Node/List values, Array instances can be modified
// in place via set (index assignment), append, etc.
type ArrayInstanceInfo struct {
	Elems []Value
}

// Get returns the element at index i. Returns zero Value and false if out of bounds.
func (ai *ArrayInstanceInfo) Get(i int) (Value, bool) {
	if i < 0 || i >= len(ai.Elems) {
		return Value{}, false
	}
	return ai.Elems[i], true
}

// Set sets the element at index i. Returns false if out of bounds.
func (ai *ArrayInstanceInfo) Set(i int, val Value) bool {
	if i < 0 || i >= len(ai.Elems) {
		return false
	}
	ai.Elems[i] = val
	return true
}

// Len returns the number of elements.
func (ai *ArrayInstanceInfo) Len() int {
	return len(ai.Elems)
}

// Append adds a value to the end of the array.
func (ai *ArrayInstanceInfo) Append(val Value) {
	ai.Elems = append(ai.Elems, val)
}

// "T_" followed by 12 lowercase hex characters (6 random bytes).
func GenerateObjectTypeID() string {
	return GenerateID("T_")
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
	ForceStack bool // lower/s
	ForceForward bool // lower/f
}

// ForwardInfo tracks forward argument collection for a deferred function call.
type ForwardInfo struct {
	FuncName      string
	ExpectedArgs  int
	CollectedArgs int
	StackArgs     int // number of sig args already consumed from the stack
	// FuncIndex records where the deferred function word sits in the stack.
	FuncIndex int
	Sig       *Signature // the matched signature, for direct execution on completion
}

// Value is a typed entry on the AQL stack.
// Every value carries a unique ID with a prefix indicating its category:
//   - "S_" for scalar values (String, Number, Boolean)
//   - "N_" for node values (List, Map, Table, Record)
//   - "W_" for word values (Word, Atom, Function, Internal/*)
//   - "T_" for type/object values (Object/*, type literals, Any, None)
//
// Each ID is the prefix followed by 12 lowercase hex characters (6 random bytes).
type Value struct {
	ID      string
	VType   Type
	Data    interface{}
	Quoted  bool // true when value was produced by the quote word; prevents auto-evaluation
	Eval    bool // true for parser-created lists that should auto-evaluate at end of Run
}

// idRand is the package-level RNG used for ID generation.
// Defaults to time-seeded; can be overridden via SetIDSeed.
var idRand = rand.New(rand.NewPCG(uint64(time.Now().UnixNano()), 0))

// idMin is the minimum random value for generated IDs (0x100000000000).
const idMin uint64 = 0x100000000000

// idMax is the exclusive upper bound so values fit in 12 hex chars.
const idMax uint64 = 0x1000000000000 - 0x100000000000

// SetIDSeed configures the package-level RNG with the given seed.
func SetIDSeed(seed int64) {
	idRand = rand.New(rand.NewPCG(uint64(seed), 0))
}

// GenerateID creates a unique ID with the given prefix followed by 12
// lowercase hex characters. The random value is >= 0x100000000000.
func GenerateID(prefix string) string {
	n := idMin + idRand.Uint64N(idMax)
	var buf [6]byte
	buf[0] = byte(n >> 40)
	buf[1] = byte(n >> 32)
	buf[2] = byte(n >> 24)
	buf[3] = byte(n >> 16)
	buf[4] = byte(n >> 8)
	buf[5] = byte(n)
	return prefix + hex.EncodeToString(buf[:])
}

// IDPrefixForType returns the ID prefix for a given type:
// "S_" for Scalar, "N_" for Node, "W_" for Word, "T_" for Object/Any/None.
func IDPrefixForType(t Type) string {
	return IDPrefixForParts(t.Parts)
}

// newValue creates a Value with an auto-generated ID based on the type category.
func newValue(t Type, data interface{}) Value {
	return Value{
		ID:    GenerateID(IDPrefixForType(t)),
		VType: t,
		Data:  data,
	}
}

// NewString creates a string value. Empty strings get type string/empty.
func NewString(s string) Value {
	if s == "" {
		return newValue(TStringEmpty, s)
	}
	return newValue(TStringProper, s)
}

// NewInteger creates a number/integer value with a literal type.
// The literal value is encoded in the type path (e.g., number/integer/5),
// making it a subtype of number/integer. This enables pattern matching
// on specific values in function signatures.
func NewInteger(n int64) Value {
	// Format always starts with "Number/Integer/" — cannot fail.
	t, _ := NewType(fmt.Sprintf("Number/Integer/%d", n))
	return newValue(t, n)
}

// NewDecimal creates a number/decimal value with a float64 payload.
func NewDecimal(f float64) Value {
	return newValue(TDecimal, f)
}

// NewBoolean creates a boolean value. The boolean payload (true/false) is the
// value; there are no Boolean/True or Boolean/False sub-types.
func NewBoolean(b bool) Value {
	return newValue(TBoolean, b)
}

// NewList creates a list value from a slice of Values.
func NewList(elems []Value) Value {
	return newValue(TList, elems)
}

// NewEvalList creates a list value that is marked for auto-evaluation
// at the end of execution. Used by the parser for source-code lists.
func NewEvalList(elems []Value) Value {
	v := newValue(TList, elems)
	v.Eval = true
	return v
}

// NewTypedList creates a typed list value with a child type constraint.
// For example, NewTypedList(NewTypeLiteral(TString)) represents [:string].
func NewTypedList(child Value) Value {
	return newValue(TList, ChildTypeInfo{Child: child})
}

// NewMap creates a map value from an ordered map of string keys to Values.
func NewMap(entries *OrderedMap) Value {
	return newValue(TMap, entries)
}

// NewEvalMap creates a map value marked for auto-evaluation at end of
// execution. Used by the parser for source-code maps.
func NewEvalMap(entries *OrderedMap) Value {
	v := newValue(TMap, entries)
	v.Eval = true
	return v
}

// NewImplicitMap creates a map value marked as implicit (from pair syntax).
// In fn signatures, implicit maps are treated as named parameter declarations
// (e.g., [x:Integer]), while explicit maps are structural patterns.
func NewImplicitMap(entries *OrderedMap) Value {
	entries.Implicit = true
	return newValue(TMap, entries)
}

// NewTypedMap creates a typed map value with a child type constraint.
// For example, NewTypedMap(NewTypeLiteral(TString)) represents {:string}.
func NewTypedMap(child Value) Value {
	return newValue(TMap, ChildTypeInfo{Child: child})
}

// NewRecordType creates a record type value from a field schema.
// The fields map contains field names as keys and type-constraint Values as values.
// For example, record{x:number, y:number} constrains maps to have exactly
// keys x and y with number-typed values.
func NewRecordType(fields *OrderedMap) Value {
	return newValue(TMap, RecordTypeInfo{Fields: fields})
}

// NewOptionsType creates an options type value from a field schema map.
func NewOptionsType(fields *OrderedMap) Value {
	return newValue(TMap, OptionsTypeInfo{Fields: fields})
}

// NewTableType creates a table type value from a record type.
// A table type constrains a list so that each element is a map conforming
// to the given record schema.
func NewTableType(record RecordTypeInfo) Value {
	return newValue(TList, TableTypeInfo{Record: record})
}

// NewAtom creates an atom value from a bare unquoted word.
func NewAtom(name string) Value {
	return newValue(TAtom, name)
}

// NewTypeLiteral creates a value representing a type itself (e.g. "number", "string").
// The Data is nil since type literals have no specific literal value.
func NewTypeLiteral(t Type) Value {
	return newValue(t, nil)
}

// NewWord creates a word value (function reference) with no modifiers.
func NewWord(name string) Value {
	return newValue(TWord, WordInfo{Name: name, ArgCount: -1})
}

// NewWordModified creates a word value with explicit modifiers.
func NewWordModified(name string, argCount int, forceStack, forceForward bool) Value {
	return newValue(TWord, WordInfo{
		Name:        name,
		ArgCount:    argCount,
		ForceStack: forceStack,
		ForceForward: forceForward,
	})
}

// NewForward creates a forward primitive value for forward argument tracking.
func NewForward(info ForwardInfo) Value {
	return newValue(TForward, info)
}

// NewOpenParen creates an open-paren marker value for sub-expression scoping.
func NewOpenParen() Value {
	return newValue(TOpenParen, nil)
}

// NewParenExpr creates a paren expression value containing items to evaluate.
// Used by the parser for paren groups in map/list value positions.
// autoEvalMap evaluates these by running the items in a sub-engine with
// paren markers, producing a single result value.
func NewParenExpr(items []Value) Value {
	return newValue(TParenExpr, items)
}

// NewMark creates a mark value with the given unique ID and the body to
// replay when the corresponding move fires. The body should contain
// the original values between the mark and its paired move.
func NewMark(id string, body ...Value) Value {
	b := make([]Value, len(body))
	copy(b, body)
	return newValue(TMark, MarkInfo{ID: id, Body: b})
}

// NewMove creates a move value targeting the mark with the given ID.
// The reason string describes why this move exists (used in error messages).
func NewMove(to string, reason string) Value {
	return newValue(TMove, MoveInfo{To: to, Reason: reason})
}

// NewMoveCont creates a move value with for-loop continuation state.
func NewMoveCont(to, reason string, cont *ForCont) Value {
	return newValue(TMove, MoveInfo{To: to, Reason: reason, Cont: cont})
}

// NewMoveIf creates a move value with if-statement continuation state.
func NewMoveIf(to, reason string, ifCont *IfCont) Value {
	return newValue(TMove, MoveInfo{To: to, Reason: reason, IfCont: ifCont})
}

// NewFnDef creates a function definition value for storage on DefStacks.
func NewFnDef(info FnDefInfo) Value {
	return newValue(TFnDef, info)
}

// NewFunction creates a function reference value. The underlying data is a
// FnDefInfo, but the type is TFunction so it can be matched by function-typed
// parameters and passed to other functions without being called.
func NewFunction(info FnDefInfo) Value {
	return newValue(TFunction, info)
}

// NewFnUndef creates a function undef spec value for targeted signature removal.
func NewFnUndef(info FnUndefInfo) Value {
	return newValue(TFnUndef, info)
}

// NewReturnCheck creates a return-check marker for fn return type validation.
func NewReturnCheck(info ReturnCheckInfo) Value {
	return newValue(TReturnCheck, info)
}

// NewDisjunct creates a disjunction type value from a list of alternatives.
func NewDisjunct(alternatives []Value) Value {
	return newValue(TDisjunct, DisjunctInfo{Alternatives: alternatives})
}

// NewObjectType creates an object type value. The type path is derived from
// the ObjectTypeInfo.Name field. If Name is empty, the ID is used as the
// type path suffix under "Object/".
func NewObjectType(info ObjectTypeInfo) Value {
	name := info.Name
	if name == "" {
		name = "Object/" + info.ID
	}
	t, _ := NewType(name)
	return newValue(t, info)
}

// NewObjectInstance creates an object instance value. The type path matches
// the object type's Name so instances are subtypes of their type's hierarchy.
func NewObjectInstance(info ObjectInstanceInfo) Value {
	name := info.TypeRef.Name
	if name == "" {
		name = "Object/" + info.TypeRef.ID
	}
	t, _ := NewType(name)
	return newValue(t, info)
}

// NewStore creates a Store value with the given type name.
// If typeName is empty, defaults to "Object/Store".
func NewStore(typeName string) Value {
	if typeName == "" {
		typeName = "Object/Store"
	}
	t, _ := NewType(typeName)
	return newValue(t, &StoreInstanceInfo{
		TypeName: typeName,
		Data:     make(map[string]Value),
	})
}

// NewStoreValue wraps an existing StoreInstanceInfo into a Value.
func NewStoreValue(si *StoreInstanceInfo) Value {
	typeName := si.TypeName
	if typeName == "" {
		typeName = "Object/Store"
	}
	t, _ := NewType(typeName)
	return newValue(t, si)
}

// NewStoreWithPrototype creates a Store value with a prototype chain.
func NewStoreWithPrototype(typeName string, prototype *StoreInstanceInfo) Value {
	if typeName == "" {
		typeName = "Object/Store"
	}
	t, _ := NewType(typeName)
	return newValue(t, &StoreInstanceInfo{
		TypeName:  typeName,
		Data:      make(map[string]Value),
		Prototype: prototype,
	})
}

// NewArray creates a mutable Array value from a slice of elements.
func NewArray(elems []Value) Value {
	data := make([]Value, len(elems))
	copy(data, elems)
	return newValue(TArray, &ArrayInstanceInfo{Elems: data})
}

// NewArrayEmpty creates an empty mutable Array value.
func NewArrayEmpty() Value {
	return newValue(TArray, &ArrayInstanceInfo{Elems: nil})
}

// NewModule creates a module descriptor value.
func NewModule(desc ModuleDesc) Value {
	return newValue(TModule, desc)
}

// ErrorInfo holds the details of an AQL error value.
type ErrorInfo struct {
	Message string // the error description
}

// NewError creates an error value from a Go error.
func NewError(err error) Value {
	return newValue(TError, ErrorInfo{Message: err.Error()})
}

// IsError reports whether this value is an error.
func (v Value) IsError() bool {
	return v.VType.Equal(TError)
}

// AsError returns the ErrorInfo for an error value.
func (v Value) AsError() ErrorInfo {
	return v.Data.(ErrorInfo)
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

// IsParenExpr reports whether this value is a paren expression.
func (v Value) IsParenExpr() bool {
	return v.VType.Equal(TParenExpr)
}

// AsParenExpr returns the items in a paren expression value.
func (v Value) AsParenExpr() []Value {
	if items, ok := v.Data.([]Value); ok {
		return items
	}
	return nil
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

// IsObjectType reports whether this value is an object type definition.
func (v Value) IsObjectType() bool {
	_, ok := v.Data.(ObjectTypeInfo)
	return ok && v.VType.Matches(TObject)
}

// AsObjectType returns the ObjectTypeInfo, panics if not an object type.
func (v Value) AsObjectType() ObjectTypeInfo {
	return v.Data.(ObjectTypeInfo)
}

// IsStore reports whether this value is a Store instance.
func (v Value) IsStore() bool {
	_, ok := v.Data.(*StoreInstanceInfo)
	return ok && v.VType.Matches(TStore)
}

// AsStore returns the StoreInstanceInfo pointer. Returns nil if not a store.
func (v Value) AsStore() *StoreInstanceInfo {
	si, ok := v.Data.(*StoreInstanceInfo)
	if !ok {
		return nil
	}
	return si
}

// IsArray reports whether this value is an Array instance.
func (v Value) IsArray() bool {
	_, ok := v.Data.(*ArrayInstanceInfo)
	return ok && v.VType.Matches(TArray)
}

// AsArray returns the ArrayInstanceInfo pointer. Returns nil if not an array.
func (v Value) AsArray() *ArrayInstanceInfo {
	ai, ok := v.Data.(*ArrayInstanceInfo)
	if !ok {
		return nil
	}
	return ai
}

// IsObjectInstance reports whether this value is an object instance.
func (v Value) IsObjectInstance() bool {
	_, ok := v.Data.(ObjectInstanceInfo)
	return ok && v.VType.Matches(TObject)
}

// AsObjectInstance returns the ObjectInstanceInfo, panics if not an object instance.
func (v Value) AsObjectInstance() ObjectInstanceInfo {
	return v.Data.(ObjectInstanceInfo)
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

// AsAtom returns the string payload. Returns "" if Data is nil.
func (v Value) AsAtom() string {
	if v.Data == nil {
		return ""
	}
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

// IsOptionsType reports whether this value is an options type (map with defaults/constraints).
func (v Value) IsOptionsType() bool {
	_, ok := v.Data.(OptionsTypeInfo)
	return ok && v.VType.Equal(TMap)
}

// AsOptionsType returns the OptionsTypeInfo, panics if not an options type.
func (v Value) AsOptionsType() OptionsTypeInfo {
	return v.Data.(OptionsTypeInfo)
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

// AsString returns the string payload. Returns "" if Data is nil (type literal).
func (v Value) AsString() string {
	if v.Data == nil {
		return ""
	}
	return v.Data.(string)
}

// AsInteger returns the int64 payload. Returns 0 if Data is nil (type literal).
func (v Value) AsInteger() int64 {
	if v.Data == nil {
		return 0
	}
	return v.Data.(int64)
}

// AsDecimal returns the float64 payload. Returns 0.0 if Data is nil (type literal).
func (v Value) AsDecimal() float64 {
	if v.Data == nil {
		return 0.0
	}
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

// AsBoolean returns the bool payload. Returns false if Data is nil (type literal).
func (v Value) AsBoolean() bool {
	if v.Data == nil {
		return false
	}
	return v.Data.(bool)
}

// AsList returns the []Value payload, or nil if the data is not a []Value.
// Also works for TableData and QueryBuilder, returning the rows.
// For QueryBuilder, this triggers materialization.
// AsList returns a read-only view of the list payload.
// Returns a ReadList with nil backing if the data is not a list.
func (v Value) AsList() ReadList {
	if v.Data == nil {
		return ReadList{}
	}
	if td, ok := v.Data.(TableData); ok {
		return ReadList{elems: td.Rows}
	}
	if qb, ok := v.Data.(QueryBuilder); ok {
		td, err := qb.Materialize()
		if err != nil {
			return ReadList{}
		}
		return ReadList{elems: td.Rows}
	}
	elems, ok := v.Data.([]Value)
	if !ok {
		return ReadList{}
	}
	return ReadList{elems: elems}
}

// AsMutableList returns the underlying []Value slice for mutation.
// Only valid for internal construction paths — never for immutable Node values.
func (v Value) AsMutableList() []Value {
	if v.Data == nil {
		return nil
	}
	elems, ok := v.Data.([]Value)
	if !ok {
		return nil
	}
	return elems
}

// AsMap returns a read-only view of the map payload, or nil if the data is
// not an *OrderedMap. Node values (Map, Options) are immutable — use this
// for all read access.
func (v Value) AsMap() ReadMap {
	if v.Data == nil {
		return nil
	}
	om, ok := v.Data.(*OrderedMap)
	if !ok {
		return nil
	}
	return om
}

// AsMutableMap returns the underlying *OrderedMap for mutation. Only valid
// for Object instances and internal construction — never for Node values.
func (v Value) AsMutableMap() *OrderedMap {
	if v.Data == nil {
		return nil
	}
	om, ok := v.Data.(*OrderedMap)
	if !ok {
		return nil
	}
	return om
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
	case v.IsParenExpr():
		return fmt.Sprintf("paren(%v)", v.AsParenExpr())
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
	case v.IsError():
		return fmt.Sprintf("error(%s)", v.AsError().Message)
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
			v2 := newValue(TList, td)
			return v2.String()
		}
		if ct, ok := v.Data.(ChildTypeInfo); ok {
			return "[:" + ct.Child.String() + "]"
		}
		elems := v.AsList().Slice()
		parts := make([]string, len(elems))
		for i, e := range elems {
			parts[i] = e.String()
		}
		return "[" + strings.Join(parts, ",") + "]"
	case v.IsArray():
		arr := v.AsArray()
		parts := make([]string, arr.Len())
		for i := 0; i < arr.Len(); i++ {
			e, _ := arr.Get(i)
			parts[i] = e.String()
		}
		return "Array[" + strings.Join(parts, ",") + "]"
	case v.IsObjectInstance():
		oi := v.AsObjectInstance()
		allFields := oi.AllFields()
		parts := make([]string, 0, allFields.Len())
		for _, k := range allFields.Keys() {
			val, _ := allFields.Get(k)
			parts = append(parts, k+":"+val.String())
		}
		name := oi.TypeRef.Name
		if name == "" {
			name = "Object/" + oi.TypeRef.ID
		}
		return name + "{" + strings.Join(parts, ",") + "}"
	case v.IsObjectType():
		ot := v.AsObjectType()
		allFields := ot.AllFields()
		parts := make([]string, 0, allFields.Len())
		for _, k := range allFields.Keys() {
			val, _ := allFields.Get(k)
			parts = append(parts, k+":"+val.String())
		}
		name := ot.Name
		if name == "" {
			name = "Object/" + ot.ID
		}
		return "object<" + name + ">{" + strings.Join(parts, ",") + "}"
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
		if ot, ok := v.Data.(OptionsTypeInfo); ok {
			parts := make([]string, 0, ot.Fields.Len())
			for _, k := range ot.Fields.Keys() {
				val, _ := ot.Fields.Get(k)
				parts = append(parts, k+":"+val.String())
			}
			return "options{" + strings.Join(parts, ",") + "}"
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

// IsTypeValue returns true if v is a type literal, an Options instance,
// or a Node that contains a leaf that is a type.
func IsTypeValue(v Value) bool {
	// Type literal: Data==nil with a real type (not None).
	if v.Data == nil && !v.VType.Equal(TNone) {
		return true
	}

	// Options type, record type, typed list/map, table type, object type.
	if v.IsOptionsType() || v.IsRecordType() || v.IsTypedList() ||
		v.IsTypedMap() || v.IsTableType() || v.IsObjectType() {
		return true
	}

	// Concrete list: check each element recursively.
	if v.VType.Matches(TList) && v.Data != nil {
		elems := v.AsList()
		if !elems.IsNil() {
			for _, elem := range elems.Slice() {
				if IsTypeValue(elem) {
					return true
				}
			}
		}
	}

	// Concrete map: check each value recursively.
	if v.VType.Matches(TMap) && v.Data != nil {
		m := v.AsMap()
		if m != nil {
			for _, key := range m.Keys() {
				val, _ := m.Get(key)
				if IsTypeValue(val) {
					return true
				}
			}
		}
	}

	return false
}
