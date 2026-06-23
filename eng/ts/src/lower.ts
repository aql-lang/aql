// Finalize: lower a recorded call-event trace (emit.ts) into a Program
// (bytecode.ts). Ported from eng/go/emit.go::Finalize + lower.go.
//
// Stage 1 uses a LOCALS-THREADED layout: every event's single result is
// stored to a frame local (slot == event index) and re-pushed wherever a
// later event or the residual consumes it. This sidesteps the operand
// stack-scheduling (swaps/reverses) eng/go/lower.go performs — locals are
// equally V8-friendly (a flat preallocated array), and a later stage can
// add stack-threading for compactness if a benchmark warrants it.
import { CodeBuilder, OpCallNative, OpPushConst, OpPushLocal, OpStoreLocal, type Program, type SigRef } from './bytecode.ts'
import type { EmitState, Operand } from './emit.ts'
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

  // Classify residual operands up front so an unknown one refuses before
  // we build any code.
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
  const sigIdx = (word: string, ref: SigRef['sig']): number => sigs.push({ word, sig: ref }) - 1

  let maxStack = 0
  const pushOperand = (o: Operand): void => {
    if (o.kind === 'const') code.emit(OpPushConst, constIdx(o.value))
    else code.emit(OpPushLocal, o.event)
  }

  for (let e = 0; e < emit.events.length; e++) {
    const ev = emit.events[e]!
    const n = ev.args.length
    // Push args in REVERSE sig order so sig[0] lands on top: the VM's
    // CALL_NATIVE reads args[i] = stack[sp-1-i].
    for (let j = n - 1; j >= 0; j--) pushOperand(ev.args[j]!)
    // Peak during this event: max(n args pushed, the 1 result before it
    // is stored).
    if (Math.max(n, 1) > maxStack) maxStack = Math.max(n, 1)
    code.emit(OpCallNative, sigIdx(ev.word, ev.sig))
    code.emit(OpStoreLocal, e)
  }

  for (const o of residualOps) pushOperand(o)
  if (residualOps.length > maxStack) maxStack = residualOps.length

  const { ops, args } = code.freeze()
  return {
    program: { ops, args, consts, sigs, maxStack, numLocals: emit.events.length },
  }
}
