// Temporary differential probe for safeParseData (TS twin of tmpprobe/main.go).
import * as fs from 'node:fs'

import { BoruError, canonValue } from '@boru-lang/core'

import { convertParsedNumber, safeParseData } from './src/index.ts'

function decode(s: string): string {
  let b = ''
  for (let i = 0; i < s.length; i++) {
    if ('\\' === s[i] && i + 1 < s.length) {
      const n = s[i + 1]
      if ('n' === n) { b += '\n'; i++; continue }
      if ('t' === n) { b += '\t'; i++; continue }
      if ('\\' === n) { b += '\\'; i++; continue }
    }
    b += s[i]
  }
  return b
}

function isPlainRecord(value: unknown): value is Record<string, unknown> {
  if (null === value || 'object' !== typeof value) return false
  const p = Object.getPrototypeOf(value)
  return null === p || Object.prototype === p
}

function render(v: unknown): string {
  const num = convertParsedNumber(v)
  if (num !== undefined) return canonValue(num)
  if (null === v || undefined === v) return 'none'
  if ('boolean' === typeof v) return v ? 'true' : 'false'
  if (v instanceof String) return "'" + v.valueOf() + "'"
  if ('string' === typeof v) return "'" + v + "'"
  if (Array.isArray(v)) return '[' + v.map(render).join(' ') + ']'
  if (isPlainRecord(v)) {
    return '{' + Object.entries(v).map(([k, c]) => k + ':' + render(c)).join(' ') + '}'
  }
  return 'UNSUPPORTED ' + Object.prototype.toString.call(v)
}

const lines = fs.readFileSync(0, 'utf8').split('\n')
if ('' === lines[lines.length - 1]) lines.pop()
const out: string[] = []
for (const line of lines) {
  const src = decode(line)
  let decoded: unknown
  try {
    decoded = safeParseData(src)
  } catch (e) {
    out.push('ERR')
    continue
  }
  try {
    out.push(render(decoded))
  } catch (e) {
    if (e instanceof BoruError) out.push('ERR ' + e.code)
    else out.push('HARNESS ' + String(e))
  }
}
process.stdout.write(out.join('\n') + '\n')
