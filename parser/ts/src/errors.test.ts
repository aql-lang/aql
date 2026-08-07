// Unit campaign for the parse-error translation layer (the TS twin of
// parser/go/parse_error.go): every tabnas code's boru-voice text,
// the {src} recovery from the library's message templates, and the
// translateParseError entry across its input kinds. JsonicError
// instances are built via Object.create so the library constructor's
// context plumbing stays out of a unit test.

import { describe, it } from 'node:test'
import { strict as assert } from 'node:assert'

import { JsonicError } from '@tabnas/jsonic'

import { BoruError } from '@boru-lang/core'
import { colorOff, errMsgOptions, parseErrText, translateParseError } from './errors.ts'

interface FakeFields {
  code: string
  message: string
  lineNumber: number
  columnNumber: number
  details: Record<string, unknown>
  txts?: () => { msg?: string; hint?: string; site?: string }
}

function fakeJsonicError(fields: FakeFields): JsonicError {
  const e = Object.create(JsonicError.prototype) as JsonicError
  Object.assign(e, fields)
  return e
}

describe('parse-error translation', () => {
  it('exports the renderer-quieting options', () => {
    assert.equal(colorOff, false)
    assert.equal(errMsgOptions.name, 'boru')
    assert.equal(errMsgOptions.suffix, false)
  })

  it('passes an existing BoruError through unchanged', () => {
    const be = new BoruError('type_error', 'already translated', '')
    assert.equal(translateParseError(be, 'src'), be)
  })

  it('wraps a plain Error as a syntax_error', () => {
    const out = translateParseError(new Error('boom'), 'src')
    assert.equal(out.code, 'syntax_error')
    assert.match(out.detail, /parse error: boom/)
  })

  it('wraps a non-Error thrown value via String()', () => {
    const out = translateParseError('just a string', 'src')
    assert.equal(out.code, 'syntax_error')
    assert.match(out.detail, /parse error: just a string/)
  })

  it('translates a JsonicError with a txts() detail line', () => {
    const e = fakeJsonicError({
      code: 'unexpected',
      message: '[boru/unexpected]: unexpected character(s): %',
      lineNumber: 1,
      columnNumber: 2,
      details: {},
      txts: () => ({ msg: 'unexpected character(s): %' }),
    })
    const out = translateParseError(e, '%')
    assert.equal(out.code, 'syntax_error')
    assert.match(out.detail, /unexpected `%`/)
  })

  it('recovers the detail from the message when txts is absent', () => {
    const e = fakeJsonicError({
      code: 'unterminated_string',
      message: "[boru/unterminated_string]: unterminated string: 'abc",
      lineNumber: 1,
      columnNumber: 1,
      details: {},
    })
    const out = translateParseError(e, "'abc")
    assert.match(out.detail, /this string is never closed: 'abc/)
  })

  it('yields an empty src for codes whose template carries none', () => {
    const e = fakeJsonicError({
      code: 'end_of_source',
      message: '[boru/end_of_source]: source ended unexpectedly',
      lineNumber: 1,
      columnNumber: 1,
      details: {},
      txts: () => ({ msg: 'source ended unexpectedly' }),
    })
    const [detail, note] = parseErrText(e as unknown as Parameters<typeof parseErrText>[0])
    assert.match(detail, /source ends in the middle/)
    assert.match(note, /unclosed/)
  })

  it('yields an empty src when the detail does not match the template', () => {
    const e = fakeJsonicError({
      code: 'unexpected',
      message: '[boru/unexpected]: some rephrased message',
      lineNumber: 1,
      columnNumber: 1,
      details: {},
      txts: () => ({ msg: 'some rephrased message' }),
    })
    const [detail] = parseErrText(e as unknown as Parameters<typeof parseErrText>[0])
    assert.match(detail, /unexpected `` /)
  })

  it('maps every templated code to its boru-voice text', () => {
    const cases: Array<[code: string, msg: string, wantDetail: RegExp]> = [
      ['unterminated_comment', 'unterminated comment: /*', /comment is never closed/],
      ['invalid_unicode', 'invalid unicode escape: \\uZZZZ', /invalid unicode escape: \\uZZZZ/],
      ['invalid_ascii', 'invalid ascii escape: \\xZZ', /invalid ascii escape: \\xZZ/],
      ['unprintable', 'unprintable character: \u0001', /unprintable character inside a string/],
    ]
    for (const [code, msg, wantDetail] of cases) {
      const e = fakeJsonicError({
        code,
        message: `[boru/${code}]: ${msg}`,
        lineNumber: 3,
        columnNumber: 4,
        details: {},
        txts: () => ({ msg }),
      })
      const [detail] = parseErrText(e as unknown as Parameters<typeof parseErrText>[0])
      assert.match(detail, wantDetail, code)
    }
  })

  it('keeps the library detail for unknown codes with the honest note', () => {
    const e = fakeJsonicError({
      code: 'unknown_rule',
      message: '[boru/unknown_rule]: rule fell off the grammar',
      lineNumber: 1,
      columnNumber: 1,
      details: {},
      txts: () => ({ msg: 'rule fell off the grammar' }),
    })
    const [detail, note] = parseErrText(e as unknown as Parameters<typeof parseErrText>[0])
    assert.equal(detail, 'rule fell off the grammar')
    assert.match(note, /parser-internal failure/)
  })
})
