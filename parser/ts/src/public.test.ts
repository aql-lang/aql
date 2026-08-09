import { strict as assert } from 'node:assert'
import { describe, it } from 'node:test'

import { BoruError, TFloat, TInteger, canonValue } from '@boru-lang/core'

import {
  convertParsedNumber,
  guardMake,
  lexTokens,
  parseConfig,
  plainify,
  safeMake,
  safeParse,
  safeParseData,
  type LexToken,
} from './index.ts'
import { normalizePlainParseError } from './public.ts'

function rawJsonicError(): any {
  try {
    safeMake().parse(']')
  } catch (error) {
    return error
  }
  assert.fail('expected Jsonic to reject the unmatched bracket')
}

describe('public parser helpers', () => {
  it('plainifies Jsonic text, arrays, and nested map nodes', () => {
    const text = new String('boxed')
    Object.defineProperty(text, '__info__', { value: { quote: "'" } })
    const nullPrototype = Object.assign(Object.create(null) as Record<string, unknown>, {
      child: text,
    })
    const input = { list: [text, { nested: nullPrototype }], nil: null }
    Object.defineProperty(input, '__info__', { value: { implicit: true } })

    assert.deepEqual(plainify(input), {
      list: ['boxed', { nested: { child: 'boxed' } }],
      nil: null,
    })
    assert.equal(plainify(7), 7)

    const date = new Date(0)
    assert.equal(plainify(date), date, 'non-Jsonic class instances retain their identity')
  })

  it('constructs and parses with fresh plain parsers', () => {
    assert.deepEqual(plainify(safeMake({}).parse('a:1')), { a: 1 })
    assert.deepEqual(plainify(safeParse('a:1,b:[2,3]')), { a: 1, b: [2, 3] })
    assert.deepEqual(plainify(safeParse("a's'")), ['a', 's'])
    assert.deepEqual(plainify(safeParse('a`b`')), ['a', 'b'])

    const arrayEnders = safeMake({ ender: [';'] }).options().ender
    assert.deepEqual(arrayEnders, [';', "'", '"', '`'])
    const stringEnders = safeMake({ ender: ';' }).options().ender
    assert.deepEqual(stringEnders, [';', "'", '"', '`'])
    const existingQuotes = safeMake({ ender: ["'", '"', '`'] }).options().ender
    assert.deepEqual(existingQuotes, ["'", '"', '`'])
    const customQuote = safeMake({ string: { chars: '«' } }).options().ender
    assert.deepEqual(customQuote, ['«'])
    const disabledStrings = safeMake({ string: { lex: false }, ender: [';'] }).options().ender
    assert.deepEqual(disabledStrings, [';'])
    assert.equal(typeof safeMake('jsonic'), 'function')
    assert.equal(guardMake(() => 7), 7)
  })

  it('normalizes plain-parser errors to Unicode code-point columns', () => {
    assert.throws(
      () => safeParse('🙂:1,]'),
      (error: unknown) => {
        const parseError = error as { lineNumber?: number; columnNumber?: number }
        return parseError.lineNumber === 1 && parseError.columnNumber === 5
      },
    )
    assert.throws(
      () => parseConfig('a:🙂,]'),
      (error: unknown) => {
        const cause = error instanceof Error
          ? error.cause as { lineNumber?: number; columnNumber?: number }
          : undefined
        return cause?.lineNumber === 1 && cause.columnNumber === 5
      },
    )
  })

  it('leaves non-Jsonic and unusable plain-parser locations intact', () => {
    const plain = new Error('plain')
    assert.equal(normalizePlainParseError(plain, 'x'), plain)

    for (const [lineNumber, columnNumber] of [
      [1.5, 1],
      [0, 1],
      [1, 1.5],
      [1, 0],
    ]) {
      const error = rawJsonicError()
      error.lineNumber = lineNumber
      error.columnNumber = columnNumber
      assert.equal(normalizePlainParseError(error, 'x'), error)
      assert.equal(error.lineNumber, lineNumber)
      assert.equal(error.columnNumber, columnNumber)
    }

    const missingLine = rawJsonicError()
    missingLine.lineNumber = 3
    missingLine.columnNumber = 1
    assert.equal(normalizePlainParseError(missingLine, 'x'), missingLine)
    assert.equal(missingLine.columnNumber, 1)
  })

  it('parseConfig accepts nested implicit maps and rejects other inputs', () => {
    assert.deepEqual(parseConfig('a:1,b:c:2,list:[3,4]'), {
      a: 1,
      b: { c: 2 },
      list: [3, 4],
    })
    for (const [src, message] of [
      ['[1,2,3]', 'options must be a map of key:value pairs, got list'],
      ['just-a-string', 'options must be a map of key:value pairs, got string'],
      ['null', 'options must be a map of key:value pairs, got scalar'],
    ] as const) {
      assert.throws(
        () => parseConfig(src),
        (error: unknown) => error instanceof Error && error.message === message,
      )
    }
    assert.throws(
      () => parseConfig("a:'unclosed"),
      (error: unknown) =>
        error instanceof Error &&
        error.message === 'invalid options syntax' &&
        error.cause instanceof Error,
    )
  })

  it('safeParseData retains numeric spelling for the public converter', () => {
    const float = convertParsedNumber(safeParseData('42.0'))
    const integer = convertParsedNumber(safeParseData('42'))

    assert.equal(float?.vType.equal(TFloat), true)
    assert.equal(integer?.vType.equal(TInteger), true)
    assert.equal(canonValue(convertParsedNumber(safeParseData('1.e2'))!), '100.0')
    for (const [src, code, row, col] of [
      ['1e400', 'float_overflow', 0, 0],
      ['1.0e400', 'float_overflow', 0, 0],
      ['0x8000000000000000', 'integer_overflow', 1, 1],
      ['+.1e400', 'syntax_error', 1, 1],
    ] as const) {
      assert.throws(
        () => convertParsedNumber(safeParseData(src)),
        (error: unknown) =>
          error instanceof BoruError &&
          error.code === code &&
          error.src === src &&
          error.row === row &&
          error.col === col,
      )
    }
    assert.throws(() => convertParsedNumber(safeParseData('1__0')))
    for (const src of ['1_.2', '1.2_', '1e_2']) {
      assert.throws(
        () => convertParsedNumber(safeParseData(src)),
        /misplaced `_` in numeric literal/,
      )
    }

    // The jsonic-superset side of the data seam: a digit-led run that is
    // not a complete Boru numeric spelling stays lenient text, exactly as
    // the stock data grammar decodes it. The data-mode matcher must never
    // split a run — `1.x` is one string, never [1, '.x'].
    for (const [src, want] of [
      ['{v: 1.2.3}', { v: '1.2.3' }],
      ['1.x', '1.x'],
      ['{p: 1.2-beta}', { p: '1.2-beta' }],
      ['1..2', '1..2'],
      ['{a: 1.5suffix}', { a: '1.5suffix' }],
      // An empty exponent after a dot declines rather than splitting.
      ['{a: 1.e}', { a: '1.e' }],
      // A signed leading-dot run followed by text declines whole.
      ['+.5x', '+.5x'],
      // Language operators (`=`, `;`) are plain text in a data grammar.
      ['{a: 1=2}', { a: '1=2' }],
      ['1;2', '1;2'],
    ] as const) {
      assert.deepEqual(plainify(safeParseData(src)), want)
    }

    const unicodeList = safeParseData('[🙂,1_]') as unknown[]
    assert.throws(
      () => convertParsedNumber(unicodeList[1]),
      (error: unknown) => {
        const diagnostic = error as { code?: string; row?: number; col?: number }
        return diagnostic.code === 'syntax_error' && diagnostic.row === 1 && diagnostic.col === 4
      },
    )
    assert.equal(convertParsedNumber('not-a-number'), undefined)
  })
})

