// Package engine is a thin shim over the eng module: it re-exports
// eng's types and functions so the surrounding lang codebase can import
// "github.com/aql-lang/aql/lang/go/native" while the actual engine
// machinery lives in the standalone eng module.
//
// Word-defining files (native_*.go, format.go, query.go, sqlite.go,
// fileio.go, conditional.go, forloop.go) continue to live here. Anything
// that's truly engine machinery (Registry, Value, *Type, signatures,
// matching, unification, …) is now in eng.
package native

import (
	"github.com/aql-lang/aql/eng/go"
)

// *Type aliases — every exported type from aqleng is re-exported here.
type (
	AqlError           = eng.AqlError
	ArrayInstanceInfo  = eng.ArrayInstanceInfo
	CalDurationData    = eng.CalDurationData
	CheckDiagnostic    = eng.CheckDiagnostic
	CheckFullStackFunc = eng.CheckFullStackFunc
	CheckSeverity      = eng.CheckSeverity
	CheckState         = eng.CheckState
	ChildTypeInfo      = eng.ChildTypeInfo
	ChildEntry         = eng.ChildEntry
	// Sealed Payload variants (post Step 5).
	Payload             = eng.Payload
	IntPayload          = eng.IntPayload
	DecPayload          = eng.DecPayload
	StrPayload          = eng.StrPayload
	BoolPayload         = eng.BoolPayload
	AtomPayload         = eng.AtomPayload
	PathPayload         = eng.PathPayload
	ListPayload         = eng.ListPayload
	MapPayload          = eng.MapPayload
	ExtensionPayload    = eng.ExtensionPayload
	MaterializerPayload = eng.MaterializerPayload
	TimePayload         = eng.TimePayload
	DurationPayload     = eng.DurationPayload
	TimezonePayload     = eng.TimezonePayload
	DefCleanupInfo      = eng.DefCleanupInfo
	DepBound            = eng.DepBound
	DepKind             = eng.DepKind
	DepScalarInfo       = eng.DepScalarInfo
	DisjunctInfo        = eng.DisjunctInfo
	Engine              = eng.Engine
	ErrorInfo           = eng.ErrorInfo
	FlowCtrl            = eng.FlowCtrl
	FnDefInfo           = eng.FnDefInfo
	FnParam             = eng.FnParam
	FnSig               = eng.FnSig
	FnSigSpec           = eng.FnSigSpec
	FnUndefInfo         = eng.FnUndefInfo
	ForCont             = eng.ForCont
	ForwardInfo         = eng.ForwardInfo
	GuardClause         = eng.GuardClause
	Handler             = eng.Handler
	IfCont              = eng.IfCont
	InterpPart          = eng.InterpPart
	IntervalInfo        = eng.IntervalInfo
	MarkInfo            = eng.MarkInfo
	MatchResult         = eng.MatchResult
	Materializer        = eng.Materializer
	TypeBehavior        = eng.TypeBehavior
	ModuleDesc          = eng.ModuleDesc
	MoveInfo            = eng.MoveInfo
	NativeFunc          = eng.NativeFunc
	NativeSig           = eng.NativeSig
	ObjectInstanceInfo  = eng.ObjectInstanceInfo
	ObjectTypeInfo      = eng.ObjectTypeInfo
	OptionsTypeInfo     = eng.OptionsTypeInfo
	OrderedMap          = eng.OrderedMap
	PathInfo            = eng.PathInfo
	ReadList            = eng.ReadList
	ReadMap             = eng.ReadMap
	RecordTypeInfo      = eng.RecordTypeInfo
	Registry            = eng.Registry
	DefTable            = eng.DefTable
	ReturnCheckInfo     = eng.ReturnCheckInfo
	ReturnsFunc         = eng.ReturnsFunc
	Signature           = eng.Signature
	SrcPos              = eng.SrcPos
	StoreInstanceInfo   = eng.StoreInstanceInfo
	TableData           = eng.TableData
	TableTypeInfo       = eng.TableTypeInfo
	TimeoutInfo         = eng.TimeoutInfo
	TraceCallback       = eng.TraceCallback
	Type                = eng.Type
	Value               = eng.Value
	WordInfo            = eng.WordInfo
)

