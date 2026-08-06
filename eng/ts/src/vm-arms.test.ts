// Unit probes for the VM's guard arms (synthetic programs — the twin
// of Go's disassemble/opcode arm probes), the matcher's literal-pattern
// arms, registry bookkeeping, and the float-canon exponent tails (the
// canon rows are pinned to the Go engine's CanonValue output).

import { describe, it } from 'node:test'
import { strict as assert } from 'node:assert'

import {
  BoruError,
  type Program,
  Registry,
  type SigRef,
  type Value,
  newInteger,
  newString,
  newTypeLiteral,
  runProgram,
} from './index.ts'
import { OpCallNativePoly, OpFallback, disassemble } from './bytecode.ts'
import { canonValue } from './canon.ts'
import { matchValues } from './match.ts'
import { TInteger } from './type.ts'
import { newFloat, newMark, newWord } from './value.ts'

function prog(p: Omit<Partial<Program>, 'ops' | 'args'> & { ops: number[]; args: number[] }): Program {
  return {
    ops: Uint8Array.from(p.ops),
    args: Int32Array.from(p.args),
    consts: p.consts ?? [],
    sigs: (p.sigs ?? []) as readonly SigRef[],
    makeMaps: p.makeMaps ?? [],
    traps: p.traps ?? [],
    fallbacks: p.fallbacks ?? [],
    polyRefs: p.polyRefs ?? [],
    maxStack: p.maxStack ?? 8,
    numLocals: p.numLocals ?? 0,
  }
}

describe('vm guard arms', () => {
  it('a poly-dispatched async handler is refused', () => {
    const r = new Registry()
    r.registerNativeFunc({
      name: 'zz',
      signatures: [{ args: [], handler: (() => Promise.resolve([])) as never }],
    })
    const p = prog({ ops: [OpCallNativePoly], args: [0], polyRefs: [{ word: 'zz', arity: 0 }] })
    assert.throws(
      () => runProgram(p, r),
      (e: unknown) => e instanceof BoruError && /async handlers are not supported/.test(e.message),
    )
  })

  it('a poly-dispatched tape-coupled result is refused', () => {
    const r = new Registry()
    r.registerNativeFunc({
      name: 'zz',
      signatures: [{ args: [], handler: () => [newWord('x')] }],
    })
    const p = prog({ ops: [OpCallNativePoly], args: [0], polyRefs: [{ word: 'zz', arity: 0 }] })
    assert.throws(
      () => runProgram(p, r),
      (e: unknown) => e instanceof BoruError && /tape-coupled handler result/.test(e.message),
    )
  })

  it('a fallback island returning a marker is refused', () => {
    const p = prog({
      ops: [OpFallback],
      args: [0],
      fallbacks: [{ tokens: [newMark('m1', [])], nIn: 0, desc: 'probe' }],
    })
    assert.throws(
      () => runProgram(p, new Registry()),
      (e: unknown) => e instanceof BoruError && /tape-coupled island result/.test(e.message),
    )
  })

  it('an unknown opcode fails loudly and disassembles generically', () => {
    const p = prog({ ops: [99], args: [7] })
    assert.throws(
      () => runProgram(p, new Registry()),
      (e: unknown) => e instanceof BoruError && /bad opcode 99/.test(e.message),
    )
    assert.match(disassemble(p), /OP_99 7/)
  })
})

describe('literal-pattern matching arms', () => {
  const entry = (patterns: Map<number, Value>) => {
    const r = new Registry()
    r.registerNativeFunc({
      name: 'pw',
      signatures: [{ args: [TInteger], handler: (a) => [a[0]!], patterns }],
    })
    return r.lookup('pw')!
  }

  it('a type-literal pattern gates by type at stack positions', () => {
    const fn = entry(new Map([[0, newTypeLiteral(TInteger)]]))
    assert.equal(matchValues(fn, [newString('x')]), null)
    assert.notEqual(matchValues(fn, [newInteger(1n)]), null)
  })
})

describe('registry bookkeeping arms', () => {
  it('re-registering a word merges forward precedence', () => {
    const r = new Registry()
    r.registerNativeFunc({ name: 'w', signatures: [{ args: [], handler: () => [] }] })
    r.registerNativeFunc({
      name: 'w',
      forwardPrecedence: true,
      signatures: [{ args: [TInteger], handler: (a) => [a[0]!] }],
    })
    const entry = r.lookup('w')!
    assert.equal(entry.forwardPrecedence, true)
    assert.equal(entry.signatures.length, 2)
  })

  it('def-stack introspection reads through', () => {
    const r = new Registry()
    assert.equal(r.hasDef('d'), false)
    assert.equal(r.defStackDepth('d'), 0)
    r.pushDef('d', newInteger(1n))
    r.pushDef('d', newInteger(2n))
    assert.equal(r.hasDef('d'), true)
    assert.equal(r.defStackDepth('d'), 2)
    assert.deepEqual(r.defNames(), ['d'])
  })

  it('snapshot and restore round-trip defs and args', () => {
    const r = new Registry()
    r.pushDef('d', newInteger(1n))
    r.pushArgs([newInteger(9n)])
    const snap = r.snapshot()
    r.pushDef('d', newInteger(2n))
    r.restore(snap)
    assert.equal(r.defStackDepth('d'), 1)
  })
})

describe('float canon exponent tails (pinned to Go CanonValue)', () => {
  it('renders the strconv forms byte-identically', () => {
    assert.equal(canonValue(newFloat(1e21)), '1e+21')
    assert.equal(canonValue(newFloat(1.25e22)), '1.25e+22')
    assert.equal(canonValue(newFloat(1.5e-7)), '0.00000015')
    assert.equal(canonValue(newFloat(1e20)), '100000000000000000000.0')
  })
})
