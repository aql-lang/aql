// Unit probes for sugar.ts's expansion table (the TS twin of Go's
// SugarExpansion arms — the bare kernel binds no sugar words, so every
// arm is driven directly with a bound registry) and type.ts's newType
// short-name machinery (leaf expansion, the ambiguity collapse, and
// the lowercase-part guard).

import { describe, it } from 'node:test'
import { strict as assert } from 'node:assert'

import {
  BoruError,
  Registry,
  type SugarInfo,
  newInteger,
  newList,
  sugarExpansion,
} from './index.ts'
import { indexDecl, newType } from '@voxgig/borucore'

function boundRegistry(): Registry {
  const r = new Registry()
  r.bindSugarWord('lambda', 'afn')
  r.bindSugarWord('force-arity', 'fa')
  r.bindSugarWord('mini', 'minilang')
  r.bindSugarWord('gen-head', 'genhead')
  r.bindSugarWord('gen-apply', 'genapply')
  return r
}

describe('sugar expansion arms', () => {
  it('simple roles lower to their bound word', () => {
    const exp = sugarExpansion(boundRegistry(), { kind: 'lambda' }, false)
    assert.equal(exp.length, 1)
    assert.equal(String(exp[0]), 'word(afn)')
  })

  it('an unbound role refuses loudly', () => {
    assert.throws(
      () => sugarExpansion(new Registry(), { kind: 'lambda' }, false),
      (e: unknown) => e instanceof BoruError && /sugar_unbound/.test(e.message),
    )
  })

  it('force-arity carries its arity operand', () => {
    const exp = sugarExpansion(boundRegistry(), { kind: 'force-arity', n: 2n }, false)
    assert.equal(exp.length, 2)
    assert.equal(exp[1]!.data, 2n)
  })

  it('mini carries the kind word and raw source', () => {
    const exp = sugarExpansion(boundRegistry(), { kind: 'mini', name: 'sql', src: 'select 1' }, false)
    assert.equal(exp.length, 3)
    assert.equal(exp[2]!.data, 'select 1')
  })

  it('type-bound wraps a gen-apply paren', () => {
    const exp = sugarExpansion(
      boundRegistry(),
      { kind: 'type-bound', items: [newInteger(1n)] },
      false,
    )
    assert.equal(exp.length, 1)
    assert.equal(exp[0]!.isParenExpr(), true)
  })

  it('angle head form emits receiver, gen-head, and the params', () => {
    const head = newList([])
    const exp = sugarExpansion(boundRegistry(), { kind: 'angle', name: 'Box', head }, true)
    assert.equal(exp.length, 3)
    assert.equal(String(exp[1]), 'word(genhead)')
    assert.equal(exp[2], head)
  })

  it('angle head form surfaces a recorded head error', () => {
    const info: SugarInfo = { kind: 'angle', name: 'Box', headErr: 'params must be words' }
    assert.throws(
      () => sugarExpansion(boundRegistry(), info, true),
      (e: unknown) => e instanceof BoruError && /params must be words/.test(e.message),
    )
  })

  it('angle use-site form wraps a gen-apply paren', () => {
    const exp = sugarExpansion(
      boundRegistry(),
      { kind: 'angle', name: 'Box', items: [newInteger(1n)] },
      false,
    )
    assert.equal(exp.length, 1)
    assert.equal(exp[0]!.isParenExpr(), true)
  })

  it('an unknown kind refuses loudly', () => {
    assert.throws(
      () => sugarExpansion(boundRegistry(), { kind: 'zzz' } as unknown as SugarInfo, false),
      (e: unknown) => e instanceof BoruError && /unknown sugar kind/.test(e.message),
    )
  })
})

describe('newType short names', () => {
  it('expands a known leaf to its full path', () => {
    assert.deepEqual([...newType('Integer').parts], ['Scalar', 'Number', 'Integer'])
    assert.deepEqual([...newType('Integer/Sub').parts], ['Scalar', 'Number', 'Integer', 'Sub'])
  })

  it('leaves an unknown head untouched', () => {
    assert.deepEqual([...newType('Zzz').parts], ['Zzz'])
  })

  it('rejects a lowercase part', () => {
    assert.throws(() => newType('Scalar/bad'), /must start with an uppercase letter/)
  })

  it('an ambiguous leaf collapses and stops expanding', () => {
    indexDecl({ path: 'Zork/Blip' })
    assert.deepEqual([...newType('Blip').parts], ['Zork', 'Blip'])
    indexDecl({ path: 'Quux/Blip' })
    assert.deepEqual([...newType('Blip').parts], ['Blip'])
  })
})
