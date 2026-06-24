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
import type { Registry } from './registry.ts'
import type { Signature } from './signature.ts'
import { newCarrier, newWord, OrderedMap, type FnDefInfo, type Value } from './value.ts'
import { TList, TMap } from './type.ts'

/**
 * fnCaptureFree reports whether a fn VALUE is self-contained — every body
 * word is either one of its params or not currently def-bound — so it can
 * be baked into a fallback island and re-applied at run time. A body that
 * references an enclosing def (a lexical capture) is NOT bakeable: the
 * captured name isn't bound when the island re-runs, so such a closure
 * falls back to the interpreter. The analogue of eng/go ComputeCaptures
 * (here used only as a yes/no bakeability gate).
 */
function fnCaptureFree(info: FnDefInfo, registry: Registry): boolean {
  const walk = (body: readonly Value[], params: ReadonlySet<string>): boolean => {
    for (const v of body) {
      if (v.isWord()) {
        const name = v.asWord().name
        if (!params.has(name) && registry.topOfDefStack(name) !== undefined) return false
      } else if (Array.isArray(v.data)) {
        // Lists and paren-expr payloads both store their tokens as data.
        if (!walk(v.data as Value[], params)) return false
      }
    }
    return true
  }
  for (const sig of info.sigs) {
    const params = new Set(sig.params.map((p) => p.name))
    if (!walk(sig.body, params)) return false
  }
  return true
}

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
  // A plain data map: inert iff every value is. A typed-map TYPE literal
  // (`{ :Integer }`) also reports isMap() but its data is a typed-map
  // structure, not an OrderedMap — treat it as an inert canonical constant
  // (handled by the data===null / fall-through below), never call asMap on it.
  if (v.isMap() && v.data instanceof OrderedMap) {
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
  // Anything surviving the filters above is inert: a scalar (data!==null) or
  // a bare type literal (data===null — `Integer`/`None`/…), both immutable
  // canonical constants (the analogue of Go's OpPushType).
  return true
}

/**
 * polyEligible reports whether a signature can be re-matched at run time
 * by OpCallNativePoly: a plain value-dispatched native — no quoted/raw-arg
 * capture (quoteArgs) and no list-eval suppression (noEvalArgs), since the
 * VM only has evaluated operands on the stack at the poly site. (runInCheckMode
 * / recordsOwnEvent / compileFallback sigs never reach recordCall.) Mirrors
 * the eligibility guards in eng/go/carrier.go::tryRecordPoly.
 */
function polyEligible(sig: Signature): boolean {
  return !(sig.quoteArgs && sig.quoteArgs.size > 0) && !(sig.noEvalArgs && sig.noEvalArgs.size > 0)
}

/** How a recorded operand is produced. */
export type Operand =
  | { readonly kind: 'const'; readonly value: Value }
  | { readonly kind: 'event'; readonly slot: number }

/**
 * A recorded native dispatch. `slot` is its global event/local index. A
 * MONO call bakes the checker-selected `sig` (lowers to OpCallNative); a
 * POLY call (`poly: true`, no `sig`) left the overload open because a
 * dynamic operand/result reached the site, and re-matches at run time
 * (lowers to OpCallNativePoly).
 */
