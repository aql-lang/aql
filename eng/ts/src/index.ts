// Public entry point for the TypeScript port of aqleng.

export { AqlError } from './error.ts'
export { cap } from './capability.ts'
export {
  CheckState,
  carrierResults,
  severityFor,
  stripToCarriers,
  toCarrier,
} from './check.ts'
export type { CheckDiagnostic, CheckSeverity } from './check.ts'
export {
  CodeBuilder,
  OpCallNative,
  OpPushConst,
  OpPushLocal,
  OpStoreLocal,
  disassemble,
} from './bytecode.ts'
export type { Op, Program, SigRef } from './bytecode.ts'
export { EmitState } from './emit.ts'
export type { CallEvent, Operand, SiteCounts } from './emit.ts'
export { finalize } from './lower.ts'
export type { FinalizeResult } from './lower.ts'
export { runProgram } from './vm.ts'
export { compile, compileCheck, runCompiled } from './compile.ts'
export type { RunCompiledResult } from './compile.ts'
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
export type { AqlType } from './type.ts'
export {
  TAbsent,
  TAny,
  TArray,
  TAtom,
  TBigDecimal,
  TBigInteger,
  TBoolean,
  TDecimal,
  TDisjunct,
  TEnum,
  TError,
  TFloat,
  TFnDef,
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
  TObject,
  TOpenParen,
  TOptions,
  TPath,
  TReach,
  TRecord,
  TScalar,
  TSplice,
  TStore,
  TString,
  TStringEmpty,
  TStringProper,
  TTable,
  TType,
  TWord,
  TXml,
  newType,
  typeNameTable,
} from './type.ts'
export {
  ChildType,
  ObjectTypeInfo,
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
  newMark,
  newMove,
  newNone,
  newObjectType,
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
