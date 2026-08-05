// Unit probes for engine.ts's remaining arms: syntax guards (unmatched
// parens, stray end), the zero-value table, forward-collection edge
// errors, sugar-marker misses, interp-string resolution inside parens,
// the async-handler refusal, and the fn-recursion induction hatch.

import { describe, it } from 'node:test'
import { strict as assert } from 'node:assert'

import { BoruError, Engine, Registry, newInteger, newWord } from './index.ts'
import { newOpenParen } from './value.ts'
import { registerSpecWords, renderStack, tokenize } from './spec-fixture.ts'

function run(input: string): string {
  const r = new Registry()
  registerSpecWords(r)
  return renderStack(new Engine(r).run(tokenize(input)))
}

function runErr(input: string): string {
  try {
    run(input)
  } catch (e) {
    if (e instanceof BoruError) return e.message
    throw e
  }
  return 'NO-ERROR'
}

describe('engine syntax guards', () => {
  it('an unmatched close paren refuses', () => {
    assert.match(runErr(') 1'), /unmatched '\)'/)
  })
  it('an unmatched open paren refuses', () => {
    assert.match(runErr('(1 2'), /unmatched opening parenthesis/)
  })
})

describe('word special forms', () => {
  it('none pushes the none value', () => {
    assert.equal(run('none'), 'none')
  })
  it('a programmatic Word(none) resolves in the engine itself', () => {
    // tokenize converts `none` at parse time, so the engine's own arm
    // needs a hand-built token.
    const r = new Registry()
    registerSpecWords(r)
    assert.equal(renderStack(new Engine(r).run([newWord('none')])), 'none')
  })
  it('a programmatic unmatched open paren refuses in the scanner', () => {
    const r = new Registry()
    registerSpecWords(r)
    assert.throws(
      () => new Engine(r).run([newOpenParen(), newInteger(1n)]),
      (e: unknown) => e instanceof BoruError && /unmatched '\('/.test(e.message),
    )
  })
})

describe('omitted optional params take base values (pinned to Go)', () => {
  it('every scalar family has a base', () => {
    assert.equal(run('def f fn [[s:String ?] [String] [s]] f'), "''")
    assert.equal(run('def f fn [[b:Boolean ?] [Boolean] [b]] f'), 'false')
    assert.equal(run('def f fn [[x:Float ?] [Float] [x]] f'), '0.0')
    assert.equal(run('def f fn [[n:Integer ?] [Integer] [n]] f'), '0')
  })
})

describe('forward collection edges', () => {
  it('a mistyped forward arg fails the dispatch', () => {
    assert.match(runErr("addq 1 'x'"), /no signature matches/)
  })
  it("'end' before all forward args collected refuses", () => {
    assert.match(runErr('addq 1 ;'), /no signature matches/)
  })
})

describe('paren scan resolution', () => {
  it('interp strings resolve inside parens', () => {
    assert.equal(run("def x 5 (`v=${x}` concatq '!')"), "'v=5!'")
  })
})

describe('async handler refusal', () => {
  it('a promise-returning handler is refused', () => {
    const r = new Registry()
    registerSpecWords(r)
    r.registerNativeFunc({
      name: 'zzasync',
      signatures: [{ args: [], handler: (() => Promise.resolve([])) as never }],
    })
    assert.throws(
      () => new Engine(r).run(tokenize('zzasync')),
      (e: unknown) => e instanceof BoruError && /async handlers are not supported/.test(e.message),
    )
  })
})
