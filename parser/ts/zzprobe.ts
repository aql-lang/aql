// TEMPORARY PROBE — delete before finishing.
/* eslint-disable @typescript-eslint/no-explicit-any */
import { Jsonic } from '@tabnas/jsonic'
import { BoruError, canonValue } from '@boru-lang/core'
import { convertParsedNumber } from './src/convert.ts'
import { setupNumberSub } from './src/grammar.ts'
import { plainify, safeMake, safeParseData } from './src/public.ts'
import * as fs from 'node:fs'

function probeAddMatcher(j: any, name: string, priority: number, fn: (lex: any, rule: any) => any): void {
  j.options({ lex: { match: { [name]: { order: priority, make: () => fn } } } })
}

// probeSafeParseData mirrors safeParseData exactly, but installs the
// decline-only data matcher (the dataMode claim-whole arm deleted).
function probeSafeParseData(src: string): unknown {
  const j = safeMake()
  probeSetupDecimalBoundaryMatcher(j, true)
  setupNumberSub(j, src)
  return j.parse(src)
}

function isPlainDataRecord(value: unknown): value is Record<string, unknown> {
  if (null === value || 'object' !== typeof value) return false
  const prototype = Object.getPrototypeOf(value)
  return null === prototype || Object.prototype === prototype
}

function renderDataNode(v: unknown): string {
  const num = convertParsedNumber(v)
  if (num !== undefined) return canonValue(num)
  if (null === v || undefined === v) return 'none'
  if ('boolean' === typeof v) return v ? 'true' : 'false'
  if (v instanceof String) return "'" + v.valueOf() + "'"
  if ('string' === typeof v) return "'" + v + "'"
  if (Array.isArray(v)) return '[' + v.map(renderDataNode).join(' ') + ']'
  if (isPlainDataRecord(v)) {
    return '{' + Object.entries(v).map(([k, c]) => k + ':' + renderDataNode(c)).join(' ') + '}'
  }
  throw new Error('unsupported data node ' + Object.prototype.toString.call(v))
}

function render(parse: (s: string) => unknown, src: string): string {
  let decoded: unknown
  try { decoded = parse(src) } catch { return 'ERR' }
  try { return renderDataNode(decoded) } catch (error) {
    if (error instanceof BoruError) return 'ERR ' + error.code
    return 'HARNESS ' + String(error)
  }
}

function decodeEsc(s: string): string {
  let out = ''
  for (let i = 0; i < s.length; i++) {
    if ('\\' === s[i] && i + 1 < s.length) {
      const n = s[i + 1]
      if ('n' === n) { out += '\n'; i++; continue }
      if ('t' === n) { out += '\t'; i++; continue }
      if ('\\' === n) { out += '\\'; i++; continue }
    }
    out += s[i]
  }
  return out
}

const pad = (s: string, n: number) => (s + ' '.repeat(Math.max(0, n - s.length)))

// --- 1. every data.tsv row: current vs decline vs expected
const tsv = fs.readFileSync('../spec/data.tsv', 'utf8').split('\n')
let rows = 0, changed = 0, mismatch = 0
tsv.forEach((line, idx) => {
  if ('' === line || (line.startsWith('#') && !line.includes('\t'))) return
  const parts = line.split('\t')
  const src = decodeEsc(parts[0]!)
  const want = decodeEsc(parts[1]!)
  rows++
  const cur = render(safeParseData, src)
  const dec = render(probeSafeParseData, src)
  if (cur !== dec) changed++
  if (dec !== want) mismatch++
  console.log('row ' + pad(String(idx + 1), 3) + pad(JSON.stringify(src), 24) +
    ' want=' + pad(want, 20) + ' current=' + pad(cur, 20) + ' decline=' + pad(dec, 20) +
    (cur === dec ? ' SAME' : ' CHANGED') + (dec === want ? ' dec:ok' : ' dec:MISMATCH') +
    (cur === want ? ' cur:ok' : ' cur:MISMATCH'))
})
console.log('TOTAL rows=' + rows + ' changed=' + changed + ' declineMismatch=' + mismatch)

