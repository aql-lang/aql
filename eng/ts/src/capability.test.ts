// Capability slot tests, mirroring aqleng/go/capability_test.go.
//
// The two-method form (setCapability + deleteCapability) replaces the
// older nil-overloaded SetCapability. Storing null/undefined is a real
// operation distinguishable from "no capability registered" via the
// (value, ok) return tuple from capability().

import { describe, it } from 'node:test'
import { strict as assert } from 'node:assert'

import { Registry, cap } from './index.ts'

describe('capability', () => {
  it('round-trips: missing → set → replace → store-null → delete', () => {
    const r = new Registry()

    {
      const [, ok] = r.capability('missing')
      assert.equal(ok, false)
    }

    r.setCapability('foo', 42)
    {
      const [v, ok] = r.capability('foo')
      assert.equal(ok, true)
      assert.equal(v, 42)
    }

    r.setCapability('foo', 'replaced')
    {
      const [v] = r.capability('foo')
      assert.equal(v, 'replaced')
    }

    // setCapability(name, null/undefined) STORES nullish — it does
    // not delete. The entry remains, but its value is null.
    r.setCapability('foo', null)
    {
      const [v, ok] = r.capability('foo')
      assert.equal(ok, true, 'capability still present after storing null')
      assert.equal(v, null)
    }
    assert.equal(r.hasCapability('foo'), true)

    // deleteCapability is the explicit way to remove.
    assert.equal(r.deleteCapability('foo'), true)
    assert.equal(r.hasCapability('foo'), false)
    assert.equal(r.deleteCapability('foo'), false, 'second delete reports false')
  })

  it('cap<T>: returns false on missing capability', () => {
    const r = new Registry()
    const [v, ok] = cap<number>(r, 'missing')
    assert.equal(ok, false)
    assert.equal(v, undefined)
  })

  it('cap<T>: type parameter is unchecked at runtime (TS erasure)', () => {
    // PARITY GAP with Go: Go's Cap[T] runs `v.(T)` which validates at
    // runtime and returns ok=false on mismatch. TS type parameters are
    // erased, so cap<T> can only do an UNCHECKED CAST. The host is
    // responsible for storing the right shape under the agreed name.
    // We pin the current behaviour here so future drift is visible.
    const r = new Registry()
    r.setCapability('answer', 'forty-two')

    const [n, ok] = cap<number>(r, 'answer')
    assert.equal(ok, true, 'TS cap<T> cannot reject the wrong type at runtime')
    // n is typed as number but actually a string at runtime.
    assert.equal(typeof n, 'string')
  })

  it('cap<T>: returns the stored value cast to T on hit', () => {
    const r = new Registry()
    r.setCapability('answer', 'forty-two')
    const [s, ok] = cap<string>(r, 'answer')
    assert.equal(ok, true)
    assert.equal(s, 'forty-two')
  })

  it('capabilityNames lists the keys in arbitrary order', () => {
    const r = new Registry()
    r.setCapability('a', 1)
    r.setCapability('b', 2)
    r.setCapability('c', 3)
    assert.deepEqual([...r.capabilityNames()].sort(), ['a', 'b', 'c'])
  })
})
