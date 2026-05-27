# Binary (Bitwise) Operations

Proposal and implementation for AQL's bitwise-operator surface. The
core seven operators are AQL built-ins; the more specialized routines
live in the `aql:bin` module.

## Naming convention

- **Core bitwise built-ins are prefixed with `b`.** `band`, `bor`,
  `bxor`, `bnot`, `bsl`, `bsr`, `busr`. The `b` mirrors C's
  bitwise operator family (`&` / `|` / `^` / `~`) and disambiguates
  from the boolean `and` / `or` / `xor` / `not`, which operate on
  truthiness and short-circuit instead of bits.
- **Module words drop the `b` prefix** because the `bin.` module
  qualifier already disambiguates: `bin.rotl`, `bin.popcount`,
  `bin.test`. No `b`-doubling.

## Argument convention

Two-operand binary ops follow AQL's standard swap-form convention
(`args[1] OP args[0]`). For shifts, the canonical surface reads as
"value shifted by count":

```
8 bsl 2       => 32     # 8 << 2
8 bsr 2       => 2      # 8 >> 2  (arithmetic, sign-extending)
-8 bsr 2      => -2
-1 busr 60    => 15     # logical shift; high bits zero-fill
```

`bnot` is unary:

```
bnot 0        => -1     # one's complement
bnot -1       => 0
```

## Integer width

AQL `Integer` is signed `int64`. All bitwise ops operate on 64-bit
twos-complement representation:

- `bsr` is **arithmetic** (sign-extending): the high bit is
  replicated as the value shifts right, so a negative input stays
  negative.
- `busr` is **logical**: the vacated high bits zero-fill.
- `bsl` matches both: the low bits zero-fill regardless of sign.

## Edge cases

- **Shift count must be non-negative.** Negative counts raise
  `[aql/binary_error]`. Use `busr` if logical right-shift is
  intended.
- **Shift count `>= 64` saturates.** `bsl` and `busr` return 0;
  `bsr` returns 0 for non-negative inputs and -1 for negative
  inputs (sign-fill). Matches Java / Rust shift semantics; differs
  from JS (which masks the count to 5 bits) and C (where the
  behavior is undefined).

## Core operators

| Word | Signature | Semantics |
|---|---|---|
| `band` | `Integer Integer -> Integer` | bitwise AND |
| `bor` | `Integer Integer -> Integer` | bitwise OR |
| `bxor` | `Integer Integer -> Integer` | bitwise XOR |
| `bnot` | `Integer -> Integer` | one's complement (`~x`) |
| `bsl` | `Integer Integer -> Integer` | shift left |
| `bsr` | `Integer Integer -> Integer` | arithmetic shift right (sign-extending) |
| `busr` | `Integer Integer -> Integer` | logical / unsigned shift right (zero-fill) |

## `aql:bin` module words

Loaded via `"aql:bin" import` and used with dot notation:

```
"aql:bin" import
255 bin.popcount      => 8
0xff bin.bitlen       => 8
0xa5 5 bin.test       => true
```

### Rotates

| Word | Signature | Semantics |
|---|---|---|
| `bin.rotl` | `Integer Integer -> Integer` | rotate left N bits (64-bit width) |
| `bin.rotr` | `Integer Integer -> Integer` | rotate right N bits |

### Bit counting

| Word | Signature | Semantics |
|---|---|---|
| `bin.popcount` | `Integer -> Integer` | population count (number of 1-bits) |
| `bin.clz` | `Integer -> Integer` | count leading zeros (64-bit) |
| `bin.ctz` | `Integer -> Integer` | count trailing zeros |
| `bin.parity` | `Integer -> Boolean` | true iff popcount is odd |
| `bin.bitlen` | `Integer -> Integer` | position of highest set bit + 1 (0 for x == 0) |

### Single-bit ops

| Word | Signature | Semantics |
|---|---|---|
| `bin.test` | `Integer Integer -> Boolean` | test bit N: `(x >> n) & 1 != 0` |
| `bin.set` | `Integer Integer -> Integer` | set bit N |
| `bin.clear` | `Integer Integer -> Integer` | clear bit N |
| `bin.toggle` | `Integer Integer -> Integer` | flip bit N |

### Slice / construct

| Word | Signature | Semantics |
|---|---|---|
| `bin.mask` | `Integer -> Integer` | low-N-bits mask: `(1 << n) - 1` |
| `bin.extract` | `Integer Integer Integer -> Integer` | extract bits `[lo, hi)` from a value |
| `bin.insert` | `Integer Integer Integer Integer -> Integer` | write `bits` into `[lo, hi)` of a value |
| `bin.reverse` | `Integer -> Integer` | reverse 64-bit order (bit 0 swaps with bit 63) |
| `bin.swap` | `Integer -> Integer` | byte-swap (endian flip) |

`bin.extract value lo hi` returns the value `(x >> lo) & ((1 << (hi-lo)) - 1)`.
`bin.insert value lo hi bits` returns `x` with bits in `[lo, hi)`
replaced by the low `(hi - lo)` bits of `bits`.

## Edge-case semantics for module words

- `bin.clz 0` → 64, `bin.ctz 0` → 64 (no bits set → full width).
- `bin.bitlen 0` → 0 (no highest bit).
- `bin.mask n` with `n <= 0` → 0; `n >= 64` → -1 (all bits set).
- `bin.test` / `bin.set` / `bin.clear` / `bin.toggle` require
  `0 <= n < 64`; otherwise `[aql:bin/range_error]`.
- `bin.extract` / `bin.insert` require `0 <= lo <= hi <= 64`;
  otherwise `[aql:bin/range_error]`.
- `bin.rotl x n` / `bin.rotr x n` mask `n` to `n mod 64` so very
  large rotates behave intuitively.

## Implementation notes

- Core ops live in `lang/go/native/native_binary.go` as
  `binaryNatives []NativeFunc`, registered in
  `lang/go/native/register.go`.
- Module ops live in `lang/go/modules/binary.go` as
  `BuildBinaryModule(parent) (ModuleDesc, error)`, registered in
  the module map in `lang/go/modules/modules.go`.
- Go primitives: `bits.OnesCount64`, `bits.LeadingZeros64`,
  `bits.TrailingZeros64`, `bits.RotateLeft64`,
  `bits.ReverseBytes64`, `bits.Reverse64` from `math/bits`.
- All handlers DepScalar-reject inputs via `AsConcreteInteger`.

## Why these, and why not more

The chosen surface covers the C operator set + the Go `math/bits`
package + the Java `Integer` static methods. Extras typically
considered but **deliberately excluded** for now:

- Arbitrary-width / arbitrary-precision: AQL Integer is fixed
  `int64`; bigint is a separate proposal.
- `wrapping_*` / `checked_*`: AQL's Integer arithmetic already
  wraps on overflow (no panic); explicit variants add API surface
  without new capability.
- Saturating shifts: niche; add when needed.
- Gray code / packing primitives: hot-path codec work that should
  live in a dedicated `aql:codec` module.
