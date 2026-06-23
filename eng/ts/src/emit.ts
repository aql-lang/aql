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
import { newCarrier, type Value } from './value.ts'
import { TList, TMap } from './type.ts'

/**
 * isInert reports whether a concrete value is bakeable as a program
 * constant: scalars/atoms/paths/booleans/none, and containers all of
 * whose members are themselves inert. Words, parens, fn defs, carriers,
 * and interp/xml templates are NOT inert — they are code or type-only,
 * so a const operand must never carry them (else the VM would push an
 * unevaluated form). Mirrors the bakeable subset of eng/go isInertConst.
 */
function isInert(v: Value): boolean {
  if (v.carrier) return false
  if (Array.isArray(v.data)) return (v.data as Value[]).every(isInert)
  if (v.isMap()) {
    const m = v.asMap()
    return m.keys().every((k) => isInert(m.get(k)!))
  }
  if (
    v.isWord() ||
    v.isForward() ||
    v.isMark() ||
    v.isMove() ||
    v.isParenExpr() ||
    v.isFnDef() ||
    v.isInterpString() ||
    v.isXmlInterp()
  ) {
    return false
  }
  return v.data !== null
}

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

/**
 * A recorded counted loop. `start`/`end`/`step` are integer operands;
 * the iterator variable is bound to `iterSlot` (a frame local) each
 * iteration, and the body's single per-iteration residual accumulates on
 * the operand stack (the loop is variadic).
 */
export interface LoopEvent {
  readonly kind: 'loop'
  readonly slot: number
  readonly start: Operand
  readonly end: Operand
  readonly step: Operand
  readonly iterSlot: number
  readonly body: Fragment
}

/** A recorded computed list (`[addq 1 2]` → assemble from element operands). */
export interface MakeListEvent {
  readonly kind: 'makeList'
  readonly slot: number
  readonly elements: readonly Operand[]
}

/** A recorded computed map (`{a:(addq 1 2)}` → assemble from value operands). */
export interface MakeMapEvent {
  readonly kind: 'makeMap'
  readonly slot: number
  readonly keys: readonly string[]
  readonly values: readonly Operand[]
}

/**
 * A recorded terminal trap: a check-mode-suppressed runtime error (a word
 * that is lenient in check mode but raises at run time) compiled as an
 * OpTrap. The program ends here — the recorder drops everything after it.
 */
export interface TrapEvent {
  readonly kind: 'trap'
  readonly slot: number
  readonly code: string
  readonly detail: string
  readonly word: string
}

export type Event = CallEvent | BranchEvent | LoopEvent | MakeListEvent | MakeMapEvent | TrapEvent

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
  /** Slot of a recorded TOP-LEVEL terminal trap (undefined = none). */
  trapSlot: number | undefined
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
    // A concrete value is a const only if it is INERT (bakeable) — a
    // raw eval-list / word-bearing container reaching a call arg is code,
    // not a constant, and must fall back rather than bake unevaluated.
    if (v.isConcrete() && isInert(v)) return { kind: 'const', value: v }
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

  /** Allocate a fresh event/local slot (e.g. for a loop iterator variable). */
  allocSlot(): number {
    return this.seq++
  }

  /** Bind a value's identity to a slot (e.g. a loop iterator carrier). */
  bindSlot(v: Value, slot: number): void {
    this.producedBy.set(v, slot)
  }

  /**
   * Give `target` the same operand provenance as `source` so they
   * classify identically. Used to INLINE a user fn: the fn's declared
   * return carrier is aliased to the body's recorded residual, so the
   * fn result threads to the body's events (the call compiles to the
   * body's straight-line code, not a separate unit). No-op when `source`
   * itself has no provenance — `target` then stays unresolvable and the
   * program falls back.
   */
  alias(target: Value, source: Value): void {
    const slot = this.producedBy.get(source)
    if (slot !== undefined) {
      this.producedBy.set(target, slot)
      return
    }
    const orig = this.origByCarrier.get(source)
    if (orig !== undefined) {
      this.origByCarrier.set(target, orig)
      return
    }
    if (source.isConcrete()) this.origByCarrier.set(target, source)
  }

  /**
   * Record a counted loop. The body fragment's residual accumulates on
   * the stack per iteration (variadic). `out` is the loop's result
   * carrier whose identity threads to the program residual.
   */
  recordLoop(start: Operand, end: Operand, step: Operand, iterSlot: number, body: Fragment, out: Value): void {
    if (this.uncompilableReason !== undefined) return
    const slot = this.seq++
    this.target().push({ kind: 'loop', slot, start, end, step, iterSlot, body })
    this.producedBy.set(out, slot)
  }

  /**
   * Record a computed list from its element operands. Returns a list
   * carrier whose identity threads to the assembled result.
   */
  recordMakeList(elements: Operand[]): Value {
    const slot = this.seq++
    this.target().push({ kind: 'makeList', slot, elements })
    const out = newCarrier(TList)
    this.producedBy.set(out, slot)
    return out
  }

  /**
   * Record a computed map from its key list + value operands. Returns a
   * map carrier whose identity threads to the assembled result.
   */
  recordMakeMap(keys: string[], values: Operand[]): Value {
    const slot = this.seq++
    this.target().push({ kind: 'makeMap', slot, keys, values })
    const out = newCarrier(TMap)
    this.producedBy.set(out, slot)
    return out
  }

  /**
   * Record a TERMINAL trap for a check-mode-suppressed runtime error: the
   * checker is lenient here but the interpreter errors, so the compiled
   * program raises the byte-identical error via OpTrap instead of refusing
   * the whole program. Only a TOP-LEVEL trap is recorded — a trap inside a
   * branch arm / loop body / fn fragment is conditional and not modelled, so
   * it returns false and the caller latches markUncompilable to fall back.
   * The first trap wins (execution can reach only one). Returns true when the
   * trap is owned here (recorded now, or already trapping). Mirrors
   * eng/go/emit.go::RecordTrap. Finalize truncates the trace at the trap and
   * drops the residual (everything after is unreachable).
   */
  recordTrap(code: string, detail: string, word: string): boolean {
    if (this.fragmentStack.length !== 0) return false
    if (this.trapSlot !== undefined) return true
    const slot = this.seq++
    this.events.push({ kind: 'trap', slot, code, detail, word })
    this.trapSlot = slot
    return true
  }
}
