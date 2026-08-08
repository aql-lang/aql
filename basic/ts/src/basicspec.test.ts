// The TS half of the base-layer parity corpus (basic/spec/*.tsv).
//
// basic/go/basicspec_test.go is the other half. The two runners share NO
// code — they read the same files and each builds the registry and renders
// the residual independently, exactly as core/spec's and parser/spec's
// pairs do. That independence is the point: shared scaffolding can hide
// the same bug from both engines (design/CORE-GO-TS-DEFECTS.0.md, blind
// spot 9).
//
// This is a SPEC, not a differential: the `expected` column is the
// documented contract, so a row can legitimately fail on BOTH engines.

import { describe, it } from 'node:test'
import { strict as assert } from 'node:assert'
import * as fs from 'node:fs'
import * as path from 'node:path'
import { fileURLToPath } from 'node:url'

import {
  BoruError,
  canon,
  Engine,
  newInteger,
  newString,
  newWord,
  Registry,
  type Value,
} from '@boru-lang/core'
import { stackNatives } from './native-stack.ts'

const __dirname = path.dirname(fileURLToPath(import.meta.url))
const SPEC_DIR = path.resolve(__dirname, '..', '..', 'spec')

interface SpecRow {
  file: string
  line: number
  prog: string
  expected: string
  note: string
}

function readSpec(file: string): SpecRow[] {
  const text = fs.readFileSync(path.join(SPEC_DIR, file), 'utf8')
  const rows: SpecRow[] = []
  const lines = text.split('\n')
  for (let n = 0; n < lines.length; n++) {
    const line = lines[n]!
    // A line is a COMMENT only when it starts with '#' AND carries no tab —
    // '#' is boru's own comment marker, so a program may begin with one.
    if ('' === line.trim() || (line.startsWith('#') && !line.includes('\t'))) continue
    const parts = line.split('\t')
    assert.ok(parts.length >= 2, `${file}:${n + 1}: need at least 2 tab-separated columns`)
    rows.push({
      file,
      line: n + 1,
      prog: parts[0]!,
      expected: parts[1]!,
      note: parts[2] ?? '',
    })
  }
  return rows
}

/**
 * token builds one program token: a decimal is an Integer, '…' is a
 * String, anything else is a Word. Deliberately tiny — the corpus is about
 * the WORDS, so the token notation stays out of the way.
 */
function token(tok: string): Value {
  if (tok.startsWith("'") && tok.endsWith("'") && tok.length >= 2) {
    return newString(tok.slice(1, -1))
  }
  if (/^-?\d+$/.test(tok)) return newInteger(BigInt(tok))
  return newWord(tok)
}

/** Registers ONLY the stack vocabulary, so a row depends on nothing else. */
function specRegistry(): Registry {
  const r = new Registry()
  for (const nf of stackNatives) r.registerNativeFunc(nf)
  return r
}

function runProgram(prog: string): string {
  const toks = prog.split(/\s+/).filter((s) => '' !== s).map(token)
  try {
    return canon(new Engine(specRegistry()).run(toks))
  } catch (e) {
    if (e instanceof BoruError) return `ERROR:${e.code}`
    return `ERROR:non_boru:${(e as Error).message}`
  }
}

describe('basic spec — stack.tsv', () => {
  const rows = readSpec('stack.tsv')
  assert.ok(rows.length > 0, 'basic/spec produced no rows — the corpus is not being read')
  for (const r of rows) {
    it(`${r.line}: ${r.prog}`, () => {
      assert.equal(runProgram(r.prog), r.expected, r.note)
    })
  }
})
