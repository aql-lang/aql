// Unit probes for value.ts's remaining arms: the predicate table over
// every value mode (the paths the engine suites reach only from one
// side), constructor round-trips, and the accessor miss paths.

import { describe, it } from 'node:test'
import { strict as assert } from 'node:assert'

import {
  TInteger,
  TList,
  TMap,
  TNone,
  TString,
} from './type.ts'
import {
  ChildType,
  ClassTypeInfo,
  ErrorInfo,
  OrderedMap,
  Value,
  asReach,
  isErrorValue,
  isReach,
  newErrorValue,
  newInteger,
  newList,
  newMap,
  newNone,
  newReach,
  newSplice,
  newTypeLiteral,
  newTypedList,
  newTypedMap,
  newWord,
} from './value.ts'

describe('value predicate table', () => {
  const none = newNone()
  const lit = newTypeLiteral(TInteger)
  const int = newInteger(5)
  const carrier = new Value(TMap, null, { carrier: true })

  it('isTypeLiteral excludes none and carriers', () => {
    assert.equal(lit.isTypeLiteral(), true)
    assert.equal(none.isTypeLiteral(), false)
    assert.equal(carrier.isTypeLiteral(), false)
    assert.equal(int.isTypeLiteral(), false)
  })

  it('isConcrete tracks payload presence', () => {
    assert.equal(int.isConcrete(), true)
    assert.equal(lit.isConcrete(), false)
    assert.equal(carrier.isConcrete(), false)
  })

  it('container predicates over every construction', () => {
    const om = new OrderedMap()
    om.set('a', int)
    const m = newMap(om)
    const l = newList([int])
    const tl = newTypedList(newTypeLiteral(TInteger))
    const tm = newTypedMap(newTypeLiteral(TString))
    assert.equal(m.isMap(), true)
    assert.equal(l.isMap(), false)
    assert.equal(tl.isTypedList(), true)
    assert.equal(l.isTypedList(), false)
    assert.equal(tm.isTypedMap(), true)
    assert.equal(m.isTypedMap(), false)
    assert.equal(tl.data instanceof ChildType, true)
  })

  it('error values carry their message and type', () => {
    const e = newErrorValue('boom')
    assert.equal(isErrorValue(e), true)
    assert.equal(isErrorValue(int), false)
    assert.equal((e.data as ErrorInfo).message, 'boom')
  })

  it('reach values round-trip and miss safely', () => {
    const r = newReach({ segments: [] } as never)
    assert.equal(isReach(r), true)
    assert.equal(isReach(int), false)
    assert.notEqual(asReach(r), undefined)
    assert.equal(asReach(int), undefined)
  })

  it('splices and words render distinctly', () => {
    const sp = newSplice(int)
    const w = newWord('go')
    assert.notEqual(sp.toString(), '')
    assert.equal(w.isWord(), true)
    assert.equal(int.isWord(), false)
  })

  it('class-type info paths compose', () => {
    const fields = new OrderedMap()
    fields.set('x', newTypeLiteral(TInteger))
    const info = new ClassTypeInfo('P', 'Class', fields)
    assert.equal(info.path, 'Class/P')
  })

  it('none and the None literal stay distinct', () => {
    assert.equal(none.isNone(), true)
    assert.equal(newTypeLiteral(TNone).isNone(), false)
    assert.equal(newTypeLiteral(TList).vType.equal(TList), true)
  })
})
