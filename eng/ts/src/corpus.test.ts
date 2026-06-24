// Full-corpus differential gate — the parity ratchet (mirrors Go's
// TestSpecCompiledDifferential + COMPILED_STATUS). Every value-mode row in
// the shared eng/spec/*.tsv corpus is run through BOTH the bytecode compiler
// (runCompiled) and the interpreter; a row the compiler accepts MUST produce
// a canon-equal residual (else it is a miscompilation — a hard failure), and
// a row it refuses falls back (still correct, but counted). The summary test
// reports the compile rate + the refusal-reason histogram and enforces a
// monotonic floor so coverage can only ratchet up.
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
const COMPILE_FLOOR = 1575

type RowResult =
  | { kind: 'skip' } // ERROR row, tokenize failure, or interpreter error (not a value row)
  | { kind: 'compiled' } // compiled and matched the interpreter
  | { kind: 'fallback'; reason: string } // refused; ran on the interpreter
  | { kind: 'mismatch'; detail: string } // compiled but diverged — a miscompilation

function fresh(): Registry {
  const r = new Registry()
  registerSpecWords(r)
  return r
}

function classifyRow(input: string, expected: string): RowResult {
  if (expected.startsWith('ERROR:')) return { kind: 'skip' }
  let values: Value[]
  try {
    values = tokenize(input)
  } catch {
    return { kind: 'skip' }
  }
  // Interpreter oracle. A row the interpreter errors on is not a value row
  // (error-parity is gated separately) — skip it here.
  let interp: string
  try {
    interp = renderStack(new Engine(fresh()).run([...values]))
  } catch (e) {
    if (e instanceof AqlError) return { kind: 'skip' }
    throw e
  }
  // Compile path.
  let result: ReturnType<typeof compileCheck>
  try {
    result = compileCheck(fresh(), [...values])
  } catch {
    // The check/record pass itself threw — treat as a fallback (the real
    // runCompiled would fall back to the interpreter, which matched above).
    return { kind: 'fallback', reason: 'check pass threw' }
  }
  if (!('program' in result)) return { kind: 'fallback', reason: normalizeReason(result.refused) }
  let compiled: string
  try {
    compiled = renderStack(runProgram(result.program, fresh()))
  } catch (e) {
    const msg = e instanceof AqlError ? `${e.code}` : (e as Error).message
    return { kind: 'mismatch', detail: `compiled threw (${msg}) where interpreter produced ${interp}` }
  }
  if (compiled !== interp) {
    return { kind: 'mismatch', detail: `compiled=${compiled} interp=${interp}` }
  }
  return { kind: 'compiled' }
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

  it('summary: compile rate + refusal histogram (ratchet)', () => {
    const value = compiled + fallback
    const top = [...reasons.entries()].sort((a, b) => b[1] - a[1]).slice(0, 20)
    const lines = [
      '',
      `  corpus parity: ${compiled}/${value} value rows compiled (${
        value > 0 ? ((100 * compiled) / value).toFixed(1) : '0'
      }%), ${fallback} fallback, ${skipped} skipped`,
      '  top refusal reasons:',
      ...top.map(([r, n]) => `    ${String(n).padStart(4)}  ${r}`),
    ]
    console.log(lines.join('\n'))
    assert.equal(mismatches.length, 0, `miscompilations:\n${mismatches.slice(0, 20).join('\n')}`)
    assert.ok(
      compiled >= COMPILE_FLOOR,
      `compiled ${compiled} < floor ${COMPILE_FLOOR} — coverage regressed`,
    )
  })
})
