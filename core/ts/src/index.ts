// Public entry point for @voxgig/borucore — the TypeScript interpreter core.
//
// The TS twin of the core/go module (design/ENG-FOUR-PIECE.0.md): values,
// types, signatures, matching, the registry, and the step loop. What is NOT
// here is the point of the package — no check pass, no compiler, no VM, no
// parser, and no dependencies at all.
//
// core/go's rule applies verbatim: NO UPWARD IMPORTS. The check and compiler
// pieces reach core only through the seam tables core itself owns —
// AnalysisImpl (analysis-hooks.ts) and EmitRecorder (emit-recorder.ts) — each
// with a NAMED inactive default that a core-only build runs and a core-side
// test pins. Here that rule is enforced structurally rather than by
// convention: this package declares no dependency on @voxgig/borueng, so a
// core file reaching upward simply fails to resolve.


export { BoruError } from './error.ts'
export { cap } from './capability.ts'
export { CheckState, severityFor, DEFAULT_CHECK_STEP_BUDGET } from './check-state.ts'
export { coerceBoolean } from './coretype.ts'
export type { CheckDiagnostic, CheckSeverity } from './check-state.ts'
export { Engine } from './engine.ts'
export type { FunctionEntry } from './registry.ts'
export { Registry } from './registry.ts'
export type {
  Handler,
  NativeFunc,
  NativeSig,
  ReturnsFunc,
  Signature,
} from './signature.ts'
export {
  signatureScore,
  sortSignatures,
} from './signature.ts'
export type { BoruType } from './type.ts'
export type { SugarInfo, SugarKind } from './value.ts'
export { sugarExpansion } from './sugar.ts'
export {
  TAbsent,
  TAny,
  TAtom,
  TBigDecimal,
  TBigInteger,
  TBoolean,
  TDecimal,
  TDisjunct,
  TEnum,
  TError,
  TFloat,
  TFnUndef,
  TForward,
  TFunction,
  TIdeal,
  TInspect,
  TInterpString,
  TInteger,
  TList,
  TMap,
  TMark,
  TMove,
  TNegation,
  TNever,
  TNode,
  TNone,
  TNumber,
  TClass,
  TOpenParen,
  TOptions,
  TPathon,
  TReach,
  TRecord,
  TScalar,
  TCloseParen,
  TEnd,
  TSplice,
  TStore,
  TString,
  TStringEmpty,
  TStringProper,
  TSugar,
  TTable,
  TType,
  TWord,
  TXml,
  builtinRank,
  newType,
  typeNameTable,
} from './type.ts'
export {
  ChildType,
  ClassTypeInfo,
  OptionsData,
  OrderedMap,
  Value,
  newAny,
  newAtom,
  newBoolean,
  newCarrier,
  newConstrainedWord,
  newDynamicCarrier,
  newDecimal,
  newDisjunct,
  newEnum,
  newFloat,
  newFnDef,
  newFnUndef,
  newForwardMarker,
  newInspect,
  newInterpString,
  newInteger,
  newList,
  newMap,
  asSugar,
  isCloseParen,
  isEnd,
  isOpenParen,
  isSugar,
  newCloseParen,
  newEnd,
  newMark,
  newMove,
  newClassType,
  newErrorValue,
  newNone,
  newOpenParen,
  newSugar,
  renderSugar,
  newOptions,
  newParenExpr,
  newString,
  newTypedList,
  newTypedMap,
  newTypeLiteral,
  newWord,
  newXml,
  newXmlInterp,
  withQuoted,
} from './value.ts'
export type {
  FnDefInfo,
  FnParam,
  FnSig,
  ForwardMarker,
  InterpSegment,
  MarkInfo,
  MoveInfo,
  WordInfo,
  XmlAttrTmpl,
  XmlChildTmpl,
  XmlElement,
  XmlTmpl,
} from './value.ts'

// ---- the seams the upper pieces install into -----------------------------
export { AnalysisImpl, installAnalysisImpl, inactiveCarrierResults, inactiveStripToCarriers } from './analysis-hooks.ts'
export type { AnalysisHooks } from './analysis-hooks.ts'
export { inactiveEmitRecorder } from './emit-recorder.ts'
export type { EmitRecorder, RecorderOperand } from './emit-recorder.ts'