// Well-known *Type values — re-exported for convenience.
var (
	DefaultBehavior = eng.DefaultBehavior
	TAny            = eng.TAny
	TArray          = eng.TArray
	TAtom           = eng.TAtom
	TBoolean        = eng.TBoolean
	// TCalDuration / TClkDuration / TDate / TDateTime / TDuration
	// moved to lang/go/engine/native_temporal.go (Step 8) — declared
	// directly in this package, no eng alias needed.
	TDecimal    = eng.TDecimal
	TDefCleanup = eng.TDefCleanup
	TDisjunct   = eng.TDisjunct
	TEnum       = eng.TEnum
	TError      = eng.TError
	// TFetchFunction / TFetchRequest / TFetchResponse moved to
	// lang/go/native/fetch.go at Step 8. References use native.TFetch*
	// directly; this aliases block no longer re-exports them.
	TFnDef    = eng.TFnDef
	TFnUndef  = eng.TFnUndef
	TForward  = eng.TForward
	TFunction = eng.TFunction
	TInspect  = eng.TInspect
	// TInstant moved to lang/go/engine/native_temporal.go (Step 8).
	TInteger      = eng.TInteger
	TInternal     = eng.TInternal
	TInterpString = eng.TInterpString
	// TInterval moved to lang/go/engine/native_misc.go (Step 8).
	TList     = eng.TList
	TListArgs = eng.TListArgs
	TMap      = eng.TMap
	TMark     = eng.TMark
	// TMatrix moved to lang/go/internal/nativemod/matrix.go (Step 8).
	TModule         = eng.TModule
	TMove           = eng.TMove
	TNever          = eng.TNever
	TNode           = eng.TNode
	TNone           = eng.TNone
	TNumber         = eng.TNumber
	TObject         = eng.TObject
	TIdeal          = eng.TIdeal
	TOpenParen      = eng.TOpenParen
	TOptions        = eng.TOptions
	TParenExpr      = eng.TParenExpr
	TPath           = eng.TPath
	TRecord         = eng.TRecord
	TResource       = eng.TResource
	TResourceEntity = eng.TResourceEntity
	TReturnCheck    = eng.TReturnCheck
	TScalar         = eng.TScalar
	TStore          = eng.TStore
	TStoreSystem    = eng.TStoreSystem
	TString         = eng.TString
	TStringEmpty    = eng.TStringEmpty
	TStringProper   = eng.TStringProper
	TTable          = eng.TTable
	// TTimeOfDay moved to lang/go/engine/native_temporal.go (Step 8).
	// TTimeout moved to lang/go/engine/native_misc.go (Step 8).
	// TTimezone moved to lang/go/engine/native_temporal.go (Step 8).
	TType = eng.TType
	TWord = eng.TWord
)

// Severity constants for diagnostic classification.
const (
	SeverityError   = eng.SeverityError
	SeverityWarning = eng.SeverityWarning
	SeverityInfo    = eng.SeverityInfo
)

// Engine-level constants.
const (
	CarrierDisjunctCap     = eng.CarrierDisjunctCap
	MaxArgs                = eng.MaxArgs
	DefaultCheckStepBudget = eng.DefaultCheckStepBudget

	DepGT  = eng.DepGT
	DepGTE = eng.DepGTE
	DepLT  = eng.DepLT
	DepLTE = eng.DepLTE
)

// Flow-control signal values exposed by the engine. These travel
// through Registry.FlowCtrl, not the error channel.
const (
	FlowNone     = eng.FlowNone
	FlowBreak    = eng.FlowBreak
	FlowContinue = eng.FlowContinue
)

