// Golden disassembly tests — pin the compiled bytecode for representative
// straight-line programs so the encoding + lowering stay stable as later
// stages extend the opcode set.
import { describe, it } from 'node:test'
import { strict as assert } from 'node:assert'

import {
  type Handler,
  type NativeFunc,
  Registry,
  TNumber,
  Value,
  compile,
  disassemble,
  newFloat,
  newInteger,
  newWord,
} from './index.ts'

function numH(intOp: (a: bigint, b: bigint) => bigint): Handler {
  return (args) => [newInteger(intOp(args[0]!.asInteger(), args[1]!.asInteger()))]
}

function reg(): Registry {
  const r = new Registry()
  const pair = [TNumber, TNumber]
  r.registerNativeFunc({
    name: 'addq',
    forwardPrecedence: true,
    signatures: [{ args: pair, handler: numH((a, b) => a + b), returns: [TNumber] }],
  } satisfies NativeFunc)
  r.registerNativeFunc({
    name: 'mulq',
    forwardPrecedence: true,
    signatures: [{ args: pair, handler: numH((a, b) => a * b), returns: [TNumber] }],
  } satisfies NativeFunc)
  return r
}

// Tiny tokenizer (numbers, words, parens) — enough for these goldens.
function tok(src: string): Value[] {
  return src
    .split(/\s+/)
    .filter((t) => t !== '')
    .map((t) => {
      if (t === '(' || t === ')') return newWord(t)
      if (/^-?\d+$/.test(t)) return newInteger(BigInt(t))
      if (/^-?\d+\.\d+$/.test(t)) return newFloat(Number.parseFloat(t))
      return newWord(t)
    })
}

function asm(src: string): string {
  const result = compile(reg(), tok(src))
  if (!('program' in result)) throw new Error(`refused: ${result.refused}`)
  return disassemble(result.program)
}

describe('bytecode disassembly (golden)', () => {
  it('single call: addq 1 2', () => {
    // args pushed in reverse sig order (2 then 1) so sig[0]=1 lands on
    // top; result stored to local 0 then re-pushed as the residual.
    assert.equal(
      asm('addq 1 2'),
      ['000  PUSH_CONST 0', '001  PUSH_CONST 1', '002  CALL_NATIVE 0 (addq)', '003  STORE_LOCAL 0', '004  PUSH_LOCAL 0'].join(
        '\n',
      ),
    )
  })

  it('nested call: addq 1 (mulq 2 3)', () => {
    // e0 = mulq(2,3) -> local 0; e1 = addq(const 1, local 0) -> local 1.
    assert.equal(
      asm('addq 1 ( mulq 2 3 )'),
      [
        '000  PUSH_CONST 0',
        '001  PUSH_CONST 1',
        '002  CALL_NATIVE 0 (mulq)',
        '003  STORE_LOCAL 0',
        '004  PUSH_LOCAL 0',
        '005  PUSH_CONST 2',
        '006  CALL_NATIVE 1 (addq)',
        '007  STORE_LOCAL 1',
        '008  PUSH_LOCAL 1',
      ].join('\n'),
    )
  })

  it('bare literal compiles to a single PUSH_CONST', () => {
    assert.equal(asm('5'), '000  PUSH_CONST 0')
  })
})
