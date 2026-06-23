// The bytecode recording pass, ported from the recording half of
// eng/go/emit.go. During a check pass with an EmitState installed, every
// native dispatch that flows through carrierResults is offered to
// recordCall, building a trace of events with operand provenance.
// lower.ts (finalize) turns the trace into a Program.
//
// Stage 1–3 recorded a flat list of straight-line native calls. Stage 4
// adds CONTROL FLOW: a branch (`if`) records its two arms as nested
// FRAGMENTS — the arm's sub-engine dispatches are captured into a
// fragment buffer rather than the main trace — and the lowerer emits
// jump-bracketed code. A global monotonic `seq` numbers every event
// across fragments and doubles as its frame-local slot.
//
// PARITY NOTE: still mono-only / concrete-operand / single-result; poly,
// dynamic, loops, user fns, containers, and fallback islands land later.
import type { Signature } from './signature.ts'
import type { Value } from './value.ts'

/** How a recorded operand is produced. */
export type Operand =
  | { readonly kind: 'const'; readonly value: Value }
  | { readonly kind: 'event'; readonly slot: number }

/** A recorded native dispatch. `slot` is its global event/local index. */
export interface CallEvent {
  readonly kind: 'call'
  readonly slot: number
  readonly word: string
  readonly sig: Signature
  readonly args: readonly Operand[]
}

/** A recorded conditional. The arms are nested fragments. */
export interface BranchEvent {
  readonly kind: 'branch'
  readonly slot: number
  readonly cond: Operand
  readonly then: Fragment
  readonly else: Fragment
}

export type Event = CallEvent | BranchEvent

/** A captured sub-trace (a branch arm) plus its single residual operand. */
export interface Fragment {
  readonly events: readonly Event[]
  readonly residual: Operand
}

/** Dispatch-site classification tally (mirrors emit.go SiteCounts). */
export interface SiteCounts {
  mono: number
  poly: number
  dynamic: number
  meta: number
}

/**
 * EmitState records the event trace for one compile pass. It hangs off
 * CheckState.emit and is fed passively by the engine's check-mode
 * dispatch — recording never alters the carriers the checker returns.
 */
export class EmitState {
  /** Top-level event trace (events recorded outside any fragment). */
  readonly events: Event[] = []
  readonly siteCounts: SiteCounts = { mono: 0, poly: 0, dynamic: 0, meta: 0 }
  /** First reason the program could not be lowered; latched (kept first). */
  uncompilableReason: string | undefined

  /** Global monotonic event counter; each event's index == its local slot. */
  private seq = 0
  /** Active fragment buffers (branch arms); recording targets the top one. */
  private readonly fragmentStack: Event[][] = []
  /** result-Value identity → producing event slot. */
  private readonly producedBy = new Map<Value, number>()
  /** stripped-literal carrier → its concrete original (for const materialisation). */
  private readonly origByCarrier = new WeakMap<Value, Value>()

  /** Total event/local-slot count (for Program.numLocals). */
  slotCount(): number {
    return this.seq
  }

  /** Latch the first reason the program is not compilable. */
  markUncompilable(reason: string): void {
    if (this.uncompilableReason === undefined) this.uncompilableReason = reason
  }

  /** Map a stripped-literal carrier back to its concrete source value. */
  rememberStripped(carrier: Value, original: Value): void {
    this.origByCarrier.set(carrier, original)
  }

  /** The event list currently being recorded into (top fragment, else top level). */
  private target(): Event[] {
    return this.fragmentStack.length > 0 ? this.fragmentStack[this.fragmentStack.length - 1]! : this.events
  }

  /**
   * Classify an operand Value: a prior event's result, a concrete
   * constant (incl. a stripped literal recovered via origByCarrier), or
   * null when its provenance is unknown (→ uncompilable).
   */
  classify(v: Value): Operand | null {
    const slot = this.producedBy.get(v)
    if (slot !== undefined) return { kind: 'event', slot }
    if (v.isConcrete()) return { kind: 'const', value: v }
    const orig = this.origByCarrier.get(v)
    if (orig !== undefined) return { kind: 'const', value: orig }
    return null
  }

  /**
   * Record one native dispatch. Admits only the monomorphic,
   * concrete-operand, single-result shape; everything else latches
   * uncompilable. `out` identity threads to consumers.
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
    if (out.some((o) => o.carrier && o.dynamic)) {
      this.markUncompilable(`${word}: dynamic/unannotated result not compilable`)
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
    const slot = this.seq++
    this.target().push({ kind: 'call', slot, word, sig, args: ops })
    this.producedBy.set(out[0]!, slot)
    this.siteCounts.mono++
  }

  /** Begin capturing dispatches into a fresh fragment buffer (a branch arm). */
  beginFragment(): void {
    this.fragmentStack.push([])
  }

  /**
   * End the current fragment, classifying its single residual value.
   * Returns null (and latches uncompilable) if the residual has no known
   * provenance.
   */
  endFragment(residualValue: Value): Fragment | null {
    const events = this.fragmentStack.pop()
    if (events === undefined) {
      this.markUncompilable('endFragment without beginFragment')
      return null
    }
    const residual = this.classify(residualValue)
    if (residual === null) {
      this.markUncompilable('branch arm residual of unknown provenance')
      return null
    }
    return { events, residual }
  }

  /**
   * Record a conditional branch with two captured arm fragments. `out`
   * is the merged result carrier whose identity threads to consumers.
   */
  recordBranch(cond: Operand, thenArm: Fragment, elseArm: Fragment, out: Value): void {
    if (this.uncompilableReason !== undefined) return
    const slot = this.seq++
    this.target().push({ kind: 'branch', slot, cond, then: thenArm, else: elseArm })
    this.producedBy.set(out, slot)
  }
}
