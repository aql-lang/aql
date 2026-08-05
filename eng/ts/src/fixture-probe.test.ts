// Fixture guard probes — the TS twin of eng/go/specfix_probe_test.go:
// the fixture words' guard arms and less-travelled paths the shared
// corpus never reaches, driven through the same harness the spec
// runner uses. Where the Go probe needed Go-constructed bindings, the
// TS rows lean on the shapes the TS fixtures accept directly; rows
// shared with the Go probe pin cross-engine fixture behavior.

import { describe, it } from 'node:test'
import { strict as assert } from 'node:assert'

import { BoruError, Engine, Registry, type Value } from './index.ts'
import { registerSpecWords, renderStack, tokenize } from './spec-fixture.ts'

function run(input: string): { ok: true; got: string } | { ok: false; err: string } {
  const r = new Registry()
  registerSpecWords(r)
  let values: Value[]
  try {
    values = tokenize(input)
  } catch (e) {
    return { ok: false, err: e instanceof Error ? e.message : String(e) }
  }
  try {
    return { ok: true, got: renderStack(new Engine(r).run(values)) }
  } catch (e) {
    if (e instanceof BoruError) return { ok: false, err: e.message }
    return { ok: false, err: e instanceof Error ? e.message : String(e) }
  }
}

describe('fixture guard probes', () => {
  const rows: Array<[input: string, want: string | { err: string }]> = [
    // def family guards (shared with the Go probe suite).
    ['def {a:Integer b:Integer} 3', { err: 'exactly one key' }],
    ['def {x:String} 3', { err: 'does not satisfy' }],
    ['def f fn [1 2]', { err: 'multiple of 3' }],
    ['fn [1 2]', { err: 'multiple of 3' }],

    // word / splice.
    ['word 5', '5'],

    // get on containers: hits, misses (the None TYPE literal, the Go
    // reference), string keys on lists, and get-on-None. (The 1-arg
    // refine remains on the fixture-parity backlog.)
    ['[10 20 30] get 1', '20'],
    ['[10 20 30] get 5', 'None'],
    ['{a:1} get a', '1'],
    ['{a:1} get b', 'None'],
    ['[1 2] get a', 'None'],
    ['{a:1} get b get 1', 'None'],

    // do's escape hatch: a body error surfaces as an Error VALUE.
    ['do [notaword_xyz]', 'error(undefined word: notaword_xyz)'],

    // refine Record guards (the Go reference messages).
    ['refine Record []', { err: 'at least one field' }],
    ['refine Record [5]', { err: 'must be a pair' }],

    // fnsig arms.
    ['fnsig [1]', { err: 'multiple of 2' }],

    // enum over words and typed lists.
    ['enum [a b]', 'a tor b'],

    // typed-def constraint fallback: an unknown constraint name goes
    // through the atomic-value converter.
    ['def x:zzz 1', { err: 'does not satisfy' }],
  ]
  for (const [input, want] of rows) {
    it(JSON.stringify(input), () => {
      const res = run(input)
      if (typeof want === 'string') {
        assert.ok(res.ok, res.ok ? '' : res.err)
        assert.equal(res.got, want)
      } else {
        assert.ok(!res.ok, res.ok ? `expected error, got ${res.got}` : '')
        assert.match(res.err, new RegExp(want.err.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')))
      }
    })
  }
})
