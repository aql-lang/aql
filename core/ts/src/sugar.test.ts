// Unit campaign for sugar.ts — one exported function, sugarExpansion, which
// no test called (0.00% function coverage at 46.67% lines).
//
// This is the engine-side half of ADR-012 rule 3: the parser emits structural
// markers that name no word, and the engine lowers each marker through the
// registry's sugar-role table. The contract worth pinning is the REFUSAL: a
// registry with no binding for a stepped role raises `sugar_unbound` rather
// than inventing a name, which is what lets the bare kernel run with zero
// bindings and a host decline a surface form cleanly.

import { describe, it } from 'node:test'
import { strict as assert } from 'node:assert'

import { BoruError } from './error.ts'
import { Registry } from './registry.ts'
import { sugarExpansion } from './sugar.ts'
import { newInteger, newList, newWord, type SugarInfo } from './value.ts'

/** A registry binding every role these tests lower. */
function boundRegistry(): Registry {
  const r = new Registry()
  for (const role of [
    'usurp',
    'stack-args',
    'forward-args',
    'lambda',
    'gen-default',
    'force-arity',
    'mini',
    'gen-apply',
    'gen-head',
  ]) {
    r.bindSugarWord(role, `w-${role}`)
  }
  return r
}

const render = (vs: ReturnType<typeof sugarExpansion>) => vs.map((v) => v.toString()).join(' ')

describe('sugarExpansion — the bare roles', () => {
  it('lowers each no-argument role to its bound word', () => {
    const r = boundRegistry()
    for (const kind of ['usurp', 'stack-args', 'forward-args', 'lambda', 'gen-default']) {
      const out = sugarExpansion(r, { kind } as SugarInfo, false)
      assert.equal(out.length, 1)
      assert.equal(out[0]!.asWord().name, `w-${kind}`)
    }
  })
})

describe('sugarExpansion — the roles that carry data', () => {
  it('lowers force-arity to its word plus the arity', () => {
    const out = sugarExpansion(boundRegistry(), { kind: 'force-arity', n: 3n } as SugarInfo, false)
    assert.equal(out[0]!.asWord().name, 'w-force-arity')
    assert.equal(out[1]!.asInteger(), 3n)
  })

  it('defaults a missing arity to zero', () => {
    const out = sugarExpansion(boundRegistry(), { kind: 'force-arity' } as SugarInfo, false)
    assert.equal(out[1]!.asInteger(), 0n)
  })

  it('lowers mini to word, name, source', () => {
    const info = { kind: 'mini', name: 'm', src: '1 add' } as SugarInfo
    const out = sugarExpansion(boundRegistry(), info, false)
    assert.equal(out[0]!.asWord().name, 'w-mini')
    assert.equal(out[1]!.asWord().name, 'm')
    assert.equal(out[2]!.asString(), '1 add')
  })

  it('lowers type-bound to a paren expression through gen-apply', () => {
    const info = { kind: 'type-bound', items: [newWord('T')] } as SugarInfo
    const out = sugarExpansion(boundRegistry(), info, false)
    assert.equal(out.length, 1)
    assert.match(render(out), /w-gen-apply/)
  })
})

describe('sugarExpansion — angle takes two forms', () => {
  it('uses the use-site paren form by default', () => {
    const info = { kind: 'angle', name: 'Box', items: [newWord('Integer')] } as SugarInfo
    const out = sugarExpansion(boundRegistry(), info, false)
    assert.equal(out.length, 1)
    assert.match(render(out), /w-gen-apply/)
  })

  it('uses the generic-def head form when headForm is set', () => {
    const info = {
      kind: 'angle',
      name: 'Box',
      head: newList([newWord('T')]),
    } as SugarInfo
    const out = sugarExpansion(boundRegistry(), info, true)
    assert.equal(out[0]!.asWord().name, 'Box')
    assert.equal(out[1]!.asWord().name, 'w-gen-head')
  })

  it('raises the recorded head error rather than expanding', () => {
    const info = { kind: 'angle', name: 'Box', headErr: 'bad head' } as SugarInfo
    assert.throws(
      () => sugarExpansion(boundRegistry(), info, true),
      (e: unknown) => e instanceof BoruError && e.message.includes('bad head'),
    )
  })
})

describe('sugarExpansion — refusal', () => {
  it('raises sugar_unbound when the registry binds no word for the role', () => {
    // The bare kernel: zero bindings, so every role refuses cleanly instead
    // of minting a name the user did not write.
    assert.throws(
      () => sugarExpansion(new Registry(), { kind: 'lambda' } as SugarInfo, false),
      (e: unknown) => e instanceof BoruError && e.message.includes('sugar role "lambda"'),
    )
  })

  it('raises sugar_unbound for an unknown kind', () => {
    assert.throws(
      () => sugarExpansion(boundRegistry(), { kind: 'not-a-role' } as unknown as SugarInfo, false),
      (e: unknown) => e instanceof BoruError && e.message.includes('unknown sugar kind'),
    )
  })

  it('refuses a role the registry binds only partially', () => {
    // angle needs gen-apply; binding the marker's own name is not enough.
    const r = new Registry()
    r.bindSugarWord('angle', 'w-angle')
    assert.throws(
      () => sugarExpansion(r, { kind: 'angle', name: 'Box', items: [] } as SugarInfo, false),
      BoruError,
    )
  })
})

describe('sugarExpansion — the lowered run is well formed', () => {
  it('produces values that render, not [object Object]', () => {
    const r = boundRegistry()
    for (const info of [
      { kind: 'lambda' },
      { kind: 'force-arity', n: 2n },
      { kind: 'mini', name: 'm', src: 's' },
      { kind: 'angle', name: 'B', items: [newInteger(1n)] },
    ] as SugarInfo[]) {
      assert.doesNotMatch(render(sugarExpansion(r, info, false)), /\[object Object\]/)
    }
  })
})
