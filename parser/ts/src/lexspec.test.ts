// The TypeScript half of parser/spec/lex.tsv. Its strict reader and renderer
// intentionally share no implementation with parser/go/lexspec_test.go.

import { strict as assert } from 'node:assert'
import * as fs from 'node:fs'
import * as path from 'node:path'
import { describe, it } from 'node:test'
import { fileURLToPath } from 'node:url'

import { lexTokens, type LexToken } from './index.ts'

const SPEC_DIR = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..', '..', 'spec')
const LEX_SPEC_ROW_COUNT = 34

interface LexSpecRow {
  line: number
  src: string
  status: 'OK' | 'ERR'
  expected: string
}

function decodeLexSpecEscapes(source: string): string {
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

function validateLexSpecTokens(name: string, line: number, expected: string): void {
  let value: unknown
  assert.doesNotThrow(() => {
    value = JSON.parse(expected)
  }, `${name}:${line}: tokens must be JSON`)
  assert.ok(Array.isArray(value), `${name}:${line}: tokens must be an array`)
  for (const [index, token] of value.entries()) {
    assert.ok(null !== token && 'object' === typeof token && !Array.isArray(token), `${name}:${line}: token ${index} must be an object`)
    const record = token as Record<string, unknown>
    assert.deepEqual(Object.keys(record), ['name', 'src', 'si'], `${name}:${line}: token ${index} fields`)
    assert.ok('string' === typeof record.name && record.name.startsWith('#'), `${name}:${line}: token ${index} name`)
    assert.equal(typeof record.src, 'string', `${name}:${line}: token ${index} source`)
    assert.ok(Number.isSafeInteger(record.si) && (record.si as number) >= 0, `${name}:${line}: token ${index} offset`)
  }
  assert.equal(JSON.stringify(value), expected, `${name}:${line}: token JSON must be canonical`)
}

function readLexSpec(): LexSpecRow[] {
  const name = 'lex.tsv'
  const lines = fs.readFileSync(path.join(SPEC_DIR, name), 'utf8').split('\n')
  const rows: LexSpecRow[] = []
  const seen = new Map<string, number>()
  for (let index = 0; index < lines.length; index++) {
    const line = lines[index]!
    if ('' === line || (line.startsWith('#') && !line.includes('\t'))) continue
    const parts = line.split('\t')
    assert.equal(parts.length, 3, `${name}:${index + 1}: need exactly 3 tab-separated columns (source, status, tokens)`)
    const src = decodeLexSpecEscapes(parts[0]!)
    assert.equal(seen.has(src), false, `${name}:${index + 1}: duplicate source (first at line ${seen.get(src)}): ${JSON.stringify(src)}`)
    seen.set(src, index + 1)
    const status = decodeLexSpecEscapes(parts[1]!)
    assert.ok('OK' === status || 'ERR' === status, `${name}:${index + 1}: status must be OK or ERR`)
    const expected = decodeLexSpecEscapes(parts[2]!)
    validateLexSpecTokens(name, index + 1, expected)
    rows.push({ line: index + 1, src, status, expected })
  }
  assert.equal(rows.length, LEX_SPEC_ROW_COUNT, `${name}: exact row-count ratchet`)
  return rows
}

function renderLexSpec(tokens: LexToken[]): string {
  return JSON.stringify(tokens.map(({ name, src, si }) => ({ name, src, si })))
}

describe('parser spec — lex.tsv', () => {
  for (const row of readLexSpec()) {
    it(`${row.line}: ${JSON.stringify(row.src)}`, () => {
      const result = lexTokens(row.src)
      assert.equal(result.ok ? 'OK' : 'ERR', row.status)
      assert.equal(renderLexSpec(result.tokens), row.expected)

      const sourceBytes = Buffer.from(row.src)
      for (const token of result.tokens) {
        const rawBytes = Buffer.from(token.src)
        assert.equal(sourceBytes.subarray(token.si, token.si + rawBytes.length).equals(rawBytes), true)
      }
    })
  }
})