function rawAtByteOffset(source: string, token: LexToken): boolean {
  const bytes = Buffer.from(source)
  return bytes.subarray(token.si, token.si + Buffer.byteLength(token.src)).equals(Buffer.from(token.src))
}

describe('formatter lexer seam', () => {
  it('preserves trivia and runs the configured context-free matchers', () => {
    const src = '0d12 +js:hello // note\n`tail'
    const result = lexTokens(src)

    assert.equal(result.ok, true)
    assert.equal(result.tokens.map((token) => token.src).join(''), src)
    for (const name of ['#SP', '#CM', '#LN', '#ML', '#BT']) {
      assert.equal(result.tokens.some((token) => token.name === name), true, name)
    }
    assert.deepEqual(result.tokens[0], { name: '#TX', src: '0d12', si: 0 })

    const underscored = lexTokens('1_.2')
    assert.deepEqual(underscored, {
      tokens: [{ name: '#NR', src: '1_.2', si: 0 }],
      ok: true,
    })
  })

  it('reports UTF-8 byte offsets, including after astral characters', () => {
    const src = 'é🙂 add 2'
    const result = lexTokens(src)

    assert.equal(result.ok, true)
    assert.deepEqual(
      result.tokens.map(({ name, si }) => [name, si]),
      [
        ['#TX', 0],
        ['#SP', 6],
        ['#TX', 7],
        ['#SP', 10],
        ['#NR', 11],
      ],
    )
    assert.equal(result.tokens.every((token) => rawAtByteOffset(src, token)), true)
  })

  it('keeps decimal and base overflow on numeric token boundaries', () => {
    for (const src of ['1.0e400', '1.e2', '0x10000000000000000']) {
      assert.deepEqual(lexTokens(src), {
        tokens: [{ name: '#NR', src, si: 0 }],
        ok: true,
      })
    }
    assert.deepEqual(lexTokens('1.x'), {
      tokens: [
        { name: '#NR', src: '1', si: 0 },
        { name: '#DT', src: '.', si: 1 },
        { name: '#TX', src: 'x', si: 2 },
      ],
      ok: true,
    })
  })

  it('distinguishes clean empty input from a lexical error', () => {
    assert.deepEqual(lexTokens(''), { tokens: [], ok: true })
    assert.deepEqual(lexTokens("'unterminated"), { tokens: [], ok: false })
  })
})
