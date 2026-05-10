// Package engine is a thin shim over the aqleng module: it re-exports
// aqleng's types and functions so the surrounding aql codebase keeps
// the import path "internal/engine" while the actual engine lives in
// the standalone aqleng module.
//
// Word-defining files (native_*.go, format.go, query.go, sqlite.go,
// fileio.go, conditional.go, forloop.go) continue to live here. Anything
// that's truly engine machinery (Registry, Value, Type, signatures,
// matching, unification, …) is now in aqleng.
package engine

import (
	aqleng "github.com/metsitaba/voxgig-exp/aqleng"
)

// Type aliases — every exported type from aqleng is re-exported here.
type (
	AqlError = aqleng.AqlError
	ArrayInstanceInfo = aqleng.ArrayInstanceInfo
	CalDurationData = aqleng.CalDurationData
	CheckDiagnostic = aqleng.CheckDiagnostic
	CheckFullStackFunc = aqleng.CheckFullStackFunc
	CheckSeverity = aqleng.CheckSeverity
	CheckState = aqleng.CheckState
	ChildTypeInfo = aqleng.ChildTypeInfo
	DefCleanupInfo = aqleng.DefCleanupInfo
	DepBound = aqleng.DepBound
	DepKind = aqleng.DepKind
	DepScalarInfo = aqleng.DepScalarInfo
	DisjunctInfo = aqleng.DisjunctInfo
	Engine = aqleng.Engine
	ErrorInfo = aqleng.ErrorInfo
	FnDefInfo = aqleng.FnDefInfo
	FnParam = aqleng.FnParam
	FnSig = aqleng.FnSig
	FnSigSpec = aqleng.FnSigSpec
	FnUndefInfo = aqleng.FnUndefInfo
	ForCont = aqleng.ForCont
	ForwardInfo = aqleng.ForwardInfo
	GuardClause = aqleng.GuardClause
	Handler = aqleng.Handler
	IfCont = aqleng.IfCont
	InterpPart = aqleng.InterpPart
	IntervalInfo = aqleng.IntervalInfo
	MarkInfo = aqleng.MarkInfo
	MatchResult = aqleng.MatchResult
	Materializer = aqleng.Materializer
	MatrixData = aqleng.MatrixData
	ModuleDesc = aqleng.ModuleDesc
	MoveInfo = aqleng.MoveInfo
	NativeFunc = aqleng.NativeFunc
	NativeSig = aqleng.NativeSig
	ObjectInstanceInfo = aqleng.ObjectInstanceInfo
	ObjectTypeInfo = aqleng.ObjectTypeInfo
	OptionsTypeInfo = aqleng.OptionsTypeInfo
	OrderedMap = aqleng.OrderedMap
	PathInfo = aqleng.PathInfo
	ReadList = aqleng.ReadList
	ReadMap = aqleng.ReadMap
	RecordTypeInfo = aqleng.RecordTypeInfo
	Registry = aqleng.Registry
	ReturnCheckInfo = aqleng.ReturnCheckInfo
	ReturnsFunc = aqleng.ReturnsFunc
	Signature = aqleng.Signature
	SrcPos = aqleng.SrcPos
	StoreInstanceInfo = aqleng.StoreInstanceInfo
	TableData = aqleng.TableData
	TableTypeInfo = aqleng.TableTypeInfo
	TimeoutInfo = aqleng.TimeoutInfo
	TraceCallback = aqleng.TraceCallback
	Type = aqleng.Type
	Value = aqleng.Value
	WordInfo = aqleng.WordInfo
)

