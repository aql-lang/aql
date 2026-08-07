// EmitRecorder — the bytecode-recording seam, owned by core.
//
// The TS twin of core/go/emit_recorder.go. The core step loop must be able
// to OFFER a dispatch, a literal push, or a container build to the compiler's
// recorder without naming a compiler symbol. Core declares the contract here;
// the compiler piece (eng/ts's EmitState) satisfies it structurally, so it
// needs no `implements` clause and core needs no import of it.
//
// DIVERGENCE FROM core/go, recorded deliberately. The Go interface's methods
// take only core types — `[]Value`, `*Signature`, `SrcPos`, `*Registry` — so
// the Go core never handles a compiler operand. The TS engine does: it calls
// classify() and threads the results straight back into recordMakeList /
// recordMakeMap. Rather than restructure the TS recording paths (a behaviour
// change) or import the compiler's Operand union upward (a boundary
// violation), core keeps the operand OPAQUE: it is minted by the recorder and
// handed back unread. `unknown` is the type that says exactly that.

import type { Value } from './value.ts'
import type { Signature } from './signature.ts'

/**
 * An operand token minted by the recorder's own classify() and handed back to
 * it unchanged. Opaque by construction — core never inspects one.
 */
export type RecorderOperand = unknown

/**
 * The recording contract the core step loop offers events through. Every
 * method is passive: a recorder may decline (return null/false) and the
 * engine's behaviour is unchanged.
 */
export interface EmitRecorder {
  /** Latch a refusal reason; the program is no longer compilable. */
  markUncompilable(reason: string): void

  /** Map a stripped-literal carrier back to its concrete source value. */
  rememberStripped(carrier: Value, original: Value): void

  /** Classify a value as an operand, or null when it cannot be one. */
  classify(v: Value): RecorderOperand | null

  /** Record a matched native dispatch. */
  recordCall(word: string, sig: Signature, args: readonly Value[], out: readonly Value[]): void

  /** Record a dispatch that fell back to the interpreter. */
  recordFallback(
    word: string,
    args: readonly Value[],
    out: readonly Value[],
    registry?: unknown,
    noEvalArgs?: ReadonlySet<number>,
  ): boolean

  /** Record a list construction from already-classified operands. */
  recordMakeList(elements: RecorderOperand[]): Value

  /** Record a map construction from already-classified operands. */
  recordMakeMap(keys: string[], values: RecorderOperand[]): Value

  /** Record a single-token runtime island threading to `out`. */
  recordValueIsland(token: Value, out: Value, desc: string): void

  /** The island token span recorded for a value, or null. */
  islandTokensFor(v: Value): readonly Value[] | null

  /** Alias `target` onto whatever slot produced `source`. */
  alias(target: Value, source: Value): void

  /**
   * The baked constant a `const` operand carries, or null for any other
   * operand kind (and for a null operand).
   *
   * Core ASKS rather than pattern-matching the operand union. The step loop
   * needs one bit of operand knowledge — "is this bakeable, and to what
   * value" — to const-fold a fully-inert computed container and to bake a
   * const-bound word into an island. Reading `op.kind === 'const'` inline
   * would put the compiler's representation inside core; this keeps the
   * operand opaque and leaves the knowledge in the compiler, which is where
   * core/go has it (its EmitRecorder methods take only core types).
   */
  constValueOf(op: RecorderOperand | null): Value | null
}

/**
 * The NAMED inactive recorder — what a core build linked without the compiler
 * piece runs. Mirrors core/go's `TheInactiveEmit`: naming it (rather than
 * leaving an anonymous object literal) is what makes the decline path
 * reachable and pinnable by a core-side test, per core/go/CLAUDE.md's seam
 * rule. Every method is the identity/decline.
 */
export const inactiveEmitRecorder: EmitRecorder = {
  markUncompilable(_reason: string): void {},
  rememberStripped(_carrier: Value, _original: Value): void {},
  classify(_v: Value): RecorderOperand | null {
    return null
  },
  recordCall(
    _word: string,
    _sig: Signature,
    _args: readonly Value[],
    _out: readonly Value[],
  ): void {},
  recordFallback(
    _word: string,
    _args: readonly Value[],
    _out: readonly Value[],
    _registry?: unknown,
    _noEvalArgs?: ReadonlySet<number>,
  ): boolean {
    return false
  },
  recordMakeList(_elements: RecorderOperand[]): Value {
    throw new Error('inactiveEmitRecorder.recordMakeList: no recorder installed')
  },
  recordMakeMap(_keys: string[], _values: RecorderOperand[]): Value {
    throw new Error('inactiveEmitRecorder.recordMakeMap: no recorder installed')
  },
  recordValueIsland(_token: Value, _out: Value, _desc: string): void {},
  islandTokensFor(_v: Value): readonly Value[] | null {
    return null
  },
  alias(_target: Value, _source: Value): void {},
  constValueOf(_op: RecorderOperand | null): Value | null {
    return null
  },
}
