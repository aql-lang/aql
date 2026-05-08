// Public entry point for the TypeScript port of aqleng.

export { AqlError } from './error.js'
export { cap } from './capability.js'
export { Engine } from './engine.js'
export type { FunctionEntry } from './registry.js'
export { Registry } from './registry.js'
export type {
  Handler,
  NativeFunc,
  NativeSig,
  Signature,
} from './signature.js'
export {
  signatureScore,
  sortSignatures,
} from './signature.js'
export type { AqlType } from './type.js'
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
} from './type.js'
export {
  Value,
  newAny,
  newAtom,
  newBoolean,
  newDecimal,
  newInteger,
  newString,
  newTypeLiteral,
  newWord,
} from './value.js'
export type { WordInfo } from './value.js'
