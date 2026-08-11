// The TypeScript half of parser/spec/data.tsv — the safe DATA-decode seam
// contract (safeParseData + convertParsedNumber, the parsers behind
// StructUtil.parse). Its strict reader and renderer intentionally share no
// implementation with parser/go/dataspec_test.go.
//
// Two expected forms:
//
//   ERR <code>   the number converter refuses a claimed spelling with that
//                BoruError code — the loud half of the contract.
//   ERR          the underlying jsonic parse itself rejects the source. Its
//                message is deliberately dependency-native (safeParseData
//                throws raw JsonicError), so only the refusal is pinned.
//
// Everything else is the canonical decode render defined in the README:
// insertion-ordered `{k:v ...}` maps, `[v ...]` lists, `'...'` strings,
// `true`/`false`, `none` for null, and every numeric token rendered through
// convertParsedNumber so int/float kind and precision are part of the pin.

import { strict as assert } from 'node:assert'
import * as fs from 'node:fs'
import * as path from 'node:path'
import { describe, it } from 'node:test'
import { fileURLToPath } from 'node:url'

import { BoruError, canonValue } from '@boru-lang/core'

import { convertParsedNumber, safeParseData } from './index.ts'
import { dataKeyOrder } from './public.ts'

const SPEC_DIR = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..', '..', 'spec')
const DATA_SPEC_ROW_COUNT = 65

interface DataSpecRow {
  line: number
  src: string
  expected: string
}

function decodeDataSpecEscapes(source: string): string {
  let decoded = ''
  for (let i = 0; i < source.length; i++) {
    if ('\\' === source[i] && i + 1 < source.length) {
      const next = source[i + 1]
      if ('n' === next) {
        decoded += '\n'
        i++
        continue
      }
      if ('t' === next) {
        decoded += '\t'
        i++
        continue
      }
      if ('\\' === next) {
        decoded += '\\'
        i++
        continue
      }
    }
    decoded += source[i]
  }
  return decoded
}

function readDataSpec(): DataSpecRow[] {
  const name = 'data.tsv'
  const lines = fs.readFileSync(path.join(SPEC_DIR, name), 'utf8').split('\n')
  const rows: DataSpecRow[] = []
  const seen = new Map<string, number>()
  for (let index = 0; index < lines.length; index++) {
    const line = lines[index]!
    if ('' === line || (line.startsWith('#') && !line.includes('\t'))) continue
    const parts = line.split('\t')
    assert.equal(
      parts.length,
      3,
      `${name}:${index + 1}: need exactly 3 tab-separated columns (source, expected, note)`,
    )
    const src = decodeDataSpecEscapes(parts[0]!)
    assert.equal(
      seen.has(src),
      false,
      `${name}:${index + 1}: duplicate source (first at line ${seen.get(src)}): ${JSON.stringify(src)}`,
    )
    seen.set(src, index + 1)
    rows.push({ line: index + 1, src, expected: decodeDataSpecEscapes(parts[1]!) })
  }
  assert.equal(rows.length, DATA_SPEC_ROW_COUNT, `${name}: exact row-count ratchet`)
  return rows
}

function isPlainDataRecord(value: unknown): value is Record<string, unknown> {
  if (null === value || 'object' !== typeof value) return false
  const prototype = Object.getPrototypeOf(value)
  return null === prototype || Object.prototype === prototype
}

// renderDataNode renders one decoded node in the canonical data form. A
// BoruError from the number converter aborts the walk; any other shape is a
// harness failure, not a render.
function renderDataNode(v: unknown): string {
  const num = convertParsedNumber(v)
  if (num !== undefined) {
    return canonValue(num)
  }
  if (null === v || undefined === v) return 'none'
  if ('boolean' === typeof v) return v ? 'true' : 'false'
  if (v instanceof String) return "'" + v.valueOf() + "'"
  if ('string' === typeof v) return "'" + v + "'"
  if (Array.isArray(v)) {
    return '[' + v.map(renderDataNode).join(' ') + ']'
  }
  if (isPlainDataRecord(v)) {
    // Insertion order, NOT Object.keys order: a plain JS object enumerates
    // integer-like keys ascending regardless of what the source wrote, so
    // `{2:9, 1:8}` would render `{1:8 2:9}` here and silently disagree with
    // the Go runner's *jsonic.OrderedMap walk. dataKeyOrder reads the order
    // the seam recorded. (Still no shared code with the Go runner — each
    // consumes its own dependency's ordering API.)
    return (
      '{' +
      dataKeyOrder(v)
        .map((key) => key + ':' + renderDataNode(v[key]))
        .join(' ') +
      '}'
    )
  }
  throw new Error(`unsupported data node ${Object.prototype.toString.call(v)}`)
}

function renderDataSpec(src: string): string {
  let decoded: unknown
  try {
    decoded = safeParseData(src)
  } catch {
    return 'ERR'
  }
  try {
    return renderDataNode(decoded)
  } catch (error) {
    if (error instanceof BoruError) {
      return 'ERR ' + error.code
    }
    throw error
  }
}

describe('parser spec — data.tsv (the safe data-decode seam)', () => {
  for (const r of readDataSpec()) {
    it(`${r.line}: ${JSON.stringify(r.src)}`, () => {
      assert.equal(renderDataSpec(r.src), r.expected, JSON.stringify(r.src))
    })
  }
})
