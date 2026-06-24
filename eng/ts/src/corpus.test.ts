// Full-corpus differential gate — the parity ratchet (mirrors Go's
// TestSpecCompiledDifferential + COMPILED_STATUS). EVERY row in the shared
// eng/spec/*.tsv corpus is run through BOTH the bytecode compiler (compileCheck
// + runProgram) and the interpreter, and the two OUTCOMES must agree:
//   - a value row the compiler accepts MUST produce a canon-equal residual;
//   - an ERROR row MUST either compile to a program that raises the SAME
//     AqlError code (a native OpTrap), or refuse and fall back to the
//     interpreter (which errors identically).
// Any silent divergence — a compiled program that succeeds where the
// interpreter errors (or vice versa), or two differing error codes — is a
// miscompilation and a HARD failure. The summary enforces a monotonic
// compile floor AND a monotonic native-error-parity floor so neither can
// regress.
import { describe, it } from 'node:test'
import { strict as assert } from 'node:assert'
import * as fs from 'node:fs'
import * as path from 'node:path'

import { canon } from './canon.ts'
import { AqlError, Engine, Registry, type Value } from './index.ts'
import { compileCheck } from './compile.ts'
import { runProgram } from './vm.ts'
import { SPEC_DIR, parseSpec, registerSpecWords, renderStack, tokenize } from './spec-fixture.ts'

// The compiled-coverage floor: at least this many value rows must compile and
// match the interpreter. Raise it as feature gaps close (never lower it).
const COMPILE_FLOOR = 1589
// The native-error-parity floor: at least this many ERROR rows must compile to
// a program that raises the interpreter's exact error code (a native OpTrap),
// rather than refusing. Raise it as trap coverage grows (never lower it).
const ERROR_PARITY_FLOOR = 27

type RowResult =
  | { kind: 'skip' } // tokenize failure (both paths fail identically)
  | { kind: 'compiled' } // value row: compiled and matched the interpreter
  | { kind: 'fallback'; reason: string } // value row: refused; ran on the interpreter
  | { kind: 'errorNative' } // ERROR row: compiled to a program that traps with the same code
  | { kind: 'errorFallback' } // ERROR row: refused; interpreter fallback errors identically
  | { kind: 'mismatch'; detail: string } // any silent divergence — a miscompilation

/** An execution outcome: a rendered residual, or an AqlError code. */
type Outcome = { ok: true; residual: string } | { ok: false; code: string }

function fresh(): Registry {
  const r = new Registry()
  registerSpecWords(r)
  return r
}

function run(fn: () => Value[]): Outcome {
  try {
    return { ok: true, residual: renderStack(fn()) }
  } catch (e) {
    if (e instanceof AqlError) return { ok: false, code: e.code }
    throw e
  }
}

function classifyRow(input: string, _expected: string): RowResult {
  let values: Value[]
  try {
    values = tokenize(input)
  } catch {
    // Both the interpreter and the compiler tokenize the same source, so a
    // tokenize failure is shared — nothing to differentiate.
    return { kind: 'skip' }
  }
  // Interpreter oracle (the trusted outcome).
  const interp = run(() => new Engine(fresh()).run([...values]))
  // Compile path. A check/record pass that throws, or a refusal, both degrade
  // to the interpreter at run time, so their outcome equals `interp` by
  // construction — error-parity holds for refusals without re-running.
  let result: ReturnType<typeof compileCheck>
  try {
    result = compileCheck(fresh(), [...values])
  } catch {
    return interp.ok ? { kind: 'fallback', reason: 'check pass threw' } : { kind: 'errorFallback' }
  }
  if (!('program' in result)) {
    return interp.ok
      ? { kind: 'fallback', reason: normalizeReason(result.refused) }
      : { kind: 'errorFallback' }
  }
  // The compiler committed to a program — its OUTCOME must match the interpreter.
  const ran = run(() => runProgram(result.program, fresh()))
  if (interp.ok && ran.ok) {
    return ran.residual === interp.residual
      ? { kind: 'compiled' }
      : { kind: 'mismatch', detail: `compiled=${ran.residual} interp=${interp.residual}` }
  }
  if (!interp.ok && !ran.ok) {
    return ran.code === interp.code
      ? { kind: 'errorNative' }
      : { kind: 'mismatch', detail: `compiled error ${ran.code} != interp error ${interp.code}` }
  }
  if (interp.ok && !ran.ok) {
    return { kind: 'mismatch', detail: `compiled errored ${ran.code} where interp produced ${interp.residual}` }
  }
  // interp errored, compiled SUCCEEDED — the silent-success class OpTrap exists
  // to prevent. The most dangerous miscompilation; surfaced explicitly.
  return { kind: 'mismatch', detail: `compiled produced ${(ran as { residual: string }).residual} where interp errored ${(interp as { code: string }).code}` }
}