export interface CallEvent {
  readonly kind: 'call'
  /** Base frame slot; the call's `nout` results occupy slot..slot+nout-1. */
  readonly slot: number
  /** Number of results the call produces (0, 1, or N). */
  readonly nout: number
  readonly word: string
  readonly sig?: Signature
  readonly poly?: true
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

/**
 * A recorded interpreter island: the word + its baked args as re-runnable
 * `tokens`, plus the threaded operand inputs `ins` (≤ 1) the VM pops and
 * preloads onto the island. Lowered to OpFallback; re-run through a
 * sub-engine at VM runtime while the surrounding compiled code keeps going.
 */
export interface FallbackEvent {
  readonly kind: 'fallback'
  readonly slot: number
  readonly word: string
  readonly tokens: readonly Value[]
  readonly ins: readonly Operand[]
}

export type Event =
  | CallEvent
  | BranchEvent
  | LoopEvent
  | MakeListEvent
  | MakeMapEvent
  | TrapEvent
  | FallbackEvent

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
  /** The registry this pass runs against (set by compileCheck); enables
   * classify's lexical-capture check for bakeable fn values. */
  registry: Registry | undefined

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
  /** island-output carrier → the self-contained token span that reproduces it
   *  (0-input islands only — `quote [..]`, an interp/xml island). Lets another
   *  island that re-runs tokens (an interp string) inline the producer span in
   *  place of a capture it cannot bake as a const. */
  private readonly islandTokens = new WeakMap<Value, readonly Value[]>()

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
    // A bare type literal (data===null, not a carrier — `Integer`, `None`,
    // `List`, …) or the none value is an immutable canonical CONSTANT — bake
    // it (Go's OpPushType / a none const). Words/marks/etc. were filtered by
    // isInert above and carry non-null data, so this admits only type nodes.
    if ((!v.carrier && v.data === null) || v.isNone()) return { kind: 'const', value: v }
    // A self-contained (capture-free) fn VALUE is an immutable constant too —
    // bake it so a word consuming a fn operand (typeof/is on `( fn […] )`)
    // compiles. A fn capturing an enclosing binding isn't bakeable.
    if (v.isFnDef() && v.isConcrete() && this.registry !== undefined && fnCaptureFree(v.asFnDef(), this.registry)) {
      return { kind: 'const', value: v }
    }
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
    // A dynamic operand or result means the checker could not commit to one
    // overload. Rather than refuse, record a POLY call for an eligible
    // builtin (the VM re-matches at run time); only refuse if that declines.
    const dynamic = args.some((a) => a.carrier && a.dynamic) || out.some((o) => o.carrier && o.dynamic)
    if (dynamic) {
      if (polyEligible(sig) && this.recordPolyCall(word, args, out)) return
      this.markUncompilable(`${word}: dynamic operand/result not compilable`)
      return
    }
    const ops = this.resolveOperands(word, args)
    if (ops === null) return
    // A call produces `out.length` results (0, 1, or N) occupying that many
    // consecutive frame slots from `base`; each out value threads to its own
    // slot, so a multi-result word (dup/swap/over) and a 0-result word (drop)
    // lower uniformly. Mirrors Go's per-result event indexing (resIdx).
    const base = this.reserveSlots(out.length)
    this.target().push({ kind: 'call', slot: base, nout: out.length, word, sig, args: ops })
    for (let i = 0; i < out.length; i++) this.producedBy.set(out[i]!, base + i)
    this.siteCounts.mono++
  }

  /** Reserve `n` consecutive frame slots, returning the base slot. */
  private reserveSlots(n: number): number {
    const base = this.seq
    this.seq += n
    return base
  }

  /** Classify each arg to an operand, latching uncompilable on a miss. */
  private resolveOperands(word: string, args: readonly Value[]): Operand[] | null {
    const ops: Operand[] = []
    for (const a of args) {
      const o = this.classify(a)
      if (o === null) {
        this.markUncompilable(`${word}: operand of unknown provenance`)
        return null
      }
      ops.push(o)
    }
    return ops
  }

  /**
   * Record a poly call: the word + its resolved operands, with no baked
   * signature (the VM re-matches the word's overloads against the concrete
   * operands at run time). Returns false (caller lets the refusal stand)
   * when an operand's provenance is unknown. Mirrors
   * eng/go/emit.go::RecordPolyCall.
   */
  recordPolyCall(word: string, args: readonly Value[], out: readonly Value[]): boolean {
    const ops: Operand[] = []
    for (const a of args) {
      const o = this.classify(a)
      if (o === null) return false
      ops.push(o)
    }
    const base = this.reserveSlots(out.length)
    this.target().push({ kind: 'call', slot: base, nout: out.length, word, poly: true, args: ops })
    for (let i = 0; i < out.length; i++) this.producedBy.set(out[i]!, base + i)
    this.siteCounts.poly++
    return true
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

  /**
   * Record an interpreter-island fallback for a dispatch the recorder
   * cannot lower natively. Each arg is classified: a const (inert literal)
   * is BAKED into the island's token sequence; an event result is THREADED
   * (popped from the operand stack and preloaded onto the island at run
   * time). Stage 9 admits at most one threaded input, and only when no
   * args are baked alongside it (a baked+threaded mix needs the positional
   * span reconstruction eng/go does for barriered words — deferred). The
   * single result `out` threads downstream. Returns false (caller lets the
   * refusal stand) when an arg's provenance is unknown or the shape is
   * beyond this stage. Mirrors eng/go/emit.go::RecordFallback.
   */
  /**
   * Record a single-value island: re-run `token` verbatim through a
   * sub-engine at run time (no threaded inputs), its result threading to
   * `out`. Used for a self-contained interpolated string / template whose
   * assembly the recorder does not model but which is faithfully reproduced
   * by re-evaluating the token. Mirrors a 0-input OpFallback.
   */
  recordValueIsland(token: Value, out: Value, desc: string): void {
    this.recordTokenIsland([token], out, desc)
  }

  /**
   * Record an island over an explicit, self-contained token span (no
   * threaded inputs): the VM re-runs `tokens` through a sub-engine and the
   * result threads to `out`. Used for a meta dispatch the recorder can't
   * model but whose tokens reproduce it (e.g. `inspect <word>`). The caller
   * is responsible for ensuring the tokens reference nothing run-time-bound.
   */
  recordTokenIsland(tokens: Value[], out: Value, desc: string): void {
    if (this.uncompilableReason !== undefined) return
    const slot = this.seq++
    this.target().push({ kind: 'fallback', slot, word: desc, tokens, ins: [] })
    this.producedBy.set(out, slot)
    this.islandTokens.set(out, tokens)
    this.siteCounts.dynamic++
  }

  /**
   * If `v` was produced by a 0-input token island, return the self-contained
   * token span that reproduces it (else null). A consumer that itself re-runs
   * tokens (an interpolated-string island capturing `def x quote [..]`) can
   * inline this span where it cannot bake `v` as a const.
   */
  islandTokensFor(v: Value): readonly Value[] | null {
    return this.islandTokens.get(v) ?? null
  }

  recordFallback(
    word: string,
    args: readonly Value[],
    out: readonly Value[],
    registry?: Registry,
    noEvalArgs?: ReadonlySet<number>,
  ): boolean {
    if (this.uncompilableReason !== undefined) return false
    if (out.length !== 1) return false
    const tokens: Value[] = [newWord(word)]
    const ins: Operand[] = []
    for (let i = 0; i < args.length; i++) {
      const a = args[i]!
      const o = this.classify(a)
      if (o !== null) {
        if (o.kind === 'const') tokens.push(o.value)
        else ins.push(o)
        continue
      }
      // A code-body arg (a noEvalArgs position) is re-run raw by the island,
      // so bake the concrete body verbatim even though it isn't inert data
      // (replayq's [ (1 addq 2) ] / nested body).
      if (noEvalArgs?.has(i) && a.isConcrete()) {
        tokens.push(a)
        continue
      }
      // A self-contained (capture-free) fn VALUE bakes into the island as a
      // CLOSURE: re-running [word … fnVal …] applies it exactly as the
      // interpreter does (Go islands non-compiled callables the same way in
      // vm.go::callDynamic). A fn capturing an enclosing def is not bakeable
      // (its captured names aren't bound at run time) → refuse, fall back.
      if (registry !== undefined && a.isConcrete() && a.isFnDef() && fnCaptureFree(a.asFnDef(), registry)) {
        tokens.push(a)
        continue
      }
      return false
    }
    if (ins.length > 1) return false
    if (ins.length === 1 && tokens.length > 1) return false // baked+threaded mix (deferred)
    const slot = this.seq++
    this.target().push({ kind: 'fallback', slot, word, tokens, ins })
    this.producedBy.set(out[0]!, slot)
    this.siteCounts.dynamic++
    return true
  }
}
