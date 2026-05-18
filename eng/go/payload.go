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
// Two strategies for satisfying Payload:
//
//  1. eng-defined struct or pointer types act as payloads directly,
//     with an unexported payloadMarker() method (added to the type's
//     definition file). No wrapper needed — the type's name already
//     conveys its role (WordInfo, ForwardInfo, RecordTypeInfo, …).
//
//  2. Primitives (int64, string, bool, float64), Go-built-in slice
//     types ([]Value, []InterpPart), and external types (time.Time,
//     time.Duration, *time.Location), and types defined in other
//     packages (lang/go/engine's QueryBuilder) cannot carry the
//     unexported marker; they are wrapped in named variant structs
//     declared below (IntPayload, ListPayload, TimePayload, …) or
//     in the catch-all ExtensionPayload{Body: any}.
//
// As of Step 5g, Payload is now a sealed interface. Any code path
// that tries to assign a non-marker-bearing value to Value.Data will
// fail to compile — `Value{Data: "hello"}`, `Value{Data: int64(5)}`,
// `Value{Data: qb}` (where qb is from another package), and similar
// mismatched-shape constructions are rejected at the type-check
// level. The seal is the kernel guarantee that fulfils the
// "make illegal values unrepresentable" goal stated in
// lang/doc/design/TYPE-DECOUPLING.0.md.
type Payload interface {
	payloadMarker()
}

// =================================================================
// Wrapper variants (Step 5b–5c) for types that can't carry methods.
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

// PathPayload wraps a PathInfo for a Scalar/Path value.
type PathPayload struct{ Info PathInfo }

// ListPayload carries the standard []Value payload for a Node/List
// value. Constructed by NewList. Wrapping []Value rather than
// adding a marker to the slice type itself is necessary because Go
// does not allow methods on built-in slice types.
type ListPayload struct{ Elems []Value }

// MapPayload carries the *OrderedMap payload for a Node/Map value.
// Constructed by NewMap. Same wrapping motivation as ListPayload.
type MapPayload struct{ M *OrderedMap }

// ParenExprPayload carries the unevaluated tokens of a paren-expression
// awaiting inline evaluation. Wrapping []Value.
type ParenExprPayload struct{ Toks []Value }

// InterpStringPayload carries the parts of a template-string
// interpolation. Wrapping []InterpPart.
type InterpStringPayload struct{ Parts []InterpPart }

// =================================================================
// External / domain payloads (Step 5f).
// =================================================================

// TimePayload carries a time.Time for Date / DateTime / Instant —
// the VType discriminates which kind it is. Body is interface{}-typed
// here so the eng package doesn't pull the `time` import; the
// dedicated NewDate / NewDateTime / NewInstant constructors handle
// the typed wrapping.
type TimePayload struct {
	T any /* time.Time */
}

// DurationPayload carries a time.Duration for TimeOfDay /
// ClkDuration; same VType-discriminator pattern as TimePayload.
type DurationPayload struct {
	D any /* time.Duration */
}

// TimezonePayload carries a *time.Location for Scalar/Time/Timezone.
type TimezonePayload struct {
	Loc any /* *time.Location */
}

// MaterializerPayload wraps an external Materializer (e.g. the
// production engine's QueryBuilder) into a payload-satisfying
// variant. The Materializer interface itself can't be required to
// satisfy Payload because its implementors live in other packages
// (lang/go/engine), so we wrap instead. AsList unwraps and calls
// .M.Materialize() to surface rows.
type MaterializerPayload struct{ M Materializer }

// =================================================================
// Type-literal / Carrier sentinels.
// =================================================================

// NonePayload is the sentinel for the VALUE `none` (the unique
// inhabitant of None). Replaces the old `noneSentinel` struct so
// it satisfies Payload. The TYPE LITERAL `None` carries Data == nil.
type NonePayload struct{}

// =================================================================
// Extension payload (Step 8 — for plugin types).
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

