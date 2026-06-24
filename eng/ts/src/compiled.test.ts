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
  AqlError,
  Engine,
  type FnParam,
  type FnSig,
  type Handler,
  type NativeFunc,
  type Operand,
  Registry,
  TAny,
  TAtom,
  TBoolean,
  TFunction,
  TInteger,
  TList,
  TNumber,
  TString,
  Value,
  coerceBoolean,
  joinCarriers,
  newBoolean,
  newCarrier,
  newDynamicCarrier,
  newFloat,
  newFnDef,
  newInteger,
  newList,
  newMap,
  newParenExpr,
  newString,
  newWord,
  OrderedMap,
  compile,
  disassemble,
  runCompiled,
  runProgram,
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
  // fn: builds a FnDefInfo from [params][returns][body] triples (params
  // are `name:Type` words). runInCheckMode so the value is constructed
  // during the record pass; `def` then binds it and a call inlines the
  // body (see analyseFnBody's compile path).
  reg({
    name: 'fn',
    forwardPrecedence: true,
    signatures: [
      {
        args: [TList],
        noEvalArgs: new Set([0]),
        runInCheckMode: true,
        handler: (args) => {
          const elems = args[0]!.asList()
          const sigs: FnSig[] = []
          for (let i = 0; i + 2 < elems.length; i += 3) {
            const params: FnParam[] = elems[i]!.asList().map((p) => {
              const nm = p.isWord() ? p.asWord().name : ''
              const colon = nm.indexOf(':')
              const name = colon > 0 ? nm.slice(0, colon) : nm
              const t = (colon > 0 ? typeTable.get(nm.slice(colon + 1)) : undefined) ?? TAny
              return { name, type: t }
            })
            const returns = elems[i + 1]!.asList().map((rt) => rt.vType)
            sigs.push({ params, returns, body: elems[i + 2]!.asList() })
          }
          return [newFnDef({ sigs })]
        },
      },
    ],
  })
  // for: counted loop, count form `for N [body]` (iterator `i` bound
  // 0..N-1). The runtime handler iterates and ACCUMULATES each
  // iteration's body residual; the check returnsFn mirrors AnalyseLoopBody
  // (single-round): bind `i` to an Integer carrier, capture the body as a
  // fragment, and record a loop event. The loop is variadic — it compiles
  // only as the program's trailing producer (else falls back).
  reg({
    name: 'for',
    forwardPrecedence: true,
    signatures: [
      {
        args: [TInteger, TList],
        noEvalArgs: new Set([1]),
        recordsOwnEvent: true,
        returns: [TList],
        handler: (args, _ctx, _stk, r) => {
          const count = args[0]!.asInteger()
          const body = args[1]!.asList()
          const out: Value[] = []
          for (let i = 0n; i < count; i++) {
            r.pushDef('i', newInteger(i))
            out.push(...new Engine(r).run([...body]))
            r.popDef('i')
          }
          return out
        },
        returnsFn: (args, r) => {
          const body = args[1]!
          const emit = r.check.emit
          if (emit === undefined) {
            const iter = newCarrier(TInteger)
            r.pushDef('i', iter)
            new Engine(r).run([...body.asList()])
            r.popDef('i')
            return [newCarrier(TList)]
          }
          const endOp = emit.classify(args[0]!) // count == loop end
          const iterSlot = emit.allocSlot()
          const iter = newCarrier(TInteger)
          emit.bindSlot(iter, iterSlot)
          r.pushDef('i', iter)
          emit.beginFragment()
          const bodyRes = new Engine(r).run([...body.asList()])
          if (bodyRes.length !== 1) emit.markUncompilable('for: body is not single-result')
          const bodyVal = bodyRes.length === 1 ? bodyRes[0]! : newDynamicCarrier(TAny)
          const bodyFrag = emit.endFragment(bodyVal)
          r.popDef('i')
          const out = newCarrier(TList)
          const startOp: Operand = { kind: 'const', value: newInteger(0n) }
          const stepOp: Operand = { kind: 'const', value: newInteger(1n) }
          if (endOp !== null && bodyFrag !== null) {
            emit.recordLoop(startOp, endOp, stepOp, iterSlot, bodyFrag, out)
          } else {
            emit.markUncompilable('for: count or body of unknown provenance')
          }
          return [out]
        },
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
  // trapq: a CHECK-LENIENT word (the orphan-`gen` analogue) — it passes the
  // static checker but raises at runtime. runInCheckMode so its handler runs
  // in the record pass: there it records a TERMINAL trap (no error) so the
  // compiled program raises the byte-identical error via OpTrap; at run time
  // it throws. A trap inside a branch/loop fragment is conditional —
  // recordTrap refuses and the program falls back to the interpreter.
  reg({
    name: 'trapq',
    forwardPrecedence: true,
    signatures: [
      {
        args: [TAny],
        runInCheckMode: true,
        returns: [],
        handler: (args, _ctx, _stk, r) => {
          if (r.check.isActive()) {
            const emit = r.check.emit
            if (emit !== undefined && !emit.recordTrap('trap_error', 'trapq: deliberate runtime trap', 'trapq')) {
              emit.markUncompilable('trapq: trap in conditional context')
            }
            return []
          }
          throw new AqlError('trap_error', 'trapq: deliberate runtime trap', 'trapq')
        },
      },
    ],
  })
  // tripleq: a word the recorder ISLANDS instead of compiling natively
  // (compileFallback) — the orphan higher-order / dynamic-dispatch analogue.
  // Its real handler (n*3) runs through a sub-engine at VM time; the
  // surrounding compiled code keeps running. Baked args ride in the island
  // tokens; a single computed arg is threaded from the operand stack.
  reg({
    name: 'tripleq',
    forwardPrecedence: true,
    signatures: [
      {
        args: [TNumber],
        compileFallback: true,
        returns: [TNumber],
        handler: (args) => {
          const v = args[0]!
          return v.vType.matches(TInteger) ? [newInteger(v.asInteger() * 3n)] : [newFloat(v.asFloat() * 3)]
        },
      },
    ],
  })
  // dynq: identity that is DYNAMIC in check mode — its returnsFn hands back a
  // dynamic Any carrier (the checker can't know the type), but at run time it
  // returns its concrete arg. A dynamic result makes the consumer a poly site.
  reg({
    name: 'dynq',
    forwardPrecedence: true,
    signatures: [
      {
        args: [TAny],
        returns: [TAny],
        returnsFn: () => [newDynamicCarrier(TAny)],
        handler: (args) => [args[0]!],
      },
    ],
  })
  // polyq: two overloads selected by operand type. With a dynamic operand the
  // checker can't commit, so the call records poly and the VM re-matches at
  // run time — Integer -> 'int', String -> 'str'.
  reg({
    name: 'polyq',
    forwardPrecedence: true,
    signatures: [
      { args: [TInteger], handler: () => [newString('int')], returns: [TString] },
      { args: [TString], handler: () => [newString('str')], returns: [TString] },
    ],
  })
  // applyq: a higher-order word that APPLIES a unary fn VALUE to an arg.
  // The fn flows in as a runtime operand (a closure), so the recorder can't
  // inline it; marked compileFallback, it islands — a capture-free fn bakes
  // into the island tokens and re-applies at run time, a capture-bearing fn
  // falls back. Mirrors a dynamic fn-value call (eng/go OpCallDynamic, whose
  // non-compiled branch islands through a sub-engine just like this).
  reg({
    name: 'applyq',
    forwardPrecedence: true,
    signatures: [
      {
        args: [TFunction, TAny],
        compileFallback: true,
        returns: [TAny],
        handler: (args, _ctx, _stk, r) => {
          const sig = args[0]!.asFnDef().sigs[0]!
          const name = sig.params[0]!.name
          r.pushDef(name, args[1]!)
          try {
            return new Engine(r).run([...sig.body])
          } finally {
            r.popDef(name)
          }
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
  // A `( … )` group as a map value becomes a ParenExpr (collapses to a
  // single residual when the map deep-evaluates). Flat parens only.
  const readParenExpr = (): Value => {
    i++ // consume '('
    const toks: Value[] = []
    for (;;) {
      while (i < src.length && (src[i] === ' ' || src[i] === '\t')) i++
      if (src[i] === ')') {
        i++
        break
      }
      toks.push(readOne())
    }
    return newParenExpr(toks)
  }
  const readMap = (): Value => {
    const om = new OrderedMap()
    for (;;) {
      while (i < src.length && (src[i] === ' ' || src[i] === '\t')) i++
      if (src[i] === '}') {
        i++
        break
      }
      let k = i
      while (k < src.length && src[k] !== ':') k++
      const key = src.slice(i, k)
      i = k + 1 // consume ':'
      while (i < src.length && (src[i] === ' ' || src[i] === '\t')) i++
      om.set(key, src[i] === '(' ? readParenExpr() : readOne())
    }
    return newMap(om)
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
    if (c === '{') {
      i++
      return readMap()
    }
    if (c === '(' || c === ')') {
      i++
      return newWord(c)
    }
    let j = i
    while (j < src.length && !' \t[](){}'.includes(src[j]!)) j++
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

// Run `fn`, capturing either its residual or the code of an AqlError it
// throws — so a compiled run and an interpreter run can be compared for
// error parity (both must error with the same code, or both succeed).
type Outcome = { residual: Value[] } | { code: string }
function outcome(fn: () => Value[]): Outcome {
  try {
    return { residual: fn() }
  } catch (e) {
    if (e instanceof AqlError) return { code: e.code }
    throw e
  }
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
  ['for 3 [i]', true], // counted loop, iterator accumulates
  ['for 3 [addq i 10]', true], // body computes per-iteration result
  ['for (lengthq [1 2 3]) [i]', true], // event-valued count
  ['for 0 [i]', true], // zero iterations -> empty residual
  ['def n 3 for n [i]', true], // def-bound count
  ['def inc fn [[n:Integer] [Integer] [addq n 1]] inc 5', true], // user fn inlined
  ['def double fn [[n:Number] [Number] [mulq n 2]] double (addq 1 2)', true], // fn with computed arg
  ['def add2 fn [[x:Integer y:Integer] [Integer] [addq x y]] add2 3 4', true], // two-param fn
  ['def id fn [[n:Integer] [Integer] [n]] id 7', true], // fn returning its param
  ['def inc fn [[n:Integer] [Integer] [addq n 1]] addq (inc 1) (inc 2)', true], // called twice
  ['[addq 1 2]', true], // computed list residual
  ['[addq 1 2 mulq 3 4]', true], // multi-element computed list
  ['[1 2 3]', true], // constant list (still compiles, as a const)
  ['{a:(addq 1 2)}', true], // computed map
  ['{a:(addq 1 2) b:(mulq 2 3)}', true], // multi-entry computed map
  ['[[addq 1 2] 9]', true], // nested computed list
  ['tripleq 5', true], // fallback island, fully baked (nIn=0)
  ['tripleq 2.5', true], // island over a float literal
  ['addq 1 (tripleq 5)', true], // island result feeds a compiled call
  ['tripleq (addq 1 2)', true], // compiled result threaded into an island (nIn=1)
  ['addq (tripleq 2) (tripleq 3)', true], // two islands feeding one compiled call
  ['def t (tripleq 4) addq t t', true], // island result bound + multiref
  ['polyq (dynq 5)', true], // dynamic operand -> poly; Integer overload at run time
  ["polyq (dynq 'x')", true], // poly selects the String overload at run time
  ['addq (dynq 1) 2', true], // poly chains: dynq poly, addq poly
  ['dynq 7', true], // bare dynamic result -> poly
  ['applyq (fn [[n:Integer] [Integer] [addq n 1]]) 5', true], // capture-free closure islanded
  ['applyq (fn [[n:Integer] [Integer] [mulq n n]]) 4', true], // multi-word closure body
]

const FALLBACK: string[] = [
  'dupq 5', // multi-result -> refused, falls back
  'if false [42]', // 2-arg if (no else) -> variadic, falls back
  'for 2 [dupq 1]', // loop body not single-result -> falls back
  'for 2 [i] addq 1 2', // loop not in trailing position -> falls back
  'def f fn [[n:Integer] [Integer] [dupq n]] f 5', // fn body count != declared -> falls back
  'lengthq [addq 1 2]', // computed-list arg (not auto-evaluated in check) -> inert guard refuses
  'def k 100 applyq (fn [[n:Integer] [Integer] [addq n k]]) 5', // closure captures k -> not bakeable -> falls back
]

// Error-parity rows: a check-lenient word compiles to a terminal OpTrap, so
// the compiled program raises the byte-identical error the interpreter does.
const TRAP: string[] = [
  'trapq 5', // bare trap -> a single TRAP
  'addq 1 2 trapq', // prefix events kept (side effects), then TRAP
]

// A trap inside a branch arm is conditional -> recordTrap refuses ->
// markUncompilable -> falls back to the interpreter (which runs the arm and
// throws). Both sides must error identically.
const TRAP_FALLBACK: string[] = ['if true [trapq 5] [9]']

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

  for (const input of TRAP) {
    it(`compiles to a trap & errors like the interpreter: ${input}`, () => {
      const result = compile(freshRegistry(), tokenize(input))
      assert.ok('program' in result, 'expected a compiled program (a trap)')
      const vm = outcome(() => runProgram(result.program, freshRegistry()))
      const interp = outcome(() => new Engine(freshRegistry()).run(tokenize(input)))
      assert.deepEqual(vm, interp)
      assert.deepEqual(vm, { code: 'trap_error' })
    })
  }

  for (const input of TRAP_FALLBACK) {
    it(`falls back on a conditional trap & errors like the interpreter: ${input}`, () => {
      const r = freshRegistry()
      const fb = outcome(() => {
        const { residual, compiled } = runCompiled(r, tokenize(input))
        assert.equal(compiled, false)
        return residual
      })
      const interp = outcome(() => new Engine(freshRegistry()).run(tokenize(input)))
      assert.deepEqual(fb, interp)
      assert.deepEqual(fb, { code: 'trap_error' })
    })
  }

  it('disassembles a trailing trap to prefix events + TRAP', () => {
    const result = compile(freshRegistry(), tokenize('addq 1 2 trapq'))
    assert.ok('program' in result, 'expected a compiled program')
    assert.equal(
      disassemble(result.program),
      [
        '000  PUSH_CONST 0', // addq arg: 2 (reverse sig order)
        '001  PUSH_CONST 1', // addq arg: 1
        '002  CALL_NATIVE 0 (addq)',
        '003  STORE_LOCAL 0', // addq result -> local 0 (side effect kept)
        '004  TRAP 0 (trap_error)', // terminal: raise, drop the residual
      ].join('\n'),
    )
  })

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

  it('disassembles a counted loop to FOR_SETUP/FOR_NEXT + back-edge', () => {
    const result = compile(freshRegistry(), tokenize('for 3 [i]'))
    assert.ok('program' in result, 'expected a compiled program')
    assert.equal(
      disassemble(result.program),
      [
        '000  PUSH_CONST 0', // step = 1
        '001  PUSH_CONST 1', // end = 3
        '002  PUSH_CONST 2', // start = 0
        '003  FOR_SETUP 0', // iterator slot 0
        '004  FOR_NEXT 7', // exhausted -> exit at pc 7
        '005  PUSH_LOCAL 0', // body: push iterator i (accumulates)
        '006  JMP 4', // back-edge to FOR_NEXT
      ].join('\n'),
    )
  })

  it('disassembles a baked island feeding a compiled call', () => {
    const result = compile(freshRegistry(), tokenize('addq 1 (tripleq 5)'))
    assert.ok('program' in result, 'expected a compiled program')
    assert.equal(
      disassemble(result.program),
      [
        '000  FALLBACK 0 (tripleq nIn=0)', // island: re-runs [tripleq 5]
        '001  STORE_LOCAL 0', // island result -> local 0
        '002  PUSH_LOCAL 0', // addq arg: the island result (reverse sig order)
        '003  PUSH_CONST 0', // addq arg: 1
        '004  CALL_NATIVE 0 (addq)',
        '005  STORE_LOCAL 1',
        '006  PUSH_LOCAL 1', // residual
      ].join('\n'),
    )
  })

  it('disassembles a compiled result threaded into an island', () => {
    const result = compile(freshRegistry(), tokenize('tripleq (addq 1 2)'))
    assert.ok('program' in result, 'expected a compiled program')
    assert.equal(
      disassemble(result.program),
      [
        '000  PUSH_CONST 0', // addq arg: 2
        '001  PUSH_CONST 1', // addq arg: 1
        '002  CALL_NATIVE 0 (addq)',
        '003  STORE_LOCAL 0', // addq result -> local 0
        '004  PUSH_LOCAL 0', // threaded island input on top of stack
        '005  FALLBACK 0 (tripleq nIn=1)', // island pops it, pushes its result
        '006  STORE_LOCAL 1',
        '007  PUSH_LOCAL 1', // residual
      ].join('\n'),
    )
  })

  it('disassembles a dynamic-operand chain to CALL_POLY', () => {
    const result = compile(freshRegistry(), tokenize('polyq (dynq 5)'))
    assert.ok('program' in result, 'expected a compiled program')
    assert.equal(
      disassemble(result.program),
      [
        '000  PUSH_CONST 0', // dynq arg: 5
        '001  CALL_POLY 0 (dynq/1)', // dynq result is dynamic -> poly
        '002  STORE_LOCAL 0',
        '003  PUSH_LOCAL 0', // polyq arg: dynq result
        '004  CALL_POLY 1 (polyq/1)', // dynamic operand -> poly
        '005  STORE_LOCAL 1',
        '006  PUSH_LOCAL 1', // residual
      ].join('\n'),
    )
  })

  it('disassembles a dynamic closure application to a baked island', () => {
    const result = compile(freshRegistry(), tokenize('applyq (fn [[n:Integer] [Integer] [addq n 1]]) 5'))
    assert.ok('program' in result, 'expected a compiled program')
    assert.equal(
      disassemble(result.program),
      [
        '000  FALLBACK 0 (applyq nIn=0)', // island re-runs [applyq <fn> 5]; the fn-value is baked
        '001  STORE_LOCAL 0',
        '002  PUSH_LOCAL 0', // residual
      ].join('\n'),
    )
  })

  it('disassembles a computed list to body + MAKE_LIST', () => {
    const result = compile(freshRegistry(), tokenize('[addq 1 2]'))
    assert.ok('program' in result, 'expected a compiled program')
    assert.equal(
      disassemble(result.program),
      [
        '000  PUSH_CONST 0', // addq arg: 2 (reverse sig order)
        '001  PUSH_CONST 1', // addq arg: 1
        '002  CALL_NATIVE 0 (addq)',
        '003  STORE_LOCAL 0', // addq result -> local 0
        '004  PUSH_LOCAL 0', // element 0
        '005  MAKE_LIST 1', // assemble [3]
        '006  STORE_LOCAL 1', // list -> local 1
        '007  PUSH_LOCAL 1', // residual
      ].join('\n'),
    )
  })
})
