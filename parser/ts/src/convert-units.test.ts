// Direct tests for converter helpers that no parse path reaches.
//
// resolveTextValue is exported from convert.ts but called by nothing — in
// EITHER port. parser/go carries the same orphan (parse.go:1245) and covers
// it with direct unit tests (parse_test.go:2137,
// parse_words_wave3_test.go:332), which is how it holds 100% with the
// function unwired. This file is the TS mirror of those tests, so the twins
// are covered the same way rather than one of them carrying a silent hole.
//
// The expectations are parser/go's, taken from the tests named above: bare
// `true`/`false` classify as Booleans, the three float specials as Floats,
// and every other bare name — INCLUDING a builtin type name or a full type
// path — as an Atom. The parser is type-name-opaque (ADR-012 rule 4); it
// resolves no names, so `Number` here is data, not a type.

import { describe, it } from 'node:test'
import { strict as assert } from 'node:assert'

import { TAtom, TBoolean, TFloat } from '@boru-lang/core'
import { resolveTextValue } from './convert.ts'

describe('resolveTextValue', () => {
  it('classifies the boolean words as Booleans', () => {
    for (const [text, want] of [
      ['true', true],
      ['false', false],
    ] as const) {
      const v = resolveTextValue(text)
      assert.ok(v.vType.matches(TBoolean), `${text} should conform to Boolean`)
      assert.equal(v.asBoolean(), want)
    }
  })

  it('classifies the three float specials as Floats', () => {
    const inf = resolveTextValue('inf')
    assert.ok(inf.vType.matches(TFloat), 'inf should conform to Float')
    assert.equal(inf.asFloat(), Infinity)

    const negInf = resolveTextValue('-inf')
    assert.ok(negInf.vType.matches(TFloat), '-inf should conform to Float')
    assert.equal(negInf.asFloat(), -Infinity)

    const nan = resolveTextValue('nan')
    assert.ok(nan.vType.matches(TFloat), 'nan should conform to Float')
    assert.ok(Number.isNaN(nan.asFloat()))
  })

  it('leaves every other bare name an Atom, type names included', () => {
    // ADR-012 rule 4: the parser is type-name-opaque. A builtin type name and
    // a full type PATH are both data here, exactly as parser/go has them.
    for (const text of ['Number', 'String', 'hello', 'Scalar/Number/Integer']) {
      const v = resolveTextValue(text)
      assert.ok(v.vType.matches(TAtom), `${text} should conform to Atom`)
      assert.equal(v.asAtom(), text)
    }
  })
})
