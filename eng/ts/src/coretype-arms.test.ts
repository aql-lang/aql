// Unit probes for coretype.ts's remaining arms: the boolean-coercion
// table across every value mode (the TS twin of the Go truthiness
// battery, which drives these paths through the fixture `if` there),
// and the structural equality walk via `is` rows.

import { describe, it } from 'node:test'
import { strict as assert } from 'node:assert'

import { Engine, Registry, coerceBoolean } from './index.ts'
import {
  newBoolean,
  newFloat,
  newInteger,
  newList,
  newMap,
  newNone,
  newString,
  newTypeLiteral,
  newTypedList,
  newWord,
  OrderedMap,
} from '@boru-lang/core'
import { TInteger, TList } from '@boru-lang/core'
import { registerSpecWords, renderStack, tokenize } from './spec-fixture.ts'

describe('boolean coercion table', () => {
  it('covers every value mode', () => {
    const om = new OrderedMap()
    om.set('a', newInteger(1))
    const cases: Array<[unknown, boolean]> = [
      [newBoolean(true), true],
      [newBoolean(false), false],
      [newInteger(0), false],
      [newInteger(3), true],
      [newFloat(0), false],
      [newFloat(0.5), true],
      [newString(''), false],
      [newString('x'), true],
      [newList([]), false],
      [newList([newInteger(1)]), true],
      [newTypedList(newTypeLiteral(TInteger)), false],
      [newTypedList(newTypeLiteral(TInteger), [newInteger(1)]), true],
      [newMap(new OrderedMap()), false],
      [newMap(om), true],
      [newNone(), false],
      [newTypeLiteral(TList), false],
      [newWord('w'), true],
    ]
    for (const [v, want] of cases) {
      assert.equal(coerceBoolean(v as never), want)
    }
  })
})

describe('structural equality via is', () => {
  const run = (input: string): string => {
    const r = new Registry()
    registerSpecWords(r)
    return renderStack(new Engine(r).run(tokenize(input)))
  }
  it('lists compare element-wise with length gates', () => {
    assert.equal(run('[1 2] is [1 2]'), 'true')
    assert.equal(run('[1 2] is [1 3]'), 'false')
    assert.equal(run('[1 2] is [1]'), 'false')
    assert.equal(run('[1 [2]] is [1 [2]]'), 'true')
  })
  it('maps compare by sorted keys then values', () => {
    assert.equal(run('{a:1 b:2} is {b:2 a:1}'), 'true')
    assert.equal(run('{a:1} is {a:2}'), 'false')
    assert.equal(run('{a:1} is {b:1}'), 'false')
    assert.equal(run('{a:1} is {a:1 b:2}'), 'false')
  })
  it('mixed shapes are unequal', () => {
    assert.equal(run('[1] is {a:1}'), 'false')
    assert.equal(run('1 is [1]'), 'false')
  })
  it('a concrete RHS unifies symmetrically (Go: terminal Unify)', () => {
    assert.equal(run('Integer is 5'), 'true')
    assert.equal(run('Number is 5'), 'true')
    assert.equal(run('Float is 5'), 'false')
    assert.equal(run("String is 'x'"), 'true')
    assert.equal(run('List is [1 2]'), 'true')
    assert.equal(run('Map is {a:1}'), 'false')
    assert.equal(run('Integer is [1 2]'), 'false')
    assert.equal(run("'x' is 'x'"), 'true')
    assert.equal(run('true is true'), 'true')
    assert.equal(run('none is none'), 'true')
    assert.equal(run('[3 2] is [Integer 2]'), 'true')
    assert.equal(run('[Integer 2] is [3 2]'), 'true')
    assert.equal(run('[{a:1 b:2}] is [{a:1}]'), 'false')
    assert.equal(run('{a:1 b:2} is {a:1}'), 'true')
  })
})

describe('enum membership mirrors unifyDisjunct', () => {
  const run = (input: string): string => {
    const r = new Registry()
    registerSpecWords(r)
    return renderStack(new Engine(r).run(tokenize(input)))
  }
  // Every row's expectation is pinned to the Go engine's answer for
  // the identical source (probed via eng/go with the specfix words).
  const rows: Array<[string, string]> = [
    ['def E (enum [[1 2] [3 4]]) [1 2] is E', 'true'],
    ['def E (enum [[1 2] [3 4]]) [1 3] is E', 'false'],
    ['def E (enum [[1 2] [3 4]]) [1] is E', 'false'],
    ['def E (enum [{a:1}]) {a:1} is E', 'true'],
    ['def E (enum [{a:1}]) {a:2} is E', 'false'],
    ['def E (enum [{a:1}]) {b:1} is E', 'false'],
    // A bare-literal candidate is admitted when an alternative's type
    // conforms to it — 1 unifies with Integer (and Number), not Float.
    ['def E (enum [1 2]) Integer is E', 'true'],
    ['def E (enum [1 2]) Float is E', 'false'],
    ['def E (enum [1 2]) Number is E', 'true'],
    ['def E (enum [1 2]) String is E', 'false'],
    ['def E (enum [1 2]) 1.0 is E', 'false'],
    ['def E (enum [1 2]) 3 is E', 'false'],
    // Concrete map alternatives are OPEN patterns: subset matching.
    ['def E (enum [{a:1}]) {a:1 b:2} is E', 'true'],
    ['def E (enum [{a:1}]) {b:2} is E', 'false'],
    // `any` unifies with the whole disjunct.
    ['def E (enum [[1 2]]) Any is E', 'true'],
    ['def E (enum [1 \'x\']) Integer is E', 'true'],
    ['def E (enum [1 \'x\']) Boolean is E', 'false'],
    ['def E (enum [[1 [2]]]) [1 [2]] is E', 'true'],
    // Nested containers unify per element; nested maps are EXACT
    // (closed) — only the top-level map alternative is an open pattern.
    ['def E (enum [[{a:1}]]) [{a:1}] is E', 'true'],
    ['def E (enum [[{a:1}]]) [{a:2}] is E', 'false'],
    ['def E (enum [[{a:1}]]) [{a:1 b:2}] is E', 'false'],
    ['def E (enum [[{a:1 b:2}]]) [{a:1}] is E', 'false'],
    ['def E (enum [[{a:Integer}]]) [{a:3}] is E', 'true'],
    ['def E (enum [[Integer 2]]) [Integer 2] is E', 'true'],
    ['def E (enum [[Integer 2]]) [3 2] is E', 'true'],
    ['def E (enum [[Integer 2]]) [Float 2] is E', 'false'],
    ['def E (enum [[Integer 2]]) [3] is E', 'false'],
  ]
  for (const [input, want] of rows) {
    it(JSON.stringify(input), () => {
      assert.equal(run(input), want)
    })
  }
})
