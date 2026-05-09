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
  TAny,
  TAtom,
  TBoolean,
  TDecimal,
  TError,
  TForward,
  TFunction,
  TInteger,
  TList,
  TMap,
  TMark,
  TMove,
  TNever,
  TNode,
  TNone,
  TNumber,
  TObject,
  TOpenParen,
  TPath,
  TRecord,
  TScalar,
  TString,
  TStringEmpty,
  TStringProper,
  TTable,
  TWord,
  newType,
  typeNameTable,
} from './type.ts'
export {
  Value,
  newAny,
  newAtom,
  newBoolean,
  newDecimal,
  newFnDef,
  newForwardMarker,
  newInteger,
  newList,
  newString,
  newTypeLiteral,
  newWord,
  withQuoted,
} from './value.ts'
export type { FnDefInfo, FnParam, ForwardMarker, WordInfo } from './value.ts'
