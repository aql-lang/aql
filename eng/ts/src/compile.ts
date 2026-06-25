// Compile orchestration, ported from lang/go/aql.go::CompileCheck +
// RunCompiled. compileCheck runs the engine in check mode with an
// EmitState installed (so the check pass doubles as the bytecode
// recording pass) and finalises the trace into a Program. runCompiled
// compiles and executes via the VM, falling back to the interpreter for
// any source the compiler refuses.
import { Engine } from './engine.ts'
import { EmitState } from './emit.ts'
import { AqlError } from './error.ts'
import { finalize, type FinalizeResult } from './lower.ts'
import type { Program } from './bytecode.ts'
import type { Registry } from './registry.ts'
import { runProgram } from './vm.ts'
import type { Value } from './value.ts'

/**
 * Compile `input` to a Program, or refuse with the first blocking
 * reason. Installs an EmitState, runs the check-mode engine to record
 * the call trace, then finalises. A program with any error-severity
 * diagnostic refuses (it would not run cleanly).
 */
export function compileCheck(registry: Registry, input: Value[]): FinalizeResult {
  const emit = new EmitState()
  registry.check.emit = emit
  emit.registry = registry // enables classify's fn-value capture check
  const done = registry.check.begin()
  let residual: Value[]
  try {
    residual = new Engine(registry).run(input)
  } catch (e) {
    // A check/record pass that throws is uncompilable — refuse so the caller
    // falls back to the interpreter (which runs the row faithfully), never
    // crashing runCompiled. Mirrors Go's RunCompiled always degrading to the
    // interpreter rather than propagating a compile-time panic.
    return { refused: `check pass threw: ${e instanceof AqlError ? e.code : (e as Error).message}` }
  } finally {
    done()
    registry.check.emit = undefined
  }
  if (registry.check.diagnostics.some((d) => d.severity === 'error')) {
    return { refused: 'source has check errors' }
  }
  return finalize(emit, residual)
}

/** A compiled run's outcome: the residual stack and whether it compiled. */
export interface RunCompiledResult {
  residual: Value[]
  compiled: boolean
  /** The refusal reason when `compiled` is false (else undefined). */
  reason?: string
}

/**
 * Compile and execute `input` through the VM; on refusal, fall back to
 * the interpreter. The residual matches the interpreter's for the same
 * source either way.
 */
export function runCompiled(registry: Registry, input: Value[]): RunCompiledResult {
  const snapshot = registry.snapshot()
  const result = compileCheck(registry, input)
  if ('program' in result) {
    return { residual: runProgram(result.program, registry), compiled: true }
  }
  registry.restore(snapshot)
  return { residual: new Engine(registry).run([...input]), compiled: false, reason: result.refused }
}

/** Compile to a Program for inspection/disassembly, or refuse. */
export function compile(registry: Registry, input: Value[]): { program: Program } | { refused: string } {
  return compileCheck(registry, input)
}
