// Signature matching for the spec subset.
//
// PARITY GAP: the Go matcher (aqleng/go/match.go) handles many cases
// the spec subset doesn't exercise — preferred /q-Word arg-0 sigs,
// pre-evaluated paren groups, pipe barriers, deferred/ForceForward
// modes, FnDef-aware forward-collection, mid-stream Mark/Move
// tokens. This implementation is the minimum needed for the current
// TSV specs.
//
// Argument ordering (mirror rule, per CLAUDE.md):
//
//   - Forward args fill sig[0..f-1] in source order (left-to-right).
//   - For ForwardPrecedence words, the remaining stack args fill
//     sig[f..N-1] NEAREST-FIRST (top of stack → sig[f]).
//   - For stack-only words, all args come from prefix in
//     DEEPEST-FIRST order (bottom of consumed run → sig[0]).
//
// Examples (sub: subtract; not commutative):
//
//   sub 10 3       → f=2 forward [10,3]                 → sig[0]=10, sig[1]=3   → 7
//   3 sub 10       → f=1 [10], p=1 [3] nearest          → sig[0]=10, sig[1]=3   → 7
//   3 10 sub       → f=0, p=2 [3,10] nearest-first      → sig[0]=10, sig[1]=3   → 7
//   10 sub 3       → f=1 [3],  p=1 [10] nearest         → sig[0]=3,  sig[1]=10  → -7  (swap form)

import type { FunctionEntry } from './registry.ts'
import type { Signature } from './signature.ts'
import { TAtom, TBoolean, TWord } from './type.ts'
import type { Value } from './value.ts'

export interface MatchResult {
  /** The matched signature. */
  sig: Signature
  /** Args in sig order (sig.args[i] type is satisfied by args[i].vType.matches(sig.args[i])). */
  args: Value[]
  /** Number of forward tokens consumed (after the word). */
  forwardCount: number
  /** Number of prefix tokens consumed (before the word). */
  prefixCount: number
}

/**
 * Try to match `fn` against the stack at position `pointer`.
 * `stack[pointer]` is the word itself; positions before are prefix
 * tokens, positions after are forward tokens.
 */
export function matchEntry(
  fn: FunctionEntry,
  stack: readonly Value[],
  pointer: number,
): MatchResult | null {
  const stackOnly = !fn.forwardPrecedence

  for (const sig of fn.signatures) {
    const n = sig.args.length

    // 0-arg sigs (fallback) skipped here; engine handles them.
    if (n === 0) continue

    if (stackOnly) {
      const r = tryStackOnly(sig, n, stack, pointer)
      if (r) return r
      continue
    }

    // ForwardPrecedence: try splits from all-forward down to all-prefix.
    for (let f = n; f >= 0; f--) {
      const p = n - f
      const r = tryForwardSplit(sig, n, f, p, stack, pointer)
      if (r) return r
    }
  }
  return null
}

function tryForwardSplit(
  sig: Signature,
  n: number,
  forwardCount: number,
  prefixCount: number,
  stack: readonly Value[],
  pointer: number,
): MatchResult | null {
  // Forward-token availability.
  if (pointer + forwardCount >= stack.length + 1 - 0) {
    if (pointer + forwardCount > stack.length - 1 && forwardCount > 0) {
      // Need forwardCount tokens at indices pointer+1 .. pointer+forwardCount.
    }
  }
  if (pointer + forwardCount > stack.length - 1) return null

  // Collect forward tokens (positions immediately after the word).
  const forwardTokens: Value[] = []
  for (let i = 0; i < forwardCount; i++) {
    const tok = stack[pointer + 1 + i]
    if (!tok) return null
    if (isStructuralBoundary(tok)) return null
    forwardTokens.push(tok)
  }

  // Collect prefix tokens (immediately preceding the word, in stack order).
  if (pointer < prefixCount) return null
  const prefixTokens: Value[] = []
  for (let i = 0; i < prefixCount; i++) {
    const tok = stack[pointer - prefixCount + i]
    if (!tok) return null
    if (isStructuralBoundary(tok)) return null
    prefixTokens.push(tok)
  }

  // Build args in sig order. For forward-precedence words with a
  // prefix tail, prefix args fill sig[f..n-1] NEAREST-FIRST.
  const args: Value[] = new Array(n)
  for (let i = 0; i < forwardCount; i++) {
    args[i] = forwardTokens[i]!
  }
  for (let j = 0; j < prefixCount; j++) {
    args[forwardCount + j] = prefixTokens[prefixCount - 1 - j]!
  }

  // Type-check.
  for (let i = 0; i < n; i++) {
    if (!sigTypeMatches(args[i]!, sig.args[i]!)) return null
  }

  // §1.1 pattern check: scalar-literal patterns are enforced on
  // every position (forward + stack); non-scalar patterns are
  // checked only on stack positions to preserve the legacy
  // "handler may further constrain inside the body" semantic.
  if (!patternsOk(sig, args, forwardCount)) return null

  return { sig, args, forwardCount, prefixCount }
}

