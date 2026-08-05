// The compiled twin of the fixture guard probes: every row runs on the
// interpreter AND through compile → runProgram (or a clean refusal),
// with the two renders pinned equal — the TS analogue of the Go
// battery's compile-or-fallback lane. Drives the fixtures' emit-mode
// arms (typed-word enforcement, inspect-under-emit, concrete
// recovery) alongside the compile/lower/vm pipeline.

import { describe, it } from 'node:test'
import { strict as assert } from 'node:assert'

import { BoruError, Engine, Registry, type Value, compile, runProgram } from './index.ts'
import { registerSpecWords, renderStack, tokenize } from './spec-fixture.ts'

function freshRegistry(): Registry {
  const r = new Registry()
  registerSpecWords(r)
  return r
}

function interpret(input: string): { ok: true; got: string } | { ok: false; err: string } {
  try {
    return { ok: true, got: renderStack(new Engine(freshRegistry()).run(tokenize(input))) }
  } catch (e) {
    if (e instanceof BoruError) return { ok: false, err: e.message }
    throw e
  }
}

function compiled(input: string): { kind: 'vm'; got: string } | { kind: 'refused' } | { kind: 'err'; err: string } {
  let values: Value[]
  try {
    values = tokenize(input)
  } catch (e) {
    if (e instanceof BoruError) return { kind: 'err', err: e.message }
    throw e
  }
  const result = compile(freshRegistry(), values)
  if ('refused' in result) return { kind: 'refused' }
  try {
    return { kind: 'vm', got: renderStack(runProgram(result.program, freshRegistry())) }
  } catch (e) {
    if (e instanceof BoruError) return { kind: 'err', err: e.message }
    throw e
  }
}

describe('fixture compiled lane', () => {
  const rows: string[] = [
    '1 addq 2',
    "'a' concatq 'b'",
    '[10 20 30] get 1',
    '[10 20 30] get 5',
    '{a:1} get a',
    '{a:1} get b',
    'def x 5 x addq 1',
    'def x (2 addq 3) x',
    'def x:5 5 x',
    'def tt Integer def y:tt 3 y',
    'do [1 addq 2]',
    'do [10 20 30]',
    'dup 1 dup',
    '1 2 swap',
    'quote zz typeof',
    'not true',
    'true and false',
    'enum [a b]',
    'fnsig [[Integer] [Integer]] typeof',
    'def f fn [[x:Integer] [Integer] [x addq 1]] f 4',
    'inspect addq',
    'def m {k:2} m get k',
  ]
  let vmCount = 0
  for (const input of rows) {
    it(JSON.stringify(input), () => {
      const iRes = interpret(input)
      const cRes = compiled(input)
      if (cRes.kind === 'refused') return
      if (!iRes.ok) {
        assert.equal(cRes.kind, 'err', `interp errored but VM did not: ${JSON.stringify(cRes)}`)
        return
      }
      assert.equal(cRes.kind, 'vm')
      if (cRes.kind === 'vm') {
        vmCount++
        assert.equal(cRes.got, iRes.got, 'compiled render must match the interpreter')
      }
    })
  }
  it('a meaningful share of rows executes on the VM', () => {
    assert.ok(vmCount >= 10, `only ${vmCount} rows ran on the VM`)
  })
})
