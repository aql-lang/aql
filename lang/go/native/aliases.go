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
	BranchRecord       = eng.BranchRecord
	EmitFragment       = eng.EmitFragment
	AqlError           = eng.AqlError
	RenderOpts         = eng.RenderOpts
	DiagSpan           = eng.DiagSpan
	DiagSuggestion     = eng.DiagSuggestion
	CalDurationData    = eng.CalDurationData
	CheckDiagnostic    = eng.CheckDiagnostic
	CheckFullStackFunc = eng.CheckFullStackFunc
	CheckSeverity      = eng.CheckSeverity
	CheckState         = eng.CheckState
	ChildTypeInfo      = eng.ChildTypeInfo
	CodeEffectInfo     = eng.CodeEffectInfo
	ReachInfo          = eng.ReachInfo
	ReachSeg           = eng.ReachSeg
	ChildEntry         = eng.ChildEntry
	// Sealed Payload variants (post Step 5).
	Payload              = eng.Payload
	IntPayload           = eng.IntPayload
	FloatPayload         = eng.FloatPayload
	StrPayload           = eng.StrPayload
	BoolPayload          = eng.BoolPayload
	AtomPayload          = eng.AtomPayload
	PathonPayload        = eng.PathonPayload
	ListPayload          = eng.ListPayload
	FlexListData         = eng.FlexListData
	MapPayload           = eng.MapPayload
	ExtensionPayload     = eng.ExtensionPayload
	MaterializerPayload  = eng.MaterializerPayload
	TimePayload          = eng.TimePayload
	DurationPayload      = eng.DurationPayload
	TimezonePayload      = eng.TimezonePayload
	DefCleanupInfo       = eng.DefCleanupInfo
	DepBound             = eng.DepBound
	DepKind              = eng.DepKind
	DepScalarInfo        = eng.DepScalarInfo
	DisjunctInfo         = eng.DisjunctInfo
	Engine               = eng.Engine
	ErrorInfo            = eng.ErrorInfo
	FlowCtrl             = eng.FlowCtrl
	CapturedBinding      = eng.CapturedBinding
	FnDefInfo            = eng.FnDefInfo
	FnParam              = eng.FnParam
	FnSig                = eng.FnSig
	FnSigSpec            = eng.FnSigSpec
	FnUndefInfo          = eng.FnUndefInfo
	ForCont              = eng.ForCont
	ForwardInfo          = eng.ForwardInfo
	GuardClause          = eng.GuardClause
	Handler              = eng.Handler
	IfCont               = eng.IfCont
	InterpPart           = eng.InterpPart
	IntervalInfo         = eng.IntervalInfo
	MarkInfo             = eng.MarkInfo
	MatchResult          = eng.MatchResult
	Materializer         = eng.Materializer
	TypeBehavior         = eng.TypeBehavior
	UnifyError           = eng.UnifyError
	ModuleDesc           = eng.ModuleDesc
	MoveInfo             = eng.MoveInfo
	NativeFunc           = eng.NativeFunc
	SigImpl              = eng.SigImpl // sealed run-impl sum (GoImpl | AQLImpl)
	GoImpl               = eng.GoImpl  // native / internal Go-handler implementation
	AQLImpl              = eng.AQLImpl // AQL body implementation (module ref / lambda / installed fn)
	GoOpt                = eng.GoOpt   // optional dispatch knob for Go(...)
	CompileEffect        = eng.CompileEffect
	CallableSpec         = eng.CallableSpec
	StoredBodySpec       = eng.StoredBodySpec
	ClassInstanceInfo    = eng.ClassInstanceInfo
	ClassTypeInfo        = eng.ClassTypeInfo
	ResourceInstanceInfo = eng.ResourceInstanceInfo
	ResourceTypeInfo     = eng.ResourceTypeInfo
	SurfaceInfo          = eng.SurfaceInfo
	GenParam             = eng.GenParam
	GenSpecInfo          = eng.GenSpecInfo
	TypeSchemaInfo       = eng.TypeSchemaInfo
	GenInstRef           = eng.GenInstRef
	SchemaKind           = eng.SchemaKind
	OptionsTypeInfo      = eng.OptionsTypeInfo
	OrderedMap           = eng.OrderedMap
	PathonInfo           = eng.PathonInfo
	ReadList             = eng.ReadList
	ReadMap              = eng.ReadMap
	RecordTypeInfo       = eng.RecordTypeInfo
	Registry             = eng.Registry
	TapeConfig           = eng.TapeConfig
	DefTable             = eng.DefTable
	ReturnCheckInfo      = eng.ReturnCheckInfo
	ReturnsFunc          = eng.ReturnsFunc
	Signature            = eng.Signature
	SrcPos               = eng.SrcPos
	StoreInstanceInfo    = eng.StoreInstanceInfo
	TableData            = eng.TableData
	TableTypeInfo        = eng.TableTypeInfo
	TimeoutInfo          = eng.TimeoutInfo
	TraceCallback        = eng.TraceCallback
	Type                 = eng.Type
	Value                = eng.Value
	WordInfo             = eng.WordInfo
)