// Well-known Type values — re-exported for convenience.
var (
	TAny = aqleng.TAny
	TArray = aqleng.TArray
	TAtom = aqleng.TAtom
	TBoolean = aqleng.TBoolean
	TCalDuration = aqleng.TCalDuration
	TClkDuration = aqleng.TClkDuration
	TDate = aqleng.TDate
	TDateTime = aqleng.TDateTime
	TDecimal = aqleng.TDecimal
	TDefCleanup = aqleng.TDefCleanup
	TDepInteger = aqleng.TDepInteger
	TDependent = aqleng.TDependent
	TDisjunct = aqleng.TDisjunct
	TDuration = aqleng.TDuration
	TError = aqleng.TError
	TFetchFunction = aqleng.TFetchFunction
	TFetchRequest = aqleng.TFetchRequest
	TFetchResponse = aqleng.TFetchResponse
	TFnDef = aqleng.TFnDef
	TFnUndef = aqleng.TFnUndef
	TForward = aqleng.TForward
	TFunction = aqleng.TFunction
	TInspect = aqleng.TInspect
	TInstant = aqleng.TInstant
	TInteger = aqleng.TInteger
	TInternal = aqleng.TInternal
	TInterpString = aqleng.TInterpString
	TInterval = aqleng.TInterval
	TList = aqleng.TList
	TListArgs = aqleng.TListArgs
	TMap = aqleng.TMap
	TMark = aqleng.TMark
	TMatrix = aqleng.TMatrix
	TModule = aqleng.TModule
	TMove = aqleng.TMove
	TNever = aqleng.TNever
	TNode = aqleng.TNode
	TNodeType = aqleng.TNodeType
	TNone = aqleng.TNone
	TNumber = aqleng.TNumber
	TObject = aqleng.TObject
	TObjectType = aqleng.TObjectType
	TOpenParen = aqleng.TOpenParen
	TOptions = aqleng.TOptions
	TParenExpr = aqleng.TParenExpr
	TPath = aqleng.TPath
	TRecord = aqleng.TRecord
	TResource = aqleng.TResource
	TResourceEntity = aqleng.TResourceEntity
	TReturnCheck = aqleng.TReturnCheck
	TScalar = aqleng.TScalar
	TScalarType = aqleng.TScalarType
	TStore = aqleng.TStore
	TStoreSystem = aqleng.TStoreSystem
	TString = aqleng.TString
	TStringEmpty = aqleng.TStringEmpty
	TStringProper = aqleng.TStringProper
	TTable = aqleng.TTable
	TTimeOfDay = aqleng.TTimeOfDay
	TTimeout = aqleng.TTimeout
	TTimezone = aqleng.TTimezone
	TType = aqleng.TType
	TWord = aqleng.TWord
)

// Severity constants for diagnostic classification.
const (
	SeverityError   = aqleng.SeverityError
	SeverityWarning = aqleng.SeverityWarning
	SeverityInfo    = aqleng.SeverityInfo
)

// Engine-level constants.
const (
	CarrierDisjunctCap     = aqleng.CarrierDisjunctCap
	MaxArgs                = aqleng.MaxArgs
	DefaultCheckStepBudget = aqleng.DefaultCheckStepBudget

	DepGT  = aqleng.DepGT
	DepGTE = aqleng.DepGTE
	DepLT  = aqleng.DepLT
	DepLTE = aqleng.DepLTE
)

// Sentinel values exposed by the engine.
var (
	BuiltinTypeIDs = aqleng.BuiltinTypeIDs
	ErrBreak       = aqleng.ErrBreak
	ErrContinue    = aqleng.ErrContinue
)

