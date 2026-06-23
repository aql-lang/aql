// Scalar `make` coercion, ported from eng/go/core_make.go +
// registry.go::ValToString. Covers the scalar target types the
// eng/spec make.tsv rows exercise: String, Integer, Number, Float,
// Boolean, Atom. Object / Array / Options / prototype overloads are
// added by a later increment.
import { formatFloat } from './canon.ts'
import { AqlError } from './error.ts'
import {
  AqlType,
  TAtom,
  TBoolean,
  TFloat,
  TInteger,
  TNumber,
  TString,
} from './type.ts'
import { newAtom, newBoolean, newFloat, newInteger, newString, Value } from './value.ts'

/** Render a concrete value as its scalar text. Mirrors ValToString. */
export function valToString(v: Value): string {
  if (!v.isConcrete()) return v.toString()
  if (v.vType.matches(TString)) return v.asString()
  if (v.vType.equal(TAtom)) return v.asAtom()
  if (v.vType.matches(TFloat)) return formatFloat(v.asFloat())
  if (v.vType.matches(TInteger)) return v.asInteger().toString()
  if (v.vType.matches(TBoolean)) return v.asBoolean() ? 'true' : 'false'
  if (v.isWord()) return v.asWord().name
  return v.toString()
}

/** Convert a source value to a target scalar type. Mirrors MakeConvert. */
export function makeConvert(src: Value, targetType: AqlType): Value {
  if (targetType.matches(TString)) {
    return newString(valToString(src))
  }
  if (targetType.matches(TFloat)) {
    const text = valToString(src)
    const f = Number(text)
    if (text.trim() === '' || Number.isNaN(f)) {
      throw new AqlError('type_error', `make: cannot convert ${JSON.stringify(text)} to float`)
    }
    return newFloat(f)
  }
  if (targetType.matches(TNumber) || targetType.matches(TInteger)) {
    const text = valToString(src)
    if (/^-?\d+$/.test(text)) return newInteger(BigInt(text))
    const f = Number(text)
    if (text.trim() === '' || Number.isNaN(f)) {
      throw new AqlError('type_error', `make: cannot convert ${JSON.stringify(text)} to number`)
    }
    return newInteger(BigInt(Math.trunc(f)))
  }
  if (targetType.matches(TBoolean)) {
    if (src.vType.matches(TBoolean)) return src
    if (src.vType.matches(TNumber)) {
      const n = src.vType.matches(TInteger) ? Number(src.asInteger()) : src.asFloat()
      return newBoolean(n !== 0)
    }
    const text = valToString(src)
    if (text === 'true') return newBoolean(true)
    if (text === 'false') return newBoolean(false)
    return newBoolean(text !== '')
  }
  if (targetType.equal(TAtom)) {
    return newAtom(valToString(src))
  }
  throw new AqlError('type_error', `make: unsupported target type ${targetType.toString()}`)
}

/**
 * The 2-arg [Scalar, Any] make handler: coerce src to the target
 * scalar type. Mirrors MakeScalarHandler (scalar subset — no Path,
 * no user-minted refinement reparent yet).
 */
export function makeScalarHandler(args: Value[]): Value[] {
  const target = args[0]!
  const src = args[1]!
  if (target.data !== null) {
    throw new AqlError('type_error', `make: expected a type literal, got ${target.toString()}`)
  }
  const targetType = target.vType
  if (src.vType.matches(targetType)) return [src]
  return [makeConvert(src, targetType)]
}
