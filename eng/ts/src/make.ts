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
  TPath,
  TString,
} from './type.ts'
import {
  newAtom,
  newBoolean,
  newFloat,
  newInteger,
  newOptions,
  newPath,
  newString,
  OrderedMap,
  Value,
} from './value.ts'

/**
 * The 2-arg [Ideal, Map] make handler. Options instances are built from
 * the source map; other Ideal kinds aren't ported yet.
 */
export function makeIdealHandler(args: Value[]): Value[] {
  const target = args[0]!
  const src = args[1]!
  if (target.data === null && target.vType.leaf() === 'Options' && src.data instanceof OrderedMap) {
    return [newOptions(src.data)]
  }
  throw new AqlError('unsupported', `make: ${target.vType.leaf()} instances not yet ported`)
}

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

/**
 * Build a path value from a string (slash-separated) or a list of
 * parts. Slash runs collapse, a leading slash means absolute, and an
 * explicit `abs` override wins. Mirrors makePath in eng/go.
 */
export function makePath(src: Value, absOverride: boolean | undefined): Value[] {
  let text: string
  if (Array.isArray(src.data)) {
    text = (src.data as Value[]).map((p) => valToString(p)).join('/')
  } else if (src.vType.matches(TString)) {
    text = valToString(src)
  } else {
    throw new AqlError('type_error', `make: Path source must be a list or string, got ${src.toString()}`)
  }
  let abs = text.startsWith('/')
  const segments = text.split('/').filter((s) => s !== '')
  if (absOverride !== undefined) abs = absOverride
  return [newPath(segments, abs)]
}

/** Convert a source value to a target scalar type. Mirrors MakeConvert. */
export function makeConvert(src: Value, targetType: AqlType): Value {
  if (targetType.equal(TPath)) {
    return makePath(src, undefined)[0]!
  }
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
  if (targetType.equal(TPath)) return makePath(src, undefined)
  if (src.vType.matches(targetType)) return [src]
  return [makeConvert(src, targetType)]
}

/**
 * The 3-arg [Scalar, Map, Any] make handler: scalar coercion with an
 * options map. Only Path consults the options (`abs`); other scalars
 * ignore them. Mirrors MakeScalarOptsHandler.
 */
export function makeScalarOptsHandler(args: Value[]): Value[] {
  const target = args[0]!
  const opts = args[1]!
  const src = args[2]!
  if (target.data === null && target.vType.equal(TPath)) {
    let abs: boolean | undefined
    if (opts.data instanceof OrderedMap) {
      const a = opts.asMap().get('abs')
      if (a !== undefined && a.vType.matches(TBoolean)) abs = a.asBoolean()
    }
    return makePath(src, abs)
  }
  return makeScalarHandler([target, src])
}