function tryStackOnly(
  sig: Signature,
  n: number,
  stack: readonly Value[],
  pointer: number,
): MatchResult | null {
  if (pointer < n) return null
  const prefixTokens: Value[] = []
  for (let i = 0; i < n; i++) {
    const tok = stack[pointer - n + i]
    if (!tok) return null
    if (isStructuralBoundary(tok)) return null
    prefixTokens.push(tok)
  }

  // Stack-only: deepest-first ordering. args[0] = deepest = prefix[0].
  const args: Value[] = []
  for (let i = 0; i < n; i++) {
    args.push(prefixTokens[i]!)
  }

  for (let i = 0; i < n; i++) {
    if (!sigTypeMatches(args[i]!, sig.args[i]!)) return null
  }
  // All positions came from the prefix → pattern check applies
  // unconditionally (forwardCount=0 means nothing is "forward").
  if (!patternsOk(sig, args, 0)) return null
  return { sig, args, forwardCount: 0, prefixCount: n }
}

/**
 * Run sig.patterns against the resolved arg values. Concrete-scalar
 * patterns (Data != null) are enforced on every position; non-scalar
 * patterns are checked only on stack positions (`idx >= forwardCount`)
 * because handlers historically used the forward-arg pattern slot for
 * shape hints rather than hard constraints. See match.go's `patternsOk`
 * for the same rule.
 */
function patternsOk(sig: Signature, args: Value[], forwardCount: number): boolean {
  if (!sig.patterns) return true
  for (const [idx, pattern] of sig.patterns) {
    if (idx < 0 || idx >= args.length) continue
    const isForward = idx < forwardCount
    if (isForward && pattern.data === null) continue
    const val = args[idx]!
    if (pattern.data === null) {
      // Type-literal pattern, stack position: arg's type must match.
      if (!val.vType.matches(pattern.vType)) return false
      continue
    }
    // Concrete pattern: kinds must match and Data must compare equal.
    if (!val.vType.matches(pattern.vType)) return false
    if (val.data === null) return false
    if (!dataEqual(val.data, pattern.data)) return false
  }
  return true
}

function dataEqual(a: unknown, b: unknown): boolean {
  if (a === b) return true
  if (typeof a === 'bigint' && typeof b === 'bigint') return a === b
  return false
}

/**
 * sigTypeMatches: a stripped-down version of the Go helper. Supports
 * Any-matches-everything, lattice subtype, and treats words / atoms /
 * booleans for the limited dispatch the spec uses.
 */
export function sigTypeMatches(v: Value, expected: import('./type.ts').AqlType): boolean {
  return v.vType.matches(expected)
}

function isStructuralBoundary(v: Value): boolean {
  // The spec subset never produces these — but we still bail on them
  // to mirror the Go matcher's behaviour.
  return v.vType.matches(TWord) === false
    ? false
    : (v.data as { name?: string } | null)?.name === '(' ||
        (v.data as { name?: string } | null)?.name === ')' ||
        (v.data as { name?: string } | null)?.name === 'end'
}

// Re-exports for engine.ts convenience.
export { TAtom, TBoolean, TWord }