// =================================================================
// Marker methods.
//
// When Step 5g seals Payload (renames the alias to an interface
// with an unexported marker), every payload-bearing type needs a
// `payloadMarker()` method. The wrapper variants above get one
// each; eng-defined struct payloads (WordInfo, ForwardInfo, …)
// get theirs in the file where they're defined. The collected list
// below documents what's intended to satisfy Payload after sealing
// — it's a checklist, not active code yet.
//
// Wrapper variants:
//   IntPayload, DecPayload, StrPayload, BoolPayload, AtomPayload,
//   PathPayload, ListPayload, MapPayload, ParenExprPayload,
//   InterpStringPayload, TimePayload, DurationPayload,
//   TimezonePayload, NonePayload, ExtensionPayload.
//
// Direct (eng-defined struct or pointer types):
//   WordInfo, ForwardInfo, MarkInfo, MoveInfo, ReturnCheckInfo,
//   DefCleanupInfo, ModuleDesc, FnDefInfo, FnUndefInfo,
//   DisjunctInfo, ChildTypeInfo, RecordTypeInfo, OptionsTypeInfo,
//   TableTypeInfo, TableData, ObjectTypeInfo, ObjectInstanceInfo,
//   *StoreInstanceInfo, *ArrayInstanceInfo, *TimeoutInfo,
//   *IntervalInfo, ErrorInfo, MatrixData, CalDurationData,
//   DepScalarInfo, Materializer (interface), noneSentinel
//   (legacy — to be removed in Step 5f).
// =================================================================

// payloadMarker is the unexported marker method that will close the
// Payload type space once Payload is renamed from an alias to a
// sealed interface (Step 5g). Defined as a no-op on every type that
// gets stored in Value.Data. Defining it here in payload.go keeps
// the catalogue centralised; the methods are dispatch-free.

// Wrapper-variant markers.
func (IntPayload) payloadMarker()          {}
func (DecPayload) payloadMarker()          {}
func (StrPayload) payloadMarker()          {}
func (BoolPayload) payloadMarker()         {}
func (AtomPayload) payloadMarker()         {}
func (PathPayload) payloadMarker()         {}
func (ListPayload) payloadMarker()         {}
func (MapPayload) payloadMarker()          {}
func (ParenExprPayload) payloadMarker()    {}
func (InterpStringPayload) payloadMarker() {}
func (TimePayload) payloadMarker()         {}
func (DurationPayload) payloadMarker()     {}
func (TimezonePayload) payloadMarker()     {}
func (MaterializerPayload) payloadMarker() {}
func (NonePayload) payloadMarker()         {}
func (ExtensionPayload) payloadMarker()    {}

// Direct eng-defined struct markers.
func (WordInfo) payloadMarker()           {}
func (ForwardInfo) payloadMarker()        {}
func (MarkInfo) payloadMarker()           {}
func (MoveInfo) payloadMarker()           {}
func (ReturnCheckInfo) payloadMarker()    {}
func (DefCleanupInfo) payloadMarker()     {}
func (ModuleDesc) payloadMarker()         {}
func (FnDefInfo) payloadMarker()          {}
func (FnUndefInfo) payloadMarker()        {}
func (DisjunctInfo) payloadMarker()       {}
func (ChildTypeInfo) payloadMarker()      {}
func (RecordTypeInfo) payloadMarker()     {}
func (OptionsTypeInfo) payloadMarker()    {}
func (TableTypeInfo) payloadMarker()      {}
func (TableData) payloadMarker()          {}
func (ObjectTypeInfo) payloadMarker()     {}
func (ObjectInstanceInfo) payloadMarker() {}
func (*StoreInstanceInfo) payloadMarker() {}
func (*ArrayInstanceInfo) payloadMarker() {}
func (*TimeoutInfo) payloadMarker()       {}
func (*IntervalInfo) payloadMarker()      {}
func (ErrorInfo) payloadMarker()          {}
func (MatrixData) payloadMarker()         {}
func (CalDurationData) payloadMarker()    {}
func (DepScalarInfo) payloadMarker()      {}
func (PathInfo) payloadMarker()           {} // legacy; replaced by PathPayload at Step 5b but may still flow through some paths

// noneSentinel is kept for backward compat with code that reads it
// directly. NewNone() now produces NonePayload below.
func (noneSentinel) payloadMarker() {}
