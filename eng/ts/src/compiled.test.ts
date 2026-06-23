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
  TAtom,
  TBoolean,
  TInteger,
  TList,
  TNumber,
  TString,
  Value,
  coerceBoolean,
  joinCarriers,
  newBoolean,
  newDynamicCarrier,
  newFloat,
  newInteger,
  newList,
  newString,
  newWord,
  compile,
  disassemble,
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
  // gtq: a Boolean comparison (a > b), real handler + Boolean return.
  reg({
    name: 'gtq',
    forwardPrecedence: true,
    signatures: [
      { args: [TNumber, TNumber], handler: (args) => [newBoolean(toFloat(args[0]!) > toFloat(args[1]!))], returns: [TBoolean] },
    ],
  })
  // if: 3-arg conditional. The runtime handler runs the chosen arm in a
  // sub-engine (like `do`); the check returnsFn analyses BOTH arms,
  // capturing each as a bytecode fragment, joins their result carriers,
  // and records a branch event (recordsOwnEvent suppresses the generic
  // recordCall). A 2-arg `if cond [then]` overload exists for the
  // interpreter but has no returnsFn, so it falls back when compiled.
  reg({
    name: 'if',
    forwardPrecedence: true,
    signatures: [
      {
        args: [TAny, TList, TList],
        noEvalArgs: new Set([1, 2]),
        recordsOwnEvent: true,
        returns: [TAny],
        handler: (args, _ctx, _stk, r) =>
          coerceBoolean(args[0]!)
            ? new Engine(r).run([...args[1]!.asList()])
            : new Engine(r).run([...args[2]!.asList()]),
        returnsFn: (args, r) => {
          const emit = r.check.emit
          const thenList = args[1]!
          const elseList = args[2]!
          if (emit === undefined) {
            const t = new Engine(r).run([...thenList.asList()])
            const e = new Engine(r).run([...elseList.asList()])
            const tv = t.length === 1 ? t[0]! : newDynamicCarrier(TAny)
            const ev = e.length === 1 ? e[0]! : newDynamicCarrier(TAny)
            return [joinCarriers(tv, ev)]
          }
          const condOp = emit.classify(args[0]!)
          emit.beginFragment()
          const tRes = new Engine(r).run([...thenList.asList()])
          if (tRes.length !== 1) emit.markUncompilable('if: then arm is not single-result')
          const tVal = tRes.length === 1 ? tRes[0]! : newDynamicCarrier(TAny)
          const tFrag = emit.endFragment(tVal)
          emit.beginFragment()
          const eRes = new Engine(r).run([...elseList.asList()])
          if (eRes.length !== 1) emit.markUncompilable('if: else arm is not single-result')
          const eVal = eRes.length === 1 ? eRes[0]! : newDynamicCarrier(TAny)
          const eFrag = emit.endFragment(eVal)
          const merged = joinCarriers(tVal, eVal)
          if (condOp === null) emit.markUncompilable('if: condition of unknown provenance')
          if (condOp !== null && tFrag !== null && eFrag !== null) {
            emit.recordBranch(condOp, tFrag, eFrag, merged)
          }
          return [merged]
        },
      },
      {
        // 2-arg form (no else) — interpreter only; falls back when compiled.
        args: [TAny, TList],
        noEvalArgs: new Set([1]),
        handler: (args, _ctx, _stk, r) =>
          coerceBoolean(args[0]!) ? new Engine(r).run([...args[1]!.asList()]) : [],
      },
    ],
  })
  // def: check-aware name binding (runInCheckMode so it binds in the
  // record pass). A bound computed value (e.g. `def a (addq 1 2)`) shares
  // one carrier; referencing it twice records two event operands pointing
  // at the same producing event — the locals-threaded lowering handles
  // the sharing with no extra machinery.
  reg({
    name: 'def',
    forwardPrecedence: true,
    signatures: [
      {
        args: [TAtom, TAny],
        quoteArgs: new Set([0]),
        runInCheckMode: true,
        returns: [],
        handler: (args, _ctx, _stk, r) => {
          const a = args[0]!
          const name = a.isWord() ? a.asWord().name : a.asAtom()
          r.pushDef(name, args[1]!)
          return []
        },
      },
    ],
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
  ['addq 1 (addq 2 (addq 3 4))', true], // deep right-nesting
  ['mulq (addq 1 2) (subq 10 5)', true], // two event operands in one call
  ['subq 10 3', true], // non-commutative arg order
  ['def x 5 addq x 1', true], // def of a literal -> const operand
  ['def a (addq 1 2) mulq a a', true], // multiref of a computed value
  ['def a (addq 1 2) def b (mulq a 10) subq b a', true], // chained shared bindings
  ['if true [1] [2]', true], // literal cond, true arm
  ['if false [1] [2]', true], // literal cond, false arm
  ['if (gtq 5 3) [addq 1 2] [mulq 2 2]', true], // computed (event) cond
  ['if (gtq 1 9) [addq 1 2] [mulq 2 2]', true], // computed cond, false arm
  ["if true [1] ['x']", true], // divergent arm types (coarse carrier join)
  ['if (gtq 5 3) [if true [10] [20]] [30]', true], // nested if in an arm
  ['addq 100 (if (gtq 5 3) [1] [2])', true], // branch result feeds a call
]

const FALLBACK: string[] = [
  'dupq 5', // multi-result -> refused, falls back
  'if false [42]', // 2-arg if (no else) -> variadic, falls back
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

  it('disassembles if-then-else to jump-bracketed code', () => {
    const result = compile(freshRegistry(), tokenize('if true [1] [2]'))
    assert.ok('program' in result, 'expected a compiled program')
    assert.equal(
      disassemble(result.program),
      [
        '000  PUSH_CONST 0', // condition (true)
        '001  JMP_IF_FALSE 5', // false -> else arm at pc 5
        '002  PUSH_CONST 1', // then arm: 1
        '003  STORE_LOCAL 0', // -> branch slot
        '004  JMP 7', // skip else
        '005  PUSH_CONST 2', // else arm: 2
        '006  STORE_LOCAL 0', // -> branch slot
        '007  PUSH_LOCAL 0', // residual: the merged result
      ].join('\n'),
    )
  })
})
