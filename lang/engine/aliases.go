// Package engine is a thin shim over the eng module: it re-exports
// eng's types and functions so the surrounding lang codebase can import
// "github.com/aql-lang/aql/lang/engine" while the actual engine
// machinery lives in the standalone eng module.
//
// Word-defining files (native_*.go, format.go, query.go, sqlite.go,
// fileio.go, conditional.go, forloop.go) continue to live here. Anything
// that's truly engine machinery (Registry, Value, Type, signatures,
// matching, unification, …) is now in eng.
package engine

import (
	"github.com/aql-lang/aql/eng"
)

// Type aliases — every exported type from aqleng is re-exported here.
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
	DefCleanupInfo     = eng.DefCleanupInfo
	DepBound           = eng.DepBound
	DepKind            = eng.DepKind
	DepScalarInfo      = eng.DepScalarInfo
	DisjunctInfo       = eng.DisjunctInfo
	Engine             = eng.Engine
	ErrorInfo          = eng.ErrorInfo
	FnDefInfo          = eng.FnDefInfo
	FnParam            = eng.FnParam
	FnSig              = eng.FnSig
	FnSigSpec          = eng.FnSigSpec
	FnUndefInfo        = eng.FnUndefInfo
	ForCont            = eng.ForCont
	ForwardInfo        = eng.ForwardInfo
	GuardClause        = eng.GuardClause
	Handler            = eng.Handler
	IfCont             = eng.IfCont
	InterpPart         = eng.InterpPart
	IntervalInfo       = eng.IntervalInfo
	MarkInfo           = eng.MarkInfo
	MatchResult        = eng.MatchResult
	Materializer       = eng.Materializer
	MatrixData         = eng.MatrixData
	ModuleDesc         = eng.ModuleDesc
	MoveInfo           = eng.MoveInfo
	NativeFunc         = eng.NativeFunc
	NativeSig          = eng.NativeSig
	ObjectInstanceInfo = eng.ObjectInstanceInfo
	ObjectTypeInfo     = eng.ObjectTypeInfo
	OptionsTypeInfo    = eng.OptionsTypeInfo
	OrderedMap         = eng.OrderedMap
	PathInfo           = eng.PathInfo
	ReadList           = eng.ReadList
	ReadMap            = eng.ReadMap
	RecordTypeInfo     = eng.RecordTypeInfo
	Registry           = eng.Registry
	ReturnCheckInfo    = eng.ReturnCheckInfo
	ReturnsFunc        = eng.ReturnsFunc
	Signature          = eng.Signature
	SrcPos             = eng.SrcPos
	StoreInstanceInfo  = eng.StoreInstanceInfo
	TableData          = eng.TableData
	TableTypeInfo      = eng.TableTypeInfo
	TimeoutInfo        = eng.TimeoutInfo
	TraceCallback      = eng.TraceCallback
	Type               = eng.Type
	Value              = eng.Value
	WordInfo           = eng.WordInfo
)

