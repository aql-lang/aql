// Canon renders values as canonical AQL source — the string a spec row's
// `expected` column is compared against. Ported from eng/go/canon.go +
// eng/go/value.go::FormatFloat so the TS engine's output matches the Go
// reference row-for-row.
//
// PARITY NOTE: this covers the value kinds the eng/spec corpus reaches
// through the current TS engine (none, type literals, scalars, atoms,
// lists, fn defs). Map / BigInteger / Reach / Flex / DepScalar branches
// are added by their owning port increments.
import { TAtom, TBoolean, TFloat, TInspect, TInteger, TList, TMap, TNone, TPath, TString } from './type.ts'
import type { FnDefInfo, XmlElement } from './value.ts'
import { ChildType, OrderedMap, Value } from './value.ts'

// canonXml renders an XML element, normalising an empty element to the
// self-closing form (<br></br> → <br/>).
function canonXml(e: XmlElement): string {
  const attrs = e.attrs.map((a) => ` ${a.name}="${a.value}"`).join('')
  if (e.children.length === 0) return `<${e.tag}${attrs}/>`
  const body = e.children.map((c) => (typeof c === 'string' ? c : canonXml(c))).join('')
  return `<${e.tag}${attrs}>${body}</${e.tag}>`
}

/**
 * canonString renders a string payload as parseable AQL source. Plain
 * content uses single quotes; content with a single quote switches to
 * double quotes; content with both quote kinds, backslashes, or control
 * characters falls back to single quotes with backslash escapes.
 * Mirrors eng/go/canon.go::canonString.
 */
export function canonString(s: string): string {
  const hasSingle = s.includes("'")
  const hasEscape = /[\\\n\t\r]/.test(s)
  if (!hasSingle && !hasEscape) return `'${s}'`
  if (!s.includes('"') && !hasEscape) return `"${s}"`
  let b = "'"
  for (const r of s) {
    switch (r) {
      case '\\':
        b += '\\\\'
        break
      case "'":
        b += "\\'"
        break
      case '\n':
        b += '\\n'
        break
      case '\t':
        b += '\\t'
        break
      case '\r':
        b += '\\r'
        break
      default:
        b += r
    }
  }
  return b + "'"
}

/**
 * formatFloat renders a float with a guaranteed decimal point so it
 * stays visually distinct from Integer, using the shortest round-trip
 * representation. Mirrors eng/go/value.go::FormatFloat.
 */
export function formatFloat(f: number): string {
  if (Number.isNaN(f)) return 'nan'
  if (f === Infinity) return 'inf'
  if (f === -Infinity) return '-inf'
  // Negative zero is a distinct bit pattern (IEEE) and renders as -0.0.
  if (f === 0 && 1 / f === -Infinity) return '-0.0'

  if (f !== 0) {
    const a = Math.abs(f)
    if (a >= 1e21 || a < 1e-10) {
      return formatExponential(f)
    }
  }
  let s = toFixedShortest(f)
  if (!/[.eE]/.test(s)) s += '.0'
  return s
}

// toFixedShortest renders a float in plain decimal ('f') form with the
// shortest precision that round-trips — the equivalent of Go's
// strconv.FormatFloat(f, 'f', -1, 64). JS's String() uses 'g'-style
// formatting (switching to exponent for large/small values), so coerce
// any exponent form back to plain decimal for the everyday range that
// FormatFloat keeps non-scientific.
function toFixedShortest(f: number): string {
  const s = String(f)
  if (!/[eE]/.test(s)) return s
  // Expand the JS exponential form to plain decimal.
  return expandExponential(s)
}

// formatExponential renders the Go strconv 'e' form: a mantissa with a
// sign-and-at-least-two-digit exponent (e.g. 1e+21 → "1e+21").
function formatExponential(f: number): string {
  let s = f.toExponential()
  // JS gives "1e+21"; Go gives "1e+21" too. Normalise the exponent to a
  // leading sign (JS already includes it) — no zero-padding needed to
  // match Go's -1 precision 'e' form for the magnitudes the specs use.
  // Drop a trailing ".0"-style mantissa? Go keeps the shortest mantissa.
  s = s.replace(/e([+-])(\d)$/, 'e$10$2')
  return s
}

