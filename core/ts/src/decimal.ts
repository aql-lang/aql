// Arbitrary-precision decimal — the TS twin of Go's apd.Decimal, for the
// BigDecimal payload.
//
// core/ts has NO dependencies and is meant to keep it that way (the module
// exists precisely so the TS core needs nothing), so this is a deliberately
// small implementation rather than a decimal library: a bigint `unscaled`
// and an integer `scale`, together denoting unscaled x 10^-scale. That is
// exactly apd's model, which is what makes byte-identical rendering
// achievable.
//
// It carries no arithmetic. Nothing in core/ts does arithmetic on a
// BigDecimal today — the parser builds one and canon renders it — and a
// half-implemented `add` would be worse than none. When BigDecimal maths
// arrives it goes here, against apd's semantics.
//
// This replaced a binary64 payload, which could not represent what Go
// represented: `0d1e400` overflowed to Infinity and rendered the
// unparseable `0dInfinity`, `0d1e-400` underflowed and rendered `0d0` —
// silently turning a nonzero value into zero — and `0d0.30` lost the
// trailing-zero scale. All three were live rows in
// parser/spec/divergent.tsv.

export class Decimal {
  /** The significand, sign included. */
  readonly unscaled: bigint;
  /**
   * Digits after the decimal point. NEGATIVE means trailing zeros before
   * it — `1e2` is unscaled 1, scale -2. Preserved exactly as written, so
   * `0.30` (scale 2) does not collapse to `0.3` (scale 1); apd keeps the
   * same distinction and `Text('f')` shows it.
   */
  readonly scale: number;
  /**
   * NEGATIVE ZERO. A bigint has no -0, so the sign of `-0.0` would be lost
   * the moment the significand is built — and it was: `-0.0` rendered
   * `0d0.0` here against Go's `-0d0.0`. apd carries an explicit `Negative`
   * flag beside its coefficient for exactly this reason, and so does this.
   * Meaningful ONLY when `unscaled` is zero; a non-zero significand
   * carries its own sign.
   */
  readonly negZero: boolean;

  constructor(unscaled: bigint, scale: number, negZero = false) {
    this.unscaled = unscaled;
    this.scale = scale;
    this.negZero = negZero && 0n === unscaled;
  }

  /**
   * Renders the plain 'f' form at every magnitude — never scientific,
   * matching apd's `Text('f')`. Trailing zeros are KEPT: the scale is part
   * of the value's identity here, not noise to normalise away.
   */
  toString(): string {
    const neg = this.unscaled < 0n || this.negZero;
    const digits = (
      this.unscaled < 0n ? -this.unscaled : this.unscaled
    ).toString();
    const sign = neg ? "-" : "";
    if (this.scale <= 0) {
      // Whole, with -scale trailing zeros. A ZERO significand grows the
      // run too: `0e5` is `000000` in apd's Text('f'), and short-circuiting
      // it to a bare `0` was a divergence — the exponent is part of the
      // value's identity here exactly as the scale is.
      return sign + digits + "0".repeat(-this.scale);
    }
    if (digits.length > this.scale) {
      const cut = digits.length - this.scale;
      return sign + digits.slice(0, cut) + "." + digits.slice(cut);
    }
    // The point falls at or left of the leading digit: pad to reach it.
    return sign + "0." + "0".repeat(this.scale - digits.length) + digits;
  }
}

/**
 * decimalFromString parses a decimal literal — optional sign, digits, an
 * optional fraction, an optional exponent — EXACTLY, with no binary64 in
 * the path. Returns undefined when the text is not a decimal literal, so
 * callers raise their own diagnostic rather than inheriting one.
 *
 * The `0d` marker is NOT accepted here: that is the parser's spelling of a
 * big literal, and this function is about the number.
 */
export function decimalFromString(src: string): Decimal | undefined {
  const m = /^([+-]?)(\d*)(?:\.(\d*))?(?:[eE]([+-]?\d+))?$/.exec(src);
  if (null === m) return undefined;
  const sign = "-" === m[1] ? -1n : 1n;
  const intPart = m[2] ?? "";
  const fracPart = m[3] ?? "";
  // A lone sign, a bare `.`, or an exponent with no significand is not a
  // number — the regex above admits those shapes, so reject them here.
  if ("" === intPart && "" === fracPart) return undefined;
  const exp = undefined === m[4] ? 0 : Number.parseInt(m[4], 10);
  const digits = intPart + fracPart;
  const unscaled = BigInt(digits);
  return new Decimal(
    sign * unscaled,
    fracPart.length - exp,
    -1n === sign && 0n === unscaled,
  );
}