// Collapse a refusal reason to a stable bucket key (strip the leading word /
// specifics) so the histogram groups like causes.
function normalizeReason(reason: string): string {
  // Reasons are typically "<word>: <cause>" — keep the cause.
  const colon = reason.indexOf(': ')
  return colon >= 0 ? reason.slice(colon + 2) : reason
}

describe('corpus (compiled differential)', () => {
  const files = fs
    .readdirSync(SPEC_DIR)
    .filter((f) => f.endsWith('.tsv'))
    .sort()

  let compiled = 0
  let fallback = 0
  let errorNative = 0
  let errorFallback = 0
  let skipped = 0
  const mismatches: string[] = []
  const reasons = new Map<string, number>()

  for (const file of files) {
    const rows = parseSpec(path.join(SPEC_DIR, file))
    describe(file.replace(/\.tsv$/, ''), () => {
      for (const row of rows) {
        it(`L${row.lineNum} ${row.input}`, () => {
          const res = classifyRow(row.input, row.expected)
          switch (res.kind) {
            case 'skip':
              skipped++
              break
            case 'compiled':
              compiled++
              break
            case 'fallback':
              fallback++
              reasons.set(res.reason, (reasons.get(res.reason) ?? 0) + 1)
              break
            case 'errorNative':
              errorNative++
              break
            case 'errorFallback':
              errorFallback++
              break
            case 'mismatch':
              mismatches.push(`${file}:L${row.lineNum} ${row.input} — ${res.detail}`)
              break
          }
          // A miscompilation is always a hard failure, surfaced per-row.
          assert.equal(res.kind === 'mismatch', false, res.kind === 'mismatch' ? res.detail : '')
        })
      }
    })
  }

  it('summary: compile rate + error-parity + refusal histogram (ratchet)', () => {
    const value = compiled + fallback
    const errorRows = errorNative + errorFallback
    const top = [...reasons.entries()].sort((a, b) => b[1] - a[1]).slice(0, 20)
    const lines = [
      '',
      `  corpus parity: ${compiled}/${value} value rows compiled (${
        value > 0 ? ((100 * compiled) / value).toFixed(1) : '0'
      }%), ${fallback} fallback, ${skipped} skipped`,
      `  error parity:  ${errorRows} error rows verified (${errorNative} native trap, ${errorFallback} fallback)`,
      '  top refusal reasons:',
      ...top.map(([r, n]) => `    ${String(n).padStart(4)}  ${r}`),
    ]
    console.log(lines.join('\n'))
    assert.equal(mismatches.length, 0, `miscompilations:\n${mismatches.slice(0, 20).join('\n')}`)
    assert.ok(
      compiled >= COMPILE_FLOOR,
      `compiled ${compiled} < floor ${COMPILE_FLOOR} — coverage regressed`,
    )
    assert.ok(
      errorNative >= ERROR_PARITY_FLOOR,
      `native error-parity ${errorNative} < floor ${ERROR_PARITY_FLOOR} — trap coverage regressed`,
    )
  })
})
