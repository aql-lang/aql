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
  OpJmp,
  OpJmpIfFalse,
  OpPushConst,
  OpPushLocal,
  OpStoreLocal,
  type Program,
  type SigRef,
} from './bytecode.ts'
import type { EmitState, Event, Operand } from './emit.ts'
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
  const constIdx = (v: Value): number => consts.push(v) - 1
  const sigIdx = (word: string, sig: SigRef['sig']): number => sigs.push({ word, sig }) - 1

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

  const lowerEvents = (events: readonly Event[]): void => {
    for (const ev of events) {
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

  lowerEvents(emit.events)
  for (const o of residualOps) pushOperand(o)

  const { ops, args } = code.freeze()
  return {
    program: { ops, args, consts, sigs, maxStack, numLocals: emit.slotCount() },
  }
}
