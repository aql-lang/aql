// The bytecode recording pass, ported from the recording half of
// eng/go/emit.go. During a check pass with an EmitState installed, every
// native dispatch that flows through carrierResults is offered to
// recordCall, building a linear trace of call events with operand
// provenance. lower.ts (Finalize) turns the trace into a Program.
//
// PARITY NOTE: Stage 1 records only the straight-line, monomorphic,
// single-result, concrete-operand shape; anything else latches
// `uncompilableReason` so the whole program falls back to the
// interpreter. Control flow, loops, user fns, poly/dynamic dispatch,
// containers, and fallback islands land in later stages.
import type { Signature } from './signature.ts'
import type { Value } from './value.ts'

/** How a recorded operand is produced. */
export type Operand =
  | { readonly kind: 'const'; readonly value: Value }
  | { readonly kind: 'event'; readonly event: number }

/** One recorded native dispatch: word + selected sig + operand refs. */
export interface CallEvent {
  readonly word: string
  readonly sig: Signature
  readonly args: readonly Operand[]
}

/** Dispatch-site classification tally (mirrors emit.go SiteCounts). */
export interface SiteCounts {
  mono: number
  poly: number
  dynamic: number
  meta: number
}

/**
 * EmitState records the call-event trace for one compile pass. It hangs
 * off CheckState.emit and is fed passively by the engine's check-mode
 * dispatch — recording never alters the carriers the checker returns, so
 * the static-analysis result is identical with or without it.
 */
export class EmitState {
  readonly events: CallEvent[] = []
  readonly siteCounts: SiteCounts = { mono: 0, poly: 0, dynamic: 0, meta: 0 }
  /** First reason the program could not be lowered; latched (kept first). */
  uncompilableReason: string | undefined

  /** result-Value identity → producing event index. */
  private readonly producedBy = new Map<Value, number>()
  /** stripped-literal carrier → its concrete original (for const materialisation). */
  private readonly origByCarrier = new WeakMap<Value, Value>()

  /** Latch the first reason the program is not compilable. */
  markUncompilable(reason: string): void {
    if (this.uncompilableReason === undefined) this.uncompilableReason = reason
  }

  /** Map a stripped-literal carrier back to its concrete source value. */
  rememberStripped(carrier: Value, original: Value): void {
    this.origByCarrier.set(carrier, original)
  }

  /**
   * Classify an operand Value: a prior event's result, a concrete
   * constant (incl. a stripped literal recovered via origByCarrier), or
   * null when its provenance is unknown (→ uncompilable).
   */
  classify(v: Value): Operand | null {
    const ev = this.producedBy.get(v)
    if (ev !== undefined) return { kind: 'event', event: ev }
    if (v.isConcrete()) return { kind: 'const', value: v }
    const orig = this.origByCarrier.get(v)
    if (orig !== undefined) return { kind: 'const', value: orig }
    return null
  }

  /**
   * Record one native dispatch. Stage 1 admits only the monomorphic,
   * concrete-operand, single-result shape; everything else latches
   * uncompilable. `args` are the matched args (sig order); `out` is the
   * carrierResults output (its identity threads to consumers).
   */
  recordCall(word: string, sig: Signature, args: readonly Value[], out: readonly Value[]): void {
    if (this.uncompilableReason !== undefined) return
    if (out.length !== 1) {
      this.markUncompilable(`${word}: ${out.length}-result dispatch not compilable (stage 1)`)
      return
    }
    if (args.some((a) => a.carrier && a.dynamic)) {
      this.markUncompilable(`${word}: dynamic operand not compilable (stage 1)`)
      return
    }
    const ops: Operand[] = []
    for (const a of args) {
      const o = this.classify(a)
      if (o === null) {
        this.markUncompilable(`${word}: operand of unknown provenance`)
        return
      }
      ops.push(o)
    }
    const idx = this.events.length
    this.events.push({ word, sig, args: ops })
    this.producedBy.set(out[0]!, idx)
    this.siteCounts.mono++
  }
}
