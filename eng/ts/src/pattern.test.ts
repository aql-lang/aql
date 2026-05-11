// Literal-value dispatch via Signature.patterns (post §1.1 fix).
//
// The old engine encoded literal values in the type path
// (`Scalar/Number/Integer/0` was a strict subtype of
// `Scalar/Number/Integer`) so signature dispatch on a specific value
// fell out of subtype matching. The new engine keeps the type at the
// kind level and routes specific-value dispatch through
// `Signature.patterns`. These tests pin that behaviour.

import { describe, it } from 'node:test'
import { strict as assert } from 'node:assert'

import {
  Engine,
  type NativeFunc,
  Registry,
  TInteger,
  newInteger,
  newString,
  newWord,
} from './index.ts'

describe('§1.1 literal-pattern dispatch', () => {
  it('integer types stay at the kind level', () => {
    const a = newInteger(5)
    const b = newInteger(7)
    assert.equal(a.vType.equal(TInteger), true, 'NewInteger(5).vType.equal(TInteger)')
    assert.equal(b.vType.equal(TInteger), true)
    assert.equal(
      a.vType.equal(b.vType),
      true,
      'two integers share the same VType under the kind-only lattice',
    )
  })

  it('signature.patterns picks the specific-value overload', () => {
    // factorial-style overloads:
    //   fact 0 → 1   (literal pattern)
    //   fact n → n   (catch-all)
    const r = new Registry()
    const fn: NativeFunc = {
      name: 'fact',
      forwardPrecedence: true,
      signatures: [
        {
          args: [TInteger],
          patterns: new Map([[0, newInteger(0n)]]),
          handler: () => [newInteger(1n)],
        },
        {
          args: [TInteger],
          handler: (args) => [newInteger(args[0]!.asInteger())],
        },
      ],
    }
    r.registerNativeFunc(fn)

    {
      const out = new Engine(r).run([newWord('fact'), newInteger(0n)])
      assert.equal(out.length, 1)
      assert.equal(out[0]!.asInteger(), 1n)
    }
    {
      const out = new Engine(r).run([newWord('fact'), newInteger(7n)])
      assert.equal(out.length, 1)
      assert.equal(out[0]!.asInteger(), 7n)
    }
  })

  it('non-matching scalar pattern falls through to a less-specific sig', () => {
    // `tag` has two overloads:
    //   patterns:{0: 99} → "ninety-nine"
    //   plain Integer    → "general"
    const r = new Registry()
    r.registerNativeFunc({
      name: 'tag',
      forwardPrecedence: true,
      signatures: [
        {
          args: [TInteger],
          patterns: new Map([[0, newInteger(99n)]]),
          handler: () => [newString('ninety-nine')],
        },
        {
          args: [TInteger],
          handler: () => [newString('general')],
        },
      ],
    })

    const e = new Engine(r)
    {
      const out = e.run([newWord('tag'), newInteger(99n)])
      assert.equal(out[0]!.asString(), 'ninety-nine')
    }
    {
      const out = e.run([newWord('tag'), newInteger(42n)])
      assert.equal(out[0]!.asString(), 'general')
    }
  })

  it('string-literal patterns dispatch on Data equality', async () => {
    const { TString } = await import('./index.ts')
    const r = new Registry()
    r.registerNativeFunc({
      name: 'route',
      forwardPrecedence: true,
      signatures: [
        {
          args: [TString],
          patterns: new Map([[0, newString('admin')]]),
          handler: () => [newString('matched-admin')],
        },
        {
          args: [TString],
          handler: () => [newString('other')],
        },
      ],
    })
    const e = new Engine(r)
    {
      const out = e.run([newWord('route'), newString('admin')])
      assert.equal(out[0]!.asString(), 'matched-admin')
    }
    {
      const out = e.run([newWord('route'), newString('user')])
      assert.equal(out[0]!.asString(), 'other')
    }
  })
})