// Function re-exports — every exported aqleng function.
var (
	AnalyseFnBody             = eng.AnalyseFnBody
	AsAtom                    = eng.AsAtom
	AsArray                   = eng.AsArray
	AsChildType               = eng.AsChildType
	AsDefCleanup              = eng.AsDefCleanup
	AsDisjunct                = eng.AsDisjunct
	AsError                   = eng.AsError
	AsInterpString            = eng.AsInterpString
	AsList                    = eng.AsList
	AsMap                     = eng.AsMap
	AsMark                    = eng.AsMark
	AsModule                  = eng.AsModule
	AsMove                    = eng.AsMove
	AsMutableList             = eng.AsMutableList
	AsMutableMap              = eng.AsMutableMap
	AsObjectInstance          = eng.AsObjectInstance
	AsObjectType              = eng.AsObjectType
	AsOptionsType             = eng.AsOptionsType
	AsParenExpr               = eng.AsParenExpr
	AsRecordType              = eng.AsRecordType
	AsReturnCheck             = eng.AsReturnCheck
	AsStore                   = eng.AsStore
	AsTableType               = eng.AsTableType
	IsArray                   = eng.IsArray
	IsAtom                    = eng.IsAtom
	IsBoolean                 = eng.IsBoolean
	IsCloseParen              = eng.IsCloseParen
	IsDefCleanup              = eng.IsDefCleanup
	IsDisjunct                = eng.IsDisjunct
	IsEnd                     = eng.IsEnd
	IsError                   = eng.IsError
	IsForward                 = eng.IsForward
	IsImplicitMap             = eng.IsImplicitMap
	IsInterpString            = eng.IsInterpString
	IsMark                    = eng.IsMark
	IsModule                  = eng.IsModule
	IsMove                    = eng.IsMove
	IsNone                    = eng.IsNone
	IsNoneShape               = eng.IsNoneShape
	IsObjectInstance          = eng.IsObjectInstance
	IsObjectType              = eng.IsObjectType
	IsOpenParen               = eng.IsOpenParen
	IsOptionsType             = eng.IsOptionsType
	IsParenExpr               = eng.IsParenExpr
	IsPath                    = eng.IsPath
	IsRecordType              = eng.IsRecordType
	IsReturnCheck             = eng.IsReturnCheck
	IsStore                   = eng.IsStore
	IsTableType               = eng.IsTableType
	IsTypeValue               = eng.IsTypeValue
	IsTypedList               = eng.IsTypedList
	IsTypedMap                = eng.IsTypedMap
	IsValueOfType             = eng.IsValueOfType
	IsWord                    = eng.IsWord
	AsBoolean                 = eng.AsBoolean
	AsDecimal                 = eng.AsDecimal
	AsForward                 = eng.AsForward
	AsInteger                 = eng.AsInteger
	AsNumber                  = eng.AsNumber
	AsPath                    = eng.AsPath
	AsString                  = eng.AsString
	AsWord                    = eng.AsWord
	BaseValue                 = eng.BaseValue
	BaseValueForConstraint    = eng.BaseValueForConstraint
	BoundToKind               = eng.BoundToKind
	CoerceBoolean             = eng.CoerceBoolean
	CommonAncestorType        = eng.CommonAncestorType
	CompareValues             = eng.CompareValues
	CowSet                    = eng.CowSet
	DataListElemTypeFromValue = eng.DataListElemTypeFromValue
	ExpandOptionalSigs        = eng.ExpandOptionalSigs
	parseFnParams             = eng.ParseFnParams
	parseFnReturns            = eng.ParseFnReturns
	resolveSigType            = eng.ResolveSigType
	resolveTypeName           = eng.ResolveTypeName
	TandValues                = eng.TandValues
	parseFnDef                = eng.ParseFnDef
	parseFnUndefSpec          = eng.ParseFnUndefSpec
	ValidateWordName          = eng.ValidateWordName
	TypeOf                    = eng.TypeOf
	TypeNameOf                = eng.TypeNameOf
	TypePathOf                = eng.TypePathOf
	ValueType                 = eng.ValueType
	NewNone                   = eng.NewNone
	FormatDecimal             = eng.FormatDecimal
	NewTypedListWithElements  = eng.NewTypedListWithElements
	NewTypedMapWithEntries    = eng.NewTypedMapWithEntries
	FindWordInSource          = eng.FindWordInSource
	FlattenDisjunctAlts       = eng.FlattenDisjunctAlts
	FnDefHasSig               = eng.FnDefHasSig
	FnDefsOverlap             = eng.FnDefsOverlap
	FnSigMatchesSpec          = eng.FnSigMatchesSpec
	FnSigSatisfiesSpec        = eng.FnSigSatisfiesSpec
	FnUndefMatchesFnDef       = eng.FnUndefMatchesFnDef
	BuiltinIDForPath          = eng.BuiltinIDForPath
	MintTestType              = eng.MintTestType
	GenerateID                = eng.GenerateID
	GenerateObjectTypeID      = eng.GenerateObjectTypeID
	IDPrefixForType           = eng.IDPrefixForType
	CanonicalType             = eng.CanonicalType
	ReparentValue             = eng.ReparentValue
	InstallDef                = eng.InstallDef
	InstallFnDef              = eng.InstallFnDef
	IsCapitalisedName         = eng.IsCapitalisedName
	IsConcrete                = eng.IsConcrete
	IsRecordShape             = eng.IsRecordShape
	IsTypeBody                = eng.IsTypeBody
	IsTypeLiteral             = eng.IsTypeLiteral
	JoinCarrierStacks         = eng.JoinCarrierStacks
	JoinCarriers              = eng.JoinCarriers
	KeepFallback              = eng.KeepFallback
	MakeAqlError              = eng.MakeAqlError
	MapFieldBoolean           = eng.MapFieldBoolean
	MapFieldDecimal           = eng.MapFieldDecimal
	MapFieldInteger           = eng.MapFieldInteger
	MapFieldString            = eng.MapFieldString
	MatchSignature            = eng.MatchSignature
	NewValueRaw               = eng.NewValueRaw
	LiteralCondValue          = eng.LiteralCondValue
	BoolWord                  = eng.BoolWord
	ApplyGuardNarrowing       = eng.ApplyGuardNarrowing
	ApplyComplementNarrowing  = eng.ApplyComplementNarrowing
	RunCarrierBodyWithDefs    = eng.RunCarrierBodyWithDefs
	InstallJoinedDefs         = eng.InstallJoinedDefs
	New                       = eng.New
	NewArray                  = eng.NewArray
	NewArrayEmpty             = eng.NewArrayEmpty
	NewReadList               = eng.NewReadList
	ContextStoreLookup        = eng.ContextStoreLookup
	ExactEqual                = eng.ExactEqual
	TraceWrap                 = eng.TraceWrap
	FlexibleMatch             = eng.FlexibleMatch
	TraceVisibleLen           = eng.TraceVisibleLen
	TraceColorize             = eng.TraceColorize
	RunTrace                  = eng.RunTrace
	PadRight                  = eng.PadRight
	DeepEqual                 = eng.DeepEqual
	FormatForPrint            = eng.FormatForPrint
	FormatValueJSON           = eng.FormatValueJSON
	NewAtom                   = eng.NewAtom
	NewBoolean                = eng.NewBoolean
	// NewCalDuration moved to lang/go/engine/native_temporal.go (Step 8).
	NewCarrier               = eng.NewCarrier
	NewCarrierTypedList      = eng.NewCarrierTypedList
	NewCarrierTypedListValue = eng.NewCarrierTypedListValue
	// NewClkDuration moved to lang/go/engine/native_temporal.go (Step 8).
	// NewDate / NewDateTime moved to lang/go/engine/native_temporal.go (Step 8).
	NewDecimal     = eng.NewDecimal
	NewDefCleanup  = eng.NewDefCleanup
	NewDepScalar   = eng.NewDepScalar
	NewDisjunct    = eng.NewDisjunct
	NewEnum        = eng.NewEnum
	NewError       = eng.NewError
	NewEvalList    = eng.NewEvalList
	NewEvalMap     = eng.NewEvalMap
	NewFnDef       = eng.NewFnDef
	NewFnUndef     = eng.NewFnUndef
	NewForward     = eng.NewForward
	NewFunction    = eng.NewFunction
	NewImplicitMap = eng.NewImplicitMap
	// NewInstant moved to lang/go/engine/native_temporal.go (Step 8).
	NewInteger      = eng.NewInteger
	NewInterpString = eng.NewInterpString
	// NewInterval moved to lang/go/engine/native_misc.go (Step 8).
	NewList               = eng.NewList
	NewMap                = eng.NewMap
	NewMark               = eng.NewMark
	NewModule             = eng.NewModule
	NewMove               = eng.NewMove
	NewMoveCont           = eng.NewMoveCont
	NewMoveIf             = eng.NewMoveIf
	NewObjectInstance     = eng.NewObjectInstance
	NewObjectType         = eng.NewObjectType
	NewOpenParen          = eng.NewOpenParen
	NewCloseParen         = eng.NewCloseParen
	NewEnd                = eng.NewEnd
	NewOptionsType        = eng.NewOptionsType
	NewOrderedMap         = eng.NewOrderedMap
	NewParenExpr          = eng.NewParenExpr
	NewPath               = eng.NewPath
	NewRecordType         = eng.NewRecordType
	NewRegistry           = eng.NewRegistry
	NewReturnCheck        = eng.NewReturnCheck
	NewStore              = eng.NewStore
	NewStoreValue         = eng.NewStoreValue
	NewStoreWithPrototype = eng.NewStoreWithPrototype
	NewString             = eng.NewString
	NewTableType          = eng.NewTableType
	// NewTimeOfDay moved to lang/go/engine/native_temporal.go (Step 8).
	// NewTimeout moved to lang/go/engine/native_misc.go (Step 8).
	// NewTimezone moved to lang/go/engine/native_temporal.go (Step 8).
	NewTop                = eng.NewTop
	NewType               = eng.NewType
	NewTypeLiteral        = eng.NewTypeLiteral
	NewTypedList          = eng.NewTypedList
	NewTypedMap           = eng.NewTypedMap
	NewWord               = eng.NewWord
	NewWordModified       = eng.NewWordModified
	NextMarkID            = eng.NextMarkID
	OpenUnifyMap          = eng.OpenUnifyMap
	RankSignatures        = eng.RankSignatures
	UnaryNumOpNative      = eng.UnaryNumOpNative
	BinaryNumOpNative     = eng.BinaryNumOpNative
	BinaryIntOpNative     = eng.BinaryIntOpNative
	RequireConcreteList   = eng.RequireConcreteList
	RequireConcreteMap    = eng.RequireConcreteMap
	ResolveTypeLiteralDef = eng.ResolveTypeLiteralDef
	ResolveTypePath       = eng.ResolveTypePath
	ResolveWordValue      = eng.ResolveWordValue
	ResolveWordsDeep      = eng.ResolveWordsDeep
	ReturnsIdentity       = eng.ReturnsIdentity
	ReturnsListElemAt     = eng.ReturnsListElemAt
	ReturnsNumericBinary  = eng.ReturnsNumericBinary
	ReturnsPreserveListAt = eng.ReturnsPreserveListAt
	ReturnsStatic         = eng.ReturnsStatic
	RunCarrierBody        = eng.RunCarrierBody
	SetIDSeed             = eng.SetIDSeed
	SeverityFor           = eng.SeverityFor
	CompareSignatures     = eng.CompareSignatures
	SimplifyDisjunctAlts  = eng.SimplifyDisjunctAlts
	SizeOf                = eng.SizeOf
	SortSignatures        = eng.SortSignatures
	StoreKey              = eng.StoreKey
	StripToCarriers       = eng.StripToCarriers
	TypeNameTable         = eng.TypeNameTable
	Unify                 = eng.Unify
	UninstallDef          = eng.UninstallDef
	UninstallFnSigs       = eng.UninstallFnSigs
	ValToString           = eng.ValToString
	ValidateTypeNameParts = eng.ValidateTypeNameParts
	ValuesEqual           = eng.ValuesEqual
	WithPos               = eng.WithPos
	// `make` helpers, ported alongside the make word in eng/go/core_make.go.
	makeConvert      = eng.MakeConvert
	makeFieldValue   = eng.MakeFieldValue
	ResolveFieldType = eng.ResolveFieldType
	// `get`/`set` helper, ported with those words to eng/go/core_storage.go.
	getKey = eng.GetKey
)
