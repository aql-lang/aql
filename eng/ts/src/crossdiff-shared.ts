// Shared row-computation for the cross-engine differential, with NO top-level
// side effects so it is safe to import from both the dumper (crossdump.ts) and
// the TS-side diff test (crossdiff.test.ts).
//
// tsRows() runs every shared-corpus row (eng/spec value mode + eng/spec/check
// check mode) through the TS engine and returns one record per row. Success
// rows carry the canonical result string (renderStack / renderCheck — which
// must equal the Go engine's eng.Canon / renderCheck); error rows carry the
// BoruError taxonomy code, so error-parity is compared by code, not message.
import * as fs from 'node:fs'
import * as path from 'node:path'

import { BoruError, Engine, Registry, type Value } from './index.ts'
import { CHECK_SPEC_DIR, parseCheckSpec, runCheckRow } from './check-fixture.ts'
import { SPEC_DIR, parseSpec, registerSpecWords, renderStack, tokenize } from './spec-fixture.ts'

export interface CrossRec {
  mode: 'value' | 'check'
  file: string
  line: number
  input: string
  ok: boolean
  out: string
}

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
    // A parse failure carries the same taxonomy code the Go side
    // reports (the jsonic parser throws BoruError) — compare by code,
    // exactly like run errors.
    if (e instanceof BoruError) return { ok: false, out: e.code }
    return { ok: false, out: `TOKENIZE:${(e as Error).message}` }
  }
  try {
    const out = new Engine(fresh()).run(values)
    return { ok: true, out: renderStack(out) }
  } catch (e) {
    if (e instanceof BoruError) return { ok: false, out: e.code }
    return { ok: false, out: `UNEXPECTED:${(e as Error).message}` }
  }
}

function checkResult(input: string): { ok: boolean; out: string } {
  try {
    return { ok: true, out: runCheckRow(input) }
  } catch (e) {
    if (e instanceof BoruError) return { ok: false, out: e.code }
    return { ok: false, out: `ERR:${(e as Error).message}` }
  }
}

export function tsRows(): CrossRec[] {
  const rows: CrossRec[] = []

  for (const file of fs.readdirSync(SPEC_DIR).filter((f) => f.endsWith('.tsv')).sort()) {
    for (const row of parseSpec(path.join(SPEC_DIR, file))) {
      const r = valueResult(row.input)
      rows.push({ mode: 'value', file, line: row.lineNum, input: row.input, ok: r.ok, out: r.out })
    }
  }

  for (const file of fs.readdirSync(CHECK_SPEC_DIR).filter((f) => f.endsWith('.tsv')).sort()) {
    for (const row of parseCheckSpec(path.join(CHECK_SPEC_DIR, file))) {
      const r = checkResult(row.input)
      rows.push({ mode: 'check', file, line: row.lineNum, input: row.input, ok: r.ok, out: r.out })
    }
  }

  return rows
}