// --- 2. stress sources + the BARE dependency
const extra = [
  '0xFF.5', '[0xFF.5]', '{a:0xFF.5}', '[-0xFF.5]', '[+0xFF.5, 1]', '{a: 0xFF.5, b: 2}',
  '0xFF.5.6', '[0xFF.5 0x1F.2]', '{a: 0x1p3}', '0o8.5', '0b12.5', '[0xFF., 1]',
  '{a: 0xFF.5:2}', '[0xFF.5=1]', '{a: 0xFF.5;x}', '[0xFF.5(1)]', '{a: 0xFF.5|x}',
  '{a: 0xFF.5?}', '{a: 0xFF.5!}', '{a: 0xFF.5<x>}', '[0xFF.5,2]', '{a: 0xFF.5#c}',
  "[0xFF.5'q']", '0xFF.5\n1', '[0xFF_.5]', '0x_.5', '0x_1.5', '{a: 0X1F.5}',
  '{a: 0O17.5}', '{a: 0B1.5}', '0xFF', '0x8000000000000000', '[0xFF, 1]',
]
for (const src of extra) {
  const cur = render(safeParseData, src)
  const dec = render(probeSafeParseData, src)
  let bare = 'ERR'
  try { bare = JSON.stringify(plainify(Jsonic.make().parse(src))) } catch { bare = 'ERR' }
  console.log(pad(JSON.stringify(src), 24) + ' current=' + pad(cur, 26) + ' decline=' + pad(dec, 26) +
    (cur === dec ? ' SAME    ' : ' CHANGED ') + ' bare=' + bare)
}
function probeSetupDecimalBoundaryMatcher(j: any, dataMode: boolean): void {
  const isDigit = (c: string | undefined): boolean =>
    undefined !== c && c >= '0' && c <= '9'
  const isDigitOrSep = (c: string | undefined): boolean => isDigit(c) || '_' === c
  const isBaseDigit = (c: string | undefined, prefix: string): boolean => {
    if (undefined === c) return false
    if ('x' === prefix || 'X' === prefix) {
      return isDigit(c) || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')
    }
    if ('o' === prefix || 'O' === prefix) return c >= '0' && c <= '7'
    return '0' === c || '1' === c
  }
  const validDecimalSeparators = (src: string): boolean => {
    for (let i = 0; i < src.length; i++) {
      if ('_' === src[i] &&
        (0 === i || i === src.length - 1 || !isDigit(src[i - 1]) || !isDigit(src[i + 1]))) {
        return false
      }
    }
    return true
  }
  const isFollowingText = (s: string, i: number): boolean => {
    if (i >= s.length) return false
    const c = s[i]!
    if (' ' === c || '\t' === c || '\n' === c || '\r' === c) return false
    if ('\'' === c || '"' === c || '#' === c) return false
    if (',:[]{}`'.includes(c)) return false
    // Language operators end a token only in language mode; a plain data
    // grammar lexes them as ordinary text continuation.
    if (';()?.!|<>'.includes(c)) return dataMode
    if ('=' === c) return dataMode ? true : '>' !== s[i + 1]
    return true
  }

  const emitPrefix = (lex: any, s: string, start: number, end: number): any => {
    const cursor = lex.pnt
    const src = s.slice(start, end)
    const tkn = validDecimalSeparators(src)
      ? lex.token('#NR', Number(src.replaceAll('_', '')), src, cursor)
      : lex.token('#TX', src, src, cursor)
    cursor.sI = end
    cursor.cI += end - start
    return tkn
  }

  probeAddMatcher(j, 'decimal_underscore', 1000004, (lex: any, _rule: any) => {
    const cursor = lex.pnt
    const s: string = lex.src
    const start: number = cursor.sI
    let si = start
    if (si >= s.length) return undefined

    const hasSign = '+' === s[si] || '-' === s[si]
    if (hasSign) {
      si++
      if (si >= s.length) return undefined
    }

    const leadingDot = '.' === s[si]
    let integerEnd = -1
    if (leadingDot) {
      // Keep the bare `.5` path on the fixed-dot matcher so it retains
      // the receiverless-dot diagnostic. Signed forms are one numeric
      // token and are rejected later with their complete source.
      if (!hasSign || !isDigit(s[si + 1])) return undefined
      si++
      while (isDigitOrSep(s[si])) si++
    } else {
      if (!isDigit(s[si])) return undefined
      // Big numbers keep their dedicated matcher (which has higher
      // priority); decline here as well so this matcher is safe alone.
      if ('0' === s[si] && undefined !== s[si + 1] && 'dD'.includes(s[si + 1]!)) {
        return undefined
      }
      // Claim base-prefixed integers even when their magnitude exceeds
      // int64. Go's stock lexer otherwise drops them to #TX while TS keeps
      // #NR, making the public lexTokens streams disagree. The converter
      // reads the exact source, so the placeholder value is irrelevant.
      if ('0' === s[si] && undefined !== s[si + 1] && 'xXoObB'.includes(s[si + 1]!)) {
        const prefix = s[si + 1]!
        let end = si + 2
        const bodyStart = end
        while (isBaseDigit(s[end], prefix) || '_' === s[end]) end++
        if (end === bodyStart || isFollowingText(s, end)) {
          // Data mode cannot decline a dot-adjacent base-prefixed run to
          // the stock scanners: they DISAGREE there (Go keeps `0xFF.5`
          // one lenient text, TS lexes the prefix as a number and splits
          // the rest into stray values). Claim the whole prose run to the
          // data boundary as one text token instead — the same string
          // Go's stock scanner produces, now guaranteed in both ports.
          // An empty body (`0x.5`, `0xzz`) still declines: the stock
          // scanners agree on those.
          return undefined
        }
        const src = s.slice(start, end)
        const tkn = lex.token('#NR', 0, src, cursor)
        cursor.sI = end
        cursor.cI += end - start
        return tkn
      }
      while (isDigitOrSep(s[si])) si++
      integerEnd = si
      if ('.' === s[si]) {
        si++
        while (isDigitOrSep(s[si])) si++
      }
    }

    const hasDot = leadingDot || (integerEnd >= 0 && integerEnd < si && '.' === s[integerEnd])

    if ('e' === s[si] || 'E' === s[si]) {
      const exponentMark = si
      si++
      if ('+' === s[si] || '-' === s[si]) si++
      const exponentStart = si
      while (isDigitOrSep(s[si])) si++
      if (si === exponentStart) {
        if (integerEnd >= 0 && hasDot) {
          if (dataMode) return undefined
          return emitPrefix(lex, s, start, integerEnd)
        }
        if (leadingDot) {
          si = exponentMark
        } else {
          return undefined
        }
      }
    }

    if (isFollowingText(s, si)) {
      // Data mode never splits a run: declining hands the whole span to
      // the stock lenient text scanner, so digit-led prose stays one
      // string instead of shedding a stray second value.
      if (dataMode) return undefined
      if (integerEnd >= 0 && hasDot) {
        return emitPrefix(lex, s, start, integerEnd)
      }
      if (!leadingDot) return undefined
    }

    const src = s.slice(start, si)
    const parsed = Number(src.replaceAll('_', ''))
    // Loose separator syntax such as `1e_` can strip to NaN. The converter
    // rejects `_` before consulting this placeholder numeric value.
    const num = Number.isNaN(parsed) ? 0 : parsed
    const tkn = lex.token('#NR', num, src, cursor)
    cursor.sI = si
    cursor.cI += si - start
    return tkn
  })
}

