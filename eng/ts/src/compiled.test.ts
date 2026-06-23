// Stage 1 of the bytecode VM: prove the record -> finalize -> execute
// pipeline end-to-end and verify it differentially against the
// interpreter. Each input is compiled and run through the VM; its
// residual (rendered via canon) must equal the interpreter's residual
// for the same source. Inputs the compiler refuses fall back to the
// interpreter (still correct). Mirrors Go's TestSpecCompiledDifferential.
//
// Fixtures here carry BOTH real handlers (so the VM produces concrete
// results) AND `returns` annotations (so the check/record pass produces
// typed carriers). Later stages extend coverage to the full spec corpus.
import { describe, it } from 'node:test'
import { strict as assert } from 'node:assert'

import { canon } from './canon.ts'
import {
  Engine,
  type Handler,
  type NativeFunc,
  Registry,
  TAny,
  TInteger,
  TList,
  TNumber,
  TString,
  Value,
  newBoolean,
  newFloat,
  newInteger,
  newList,
  newString,
  newWord,
  runCompiled,
  typeNameTable,
} from './index.ts'

const typeTable = typeNameTable()

function toFloat(v: Value): number {
  return v.vType.matches(TInteger) ? Number(v.asInteger()) : v.asFloat()
}

// numH: real numeric binary op — integer when both args are integers,
// else float. (Handlers run in the VM, so they must be correct.)
function numH(intOp: (a: bigint, b: bigint) => bigint, fltOp: (a: number, b: number) => number): Handler {
  return (args) => {
    const a = args[0]!
    const b = args[1]!
    if (a.vType.matches(TInteger) && b.vType.matches(TInteger)) {
      return [newInteger(intOp(a.asInteger(), b.asInteger()))]
    }
    return [newFloat(fltOp(toFloat(a), toFloat(b)))]
  }
}

function registerFixtures(r: Registry): void {
  const reg = (fn: NativeFunc): void => r.registerNativeFunc(fn)
  const numberPair = [TNumber, TNumber]
  reg({
    name: 'addq',
    forwardPrecedence: true,
    signatures: [{ args: numberPair, handler: numH((a, b) => a + b, (a, b) => a + b), returns: [TNumber] }],
  })
  reg({
    name: 'subq',
    forwardPrecedence: true,
    signatures: [{ args: numberPair, handler: numH((a, b) => b - a, (a, b) => b - a), returns: [TNumber] }],
  })
  reg({
    name: 'mulq',
    forwardPrecedence: true,
    signatures: [{ args: numberPair, handler: numH((a, b) => a * b, (a, b) => a * b), returns: [TNumber] }],
  })
  reg({
    name: 'negq',
    signatures: [
      {
        args: [TNumber],
        barrierPos: 1,
        handler: (args) => {
          const v = args[0]!
          return v.vType.matches(TInteger) ? [newInteger(-v.asInteger())] : [newFloat(-v.asFloat())]
        },
        returns: [TNumber],
      },
    ],
  })
  reg({
    name: 'concatq',
    forwardPrecedence: true,
    signatures: [
      {
        args: [TString, TString],
        handler: (args) => [newString(args[0]!.asString() + args[1]!.asString())],
        returns: [TString],
      },
    ],
  })
  reg({
    name: 'lengthq',
    forwardPrecedence: true,
    signatures: [
      { args: [TList], handler: (args) => [newInteger(BigInt(args[0]!.asList().length))], returns: [TInteger] },
    ],
  })
  // dupq returns TWO results — exercises the "multi-result" refusal so
  // the input falls back to the interpreter rather than compiling.
  reg({
    name: 'dupq',
    forwardPrecedence: true,
    signatures: [{ args: [TAny], handler: (args) => [args[0]!, args[0]!], returns: [TAny, TAny] }],
  })
}

// Minimal tokenizer: words, ints, floats, single/double strings, lists,
// and `( )` paren groups (emitted as bare word tokens the engine's paren
// handling consumes).
function tokenize(src: string): Value[] {
  let i = 0
  const readList = (): Value[] => {
    const out: Value[] = []
    for (;;) {
      while (i < src.length && (src[i] === ' ' || src[i] === '\t')) i++
      if (i >= src.length) throw new Error('unterminated list')
      if (src[i] === ']') {
        i++
        return out
      }
      out.push(readOne())
    }
  }
  const readOne = (): Value => {
    while (i < src.length && (src[i] === ' ' || src[i] === '\t')) i++
    const c = src[i]!
    if (c === "'" || c === '"') {
      let j = i + 1
      while (j < src.length && src[j] !== c) j++
      const s = newString(src.slice(i + 1, j))
      i = j + 1
      return s
    }
    if (c === '[') {
      i++
      return newList(readList(), { eval: true })
    }
    if (c === '(' || c === ')') {
      i++
      return newWord(c)
    }
    let j = i
    while (j < src.length && !' \t[]()'.includes(src[j]!)) j++
    const tok = src.slice(i, j)
    i = j
    if (tok === 'true' || tok === 'false') return newBoolean(tok === 'true')
    if (/^-?\d+$/.test(tok)) return newInteger(BigInt(tok))
    if (/^-?\d+\.\d+$/.test(tok)) return newFloat(Number.parseFloat(tok))
    const t = typeTable.get(tok)
    if (t !== undefined) return new Value(t, null)
    return newWord(tok)
  }
  const out: Value[] = []
  while (i < src.length) {
    while (i < src.length && (src[i] === ' ' || src[i] === '\t')) i++
    if (i >= src.length) break
    out.push(readOne())
  }
  return out
}

function freshRegistry(): Registry {
  const r = new Registry()
  registerFixtures(r)
  return r
}

// A row: input source, and whether we expect it to take the compiled
// path (true) or fall back (false).
const COMPILED: Array<[string, true]> = [
  ['addq 1 2', true],
  ['subq 5 2', true],
  ['mulq 2 3', true],
  ['addq 1 2.0', true],
  ['negq 5', true],
  ["concatq 'a' 'b'", true],
  ['lengthq [1 2 3]', true],
  ['5', true],
  ["'hi'", true],
  ['5 6', true],
  ['addq 1 2 mulq 3', true],
  ['addq 1 (mulq 2 3)', true],
  ["concatq 'x' (concatq 'y' 'z')", true],
]

const FALLBACK: string[] = [
  'dupq 5', // multi-result -> refused, falls back
]

describe('compiled (stage 1)', () => {
  for (const [input, wantCompiled] of COMPILED) {
    it(`compiles & matches: ${input}`, () => {
      const { residual, compiled, reason } = runCompiled(freshRegistry(), tokenize(input))
      assert.equal(compiled, wantCompiled, `expected compiled=${wantCompiled}${reason ? ` (refused: ${reason})` : ''}`)
      const interp = new Engine(freshRegistry()).run(tokenize(input))
      assert.equal(canon(residual), canon(interp))
    })
  }

  for (const input of FALLBACK) {
    it(`falls back & matches: ${input}`, () => {
      const { residual, compiled } = runCompiled(freshRegistry(), tokenize(input))
      assert.equal(compiled, false)
      const interp = new Engine(freshRegistry()).run(tokenize(input))
      assert.equal(canon(residual), canon(interp))
    })
  }
})
