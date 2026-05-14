package eng

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

// NewReadList wraps a slice of Values as a ReadList. External callers
// (outside the aqleng package) use this constructor because the elems
// field is unexported.
func NewReadList(elems []Value) ReadList {
	return ReadList{elems: elems}
}

// Get returns the element at index i.
// Internal use only — caller must ensure 0 <= i < Len().
func (l ReadList) Get(i int) Value {
	return l.elems[i]
}

// GetOk returns the element at index i and true, or the zero Value and false
// if i is out of bounds. Safe for use at system boundaries.
func (l ReadList) GetOk(i int) (Value, bool) {
	if i < 0 || i >= len(l.elems) {
		return Value{}, false
	}
	return l.elems[i], true
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
	Implicit bool           // true when created from implicit pair syntax (e.g., [x:Integer])
	Meta     map[string]any // optional metadata for parser/engine communication
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

// Keys returns the keys in insertion order (defensive copy).
func (m *OrderedMap) Keys() []string {
	out := make([]string, len(m.keys))
	copy(out, m.keys)
	return out
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

// PathInfo holds the data for a Scalar/Path value.
// A Path represents a filesystem path as a sequence of parts.
// Absolute paths start from the root (Abs = true).
type PathInfo struct {
	Parts []string // path segments (e.g. ["usr", "local", "bin"])
	Abs   bool     // true for absolute paths (e.g. /usr/local/bin)
}

// String returns the OS path string for this path.
func (p PathInfo) String() string {
	joined := strings.Join(p.Parts, "/")
	if p.Abs {
		return "/" + joined
	}
	return joined
}

// ChildTypeInfo holds the child-type constraint for a typed list or
// typed map.
//
// For example, [:String] constrains all list elements to be strings,
// and {:String} constrains all map values to be strings. Elements is
// non-nil when the source carried both literal elements AND a child
// constraint (e.g. `[{x:1} :{x:Integer} {x:2}]`); each element is
// validated against Child by `is` and similar predicates.
type ChildTypeInfo struct {
	Child    Value
	Elements []Value // optional: concrete elements alongside the child constraint
	Entries  []ChildEntry
}

// ChildEntry is a (key, value) pair retained for typed maps that
// carry concrete entries alongside their child constraint
// (`{a:1 :{x:Integer} b:2}`).
type ChildEntry struct {
	Key   string
	Value Value
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
	Type     *Type
	Pattern  *Value // optional: map/list pattern for structural matching
	Optional bool   // true if this param was marked optional via ?
}

// FnSig describes one overload of a function definition.
type FnSig struct {
	Params     []FnParam
	Returns    []*Type // declared return types (nil = unchecked)
	Body       []Value
	BarrierPos int // 0 = no barrier; >0 = forward stops at this position
}

// FnDefInfo holds the parsed function specification for a def-defined function.
// Name is the function's registered name (set by InstallDef). If Registry is
// non-nil, the function was defined in a module and should execute in that
// registry's context (closure semantics).
//
// Signatures is the compiled dispatch table (typed args + Go handlers).
// For Go builtins, Sigs is nil and Signatures holds the native handlers.
// For AQL fn defs, InstallFnDef converts Sigs into Signatures with handler
// closures that splice body tokens. Whether the engine tries forward
// collection is determined per-signature via Signature.BarrierPos —
// derive the word-level summary via fn.HasForwardSigs.
type FnDefInfo struct {
	Name           string
	Sigs           []FnSig     // AQL-defined overloads (nil for Go-implemented words)
	Signatures     []Signature // compiled dispatch table
	MaxForwardArgs int         // longest forward arg count across all sigs (respecting barriers)
	Registry       *Registry
}

// HasForwardSigs reports whether any compiled signature has a non-zero
// BarrierPos — i.e. at least one signature wants to collect args from
// tokens following the word. Used by the dispatcher to decide whether
// pre-evaluation of upcoming parens is worthwhile and whether a
// stack-only retry should be attempted on first-pass match failure.
// The result is derived from the sigs, not stored — Signature is the
// source of truth.
func (fn *FnDefInfo) HasForwardSigs() bool {
	if fn == nil {
		return false
	}
	for i := range fn.Signatures {
		if fn.Signatures[i].BarrierPos > 0 {
			return true
		}
	}
	return false
}

// FnSigSpec describes a signature specification without a body, used for
// targeted undef of specific function signatures.
type FnSigSpec struct {
	Params  []FnParam
	Returns []*Type
}

// FnUndefInfo holds signature specs for targeted undef of function signatures.
type FnUndefInfo struct {
	Sigs []FnSigSpec
}

// ReturnCheckInfo carries expected return types for fn-defined function validation.
type ReturnCheckInfo struct {
	FuncName     string
	Returns      []*Type
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
	Fields          *OrderedMap     // own fields (field name → type-constraint Value)
	Parent          *ObjectTypeInfo // parent object type (nil if direct child of Object)
	ID              string          // unique internal ID: "T_" + 12 hex chars
	Name            string          // full type path (e.g. "Object/Foo/Bar")
	Type            *Type           // canonical *Type identity; populated by MintType during installation
	cachedAllFields *OrderedMap     // lazily computed merged field map (immutable after first call)
}

// AllFields returns all fields including inherited ones. Parent fields come
// first, followed by the type's own fields. Own fields override inherited
// fields with the same name. The result is cached since ObjectTypeInfo is
// immutable after registration.
func (o *ObjectTypeInfo) AllFields() *OrderedMap {
	if o.cachedAllFields != nil {
		return o.cachedAllFields
	}
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
	o.cachedAllFields = result
	return result
}

// ObjectInstanceInfo holds a concrete instance of an object type.
// TypeRef points back to the ObjectTypeInfo that created this instance.
// Fields holds the type's own resolved field values.
// Prototype points to the parent instance (like JavaScript prototypes):
// field lookups fall through to the prototype chain for inherited fields.
type ObjectInstanceInfo struct {
	TypeRef   *ObjectTypeInfo     // the object type this is an instance of
	Fields    *OrderedMap         // own field name → resolved Value
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
	TypeName  string             // full type path, e.g. "Object/Store" or "Object/Store/System"
	Data      map[string]Value   // own key-value pairs (COW layer)
	Prototype *StoreInstanceInfo // prototype chain for key lookup / COW base
	Parent    *StoreInstanceInfo // containing Store (for COW propagation), nil if root
	ParentKey string             // key in Parent that references this Store
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
// AQL code should use the set word which does COW via CowSet.
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
	ID      string                 // generated internal identifier
	Exports map[string]*OrderedMap // export name → export map (name → value)
}

// WordInfo carries the name and optional modifiers for a function reference.
type WordInfo struct {
	Name         string
	ArgCount     int  // -1 = unspecified
	ForceStack   bool // lower/s
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
	ID        string
	VType     *Type
	Data      Payload // the kernel-known data payload; see payload.go for variants
	Quoted    bool   // true when value was produced by the quote word; prevents auto-evaluation
	Eval      bool   // true for parser-created lists that should auto-evaluate at end of Run
	Pos       SrcPos // source position for error reporting (zero value = unknown)
	Undefined bool   // true when atom was created from an undefined word (error if left on result stack)
	Carrier   bool   // true when this is a static-typecheck carrier (type-only, Data stripped of concrete payload)
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
func IDPrefixForType(t *Type) string {
	if t == nil {
		return "T_"
	}
	root := t.Root()
	if root == nil {
		return "T_"
	}
	switch root.Name {
	case "Scalar":
		return "S_"
	case "Node":
		return "N_"
	case "Word":
		return "W_"
	}
	return "T_"
}

// NewValueRaw creates a Value with an auto-generated ID based on the type category.
func NewValueRaw(t *Type, data interface{}) Value {
	return Value{
		ID:    GenerateID(IDPrefixForType(t)),
		VType: t,
		Data:  data,
	}
}

// NewString creates a string value tagged with the appropriate
// String subtype: EmptyString for "" (the unique inhabitant of the
// EmptyString singleton type) and ProperString for any non-empty
// payload. Both subtypes match Scalar/String via the type lattice
// (TStringProper.Matches(TString) and TStringEmpty.Matches(TString)
// are true), so signatures declared on TString continue to dispatch
// transparently — the difference is observable only via typeof,
// pattern dispatch, or explicit subtype-equality checks.
//
// Specific-value dispatch is still primarily routed through
// Signature.Patterns; the empty/proper split provides a coarser
// "value-shape at the type level" signal so user code can branch on
// emptiness without resorting to a length comparison.
func NewString(s string) Value {
	if s == "" {
		return NewValueRaw(TStringEmpty, s)
	}
	return NewValueRaw(TStringProper, s)
}

// NewInteger creates a number/integer value with VType = Scalar/Number/Integer.
// Specific-value dispatch (e.g. `def fact[0] (1)`) routes through
// Signature.Patterns, not through a per-value type-path leaf. See
// the NewString comment for the rationale.
func NewInteger(n int64) Value {
	return NewValueRaw(TInteger, n)
}

// NewDecimal creates a number/decimal value with a float64 payload.
func NewDecimal(f float64) Value {
	return NewValueRaw(TDecimal, f)
}

// FormatDecimal renders a float64 with a guaranteed decimal point so the
// type stays visually distinct from Integer. Uses 'f' format with -1
// precision (shortest round-trip), then appends ".0" when the result
// has neither a fractional part nor an exponent. Float artefacts like
// 0.1 + 0.2 = 0.30000000000000004 are preserved verbatim — see the
// note in spec/SPEC_REPORT.md §2 on the apd-port plan if exact
// decimal arithmetic is required.
func FormatDecimal(f float64) string {
	s := strconv.FormatFloat(f, 'f', -1, 64)
	if !strings.ContainsAny(s, ".eE") {
		s += ".0"
	}
	return s
}

// formatDecimal is the lowercase alias retained for in-package call
// sites that pre-date the exported form.
func formatDecimal(f float64) string { return FormatDecimal(f) }

// NewBoolean creates a boolean value. The boolean payload (true/false) is the
// value; there are no Boolean/True or Boolean/False sub-types.
func NewBoolean(b bool) Value {
	return NewValueRaw(TBoolean, b)
}

// NewList creates a list value from a slice of Values.
func NewList(elems []Value) Value {
	return NewValueRaw(TList, elems)
}

// NewEvalList creates a list value that is marked for auto-evaluation
// at the end of execution. Used by the parser for source-code lists.
func NewEvalList(elems []Value) Value {
	v := NewValueRaw(TList, elems)
	v.Eval = true
	return v
}

// NewTypedList creates a typed list value with a child type constraint.
// For example, NewTypedList(NewTypeLiteral(TString)) represents [:string].
func NewTypedList(child Value) Value {
	return NewValueRaw(TList, ChildTypeInfo{Child: child})
}

// NewMap creates a map value from an ordered map of string keys to Values.
func NewMap(entries *OrderedMap) Value {
	return NewValueRaw(TMap, entries)
}

// NewEvalMap creates a map value marked for auto-evaluation at end of
// execution. Used by the parser for source-code maps.
func NewEvalMap(entries *OrderedMap) Value {
	v := NewValueRaw(TMap, entries)
	v.Eval = true
	return v
}

// NewTypedListWithElements creates a typed list value carrying both
// concrete elements and a child-type constraint. Used by the parser
// for `[v0 :T v1]` syntax. Each element is validated against the
// child constraint by `is` and similar predicates.
func NewTypedListWithElements(child Value, elems []Value) Value {
	return NewValueRaw(TList, ChildTypeInfo{Child: child, Elements: elems})
}

// NewTypedMapWithEntries creates a typed map value carrying both
// concrete (key, value) entries and a child-type constraint. Used by
// the parser for `{k:v :T}` syntax.
func NewTypedMapWithEntries(child Value, entries []ChildEntry) Value {
	return NewValueRaw(TMap, ChildTypeInfo{Child: child, Entries: entries})
}

// NewImplicitMap creates a map value marked as implicit (from pair syntax).
// In fn signatures, implicit maps are treated as named parameter declarations
// (e.g., [x:Integer]), while explicit maps are structural patterns.
func NewImplicitMap(entries *OrderedMap) Value {
	entries.Implicit = true
	return NewValueRaw(TMap, entries)
}

// IsImplicitMap reports whether v is a Map value whose backing
// OrderedMap was constructed from implicit-pair syntax (e.g.
// `{x:Integer}` or `[x:Integer]` inside an fn sig). Used to
// discriminate record-shape patterns from concrete maps.
func (v Value) IsImplicitMap() bool {
	if !v.VType.Equal(TMap) || v.Data == nil {
		return false
	}
	m, ok := v.Data.(*OrderedMap)
	return ok && m != nil && m.Implicit
}

// NewTypedMap creates a typed map value with a child type constraint.
// For example, NewTypedMap(NewTypeLiteral(TString)) represents {:string}.
func NewTypedMap(child Value) Value {
	return NewValueRaw(TMap, ChildTypeInfo{Child: child})
}

// NewRecordType creates a record type value from a field schema.
// The fields map contains field names as keys and type-constraint Values as values.
// For example, record{x:number, y:number} constrains maps to have exactly
// keys x and y with number-typed values.
func NewRecordType(fields *OrderedMap) Value {
	return NewValueRaw(TMap, RecordTypeInfo{Fields: fields})
}

// NewOptionsType creates an options type value from a field schema map.
func NewOptionsType(fields *OrderedMap) Value {
	return NewValueRaw(TMap, OptionsTypeInfo{Fields: fields})
}

// NewTableType creates a table type value from a record type.
// A table type constrains a list so that each element is a map conforming
// to the given record schema.
func NewTableType(record RecordTypeInfo) Value {
	return NewValueRaw(TList, TableTypeInfo{Record: record})
}

// NewAtom creates an atom value from a bare unquoted word.
func NewAtom(name string) Value {
	return NewValueRaw(TAtom, name)
}

// NewPath creates a Path value from parts and an absolute flag.
func NewPath(parts []string, abs bool) Value {
	p := make([]string, len(parts))
	copy(p, parts)
	return NewValueRaw(TPath, PathInfo{Parts: p, Abs: abs})
}

// NewTypeLiteral creates a value representing a type itself (e.g. "number", "string").
// The Data is nil since type literals have no specific literal value.
func NewTypeLiteral(t *Type) Value {
	return NewValueRaw(t, nil)
}

// noneSentinel is the non-nil Data payload that distinguishes the
// VALUE `none` (the unique inhabitant of None) from the TYPE LITERAL
// `None` (which has Data == nil like every other type literal).
// Renderers and the matcher use the Data!=nil discriminator to print
// the value as "none" and the type literal as "None".
type noneSentinel struct{}

// NewNone creates the value `none` — the unique inhabitant of the
// None type. Distinct from NewTypeLiteral(TNone) (the type itself).
func NewNone() Value {
	return NewValueRaw(TNone, noneSentinel{})
}

// Is reports whether v satisfies type t, routed through t.Behavior.
// The canonical dispatch point for "is v a T?" — used by handlers,
// the matcher, and `is` / `guard`. Default Behavior delegates to the
// lattice walk (v.VType.Matches(t)); types with custom Behavior
// override (predicate types invoke their body, record types check
// field-by-field conformance, etc.).
//
// Safe on nil t: returns false. Safe on nil Behavior: callers should
// not encounter this state because every Type registered through
// the kernel paths carries a non-nil Behavior, but the method
// defends against it anyway.
func (v Value) Is(t *Type) bool {
	if t == nil {
		return false
	}
	if t.Behavior == nil {
		return v.VType.Matches(t)
	}
	return t.Behavior.Match(v, t)
}

// IsNone reports whether v is the value `none` (not the None type
// literal). The check distinguishes the inhabitant from the type.
func (v Value) IsNone() bool {
	if !v.VType.Equal(TNone) {
		return false
	}
	_, ok := v.Data.(noneSentinel)
	return ok
}

// NewWord creates a word value (function reference) with no modifiers.
func NewWord(name string) Value {
	return NewValueRaw(TWord, WordInfo{Name: name, ArgCount: -1})
}

// NewWordModified creates a word value with explicit modifiers.
func NewWordModified(name string, argCount int, forceStack, forceForward bool) Value {
	return NewValueRaw(TWord, WordInfo{
		Name:         name,
		ArgCount:     argCount,
		ForceStack:   forceStack,
		ForceForward: forceForward,
	})
}

// NewForward creates a forward primitive value for forward argument tracking.
func NewForward(info ForwardInfo) Value {
	return NewValueRaw(TForward, info)
}

// NewOpenParen creates an open-paren marker value for sub-expression scoping.
func NewOpenParen() Value {
	return NewValueRaw(TOpenParen, nil)
}

// NewCloseParen creates a close-paren marker value. Emitted by the
// parser for `)` so the engine can recognise it by VType identity
// instead of by string compare.
func NewCloseParen() Value {
	return NewValueRaw(TCloseParen, nil)
}

// NewEnd creates an end-marker value (the `end` / `;` keyword).
// Emitted by the parser so the engine can recognise it by VType
// identity instead of by string compare.
func NewEnd() Value {
	return NewValueRaw(TEnd, nil)
}

// NewParenExpr creates a paren expression value containing items to evaluate.
// Used by the parser for paren groups in map/list value positions.
// autoEvalMap evaluates these by running the items in a sub-engine with
// paren markers, producing a single result value.
func NewParenExpr(items []Value) Value {
	return NewValueRaw(TParenExpr, items)
}

// InterpPart represents one segment of an interpolated string.
// If Expr is nil, Lit is a literal string segment.
// If Expr is non-nil, it contains parsed AQL values to evaluate.
type InterpPart struct {
	Lit  string
	Expr []Value
}

// NewInterpString creates an interpolated string value from alternating
// literal and expression parts. The engine evaluates expression parts in
// a sub-engine, converts results to strings, and concatenates everything.
func NewInterpString(parts []InterpPart) Value {
	return NewValueRaw(TInterpString, parts)
}

// NewMark creates a mark value with the given unique ID and the body to
// replay when the corresponding move fires. The body should contain
// the original values between the mark and its paired move.
func NewMark(id string, body ...Value) Value {
	b := make([]Value, len(body))
	copy(b, body)
	return NewValueRaw(TMark, MarkInfo{ID: id, Body: b})
}

// NewMove creates a move value targeting the mark with the given ID.
// The reason string describes why this move exists (used in error messages).
func NewMove(to string, reason string) Value {
	return NewValueRaw(TMove, MoveInfo{To: to, Reason: reason})
}

// NewMoveCont creates a move value with for-loop continuation state.
func NewMoveCont(to, reason string, cont *ForCont) Value {
	return NewValueRaw(TMove, MoveInfo{To: to, Reason: reason, Cont: cont})
}

// NewMoveIf creates a move value with if-statement continuation state.
func NewMoveIf(to, reason string, ifCont *IfCont) Value {
	return NewValueRaw(TMove, MoveInfo{To: to, Reason: reason, IfCont: ifCont})
}

// NewFnDef creates a function definition value for storage on DefStacks.
func NewFnDef(info FnDefInfo) Value {
	return NewValueRaw(TFnDef, info)
}

// NewFunction creates a function reference value. The underlying data is a
// FnDefInfo, but the type is TFunction so it can be matched by function-typed
// parameters and passed to other functions without being called.
func NewFunction(info FnDefInfo) Value {
	return NewValueRaw(TFunction, info)
}

// NewFnUndef creates a function undef spec value for targeted signature removal.
func NewFnUndef(info FnUndefInfo) Value {
	return NewValueRaw(TFnUndef, info)
}

// NewReturnCheck creates a return-check marker for fn return type validation.
func NewReturnCheck(info ReturnCheckInfo) Value {
	return NewValueRaw(TReturnCheck, info)
}

// DefCleanupInfo holds a snapshot of DefStacks lengths taken before fn body
// execution. When the engine encounters a DefCleanup marker, it pops any
// defs that were added during body execution back to the snapshot state.
type DefCleanupInfo struct {
	Snapshot map[string]int
	Registry *Registry
}

// NewDefCleanup creates a def-cleanup marker for fn body local def cleanup.
func NewDefCleanup(info DefCleanupInfo) Value {
	return NewValueRaw(TDefCleanup, info)
}

// NewDisjunct creates a disjunction type value from a list of alternatives.
func NewDisjunct(alternatives []Value) Value {
	return NewValueRaw(TDisjunct, DisjunctInfo{Alternatives: alternatives})
}

// NewEnum creates an Enum value (Type/Disjunct/Enum) — a fixed
// enumeration of named values. Structurally identical to a Disjunct
// (same DisjunctInfo payload) but tagged with the more specific Enum
// type so `typeof` reports `Enum` and the value can be distinguished
// from a general type-disjunct.
func NewEnum(alternatives []Value) Value {
	return NewValueRaw(TEnum, DisjunctInfo{Alternatives: alternatives})
}

// NewObjectType creates an object type value. The caller must
// provide the canonical *Type identity — typically minted via
// r.Types.MintType for named types being installed, or for anonymous
// `object {…}` declarations. The info's Type field is set to t for
// downstream code that needs the parent's *Type when extending.
func NewObjectType(t *Type, info ObjectTypeInfo) Value {
	info.Type = t
	return NewValueRaw(t, info)
}

// NewObjectInstance creates an object instance value of the given
// type. The caller must provide the type's *Type identity (typically
// info.TypeRef.Type for the type currently being instantiated).
func NewObjectInstance(t *Type, info ObjectInstanceInfo) Value {
	return NewValueRaw(t, info)
}

// NewStore creates a Store value of the given type. Pass TStore for
// the builtin Object/Store, TStoreSystem for Object/Store/System, or
// a user-defined *Type for a custom store subtype. The Value's
// TypeName is derived from t.Path() for legacy prototype-chain code
// that compares store types by path string.
func NewStore(t *Type) Value {
	if t == nil {
		t = TStore
	}
	return NewValueRaw(t, &StoreInstanceInfo{
		TypeName: t.Path(),
		Data:     make(map[string]Value),
	})
}

// NewStoreValue wraps an existing StoreInstanceInfo into a Value.
// Pass t = nil to default to TStore.
func NewStoreValue(t *Type, si *StoreInstanceInfo) Value {
	if t == nil {
		t = TStore
	}
	return NewValueRaw(t, si)
}

// NewStoreWithPrototype creates a Store value of the given type with
// a prototype chain. Pass t = nil to default to TStore.
func NewStoreWithPrototype(t *Type, prototype *StoreInstanceInfo) Value {
	if t == nil {
		t = TStore
	}
	return NewValueRaw(t, &StoreInstanceInfo{
		TypeName:  t.Path(),
		Data:      make(map[string]Value),
		Prototype: prototype,
	})
}

// NewArray creates a mutable Array value from a slice of elements.
func NewArray(elems []Value) Value {
	data := make([]Value, len(elems))
	copy(data, elems)
	return NewValueRaw(TArray, &ArrayInstanceInfo{Elems: data})
}

// NewArrayEmpty creates an empty mutable Array value.
func NewArrayEmpty() Value {
	return NewValueRaw(TArray, &ArrayInstanceInfo{Elems: nil})
}

// NewModule creates a module descriptor value.
func NewModule(desc ModuleDesc) Value {
	return NewValueRaw(TModule, desc)
}

// MatrixData holds a dense matrix as a flat float64 slice in row-major order.
type MatrixData struct {
	Data []float64
	Rows int
	Cols int
}

// NewDate creates a Date value from a time.Time (date only, midnight UTC).
func NewDate(t time.Time) Value {
	return NewValueRaw(TDate, t)
}

// AsDate returns the time.Time for a Date value.
func (v Value) AsDate() time.Time {
	if t, ok := v.Data.(time.Time); ok {
		return t
	}
	return time.Time{}
}

// NewDateTime creates a DateTime value from a time.Time (date+time, no timezone).
func NewDateTime(t time.Time) Value {
	return NewValueRaw(TDateTime, t)
}

// AsDateTime returns the time.Time for a DateTime value.
func (v Value) AsDateTime() time.Time {
	if t, ok := v.Data.(time.Time); ok {
		return t
	}
	return time.Time{}
}

// NewInstant creates an Instant value from a time.Time (absolute UTC timestamp).
func NewInstant(t time.Time) Value {
	return NewValueRaw(TInstant, t.UTC())
}

// AsInstant returns the time.Time for an Instant value.
func (v Value) AsInstant() time.Time {
	if t, ok := v.Data.(time.Time); ok {
		return t
	}
	return time.Time{}
}

// NewTimeOfDay creates a TimeOfDay value from a time.Duration (offset from midnight).
func NewTimeOfDay(d time.Duration) Value {
	return NewValueRaw(TTimeOfDay, d)
}

// AsTimeOfDay returns the time.Duration for a TimeOfDay value.
func (v Value) AsTimeOfDay() time.Duration {
	if d, ok := v.Data.(time.Duration); ok {
		return d
	}
	return 0
}

// CalDurationData holds a calendar duration (years, months, days).
type CalDurationData struct {
	Years  int
	Months int
	Days   int
}

// NewCalDuration creates a CalDuration value.
func NewCalDuration(years, months, days int) Value {
	return NewValueRaw(TCalDuration, CalDurationData{Years: years, Months: months, Days: days})
}

// AsCalDuration returns the CalDurationData for a CalDuration value.
func (v Value) AsCalDuration() (CalDurationData, bool) {
	if d, ok := v.Data.(CalDurationData); ok {
		return d, true
	}
	return CalDurationData{}, false
}

// NewClkDuration creates a ClkDuration value from a time.Duration.
func NewClkDuration(d time.Duration) Value {
	return NewValueRaw(TClkDuration, d)
}

// AsClkDuration returns the time.Duration for a ClkDuration value.
func (v Value) AsClkDuration() (time.Duration, bool) {
	if d, ok := v.Data.(time.Duration); ok {
		return d, true
	}
	return 0, false
}

// NewTimezone creates a Timezone value from a *time.Location.
func NewTimezone(loc *time.Location) Value {
	return NewValueRaw(TTimezone, loc)
}

// AsTimezone returns the *time.Location for a Timezone value.
func (v Value) AsTimezone() *time.Location {
	if loc, ok := v.Data.(*time.Location); ok {
		return loc
	}
	return nil
}

// NewMatrix creates a Matrix value from a MatrixData.
func NewMatrix(m MatrixData) Value {
	return NewValueRaw(TMatrix, m)
}

// AsMatrix returns the MatrixData for a Matrix value.
func (v Value) AsMatrix() MatrixData {
	if m, ok := v.Data.(MatrixData); ok {
		return m
	}
	return MatrixData{}
}

// ErrorInfo holds the details of an AQL error value.
type ErrorInfo struct {
	Message string // the error description
}

// NewError creates an error value from a Go error.
func NewError(err error) Value {
	return NewValueRaw(TError, ErrorInfo{Message: err.Error()})
}

// IsError reports whether this value is an error.
func (v Value) IsError() bool {
	return v.VType.Equal(TError)
}

// AsError returns the ErrorInfo for an error value.
func (v Value) AsError() (ErrorInfo, error) {
	info, ok := v.Data.(ErrorInfo)
	if !ok {
		return ErrorInfo{}, fmt.Errorf("AsError: not an error value (got %T)", v.Data)
	}
	return info, nil
}

// TimeoutInfo holds a pending timeout handle.
type TimeoutInfo struct {
	ID    string      // unique identifier
	Ms    int64       // delay in milliseconds
	Timer *time.Timer // underlying Go timer (nil after cancel)
}

// NewTimeout creates a Timeout value.
func NewTimeout(info *TimeoutInfo) Value {
	return NewValueRaw(TTimeout, info)
}

// IsTimeout reports whether this value is a Timeout.
func (v Value) IsTimeout() bool {
	return v.VType.Equal(TTimeout)
}

// AsTimeout returns the TimeoutInfo for a Timeout value.
func (v Value) AsTimeout() (*TimeoutInfo, error) {
	info, ok := v.Data.(*TimeoutInfo)
	if !ok {
		return nil, fmt.Errorf("AsTimeout: not a timeout value (got %T)", v.Data)
	}
	return info, nil
}

// IntervalInfo holds a repeating interval handle.
type IntervalInfo struct {
	ID     string        // unique identifier
	Ms     int64         // interval in milliseconds
	Ticker *time.Ticker  // underlying Go ticker (nil after cancel)
	Done   chan struct{} // closed to signal cancellation
}

// NewInterval creates an Interval value.
func NewInterval(info *IntervalInfo) Value {
	return NewValueRaw(TInterval, info)
}

// IsInterval reports whether this value is an Interval.
func (v Value) IsInterval() bool {
	return v.VType.Equal(TInterval)
}

// AsInterval returns the IntervalInfo for an Interval value.
func (v Value) AsInterval() (*IntervalInfo, error) {
	info, ok := v.Data.(*IntervalInfo)
	if !ok {
		return nil, fmt.Errorf("AsInterval: not an interval value (got %T)", v.Data)
	}
	return info, nil
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

// IsCloseParen reports whether this value is a close-paren marker.
func (v Value) IsCloseParen() bool {
	return v.VType.Equal(TCloseParen)
}

// IsEnd reports whether this value is an end-marker.
func (v Value) IsEnd() bool {
	return v.VType.Equal(TEnd)
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

// IsInterpString reports whether this value is an interpolated string.
func (v Value) IsInterpString() bool {
	return v.VType.Equal(TInterpString)
}

// AsInterpString returns the parts of an interpolated string value.
func (v Value) AsInterpString() []InterpPart {
	if parts, ok := v.Data.([]InterpPart); ok {
		return parts
	}
	return nil
}

// IsMark reports whether this value is a mark.
func (v Value) IsMark() bool {
	return v.VType.Equal(TMark)
}

// AsMark returns the MarkInfo, panics if not a mark.
func (v Value) AsMark() (MarkInfo, error) {
	info, ok := v.Data.(MarkInfo)
	if !ok {
		return MarkInfo{}, fmt.Errorf("AsMark: not a mark value (got %T)", v.Data)
	}
	return info, nil
}

// IsMove reports whether this value is a move.
func (v Value) IsMove() bool {
	return v.VType.Equal(TMove)
}

// AsMove returns the MoveInfo, panics if not a move.
func (v Value) AsMove() (MoveInfo, error) {
	info, ok := v.Data.(MoveInfo)
	if !ok {
		return MoveInfo{}, fmt.Errorf("AsMove: not a move value (got %T)", v.Data)
	}
	return info, nil
}

// IsReturnCheck reports whether this value is a return-check marker.
func (v Value) IsReturnCheck() bool {
	return v.VType.Equal(TReturnCheck)
}

// AsReturnCheck returns the ReturnCheckInfo, panics if not a return-check.
func (v Value) AsReturnCheck() (ReturnCheckInfo, error) {
	info, ok := v.Data.(ReturnCheckInfo)
	if !ok {
		return ReturnCheckInfo{}, fmt.Errorf("AsReturnCheck: not a return-check value (got %T)", v.Data)
	}
	return info, nil
}

// IsDefCleanup reports whether this value is a def-cleanup marker.
func (v Value) IsDefCleanup() bool {
	return v.VType.Equal(TDefCleanup)
}

// AsDefCleanup returns the DefCleanupInfo, panics if not a def-cleanup.
func (v Value) AsDefCleanup() (DefCleanupInfo, error) {
	info, ok := v.Data.(DefCleanupInfo)
	if !ok {
		return DefCleanupInfo{}, fmt.Errorf("AsDefCleanup: not a def-cleanup value (got %T)", v.Data)
	}
	return info, nil
}

// IsDisjunct reports whether this value is a disjunction type — a
// plain Disjunct (Type/Disjunct) or any subtype such as an Enum
// (Type/Disjunct/Enum).
func (v Value) IsDisjunct() bool {
	_, ok := v.Data.(DisjunctInfo)
	return ok && v.VType.Matches(TDisjunct)
}

// AsDisjunct returns the DisjunctInfo, panics if not a disjunct.
func (v Value) AsDisjunct() (DisjunctInfo, error) {
	info, ok := v.Data.(DisjunctInfo)
	if !ok {
		return DisjunctInfo{}, fmt.Errorf("AsDisjunct: not a disjunct value (got %T)", v.Data)
	}
	return info, nil
}

// IsObjectType reports whether this value is an object type definition.
func (v Value) IsObjectType() bool {
	_, ok := v.Data.(ObjectTypeInfo)
	return ok && v.VType.Matches(TObject)
}

// AsObjectType returns the ObjectTypeInfo, panics if not an object type.
func (v Value) AsObjectType() (ObjectTypeInfo, error) {
	info, ok := v.Data.(ObjectTypeInfo)
	if !ok {
		return ObjectTypeInfo{}, fmt.Errorf("AsObjectType: not an object type value (got %T)", v.Data)
	}
	return info, nil
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
func (v Value) AsObjectInstance() (ObjectInstanceInfo, error) {
	info, ok := v.Data.(ObjectInstanceInfo)
	if !ok {
		return ObjectInstanceInfo{}, fmt.Errorf("AsObjectInstance: not an object instance value (got %T)", v.Data)
	}
	return info, nil
}

// IsModule reports whether this value is a module descriptor.
func (v Value) IsModule() bool {
	return v.VType.Equal(TModule)
}

// AsModule returns the ModuleDesc, panics if not a module.
func (v Value) AsModule() (ModuleDesc, error) {
	info, ok := v.Data.(ModuleDesc)
	if !ok {
		return ModuleDesc{}, fmt.Errorf("AsModule: not a module value (got %T)", v.Data)
	}
	return info, nil
}

// IsAtom reports whether this value is an atom.
// IsPath reports whether this value is a Path.
func (v Value) IsPath() bool {
	_, ok := v.Data.(PathInfo)
	return ok && v.VType.Equal(TPath)
}

// AsPath returns the PathInfo, or an error if the value is not a path.
func (v Value) AsPath() (PathInfo, error) {
	info, ok := v.Data.(PathInfo)
	if !ok {
		return PathInfo{}, fmt.Errorf("AsPath: not a path value (got %T)", v.Data)
	}
	return info, nil
}

func (v Value) IsAtom() bool {
	return v.VType.Equal(TAtom)
}

// AsAtom returns the string payload. Returns "" if Data is nil.
func (v Value) AsAtom() (string, error) {
	if v.Data == nil {
		return "", fmt.Errorf("AsAtom: nil data")
	}
	s, ok := v.Data.(string)
	if !ok {
		return "", fmt.Errorf("AsAtom: not an atom value (got %T)", v.Data)
	}
	return s, nil
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
func (v Value) AsRecordType() (RecordTypeInfo, error) {
	info, ok := v.Data.(RecordTypeInfo)
	if !ok {
		return RecordTypeInfo{}, fmt.Errorf("AsRecordType: not a record type value (got %T)", v.Data)
	}
	return info, nil
}

// IsOptionsType reports whether this value is an options type (map with defaults/constraints).
func (v Value) IsOptionsType() bool {
	_, ok := v.Data.(OptionsTypeInfo)
	return ok && v.VType.Equal(TMap)
}

// AsOptionsType returns the OptionsTypeInfo, panics if not an options type.
func (v Value) AsOptionsType() (OptionsTypeInfo, error) {
	info, ok := v.Data.(OptionsTypeInfo)
	if !ok {
		return OptionsTypeInfo{}, fmt.Errorf("AsOptionsType: not an options type value (got %T)", v.Data)
	}
	return info, nil
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
		if _, ok := v.Data.(Materializer); ok {
			return true
		}
	}
	return false
}

// AsTableType returns the TableTypeInfo, panics if not a table type.
func (v Value) AsTableType() (TableTypeInfo, error) {
	if td, ok := v.Data.(TableData); ok {
		return TableTypeInfo{Record: td.Record}, nil
	}
	if mz, ok := v.Data.(Materializer); ok {
		return TableTypeInfo{Record: mz.SourceRecord()}, nil
	}
	info, ok := v.Data.(TableTypeInfo)
	if !ok {
		return TableTypeInfo{}, fmt.Errorf("AsTableType: not a table type value (got %T)", v.Data)
	}
	return info, nil
}

// AsChildType returns the ChildTypeInfo, panics if not a typed list or typed map.
func (v Value) AsChildType() (ChildTypeInfo, error) {
	info, ok := v.Data.(ChildTypeInfo)
	if !ok {
		return ChildTypeInfo{}, fmt.Errorf("AsChildType: not a child type value (got %T)", v.Data)
	}
	return info, nil
}

// AsWord returns the WordInfo, panics if not a word.
func (v Value) AsWord() (WordInfo, error) {
	info, ok := v.Data.(WordInfo)
	if !ok {
		return WordInfo{}, fmt.Errorf("AsWord: not a word value (got %T)", v.Data)
	}
	return info, nil
}

// AsForward returns the ForwardInfo, panics if not a forward.
func (v Value) AsForward() (ForwardInfo, error) {
	info, ok := v.Data.(ForwardInfo)
	if !ok {
		return ForwardInfo{}, fmt.Errorf("AsForward: not a forward value (got %T)", v.Data)
	}
	return info, nil
}

// AsString returns the string payload. Returns "" if Data is nil (type literal).
func (v Value) AsString() (string, error) {
	if v.Data == nil {
		return "", fmt.Errorf("AsString: nil data")
	}
	s, ok := v.Data.(string)
	if !ok {
		return "", fmt.Errorf("AsString: not a string value (got %T)", v.Data)
	}
	return s, nil
}

// AsInteger returns the int64 payload. Returns 0 if Data is nil (type literal).
func (v Value) AsInteger() (int64, error) {
	if v.Data == nil {
		return 0, fmt.Errorf("AsInteger: nil data")
	}
	n, ok := v.Data.(int64)
	if !ok {
		return 0, fmt.Errorf("AsInteger: not an integer value (got %T)", v.Data)
	}
	return n, nil
}

// AsDecimal returns the float64 payload. Returns 0.0 if Data is nil (type literal).
func (v Value) AsDecimal() (float64, error) {
	if v.Data == nil {
		return 0.0, fmt.Errorf("AsDecimal: nil data")
	}
	f, ok := v.Data.(float64)
	if !ok {
		return 0.0, fmt.Errorf("AsDecimal: not a decimal value (got %T)", v.Data)
	}
	return f, nil
}

// AsNumber returns the numeric value as float64 regardless of whether it is
// an integer or decimal.
func (v Value) AsNumber() (float64, error) {
	if v.VType.Matches(TDecimal) {
		f, err := v.AsDecimal()
		return f, err
	}
	n, err := v.AsInteger()
	return float64(n), err
}

// AsBoolean returns the bool payload. Returns false if Data is nil (type literal).
func (v Value) AsBoolean() (bool, error) {
	if v.Data == nil {
		return false, fmt.Errorf("AsBoolean: nil data")
	}
	b, ok := v.Data.(bool)
	if !ok {
		return false, fmt.Errorf("AsBoolean: not a boolean value (got %T)", v.Data)
	}
	return b, nil
}

// AsList returns the []Value payload, or nil if the data is not a []Value.
// Also works for TableData and Materializer, returning the rows.
// For Materializer, this triggers materialization.
// AsList returns a read-only view of the list payload.
// Returns a ReadList with nil backing if the data is not a list.
func (v Value) AsList() ReadList {
	if v.Data == nil {
		return ReadList{}
	}
	if td, ok := v.Data.(TableData); ok {
		return ReadList{elems: td.Rows}
	}
	if mz, ok := v.Data.(Materializer); ok {
		td, err := mz.Materialize()
		if err != nil {
			return ReadList{}
		}
		return ReadList{elems: td.Rows}
	}
	if elems, ok := v.Data.([]Value); ok {
		return ReadList{elems: elems}
	}
	// Typed list carrying both a child constraint and concrete
	// elements (`[v0 :T v1]`). Surface the elements so list-aware
	// operations (lengthq, firstq, is) see them.
	if ci, ok := v.Data.(ChildTypeInfo); ok && len(ci.Elements) > 0 {
		return ReadList{elems: ci.Elements}
	}
	return ReadList{}
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
	if om, ok := v.Data.(*OrderedMap); ok {
		return om
	}
	// Typed map carrying both a child constraint and concrete entries
	// (`{k:v :T}`). Surface the entries as an OrderedMap so map-aware
	// operations see them.
	if ci, ok := v.Data.(ChildTypeInfo); ok && len(ci.Entries) > 0 {
		om := NewOrderedMap()
		for _, e := range ci.Entries {
			om.Set(e.Key, e.Value)
		}
		return om
	}
	return nil
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
	// Behavior-driven format delegation: types that supply a custom
	// TypeBehavior route through their Format. The DefaultBehavior
	// sentinel falls through to the kernel switch below, preserving
	// the fast path for primitive / structural / loop-control values.
	//
	// Type literals (Data==nil) are NOT delegated — they render as
	// their leaf type name uniformly across all types, including
	// types with custom Behaviors. See the Data==nil arm in the
	// switch below.
	if v.Data != nil && v.VType != nil && v.VType.Behavior != nil && v.VType.Behavior != DefaultBehavior {
		return v.VType.Behavior.Format(v)
	}
	switch {
	case v.IsWord():
		w, _ := v.AsWord()
		return fmt.Sprintf("word(%s)", w.Name)
	case v.IsForward():
		f, _ := v.AsForward()
		return fmt.Sprintf("forward(%s,%d/%d)", f.FuncName, f.CollectedArgs, f.ExpectedArgs)
	case v.IsOpenParen():
		return "("
	case v.IsCloseParen():
		return ")"
	case v.IsEnd():
		return "end"
	case v.IsParenExpr():
		return fmt.Sprintf("paren(%v)", v.AsParenExpr())
	case v.IsMark():
		_as2, _ := v.AsMark()
		return fmt.Sprintf("mark(%s)", _as2.ID)
	case v.IsMove():
		m, _ := v.AsMove()
		return fmt.Sprintf("move(%s,%s)", m.To, m.Reason)
	case v.IsReturnCheck():
		rc, _ := v.AsReturnCheck()
		return fmt.Sprintf("returncheck(%s)", rc.FuncName)
	case v.IsDefCleanup():
		return "__dc"
	case v.IsModule():
		md, _ := v.AsModule()
		return fmt.Sprintf("module(%s)", md.ID)
	case v.IsError():
		_as3, _ := v.AsError()
		return fmt.Sprintf("error(%s)", _as3.Message)
	case v.Data == nil:
		// Type literal with no specific value (e.g. "Integer", "List").
		// Render as the LEAF — type names are globally unique so the
		// leaf alone is unambiguous.
		return v.VType.Leaf()
	case v.IsDepScalar():
		// Must come before TString / TInteger / TDecimal matches: the
		// lattice override makes DepString.Matches(TString) (and the
		// numeric counterparts) true, so without this case the value
		// payload would be cast to the wrong concrete type.
		return renderDepScalar(v)
	case v.VType.Matches(TString):
		return fmt.Sprintf("'%s'", v.Data)
	case v.VType.Equal(TAtom):
		s, _ := v.Data.(string)
		return s
	case v.VType.Matches(TDecimal):
		_as4, _ := v.AsDecimal()
		return formatDecimal(_as4)
	case v.VType.Matches(TInteger):
		return fmt.Sprintf("%d", v.Data)
	case v.VType.Matches(TBoolean):
		_as5, _ := v.AsBoolean()
		if _as5 {
			return "true"
		}
		return "false"
	// Domain types (Instant, DateTime, Date, TimeOfDay, CalDuration,
	// ClkDuration, Timezone, Matrix, Timeout, Interval) now render
	// via their per-Type Behavior installed by
	// coretype_format_behaviors.go and dispatched at the top of this
	// function. Their old switch arms have been removed.
	case v.IsPath():
		_as6, _ := v.AsPath()
		return _as6.String()
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
		if mz, ok := v.Data.(Materializer); ok {
			td, err := mz.Materialize()
			if err != nil {
				return "query(error:" + err.Error() + ")"
			}
			v2 := NewValueRaw(TList, td)
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
	// Timeout / Interval render via their per-Type Behavior — see
	// coretype_format_behaviors.go. Their arms have been removed
	// from this switch.
	case v.IsObjectInstance():
		oi, _ := v.AsObjectInstance()
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
		ot, _ := v.AsObjectType()
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
		di, _ := v.AsDisjunct()
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
