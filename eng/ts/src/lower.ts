// Finalize: lower a recorded event trace (emit.ts) into a Program
// (bytecode.ts). Ported from eng/go/emit.go::Finalize + lower.go.
//
// LAYOUT: every event's single result is stored to a frame local
// (slot == the event's global seq) and re-pushed wherever a later event
// or the residual consumes it. This locals-threaded scheme handles
// sharing and control flow uniformly and is V8-friendly (flat
// preallocated locals array). A branch lowers its condition + a
// JmpIfFalse, then each arm computes its result into the SAME branch
// slot, bracketed by a Jmp — so after the branch the merged result lives
// in locals[branchSlot].
import {
  CodeBuilder,
  OpCallNative,
  OpForNext,
  OpForSetup,
  OpJmp,
  OpJmpIfFalse,
  OpMakeList,
  OpMakeMap,
  OpPushConst,
  OpPushLocal,
  OpStoreLocal,
  type Program,
  type SigRef,
} from './bytecode.ts'
import type { EmitState, Event, LoopEvent, Operand } from './emit.ts'
import type { Value } from './value.ts'

/** Either a compiled Program or the first reason compilation refused. */
export type FinalizeResult = { program: Program } | { refused: string }

/**
 * Lower the trace + residual to a Program, or refuse with a reason. The
 * residual is the check pass's final carrier stack (bottom → top); each
 * residual value must classify to a const or a recorded event result.
 */
export function finalize(emit: EmitState, residual: readonly Value[]): FinalizeResult {
  if (emit.uncompilableReason !== undefined) return { refused: emit.uncompilableReason }

  const residualOps: Operand[] = []
  for (const r of residual) {
    const o = emit.classify(r)
    if (o === null) return { refused: 'residual value of unknown provenance' }
    residualOps.push(o)
  }

  const code = new CodeBuilder()
  const consts: Value[] = []
  const sigs: SigRef[] = []
  const makeMaps: string[][] = []
  const constIdx = (v: Value): number => consts.push(v) - 1
  const sigIdx = (word: string, sig: SigRef['sig']): number => sigs.push({ word, sig }) - 1
  const makeMapIdx = (keys: readonly string[]): number => makeMaps.push([...keys]) - 1

  // Simulated operand-stack depth, tracked to size the VM stack.
  let sp = 0
  let maxStack = 0
  const note = (): void => {
    if (sp > maxStack) maxStack = sp
  }
  const pushOperand = (o: Operand): void => {
    if (o.kind === 'const') code.emit(OpPushConst, constIdx(o.value))
    else code.emit(OpPushLocal, o.slot)
    sp++
    note()
  }

  // First refusal reason hit while lowering (e.g. a loop in a position
  // this stage does not compile). Checked after lowering.
  let refused: string | undefined

  const lowerEvents = (events: readonly Event[]): void => {
    for (const ev of events) {
      if (refused !== undefined) return
      if (ev.kind === 'call') {
        const n = ev.args.length
        // Push args in REVERSE sig order so sig[0] lands on top.
        for (let j = n - 1; j >= 0; j--) pushOperand(ev.args[j]!)
        code.emit(OpCallNative, sigIdx(ev.word, ev.sig))
        sp = sp - n + 1 // pop n args, push 1 result
        note()
        code.emit(OpStoreLocal, ev.slot)
        sp-- // result moved into a local
        continue
      }
      if (ev.kind === 'loop') {
        // A loop is variadic; it only compiles as the trailing producer
        // (handled in finalize). Anywhere else (a fragment, a non-final
        // position) is not lowerable this stage.
        refused = 'loop in non-trailing position not compilable (stage 5)'
        return
      }
      if (ev.kind === 'makeList') {
        for (const o of ev.elements) pushOperand(o) // forward order: element 0 deepest
        code.emit(OpMakeList, ev.elements.length)
        sp = sp - ev.elements.length + 1
        note()
        code.emit(OpStoreLocal, ev.slot)
        sp--
        continue
      }
      if (ev.kind === 'makeMap') {
        for (const o of ev.values) pushOperand(o) // value for keys[0] deepest
        code.emit(OpMakeMap, makeMapIdx(ev.keys))
        sp = sp - ev.values.length + 1
        note()
        code.emit(OpStoreLocal, ev.slot)
        sp--
        continue
      }
      // Branch: cond on top -> JmpIfFalse(else); then arm -> store to the
      // branch slot -> Jmp(end); else arm -> store to the branch slot.
      pushOperand(ev.cond)
      const jf = code.emit(OpJmpIfFalse, 0)
      sp-- // JmpIfFalse pops the condition
      lowerEvents(ev.then.events)
      pushOperand(ev.then.residual)
      code.emit(OpStoreLocal, ev.slot)
      sp--
      const jend = code.emit(OpJmp, 0)
      code.patch(jf, code.length) // else arm starts here
      lowerEvents(ev.else.events)
      pushOperand(ev.else.residual)
      code.emit(OpStoreLocal, ev.slot)
      sp--
      code.patch(jend, code.length) // merge point
    }
  }

  // Lower a counted loop in place: its per-iteration body residual is
  // pushed and LEFT on the stack so the values accumulate. The result is
  // variadic — only the program residual absorbs it.
  const lowerLoop = (ev: LoopEvent): void => {
    pushOperand(ev.step)
    pushOperand(ev.end)
    pushOperand(ev.start) // start on top
    code.emit(OpForSetup, ev.iterSlot)
    sp -= 3
    const head = code.length
    const fn = code.emit(OpForNext, 0) // exit target backpatched below
    lowerEvents(ev.body.events)
    pushOperand(ev.body.residual) // accumulates (not stored)
    code.emit(OpJmp, head) // back-edge to FOR_NEXT
    code.patch(fn, code.length) // FOR_NEXT exits here
  }

  // Trailing-loop case: the last top-level event is a loop whose result
  // IS the program residual. Lower the prefix normally, then the loop
  // inline (leaving its accumulated values as the residual).
  const last = emit.events[emit.events.length - 1]
  const trailingLoop =
    last !== undefined &&
    last.kind === 'loop' &&
    residualOps.length === 1 &&
    residualOps[0]!.kind === 'event' &&
    residualOps[0]!.slot === last.slot

  if (trailingLoop) {
    lowerEvents(emit.events.slice(0, -1))
    if (refused !== undefined) return { refused }
    lowerLoop(last as LoopEvent)
  } else {
    lowerEvents(emit.events)
    if (refused !== undefined) return { refused }
    for (const o of residualOps) pushOperand(o)
  }

  const { ops, args } = code.freeze()
  return {
    program: { ops, args, consts, sigs, makeMaps, maxStack, numLocals: emit.slotCount() },
  }
}