// Well-known *Type values — re-exported for convenience.
var (
	DefaultBehavior = eng.DefaultBehavior
	TAny            = eng.TAny
	TAtom           = eng.TAtom
	TBoolean        = eng.TBoolean
	// TCalendarDuration / TClockDuration / TDate / TDateTime / TDuration
	// moved to lang/go/engine/native_temporal.go (Step 8) — declared
	// directly in this package, no eng alias needed.
	TFloat      = eng.TFloat
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
	TBigInteger   = eng.TBigInteger
	TBigDecimal   = eng.TBigDecimal
	TInternal     = eng.TInternal
	TInterpString = eng.TInterpString
	// TInterval moved to lang/go/engine/native_misc.go (Step 8).
	TList     = eng.TList
	TListArgs = eng.TListArgs
	TFlexList = eng.TFlexList
	TMap      = eng.TMap
	TFlexMap  = eng.TFlexMap
	TXml      = eng.TXml
	TFlexXml  = eng.TFlexXml
	TMark     = eng.TMark
	// TMatrix moved to lang/go/internal/nativemod/matrix.go (Step 8).
	TMove           = eng.TMove
	TNever          = eng.TNever
	TNode           = eng.TNode
	TNone           = eng.TNone
	TNumber         = eng.TNumber
	TClass          = eng.TClass
	TSurface        = eng.TSurface
	TSelf           = eng.TSelf
	TTypeParam      = eng.TTypeParam
	TGenSpec        = eng.TGenSpec
	TGenParam       = eng.TGenParam
	TIdeal          = eng.TIdeal
	TOpenParen      = eng.TOpenParen
	TOptions        = eng.TOptions
	TParenExpr      = eng.TParenExpr
	TReach          = eng.TReach
	TPathon         = eng.TPathon
	TMicron         = eng.TMicron
	TEmailon        = eng.TEmailon
	TUrlon          = eng.TUrlon
	TIpon           = eng.TIpon
	THoston         = eng.THoston
	TSemveron       = eng.TSemveron
	TCidron         = eng.TCidron
	TMacon          = eng.TMacon
	TColoron        = eng.TColoron
	TMimon          = eng.TMimon
	TQion           = eng.TQion
	TPhonon         = eng.TPhonon
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

// Generic-schema kind constants (eng.SchemaKind).
const (
	SchemaRecord = eng.SchemaRecord
	SchemaClass  = eng.SchemaClass
	SchemaFnSig  = eng.SchemaFnSig
	SchemaFn     = eng.SchemaFn
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

	// Compile-effect classifications (eng.CompileEffect) for the bytecode
	// recorder — declared on a Signature instead of a name-keyed eng table.
	CompileDefault          = eng.CompileDefault
	CompileReadsFn          = eng.CompileReadsFn
	CompileStoresFn         = eng.CompileStoresFn
	CompileModuleFold       = eng.CompileModuleFold
	CompileIslandPure       = eng.CompileIslandPure
	CompileScalarFold       = eng.CompileScalarFold
	CompileFallbackBody     = eng.CompileFallbackBody
	CompileQuoteInert       = eng.CompileQuoteInert
	CompileDiverges         = eng.CompileDiverges
	CompileValueDiverges    = eng.CompileValueDiverges
	CompileStoresBody       = eng.CompileStoresBody
	CompileStoresBodyList   = eng.CompileStoresBodyList
	CompileFnHandlerStrict  = eng.CompileFnHandlerStrict
	CompileExecutesBody     = eng.CompileExecutesBody
	CompileRunsBodyIsolated = eng.CompileRunsBodyIsolated
	CompileDynBody          = eng.CompileDynBody

	// CallableSpec.BodyOut's whole-residual sentinel (eng.BodyOutResidual):
	// the driving handler returns the body's entire residual (`do`).
	BodyOutResidual = eng.BodyOutResidual
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
	AnalyseLoopBody           = eng.AnalyseLoopBody
	AsAtom                    = eng.AsAtom
	AsChildType               = eng.AsChildType
	AsDefCleanup              = eng.AsDefCleanup
	AsDisjunct                = eng.AsDisjunct
	AsError                   = eng.AsError
	AsInterpString            = eng.AsInterpString
	AsList                    = eng.AsList
	AsMap                     = eng.AsMap
	AsMark                    = eng.AsMark
	AsMove                    = eng.AsMove
	AsMutableList             = eng.AsMutableList
	AsMutableMap              = eng.AsMutableMap
	AsFlexList                = eng.AsFlexList
	AsFlexXml                 = eng.AsFlexXml
	IsXmlValue                = eng.IsXmlValue
	XmlParts                  = eng.XmlParts
	AsClassInstance           = eng.AsClassInstance
	AsClassType               = eng.AsClassType
	AsResourceInstance        = eng.AsResourceInstance
	AsResourceType            = eng.AsResourceType
	AsOptionsType             = eng.AsOptionsType
	AsParenExpr               = eng.AsParenExpr
	AsRecordType              = eng.AsRecordType
	AsReturnCheck             = eng.AsReturnCheck
	AsStore                   = eng.AsStore
	AsTableType               = eng.AsTableType
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
	IsSplice                  = eng.IsSplice
	AsSplice                  = eng.AsSplice
	Canon                     = eng.Canon
	IsMove                    = eng.IsMove
	IsNone                    = eng.IsNone
	IsNoneShape               = eng.IsNoneShape
	IsClassInstance           = eng.IsClassInstance
	IsClassType               = eng.IsClassType
	IsResourceInstance        = eng.IsResourceInstance
	IsResourceType            = eng.IsResourceType
	IsFlatInstance            = eng.IsFlatInstance
	IsOpenParen               = eng.IsOpenParen
	IsOptionsType             = eng.IsOptionsType
	IsParenExpr               = eng.IsParenExpr
	IsReach                   = eng.IsReach
	AsReach                   = eng.AsReach
	NewReach                  = eng.NewReach
	NewReachFromKeys          = eng.NewReachFromKeys
	ApplyReach                = eng.ApplyReach
	IsPathon                  = eng.IsPathon
	IsRecordType              = eng.IsRecordType
	IsReturnCheck             = eng.IsReturnCheck
	IsStore                   = eng.IsStore
	IsTableType               = eng.IsTableType
	IsTypeValue               = eng.IsTypeValue
	IsTypedList               = eng.IsTypedList
	IsTypedMap                = eng.IsTypedMap
	IsFlexMap                 = eng.IsFlexMap
	IsFlexList                = eng.IsFlexList
	IsFlexNode                = eng.IsFlexNode
	IsValueOfType             = eng.IsValueOfType
	IsWord                    = eng.IsWord
	AsBoolean                 = eng.AsBoolean
	AsFloat                   = eng.AsFloat
	AsForward                 = eng.AsForward
	AsInteger                 = eng.AsInteger
	AsNumber                  = eng.AsNumber
	AsFloatApprox             = eng.AsFloatApprox
	AsBigInteger              = eng.AsBigInteger
	AsBigDecimal              = eng.AsBigDecimal
	AsPathon                  = eng.AsPathon
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
	StaticListLen             = eng.StaticListLen
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
	NewNegation               = eng.NewNegation
	IsNegation                = eng.IsNegation
	AsNegation                = eng.AsNegation
	FormatFloat               = eng.FormatFloat
	NewTypedListWithElements  = eng.NewTypedListWithElements
	NewTypedMapWithEntries    = eng.NewTypedMapWithEntries
	FlattenDisjunctAlts       = eng.FlattenDisjunctAlts
	FnDefHasSig               = eng.FnDefHasSig
	SubstituteSelf            = eng.SubstituteSelf
	NewGenSpec                = eng.NewGenSpec
	IsGenSpec                 = eng.IsGenSpec
	AsGenSpec                 = eng.AsGenSpec
	NewGenParamValue          = eng.NewGenParamValue
	IsGenParamValue           = eng.IsGenParamValue
	AsGenParamValue           = eng.AsGenParamValue
	NewTypeSchema             = eng.NewTypeSchema
	IsTypeSchema              = eng.IsTypeSchema
	AsTypeSchema              = eng.AsTypeSchema
	NewGenInstRef             = eng.NewGenInstRef
	IsGenInstRef              = eng.IsGenInstRef
	AsGenInstRef              = eng.AsGenInstRef
	MintTypeParam             = eng.MintTypeParam
	AttachGenBound            = eng.AttachGenBound
	IsTypeParamNode           = eng.IsTypeParamNode
	TypeParamName             = eng.TypeParamName
	PushGenBindings           = eng.PushGenBindings
	PopGenBindings            = eng.PopGenBindings
	InstantiateSchema         = eng.InstantiateSchema
	SubstituteTypeParams      = eng.SubstituteTypeParams
	SchemaInfoOf              = eng.SchemaInfoOf
	NewSurfaceType            = eng.NewSurfaceType
	AsSurfaceType             = eng.AsSurfaceType
	IsSurfaceType             = eng.IsSurfaceType
	FnDefsOverlap             = eng.FnDefsOverlap
	FnSigMatchesSpec          = eng.FnSigMatchesSpec
	FnSigSatisfiesSpec        = eng.FnSigSatisfiesSpec
	FnUndefMatchesFnDef       = eng.FnUndefMatchesFnDef
	BuiltinIDForPath          = eng.BuiltinIDForPath
	MintTestType              = eng.MintTestType
	GenerateID                = eng.GenerateID
	GenerateObjectTypeID      = eng.GenerateObjectTypeID
	BeginIDMintScope          = eng.BeginIDMintScope
	IDPrefixForType           = eng.IDPrefixForType
	CanonicalType             = eng.CanonicalType
	ResetModuleExportGrowth   = eng.ResetModuleExportGrowth
	MakeClassFieldValue       = eng.MakeClassFieldValue
	MakeClassInstance         = eng.MakeClassInstance
	MakeResource              = eng.MakeResource
	ClassFields               = eng.ClassFields
	FlatInstanceFields        = eng.FlatInstanceFields
	ReparentValue             = eng.ReparentValue
	InstallDef                = eng.InstallDef
	InstallFnDef              = eng.InstallFnDef
	InstallWordExtension      = eng.InstallWordExtension
	TransplantExtension       = eng.TransplantExtension
	IsWordExtension           = eng.IsWordExtension
	IsSealedWord              = eng.IsSealedWord
	HasLockedSigs             = eng.HasLockedSigs
	NewWordExtension          = eng.NewWordExtension
	IsBareTypeNode            = eng.IsBareTypeNode
	IsCapitalisedName         = eng.IsCapitalisedName
	IsConcrete                = eng.IsConcrete
	IsRecordShape             = eng.IsRecordShape
	IsTypeBody                = eng.IsTypeBody
	IsTypeLiteral             = eng.IsTypeLiteral
	JoinCarrierStacks         = eng.JoinCarrierStacks
	JoinCarriers              = eng.JoinCarriers
	FoldVariadicArms          = eng.FoldVariadicArms
	MakeAqlError              = eng.MakeAqlError
	ResolveColor              = eng.ResolveColor
	RenderCheckDiagnostic     = eng.RenderCheckDiagnostic
	MapFieldBoolean           = eng.MapFieldBoolean
	MapFieldFloat             = eng.MapFieldFloat
	MapFieldInteger           = eng.MapFieldInteger
	MapFieldString            = eng.MapFieldString
	MatchSignature            = eng.MatchSignature
	NewValueRaw               = eng.NewValueRaw
	NewSplice                 = eng.NewSplice
	LiteralCondValue          = eng.LiteralCondValue
	BoolWord                  = eng.BoolWord
	ApplyGuardNarrowing       = eng.ApplyGuardNarrowing
	ApplyComplementNarrowing  = eng.ApplyComplementNarrowing
	RunCarrierBodyWithDefs    = eng.RunCarrierBodyWithDefs
	RunCarrierCondBody        = eng.RunCarrierCondBody
	InstallJoinedDefs         = eng.InstallJoinedDefs
	New                       = eng.New
	RunPooled                 = eng.RunPooled
	RunPooledTop              = eng.RunPooledTop
	RunResolved               = eng.RunResolved
	InvokeBody                = eng.InvokeBody
	ConvertIdealToMap         = eng.ConvertIdealToMap
	ConvertIdealToList        = eng.ConvertIdealToList
	IsCompiledClosure         = eng.IsCompiledClosure
	ClosureWantsKeyVal        = eng.ClosureWantsKeyVal
	CloneValue                = eng.CloneValue
	NewSyncWriter             = eng.NewSyncWriter
	NewReadList               = eng.NewReadList
	ContextStoreLookup        = eng.ContextStoreLookup
	ExactEqual                = eng.ExactEqual
	TraceWrap                 = eng.TraceWrap
	FlexibleMatch             = eng.FlexibleMatch
	TraceVisibleLen           = eng.TraceVisibleLen
	TraceColorize             = eng.TraceColorize
	RunTrace                  = eng.RunTrace
	WalkBodyWords             = eng.WalkBodyWords
	PadRight                  = eng.PadRight
	DeepEqual                 = eng.DeepEqual
	FormatForPrint            = eng.FormatForPrint
	FormatValueJSON           = eng.FormatValueJSON
	NewAtom                   = eng.NewAtom
	AtomReferent              = eng.AtomReferent
	SetAtomReferent           = eng.SetAtomReferent
	NewBoolean                = eng.NewBoolean
	// NewCalendarDuration moved to lang/go/engine/native_temporal.go (Step 8).
	NewCarrier               = eng.NewCarrier
	StampFnValueInPlace      = eng.StampFnValueInPlace
	NewElementCarrier        = eng.NewElementCarrier
	ElementCarrierFromValue  = eng.ElementCarrierFromValue
	NewCarrierTypedList      = eng.NewCarrierTypedList
	NewCarrierTypedListValue = eng.NewCarrierTypedListValue
	NewDynamicCarrier        = eng.NewDynamicCarrier
	NewDynamicCarrierValue   = eng.NewDynamicCarrierValue
	// NewClockDuration moved to lang/go/engine/native_temporal.go (Step 8).
	// NewDate / NewDateTime moved to lang/go/engine/native_temporal.go (Step 8).
	NewFloat       = eng.NewFloat
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
	NewInteger       = eng.NewInteger
	NewBigInteger    = eng.NewBigInteger
	NewBigDecimal    = eng.NewBigDecimal
	FormatBigInteger = eng.FormatBigInteger
	FormatBigDecimal = eng.FormatBigDecimal
	NewInterpString  = eng.NewInterpString
	// NewInterval moved to lang/go/engine/native_misc.go (Step 8).
	NewList               = eng.NewList
	NewFlexList           = eng.NewFlexList
	NewMap                = eng.NewMap
	NewFlexMap            = eng.NewFlexMap
	NewMark               = eng.NewMark
	NewMove               = eng.NewMove
	NewMoveCont           = eng.NewMoveCont
	NewMoveIf             = eng.NewMoveIf
	NewClassInstance      = eng.NewClassInstance
	NewClassType          = eng.NewClassType
	NewResourceInstance   = eng.NewResourceInstance
	NewResourceType       = eng.NewResourceType
	NewOpenParen          = eng.NewOpenParen
	NewCloseParen         = eng.NewCloseParen
	NewEnd                = eng.NewEnd
	NewOptionsType        = eng.NewOptionsType
	NewOrderedMap         = eng.NewOrderedMap
	NewParenExpr          = eng.NewParenExpr
	NewPathon             = eng.NewPathon
	NewPathonVol          = eng.NewPathonVol
	NewRecordType         = eng.NewRecordType
	NewRegistry           = eng.NewRegistry
	NewReturnCheck        = eng.NewReturnCheck
	NewStore              = eng.NewStore
	NewStoreValue         = eng.NewStoreValue
	NewStoreWithPrototype = eng.NewStoreWithPrototype
	NewString             = eng.NewString
	NewXmlElement         = eng.NewXmlElement
	NewTableType          = eng.NewTableType
	// NewTimeOfDay moved to lang/go/engine/native_temporal.go (Step 8).
	// NewTimeout moved to lang/go/engine/native_misc.go (Step 8).
	// NewTimezone moved to lang/go/engine/native_temporal.go (Step 8).
	NewTop                 = eng.NewTop
	NewEmitState           = eng.NewEmitState
	NewType                = eng.NewType
	NewTypeLiteral         = eng.NewTypeLiteral
	NewBoundedType         = eng.NewBoundedType
	NewBoundedTypeBody     = eng.NewBoundedTypeBody
	NewTypedList           = eng.NewTypedList
	NewTypedMap            = eng.NewTypedMap
	NewWord                = eng.NewWord
	NewWordModified        = eng.NewWordModified
	Go                     = eng.Go
	AQL                    = eng.AQL
	RunInCheck             = eng.RunInCheck
	Park                   = eng.Park
	FullStack              = eng.FullStack
	CheckFullStack         = eng.CheckFullStack
	NextMarkID             = eng.NextMarkID
	OpenUnifyMap           = eng.OpenUnifyMap
	RankSignatures         = eng.RankSignatures
	UnaryNumOpNative       = eng.UnaryNumOpNative
	BinaryNumOpNative      = eng.BinaryNumOpNative
	BinaryIntOpNative      = eng.BinaryIntOpNative
	RequireConcreteList    = eng.RequireConcreteList
	RequireConcreteMap     = eng.RequireConcreteMap
	ResolveTypeLiteralDef  = eng.ResolveTypeLiteralDef
	ResolveTypePath        = eng.ResolveTypePath
	ResolveWordValue       = eng.ResolveWordValue
	ResolveWordsDeep       = eng.ResolveWordsDeep
	ReturnsFreshInstance   = eng.ReturnsFreshInstance
	ReturnsIdentity        = eng.ReturnsIdentity
	ReturnsListElemAt      = eng.ReturnsListElemAt
	ReturnsNumericBinary   = eng.ReturnsNumericBinary
	ReturnsAddConcat       = eng.ReturnsAddConcat
	ReturnsPreserveListAt  = eng.ReturnsPreserveListAt
	ReturnsStatic          = eng.ReturnsStatic
	DryPassReturns         = eng.DryPassReturns
	DryPassWrap            = eng.DryPassWrap
	CheckListIndex         = eng.CheckListIndex
	CheckAtIndices         = eng.CheckAtIndices
	NewCarrierTypedListLen = eng.NewCarrierTypedListLen
	RunCarrierBody         = eng.RunCarrierBody
	RunCarrierBodyKeepDefs = eng.RunCarrierBodyKeepDefs
	AnalyseCodeEffect      = eng.AnalyseCodeEffectCarrier
	SetIDSeed              = eng.SetIDSeed
	SeverityFor            = eng.SeverityFor
	CompareSignatures      = eng.CompareSignatures
	SimplifyDisjunctAlts   = eng.SimplifyDisjunctAlts
	SizeOf                 = eng.SizeOf
	SortSignatures         = eng.SortSignatures
	StoreKey               = eng.StoreKey
	StripToCarriers        = eng.StripToCarriers
	TypeNameTable          = eng.TypeNameTable
	Unify                  = eng.Unify
	UnifyR                 = eng.UnifyR
	UnifyExplain           = eng.UnifyExplain
	UnifyExplainR          = eng.UnifyExplainR
	UninstallDef           = eng.UninstallDef
	UninstallFnSigs        = eng.UninstallFnSigs
	ValToString            = eng.ValToString
	ValidateTypeNameParts  = eng.ValidateTypeNameParts
	ValuesEqual            = eng.ValuesEqual
	WithPos                = eng.WithPos
	// `make` helpers, ported alongside the make word in eng/go/core_make.go.
	makeConvert      = eng.MakeConvert
	makeFieldValue   = eng.MakeFieldValue
	ResolveFieldType = eng.ResolveFieldType
	// `get`/`set` helper, ported with those words to eng/go/core_storage.go.
	getKey = eng.GetKey
)