// expandExponential turns a JS exponential literal into plain decimal.
function expandExponential(s: string): string {
  const neg = s.startsWith('-')
  if (neg) s = s.slice(1)
  const m = /^(\d+)(?:\.(\d+))?[eE]([+-]?\d+)$/.exec(s)
  if (!m) return (neg ? '-' : '') + s
  const intPart = m[1]!
  const fracPart = m[2] ?? ''
  const exp = Number.parseInt(m[3]!, 10)
  const digits = intPart + fracPart
  const pointPos = intPart.length + exp
  let out: string
  if (pointPos <= 0) {
    out = '0.' + '0'.repeat(-pointPos) + digits
  } else if (pointPos >= digits.length) {
    out = digits + '0'.repeat(pointPos - digits.length)
  } else {
    out = digits.slice(0, pointPos) + '.' + digits.slice(pointPos)
  }
  // Trim a trailing fractional zero run produced by the expansion.
  if (out.includes('.')) out = out.replace(/0+$/, '').replace(/\.$/, '')
  return (neg ? '-' : '') + out
}

/** Canon renders a stack of values as canonical AQL source. */
export function canon(stack: Value[]): string {
  return stack.map(canonValue).join(' ')
}

/** canonValue renders one value as canonical AQL source. */
export function canonValue(v: Value): string {
  // The `none` value renders lowercase; a bare type literal (including
  // the `None` type) renders as its user-facing leaf name.
  if (v.isNone()) return 'none'
  if (v.data === null) {
    return v.vType.leaf()
  }
  if (v.vType.matches(TInteger)) {
    return v.asInteger().toString()
  }
  if (v.vType.matches(TFloat)) {
    return formatFloat(v.asFloat())
  }
  if (v.vType.matches(TString)) {
    return canonString(v.asString())
  }
  if (v.vType.matches(TBoolean)) {
    return v.asBoolean() ? 'true' : 'false'
  }
  if (v.vType.equal(TAtom)) {
    return `${v.asAtom()}/q`
  }
  if (v.vType.equal(TPath) && v.data !== null && typeof v.data === 'object' && 'segments' in v.data) {
    const p = v.data as { segments: string[]; abs: boolean }
    return (p.abs ? '/' : '') + p.segments.join('/')
  }
  if (v.data instanceof ChildType) {
    const ct = v.data
    const open = v.isTypedMap() ? '{' : '['
    const close = v.isTypedMap() ? '}' : ']'
    const parts = [`:${canonValue(ct.child)}`, ...ct.elements.map(canonValue)]
    return `${open}${parts.join(' ')}${close}`
  }
  if (v.vType.matches(TList) && Array.isArray(v.data)) {
    const body = `[${v.asList().map(canonValue).join(' ')}]`
    return v.quoted ? `(quote ${body})` : body
  }
  if (v.vType.equal(TMap) && v.data instanceof OrderedMap) {
    const m = v.data
    const parts = m.sortedKeys().map((k) => `${k}:${canonValue(m.get(k)!)}`)
    return `{${parts.join(' ')}}`
  }
  // An inspection map renders in insertion order with bare word values
  // (e.g. `kind:native`), unlike a plain map.
  if (v.vType.equal(TInspect) && v.data instanceof OrderedMap) {
    const m = v.data
    const parts = m.keys().map((k) => {
      const val = m.get(k)!
      const rendered = val.isWord() ? val.asWord().name : canonValue(val)
      return `${k}:${rendered}`
    })
    return `{${parts.join(' ')}}`
  }
  if (v.isXml()) {
    return canonXml(v.data as XmlElement)
  }
  if (v.isFnDef()) {
    return canonFnDef(v.asFnDef())
  }
  // Disjunct / Enum: alternatives joined by '|' (atoms render bare,
  // type literals as their leaf).
  if (v.isDisjunct()) {
    return v
      .asDisjunct()
      .alternatives.map((a) =>
        a.vType.equal(TAtom) && a.data !== null
          ? a.asAtom()
          : a.data === null
            ? a.vType.leaf()
            : canonValue(a),
      )
      .join('|')
  }
  return v.toString()
}

// canonFnDef renders a function value's discriminating canonical form.
// The TS FnDefInfo carries a single params/returns/body triple (the
// spec subset), so this renders that one signature. Mirrors the shape
// of eng/go/canon.go::canonFnDef for a single authored sig.
function canonFnDef(fd: FnDefInfo): string {
  const sigs = fd.sigs
    .map((s) => {
      const params = s.params.map((p) => `${p.name}:${p.type.toString()}`).join(' ')
      const returns = s.returns.map((r) => r.toString()).join(' ')
      const body = s.body.map(canonValue).join(' ')
      return `[${params}][${returns}][${body}]`
    })
    .join(' ')
  return `fn [${sigs}]`
}