// Well-known Type values — re-exported for convenience.
var (
	TAny            = eng.TAny
	TArray          = eng.TArray
	TAtom           = eng.TAtom
	TBoolean        = eng.TBoolean
	TCalDuration    = eng.TCalDuration
	TClkDuration    = eng.TClkDuration
	TDate           = eng.TDate
	TDateTime       = eng.TDateTime
	TDecimal        = eng.TDecimal
	TDefCleanup     = eng.TDefCleanup
	TDepInteger     = eng.TDepInteger
	TDependent      = eng.TDependent
	TDisjunct       = eng.TDisjunct
	TDuration       = eng.TDuration
	TError          = eng.TError
	TFetchFunction  = eng.TFetchFunction
	TFetchRequest   = eng.TFetchRequest
	TFetchResponse  = eng.TFetchResponse
	TFnDef          = eng.TFnDef
	TFnUndef        = eng.TFnUndef
	TForward        = eng.TForward
	TFunction       = eng.TFunction
	TInspect        = eng.TInspect
	TInstant        = eng.TInstant
	TInteger        = eng.TInteger
	TInternal       = eng.TInternal
	TInterpString   = eng.TInterpString
	TInterval       = eng.TInterval
	TList           = eng.TList
	TListArgs       = eng.TListArgs
	TMap            = eng.TMap
	TMark           = eng.TMark
	TMatrix         = eng.TMatrix
	TModule         = eng.TModule
	TMove           = eng.TMove
	TNever          = eng.TNever
	TNode           = eng.TNode
	TNodeType       = eng.TNodeType
	TNone           = eng.TNone
	TNumber         = eng.TNumber
	TObject         = eng.TObject
	TObjectType     = eng.TObjectType
	TOpenParen      = eng.TOpenParen
	TOptions        = eng.TOptions
	TParenExpr      = eng.TParenExpr
	TPath           = eng.TPath
	TRecord         = eng.TRecord
	TResource       = eng.TResource
	TResourceEntity = eng.TResourceEntity
	TReturnCheck    = eng.TReturnCheck
	TScalar         = eng.TScalar
	TScalarType     = eng.TScalarType
	TStore          = eng.TStore
	TStoreSystem    = eng.TStoreSystem
	TString         = eng.TString
	TStringEmpty    = eng.TStringEmpty
	TStringProper   = eng.TStringProper
	TTable          = eng.TTable
	TTimeOfDay      = eng.TTimeOfDay
	TTimeout        = eng.TTimeout
	TTimezone       = eng.TTimezone
	TType           = eng.TType
	TWord           = eng.TWord
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

// Sentinel values exposed by the engine.
var (
	BuiltinTypeIDs = eng.BuiltinTypeIDs
	ErrBreak       = eng.ErrBreak
	ErrContinue    = eng.ErrContinue
)

// Function re-exports — every exported aqleng function.
var (
	AnalyseFnBody              = eng.AnalyseFnBody
	BaseValue                  = eng.BaseValue
	BaseValueForConstraint     = eng.BaseValueForConstraint
	BoundToKind                = eng.BoundToKind
	CoerceBoolean              = eng.CoerceBoolean
	CommonAncestorType         = eng.CommonAncestorType
	CompareValues              = eng.CompareValues
	CowSet                     = eng.CowSet
	DataListElemTypeFromValue  = eng.DataListElemTypeFromValue
	DependentLeafBaseType      = eng.DependentLeafBaseType
	DependentLeafFromType      = eng.DependentLeafFromType
	ExpandOptionalSigs         = eng.ExpandOptionalSigs
	parseFnParams              = eng.ParseFnParams
	parseFnReturns             = eng.ParseFnReturns
	resolveSigType             = eng.ResolveSigType
	resolveTypeName            = eng.ResolveTypeName
	lookupDefType              = eng.LookupDefType
	resolveDefType             = eng.ResolveDefType
	TandValues                 = eng.TandValues
	parseFnDef                 = eng.ParseFnDef
	parseFnUndefSpec           = eng.ParseFnUndefSpec
	outputSigIsConcreteReturns = eng.OutputSigIsConcreteReturns
	isSigTypeValue             = eng.IsSigTypeValue
	outputSigValues            = eng.OutputSigValues
	ValidateWordName           = eng.ValidateWordName
	TypeOf                     = eng.TypeOf
	NewNone                    = eng.NewNone
	FormatDecimal              = eng.FormatDecimal
	NewTypedListWithElements   = eng.NewTypedListWithElements
	NewTypedMapWithEntries     = eng.NewTypedMapWithEntries
	FindWordInSource           = eng.FindWordInSource
	FlattenDisjunctAlts        = eng.FlattenDisjunctAlts
	FnDefHasSig                = eng.FnDefHasSig
	FnDefsOverlap              = eng.FnDefsOverlap
	FnSigMatchesSpec           = eng.FnSigMatchesSpec
	FnSigSatisfiesSpec         = eng.FnSigSatisfiesSpec
	FnUndefMatchesFnDef        = eng.FnUndefMatchesFnDef
	FormatFixedTypeID          = eng.FormatFixedTypeID
	GenerateID                 = eng.GenerateID
	GenerateObjectTypeID       = eng.GenerateObjectTypeID
	IDPrefixForParts           = eng.IDPrefixForParts
	IDPrefixForType            = eng.IDPrefixForType
	InstallDef                 = eng.InstallDef
	InstallFnDef               = eng.InstallFnDef
	IsBreak                    = eng.IsBreak
	IsCapitalisedName          = eng.IsCapitalisedName
	IsConcrete                 = eng.IsConcrete
	IsContinue                 = eng.IsContinue
	IsMetaType                 = eng.IsMetaType
	IsRecordShape              = eng.IsRecordShape
	IsTypeBody                 = eng.IsTypeBody
	IsTypeLiteral              = eng.IsTypeLiteral
	IsTypeValue                = eng.IsTypeValue
	JoinCarrierStacks          = eng.JoinCarrierStacks
	JoinCarriers               = eng.JoinCarriers
	KeepFallback               = eng.KeepFallback
	MakeAqlError               = eng.MakeAqlError
	MapFieldBoolean            = eng.MapFieldBoolean
	MapFieldDecimal            = eng.MapFieldDecimal
	MapFieldInteger            = eng.MapFieldInteger
	MapFieldString             = eng.MapFieldString
	MatchSignature             = eng.MatchSignature
	MetatypeFor                = eng.MetatypeFor
	NewValueRaw                = eng.NewValueRaw
	LiteralCondValue           = eng.LiteralCondValue
	BoolWord                   = eng.BoolWord
	ApplyGuardNarrowing        = eng.ApplyGuardNarrowing
	ApplyComplementNarrowing   = eng.ApplyComplementNarrowing
	RunCarrierBodyWithDefs     = eng.RunCarrierBodyWithDefs
	InstallJoinedDefs          = eng.InstallJoinedDefs
	New                        = eng.New
	NewArray                   = eng.NewArray
	NewArrayEmpty              = eng.NewArrayEmpty
	NewReadList                = eng.NewReadList
	ContextStoreLookup         = eng.ContextStoreLookup
	ExactEqual                 = eng.ExactEqual
	TraceWrap                  = eng.TraceWrap
	FlexibleMatch              = eng.FlexibleMatch
	TraceVisibleLen            = eng.TraceVisibleLen
	TraceColorize              = eng.TraceColorize
	RunTrace                   = eng.RunTrace
	PadRight                   = eng.PadRight
	DeepEqual                  = eng.DeepEqual
	FormatForPrint             = eng.FormatForPrint
	FormatValueJSON            = eng.FormatValueJSON
	NewAtom                    = eng.NewAtom
	NewBoolean                 = eng.NewBoolean
	NewCalDuration             = eng.NewCalDuration
	NewCarrier                 = eng.NewCarrier
	NewCarrierTypedList        = eng.NewCarrierTypedList
	NewCarrierTypedListValue   = eng.NewCarrierTypedListValue
	NewClkDuration             = eng.NewClkDuration
	NewDate                    = eng.NewDate
	NewDateTime                = eng.NewDateTime
	NewDecimal                 = eng.NewDecimal
	NewDefCleanup              = eng.NewDefCleanup
	NewDepScalar               = eng.NewDepScalar
	NewDisjunct                = eng.NewDisjunct
	NewError                   = eng.NewError
	NewEvalList                = eng.NewEvalList
	NewEvalMap                 = eng.NewEvalMap
	NewFnDef                   = eng.NewFnDef
	NewFnUndef                 = eng.NewFnUndef
	NewForward                 = eng.NewForward
	NewFunction                = eng.NewFunction
	NewImplicitMap             = eng.NewImplicitMap
	NewInstant                 = eng.NewInstant
	NewInteger                 = eng.NewInteger
	NewInterpString            = eng.NewInterpString
	NewInterval                = eng.NewInterval
	NewList                    = eng.NewList
	NewMap                     = eng.NewMap
	NewMark                    = eng.NewMark
	NewMatrix                  = eng.NewMatrix
	NewModule                  = eng.NewModule
	NewMove                    = eng.NewMove
	NewMoveCont                = eng.NewMoveCont
	NewMoveIf                  = eng.NewMoveIf
	NewObjectInstance          = eng.NewObjectInstance
	NewObjectType              = eng.NewObjectType
	NewOpenParen               = eng.NewOpenParen
	NewOptionsType             = eng.NewOptionsType
	NewOrderedMap              = eng.NewOrderedMap
	NewParenExpr               = eng.NewParenExpr
	NewPath                    = eng.NewPath
	NewRecordType              = eng.NewRecordType
	NewRegistry                = eng.NewRegistry
	NewReturnCheck             = eng.NewReturnCheck
	NewStore                   = eng.NewStore
	NewStoreValue              = eng.NewStoreValue
	NewStoreWithPrototype      = eng.NewStoreWithPrototype
	NewString                  = eng.NewString
	NewTableType               = eng.NewTableType
	NewTimeOfDay               = eng.NewTimeOfDay
	NewTimeout                 = eng.NewTimeout
	NewTimezone                = eng.NewTimezone
	NewTop                     = eng.NewTop
	NewType                    = eng.NewType
	NewTypeLiteral             = eng.NewTypeLiteral
	NewTypedList               = eng.NewTypedList
	NewTypedMap                = eng.NewTypedMap
	NewWord                    = eng.NewWord
	NewWordModified            = eng.NewWordModified
	NextMarkID                 = eng.NextMarkID
	OpenUnifyMap               = eng.OpenUnifyMap
	RankSignatures             = eng.RankSignatures
	UnaryNumOpNative           = eng.UnaryNumOpNative
	BinaryNumOpNative          = eng.BinaryNumOpNative
	BinaryIntOpNative          = eng.BinaryIntOpNative
	ComparisonNatives          = eng.ComparisonNatives
	PrintNatives               = eng.PrintNatives
	TraceNatives               = eng.TraceNatives
	UnifyNatives               = eng.UnifyNatives
	RequireConcreteList        = eng.RequireConcreteList
	RequireConcreteMap         = eng.RequireConcreteMap
	ResolveTypeLiteralDef      = eng.ResolveTypeLiteralDef
	ResolveTypePath            = eng.ResolveTypePath
	ResolveWordValue           = eng.ResolveWordValue
	ResolveWordsDeep           = eng.ResolveWordsDeep
	ReturnsIdentity            = eng.ReturnsIdentity
	ReturnsListElemAt          = eng.ReturnsListElemAt
	ReturnsNumericBinary       = eng.ReturnsNumericBinary
	ReturnsPreserveListAt      = eng.ReturnsPreserveListAt
	ReturnsStatic              = eng.ReturnsStatic
	RunCarrierBody             = eng.RunCarrierBody
	SetIDSeed                  = eng.SetIDSeed
	SeverityFor                = eng.SeverityFor
	SignatureScore             = eng.SignatureScore
	SimplifyDisjunctAlts       = eng.SimplifyDisjunctAlts
	SortSignatures             = eng.SortSignatures
	StoreKey                   = eng.StoreKey
	StripToCarriers            = eng.StripToCarriers
	TypeNameTable              = eng.TypeNameTable
	Unify                      = eng.Unify
	UninstallDef               = eng.UninstallDef
	UninstallFnSigs            = eng.UninstallFnSigs
	ValToString                = eng.ValToString
	ValidateTypeNameParts      = eng.ValidateTypeNameParts
	ValuesEqual                = eng.ValuesEqual
	WithPos                    = eng.WithPos
	// `make` helpers, ported alongside the make word in eng/go/core_make.go.
	makeConvert      = eng.MakeConvert
	makeFieldValue   = eng.MakeFieldValue
	ResolveFieldType = eng.ResolveFieldType
	// `get`/`set` helper, ported with those words to eng/go/core_storage.go.
	getKey = eng.GetKey
)
