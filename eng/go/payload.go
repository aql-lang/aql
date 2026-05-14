package eng

// Payload is the static type of Value.Data — the kernel-known shape
// of the data a Value carries. During the type-decoupling migration
// (Step 5 of TYPE-DECOUPLING.0.md), Payload is a `= any` alias so
// constructors and type assertions can migrate one batch at a time
// without breaking the build. The final commit of Step 5 will
// rename this to a sealed interface (an unexported marker method
// on each variant), at which point the compiler catches any
// remaining non-variant assignment to Value.Data.
//
// The transition is structured so that at every intermediate commit:
//
//   - Every NewX constructor that has been migrated produces the
//     new variant payload (e.g. IntPayload{N: 5}).
//   - The corresponding AsX accessor asserts the variant payload
//     first, then falls through to the legacy raw payload if a
//     caller built a Value through some other path.
//   - Tests pass.
//
// Once every constructor and every assertion is on variants, the
// fallback paths in AsX get removed and Payload is sealed.
type Payload = any

// =================================================================
// Primitive payloads (Step 5b)
// =================================================================

// IntPayload carries the int64 payload for a Scalar/Number/Integer
// value. Constructed by NewInteger.
type IntPayload struct{ N int64 }

// DecPayload carries the float64 payload for a Scalar/Number/Decimal
// value. Constructed by NewDecimal.
type DecPayload struct{ F float64 }

// StrPayload carries the string payload for a Scalar/String value.
// Constructed by NewString.
type StrPayload struct{ S string }

// BoolPayload carries the bool payload for a Scalar/Boolean value.
// Constructed by NewBoolean.
type BoolPayload struct{ B bool }

// AtomPayload carries the unquoted-name payload for a Scalar/Atom
// value. Constructed by NewAtom.
type AtomPayload struct{ Name string }

// =================================================================
// Structural payloads (Step 5c)
// =================================================================

// ListPayload carries the standard []Value payload for a Node/List
// value. Constructed by NewList.
type ListPayload struct{ Elems []Value }

// MapPayload carries the *OrderedMap payload for a Node/Map value.
// Constructed by NewMap.
type MapPayload struct{ M *OrderedMap }

// =================================================================
// Path payload (Step 5b)
// =================================================================

// PathPayload carries the PathInfo payload for a Scalar/Path value.
// Constructed by NewPath.
type PathPayload struct{ Info PathInfo }

// =================================================================
// Word / control-token payloads (Step 5d)
// =================================================================

// WordPayload carries the WordInfo payload for a Word value (a
// callable word name in the source program).
type WordPayload struct{ Info WordInfo }

// ForwardPayload carries the ForwardInfo payload for a Forward
// marker — the interpreter loop's "pending forward-arg collection"
// state.
type ForwardPayload struct{ Info ForwardInfo }

// MarkPayload carries the MarkInfo payload for a Mark control token
// (loop / iteration anchor).
type MarkPayload struct{ Info MarkInfo }

// MovePayload carries the MoveInfo payload for a Move control token
// (jump to a Mark).
type MovePayload struct{ Info MoveInfo }

// ReturnCheckPayload carries the ReturnCheckInfo payload for a
// Returncheck token (function-return type guard).
type ReturnCheckPayload struct{ Info ReturnCheckInfo }

// DefCleanupPayload carries the DefCleanupInfo payload for a
// DefCleanup control token (scoped def removal).
type DefCleanupPayload struct{ Info DefCleanupInfo }

// ParenExprPayload carries the token-list payload for a paren
// expression awaiting inline evaluation.
type ParenExprPayload struct{ Toks []Value }

// InterpStringPayload carries the parts of a template-string
// interpolation.
type InterpStringPayload struct{ Parts []InterpPart }

// ModulePayload carries the ModuleDesc payload for a Module value
// (the result of `'foo.aql' module`).
type ModulePayload struct{ Desc ModuleDesc }

// =================================================================
// Object payloads (Step 5e)
// =================================================================

// StorePayload carries the *StoreInstanceInfo payload for an
// Object/Store value (mutable key-value store).
type StorePayload struct{ Info *StoreInstanceInfo }

// ArrayPayload carries the *ArrayInstanceInfo payload for an
// Object/Array value (mutable typed array).
type ArrayPayload struct{ Info *ArrayInstanceInfo }

// ObjectInstancePayload carries an ObjectInstanceInfo payload for
// an Object/* user instance (created via `make Foo {…}`).
type ObjectInstancePayload struct{ Info ObjectInstanceInfo }

