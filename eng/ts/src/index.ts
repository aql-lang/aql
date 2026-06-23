// Public entry point for the TypeScript port of aqleng.

export { AqlError } from './error.ts'
export { cap } from './capability.ts'
export { Engine } from './engine.ts'
export type { FunctionEntry } from './registry.ts'
export { Registry } from './registry.ts'
export type {
  Handler,
  NativeFunc,
  NativeSig,
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
  Value,
  newAny,
  newAtom,
  newBoolean,
  newDecimal,
  newFloat,
  newFnDef,
  newForwardMarker,
  newInteger,
  newList,
  newMark,
  newMove,
  newNone,
  newString,
  newTypeLiteral,
  newWord,
  withQuoted,
} from './value.ts'
export type { FnDefInfo, FnParam, ForwardMarker, MarkInfo, MoveInfo, WordInfo } from './value.ts'