// --- 3. unicode / quote / ender stress
const stress = [
  '{a: 0xFF.5\u{1F642}}', '[0xFF.5\u{1F642}, 1]', '0xFF.5é', '{a: 0xFF.5"q"}', '[0xFF.5`t`]',
  '{a: 0xFF.5\tb}', '[0xFF.5]', '{a: 0xFF.5}', '{a: 0xFF.5', '[0xFF.5',
  '{a: 0xFF.5 }', '[0xFF.5 , 1]', '{a: 0xFF.5\n b: 2}', '{a: 0xFF.5,}',
  '0xFF.5\t', '0xFF.5 ', '0xFF.5#', '{a: 0xFF.5{b:1}}', '[0xFF.5[1]]',
  '{a: 0xFF.5:2, b:3}', '{0xFF.5: 1}', '[0xFF.5\\n]', '{a: 0xFF.5\\}',
  '0x1.5e10', '0xFF.5e', '0xFFe5', '0xFF.5_', '0xFF_', '0x_',
]
let stressChanged = 0
for (const src of stress) {
  const cur = render(safeParseData, src)
  const dec = render(probeSafeParseData, src)
  if (cur !== dec) stressChanged++
  console.log(pad(JSON.stringify(src), 26) + ' current=' + pad(cur, 30) + ' decline=' + pad(dec, 30) +
    (cur === dec ? ' SAME' : ' CHANGED'))
}
console.log('TOTAL stress=' + stress.length + ' changed=' + stressChanged)
