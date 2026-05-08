// Unified signature matching (post §1.4).
//
// Every sig declares its boundary via `barrierPos`: the count of
// leading args that may be collected from forward tokens. Args at
// positions [barrierPos..N-1] always come from the stack, top-down.
// There is no stack-only/forward-precedence distinction at the word
// level — registration just sets the per-sig boundary.
//
// Algorithm for one sig with N args and forward limit B:
//
//   1. Forward phase: walk sig[0..B-1] in order, consuming forward
//      tokens one at a time from positions immediately after the
//      word. Stop on a structural boundary, a function word, or a
//      type mismatch.
//   2. Stack phase: walk the remaining sig positions [fwd..N-1] in
//      order, consuming stack values from the TOP DOWN. sig[fwd] =
//      top-of-stack, sig[fwd+1] = next-deeper, etc.
//
// Examples (sub: subtract; not commutative):
//
//   sub 10 3   → fwd=2 [10,3]                  → sig[0]=10, sig[1]=3   → 7
//   3 sub 10   → fwd=1 [10], stack top=3       → sig[0]=10, sig[1]=3   → 7
//   3 10 sub   → fwd=0, stack top=10, next=3   → sig[0]=10, sig[1]=3   → 7
//   10 sub 3   → fwd=1 [3],  stack top=10      → sig[0]=3,  sig[1]=10  → -7  (swap form)

import type { FunctionEntry } from './registry.ts'
import type { Signature } from './signature.ts'
import { TWord } from './type.ts'
import type { Value, WordInfo } from './value.ts'

export interface MatchResult {
  sig: Signature
  /** Args in sig order. */
  args: Value[]
  /** Number of forward tokens consumed (after the word). */
  forwardCount: number
  /** Number of prefix (stack) tokens consumed (before the word). */
  prefixCount: number
}

export function matchEntry(
  fn: FunctionEntry,
  stack: readonly Value[],
  pointer: number,
): MatchResult | null {
  // The dispatching word lives at stack[pointer]. Its WordInfo may
  // carry /s or /f modifiers that override the sig's BarrierPos.
  const wordInfo = readWordInfo(stack, pointer)
  for (const sig of fn.signatures) {
    const n = sig.args.length
    if (n === 0) continue

    const r = tryMatch(sig, n, stack, pointer, wordInfo)
    if (r) return r
  }
  return null
}

function readWordInfo(stack: readonly Value[], pointer: number): WordInfo | undefined {
  const v = stack[pointer]
  if (!v || !v.isWord()) return undefined
  return v.asWord()
}

function tryMatch(
  sig: Signature,
  n: number,
  stack: readonly Value[],
  pointer: number,
  word: WordInfo | undefined,
): MatchResult | null {
  // sig.barrierPos: 0 = boundary at start (all stack), N = boundary
  // at end (all forward-eligible). The Registry has already
  // normalised this on registration. /s and /f modifiers on the call
  // site override it: /s → 0, /f → N.
  let forwardLimit = sig.barrierPos ?? 0
  if (word?.forceStack) forwardLimit = 0
  else if (word?.forceForward) forwardLimit = n

  const args: Value[] = new Array(n)

  // Phase 1: forward.
  let fwd = 0
  let scanIdx = pointer + 1
  while (fwd < forwardLimit && scanIdx < stack.length) {
    const tok = stack[scanIdx]
    if (!tok) break
    if (isStructuralBoundary(tok)) break
    if (!sigTypeMatches(tok, sig.args[fwd]!)) break
    args[fwd] = tok
    fwd++
    scanIdx++
  }

  // /f forbids stack supplementation: every sig position must come
  // from forward. If the forward scan stopped short, this sig fails.
  if (word?.forceForward && fwd < n) return null

  // Phase 2: stack, top-down.
  const remaining = n - fwd
  if (pointer < remaining) return null
  for (let j = 0; j < remaining; j++) {
    const stackVal = stack[pointer - 1 - j]
    if (!stackVal) return null
    if (isStructuralBoundary(stackVal)) return null
    const sigIdx = fwd + j
    if (!sigTypeMatches(stackVal, sig.args[sigIdx]!)) return null
    args[sigIdx] = stackVal
  }

  if (!patternsOk(sig, args, fwd)) return null
  return { sig, args, forwardCount: fwd, prefixCount: remaining }
}

export function sigTypeMatches(v: Value, expected: import('./type.ts').AqlType): boolean {
  return v.vType.matches(expected)
}

function isStructuralBoundary(v: Value): boolean {
  if (!v.vType.matches(TWord)) return false
  const name = (v.data as { name?: string } | null)?.name
  return name === '(' || name === ')' || name === 'end'
}

/**
 * Run sig.patterns against resolved arg values. Concrete-scalar
 * patterns (Data != null on a scalar) are enforced on every
 * position; non-scalar (record/map shape) patterns are enforced
 * only on stack positions, preserving the legacy semantic that
 * handlers may further constrain forward args inside the body.
 * Mirrors match.go's `patternsOk`.
 */
function patternsOk(sig: Signature, args: Value[], forwardCount: number): boolean {
  if (!sig.patterns) return true
  for (const [idx, pattern] of sig.patterns) {
    if (idx < 0 || idx >= args.length) continue
    const isForward = idx < forwardCount
    if (isForward && pattern.data === null) continue
    const val = args[idx]!
    if (pattern.data === null) {
      if (!val.vType.matches(pattern.vType)) return false
      continue
    }
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