// Function re-exports — every exported aqleng function.
var (
	AnalyseFnBody = aqleng.AnalyseFnBody
	BaseValue = aqleng.BaseValue
	BaseValueForConstraint = aqleng.BaseValueForConstraint
	BoundToKind = aqleng.BoundToKind
	CoerceBoolean = aqleng.CoerceBoolean
	CommonAncestorType = aqleng.CommonAncestorType
	CompareValues = aqleng.CompareValues
	CowSet = aqleng.CowSet
	DataListElemTypeFromValue = aqleng.DataListElemTypeFromValue
	DependentLeafBaseType = aqleng.DependentLeafBaseType
	DependentLeafFromType = aqleng.DependentLeafFromType
	ExpandOptionalSigs = aqleng.ExpandOptionalSigs
	parseFnParams      = aqleng.ParseFnParams
	parseFnReturns     = aqleng.ParseFnReturns
	resolveSigType     = aqleng.ResolveSigType
	resolveTypeName    = aqleng.ResolveTypeName
	lookupDefType      = aqleng.LookupDefType
	resolveDefType     = aqleng.ResolveDefType
	TandValues         = aqleng.TandValues
	parseFnDef                 = aqleng.ParseFnDef
	outputSigIsConcreteReturns = aqleng.OutputSigIsConcreteReturns
	isSigTypeValue             = aqleng.IsSigTypeValue
	outputSigValues            = aqleng.OutputSigValues
	ValidateWordName           = aqleng.ValidateWordName
	TypeOf                     = aqleng.TypeOf
	NewNone                    = aqleng.NewNone
	FindWordInSource = aqleng.FindWordInSource
	FlattenDisjunctAlts = aqleng.FlattenDisjunctAlts
	FnDefHasSig = aqleng.FnDefHasSig
	FnDefsOverlap = aqleng.FnDefsOverlap
	FnSigMatchesSpec = aqleng.FnSigMatchesSpec
	FnSigSatisfiesSpec = aqleng.FnSigSatisfiesSpec
	FnUndefMatchesFnDef = aqleng.FnUndefMatchesFnDef
	FormatFixedTypeID = aqleng.FormatFixedTypeID
	GenerateID = aqleng.GenerateID
	GenerateObjectTypeID = aqleng.GenerateObjectTypeID
	IDPrefixForParts = aqleng.IDPrefixForParts
	IDPrefixForType = aqleng.IDPrefixForType
	InstallDef = aqleng.InstallDef
	InstallFnDef = aqleng.InstallFnDef
	IsBreak = aqleng.IsBreak
	IsCapitalisedName = aqleng.IsCapitalisedName
	IsConcrete = aqleng.IsConcrete
	IsContinue = aqleng.IsContinue
	IsMetaType = aqleng.IsMetaType
	IsTypeBody = aqleng.IsTypeBody
	IsTypeLiteral = aqleng.IsTypeLiteral
	IsTypeValue = aqleng.IsTypeValue
	JoinCarrierStacks = aqleng.JoinCarrierStacks
	JoinCarriers = aqleng.JoinCarriers
	KeepFallback = aqleng.KeepFallback
	MakeAqlError = aqleng.MakeAqlError
	MapFieldBoolean = aqleng.MapFieldBoolean
	MapFieldDecimal = aqleng.MapFieldDecimal
	MapFieldInteger = aqleng.MapFieldInteger
	MapFieldString = aqleng.MapFieldString
	MatchSignature = aqleng.MatchSignature
	MetatypeFor = aqleng.MetatypeFor
	NewValueRaw = aqleng.NewValueRaw
	LiteralCondValue = aqleng.LiteralCondValue
	BoolWord = aqleng.BoolWord
	ApplyGuardNarrowing = aqleng.ApplyGuardNarrowing
	ApplyComplementNarrowing = aqleng.ApplyComplementNarrowing
	RunCarrierBodyWithDefs = aqleng.RunCarrierBodyWithDefs
	InstallJoinedDefs = aqleng.InstallJoinedDefs
	New = aqleng.New
	NewArray = aqleng.NewArray
	NewArrayEmpty = aqleng.NewArrayEmpty
	NewReadList = aqleng.NewReadList
	ContextStoreLookup = aqleng.ContextStoreLookup
	ExactEqual = aqleng.ExactEqual
	TraceWrap = aqleng.TraceWrap
	FlexibleMatch = aqleng.FlexibleMatch
	TraceVisibleLen = aqleng.TraceVisibleLen
	TraceColorize = aqleng.TraceColorize
	RunTrace = aqleng.RunTrace
	PadRight = aqleng.PadRight
	DeepEqual = aqleng.DeepEqual
	FormatForPrint = aqleng.FormatForPrint
	FormatValueJSON = aqleng.FormatValueJSON
	NewAtom = aqleng.NewAtom
	NewBoolean = aqleng.NewBoolean
	NewCalDuration = aqleng.NewCalDuration
	NewCarrier = aqleng.NewCarrier
	NewCarrierTypedList = aqleng.NewCarrierTypedList
	NewCarrierTypedListValue = aqleng.NewCarrierTypedListValue
	NewClkDuration = aqleng.NewClkDuration
	NewDate = aqleng.NewDate
	NewDateTime = aqleng.NewDateTime
	NewDecimal = aqleng.NewDecimal
	NewDefCleanup = aqleng.NewDefCleanup
	NewDepScalar = aqleng.NewDepScalar
	NewDisjunct = aqleng.NewDisjunct
	NewError = aqleng.NewError
	NewEvalList = aqleng.NewEvalList
	NewEvalMap = aqleng.NewEvalMap
	NewFnDef = aqleng.NewFnDef
	NewFnUndef = aqleng.NewFnUndef
	NewForward = aqleng.NewForward
	NewFunction = aqleng.NewFunction
	NewImplicitMap = aqleng.NewImplicitMap
	NewInstant = aqleng.NewInstant
	NewInteger = aqleng.NewInteger
	NewInterpString = aqleng.NewInterpString
	NewInterval = aqleng.NewInterval
	NewList = aqleng.NewList
	NewMap = aqleng.NewMap
	NewMark = aqleng.NewMark
	NewMatrix = aqleng.NewMatrix
	NewModule = aqleng.NewModule
	NewMove = aqleng.NewMove
	NewMoveCont = aqleng.NewMoveCont
	NewMoveIf = aqleng.NewMoveIf
	NewObjectInstance = aqleng.NewObjectInstance
	NewObjectType = aqleng.NewObjectType
	NewOpenParen = aqleng.NewOpenParen
	NewOptionsType = aqleng.NewOptionsType
	NewOrderedMap = aqleng.NewOrderedMap
	NewParenExpr = aqleng.NewParenExpr
	NewPath = aqleng.NewPath
	NewRecordType = aqleng.NewRecordType
	NewRegistry = aqleng.NewRegistry
	NewReturnCheck = aqleng.NewReturnCheck
	NewStore = aqleng.NewStore
	NewStoreValue = aqleng.NewStoreValue
	NewStoreWithPrototype = aqleng.NewStoreWithPrototype
	NewString = aqleng.NewString
	NewTableType = aqleng.NewTableType
	NewTimeOfDay = aqleng.NewTimeOfDay
	NewTimeout = aqleng.NewTimeout
	NewTimezone = aqleng.NewTimezone
	NewTop = aqleng.NewTop
	NewType = aqleng.NewType
	NewTypeLiteral = aqleng.NewTypeLiteral
	NewTypedList = aqleng.NewTypedList
	NewTypedMap = aqleng.NewTypedMap
	NewWord = aqleng.NewWord
	NewWordModified = aqleng.NewWordModified
	NextMarkID = aqleng.NextMarkID
	OpenUnifyMap = aqleng.OpenUnifyMap
	RankSignatures = aqleng.RankSignatures
	UnaryNumOpNative = aqleng.UnaryNumOpNative
	BinaryNumOpNative = aqleng.BinaryNumOpNative
	BinaryIntOpNative = aqleng.BinaryIntOpNative
	ComparisonNatives = aqleng.ComparisonNatives
	PrintNatives = aqleng.PrintNatives
	TraceNatives = aqleng.TraceNatives
	UnifyNatives = aqleng.UnifyNatives
	RequireConcreteList = aqleng.RequireConcreteList
	RequireConcreteMap = aqleng.RequireConcreteMap
	ResolveTypeLiteralDef = aqleng.ResolveTypeLiteralDef
	ResolveTypePath = aqleng.ResolveTypePath
	ResolveWordValue = aqleng.ResolveWordValue
	ResolveWordsDeep = aqleng.ResolveWordsDeep
	ReturnsIdentity = aqleng.ReturnsIdentity
	ReturnsListElemAt = aqleng.ReturnsListElemAt
	ReturnsNumericBinary = aqleng.ReturnsNumericBinary
	ReturnsPreserveListAt = aqleng.ReturnsPreserveListAt
	ReturnsStatic = aqleng.ReturnsStatic
	RunCarrierBody = aqleng.RunCarrierBody
	SetIDSeed = aqleng.SetIDSeed
	SeverityFor = aqleng.SeverityFor
	SignatureScore = aqleng.SignatureScore
	SimplifyDisjunctAlts = aqleng.SimplifyDisjunctAlts
	SortSignatures = aqleng.SortSignatures
	StoreKey = aqleng.StoreKey
	StripToCarriers = aqleng.StripToCarriers
	TypeNameTable = aqleng.TypeNameTable
	Unify = aqleng.Unify
	UninstallDef = aqleng.UninstallDef
	UninstallFnSigs = aqleng.UninstallFnSigs
	ValToString = aqleng.ValToString
	ValidateTypeNameParts = aqleng.ValidateTypeNameParts
	ValuesEqual = aqleng.ValuesEqual
	WithPos = aqleng.WithPos
)