// ObjectTypePayload carries an ObjectTypeInfo payload — a type
// declaration like `type Foo object {…}`.
type ObjectTypePayload struct{ Info ObjectTypeInfo }

// ErrorPayload carries an ErrorInfo payload (an Error value
// produced when a handler returns an AqlError).
type ErrorPayload struct{ Info ErrorInfo }

// =================================================================
// Structural type payloads (Step 5c continued — record, options,
// table, child-type, disjunct shapes)
// =================================================================

// RecordTypePayload carries a RecordTypeInfo — a `type Foo record
// {…}` declaration.
type RecordTypePayload struct{ Info RecordTypeInfo }

// OptionsTypePayload carries an OptionsTypeInfo — an options-map
// schema.
type OptionsTypePayload struct{ Info OptionsTypeInfo }

// TableTypePayload carries a TableTypeInfo — a table-shape type.
type TableTypePayload struct{ Info TableTypeInfo }

// TableDataPayload carries TableData — the concrete rows of a
// table value.
type TableDataPayload struct{ Data TableData }

// MaterializerPayload carries a Materializer — a deferred query
// value that produces rows on demand.
type MaterializerPayload struct{ M Materializer }

// ChildTypePayload carries a ChildTypeInfo — a typed list / map
// shape like `[:T]` or `{:T}`, possibly with concrete elements
// or entries.
type ChildTypePayload struct{ Info ChildTypeInfo }

// DisjunctPayload carries a DisjunctInfo — a union type like
// `Integer | String`, including enum subtypes.
type DisjunctPayload struct{ Info DisjunctInfo }

// =================================================================
// DepScalar payload (Step 5f)
// =================================================================

// DepScalarPayload carries a DepScalarInfo — the bounded-scalar
// constraint payload for a Type/Dependent/Dep<X> value.
type DepScalarPayload struct{ Info DepScalarInfo }

// =================================================================
// None payload (the unique inhabitant of None)
// =================================================================

// NonePayload is the Data-payload sentinel that distinguishes the
// VALUE `none` (the unique inhabitant of None) from the TYPE LITERAL
// `None` (which has Data == nil). Renderers and the matcher use
// the Data!=nil discriminator.
type NonePayload struct{}

// =================================================================
// Domain payloads (Step 5f — these move out to nativemod at Step 8)
// =================================================================

// TimePayload carries a time.Time for Date / DateTime / Instant.
// The VType discriminates which kind it is.
type TimePayload struct{ T any /* time.Time, kept as any to avoid pulling time import here */ }

// DurationPayload carries a time.Duration for TimeOfDay /
// ClkDuration. The VType discriminates which kind it is.
type DurationPayload struct{ D any /* time.Duration */ }

// CalDurationPayload carries a CalDurationData (years/months/days).
type CalDurationPayload struct{ Data CalDurationData }

// TimezonePayload carries a *time.Location for Scalar/Time/Timezone.
type TimezonePayload struct{ Loc any /* *time.Location */ }

// MatrixPayload carries a MatrixData for Scalar/Number/Matrix.
type MatrixPayload struct{ Data MatrixData }

// TimeoutPayload carries a *TimeoutInfo for Object/Timeout.
type TimeoutPayload struct{ Info *TimeoutInfo }

// IntervalPayload carries an *IntervalInfo for Object/Interval.
type IntervalPayload struct{ Info *IntervalInfo }

// =================================================================
// Extension payload (Step 8 — for plugin types)
// =================================================================

// ExtensionPayload is the one explicit escape hatch from the closed
// Payload type space. Plugin types — Color, Date as an external,
// any host-supplied domain type — flow through this variant. The
// Body is opaque to the kernel; only the owning type's
// TypeBehavior dereferences it. Use `eng.NewExtension(t, body)` to
// construct.
//
// Inside the owning module, the Body assertion is unsafe in
// principle but lives in exactly one place (the module's Behavior
// implementation) and is tested there.
type ExtensionPayload struct{ Body any }

// NewExtension constructs a Value carrying an ExtensionPayload.
// Plugin modules use this when introducing a new type that flows
// through ExtensionPayload rather than dedicated kernel variants.
func NewExtension(t *Type, body any) Value {
	return Value{
		ID:    GenerateID(IDPrefixForType(t)),
		VType: t,
		Data:  ExtensionPayload{Body: body},
	}
}
