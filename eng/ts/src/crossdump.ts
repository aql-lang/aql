// Cross-engine differential dumper.
//
// Emits one JSONL record per shared-corpus row so the Go cross-diff harness
// (test/go/engspec/crossdiff_test.go) can compare the TS engine's result to
// the Go engine's, row by row. This is the TS half of the "each engine
// validates against the other" contract: the Go test runs the identical
// eng/spec corpus through its own kernel and diffs the two result streams,
// reporting agreements, divergences (hard fail), and functionality gaps
// (one engine produces a value where the other errors — reported, not failed).
//
// Each record: {mode, file, line, input, ok, out}
//   - ok=true  → `out` is the canonical result string (renderStack), which
//     must equal the Go engine's eng.Canon for the same row.
//   - ok=false → `out` is the AqlError code (or a TOKENIZE:/UNEXPECTED: tag),
//     so error-parity is compared by taxonomy code, not message text.
//
// Run standalone for inspection:  node --experimental-strip-types src/crossdump.ts
import * as fs from 'node:fs'
import * as path from 'node:path'

import { AqlError, Engine, Registry, type Value } from './index.ts'
import { SPEC_DIR, parseSpec, registerSpecWords, renderStack, tokenize } from './spec-fixture.ts'

function fresh(): Registry {
  const r = new Registry()
  registerSpecWords(r)
  return r
}

function valueResult(input: string): { ok: boolean; out: string } {
  let values: Value[]
  try {
    values = tokenize(input)
  } catch (e) {
    return { ok: false, out: `TOKENIZE:${(e as Error).message}` }
  }
  try {
    const out = new Engine(fresh()).run(values)
    return { ok: true, out: renderStack(out) }
  } catch (e) {
    if (e instanceof AqlError) return { ok: false, out: e.code }
    return { ok: false, out: `UNEXPECTED:${(e as Error).message}` }
  }
}

const lines: string[] = []
const files = fs
  .readdirSync(SPEC_DIR)
  .filter((f) => f.endsWith('.tsv'))
  .sort()
for (const file of files) {
  for (const row of parseSpec(path.join(SPEC_DIR, file))) {
    const r = valueResult(row.input)
    lines.push(
      JSON.stringify({ mode: 'value', file, line: row.lineNum, input: row.input, ok: r.ok, out: r.out }),
    )
  }
}
process.stdout.write(lines.join('\n') + '\n')
