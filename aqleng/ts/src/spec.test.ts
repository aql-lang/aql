// Spec-driven tests, mirroring aqleng/go/spec_test.go.
//
// Reads the SAME .tsv files at ../test/spec/ relative to this
// project (i.e. aqleng/test/spec/) and runs each row through the
// TypeScript engine. Uses the built-in node:test runner (Node 24+);
// the project relies on Node's experimental type-stripping so the
// .ts source executes without a transpile step.

import { describe, it } from 'node:test'
import { strict as assert } from 'node:assert'
import * as fs from 'node:fs'
import * as path from 'node:path'
import { fileURLToPath } from 'node:url'

import {
  AqlError,
  Engine,
  type Handler,
  type NativeFunc,
  Registry,
  TAny,
  TBoolean,
  TDecimal,
  TInteger,
  TNone,
  TString,
  TList,
  TWord,
  Value,
  type FnParam,
  type WordInfo,
  newBoolean,
  newDecimal,
  newFnDef,
  newInteger,
  newList,
  newString,
  newTypeLiteral,
  newWord,
  typeNameTable,
} from './index.ts'

const __filename = fileURLToPath(import.meta.url)
const __dirname = path.dirname(__filename)
const SPEC_DIR = path.resolve(__dirname, '..', '..', 'test', 'spec')

function registerSpecWords(r: Registry): void {
  const intHandler =
    (op: (a: bigint, b: bigint) => bigint): Handler =>
    (args) =>
      [newInteger(op(args[0]!.asInteger(), args[1]!.asInteger()))]

  const reg = (fn: NativeFunc): void => r.registerNativeFunc(fn)

  reg({
    name: 'add',
    forwardPrecedence: true,
    signatures: [{ args: [TInteger, TInteger], handler: intHandler((a, b) => a + b) }],
  })
  reg({
    name: 'sub',
    forwardPrecedence: true,
    signatures: [{ args: [TInteger, TInteger], handler: intHandler((a, b) => a - b) }],
  })
  reg({
    name: 'mul',
    forwardPrecedence: true,
    signatures: [{ args: [TInteger, TInteger], handler: intHandler((a, b) => a * b) }],
  })
  reg({
    name: 'neg',
    signatures: [
      {
        args: [TInteger],
        handler: (args) => [newInteger(-args[0]!.asInteger())],
      },
    ],
  })
  reg({
    name: 'dup',
    signatures: [
      {
        args: [TAny],
        handler: (args) => [args[0]!, args[0]!],
      },
    ],
  })
  reg({
    name: 'swap',
    signatures: [
      {
        args: [TAny, TAny],
        // Under the unified §1.4 dispatch rule, args[0] is the top
        // of the stack and args[1] is the next-deeper. The splice
        // writes the handler's return slice in source order, so
        // returning [args[0], args[1]] places the old top at the
        // deeper position and the old next-deeper at the top — the
        // two values come out swapped on the stack.
        handler: (args) => [args[0]!, args[1]!],
      },
    ],
  })
  reg({
    name: 'drop',
    signatures: [
      {
        args: [TAny],
        handler: () => [],
      },
    ],
  })
  reg({
    name: 'concat',
    forwardPrecedence: true,
    signatures: [
      {
        args: [TString, TString],
        handler: (args) => [newString(args[0]!.asString() + args[1]!.asString())],
      },
    ],
  })
  reg({
    name: 'not',
    forwardPrecedence: true,
    signatures: [
      {
        args: [TBoolean],
        handler: (args) => [newBoolean(!args[0]!.asBoolean())],
      },
    ],
  })
  reg({
    name: 'describe',
    forwardPrecedence: true,
    signatures: [
      {
        args: [TInteger],
        handler: (args) => [newString(`int:${args[0]!.asInteger().toString()}`)],
      },
      {
        args: [TString],
        handler: (args) => [newString(`str:${args[0]!.asString()}`)],
      },
    ],
  })
  reg({
    name: 'tag',
    forwardPrecedence: true,
    signatures: [
      {
        args: [TAny],
        handler: () => [newString('any')],
      },
      {
        args: [TInteger],
        handler: () => [newString('specific')],
      },
    ],
  })

  // fact, code, route: §1.1 literal-pattern dispatch via patterns.
  // Each declares a specific-value overload first plus a catch-all.
  reg({
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
  })
  reg({
    name: 'code',
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
  reg({
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

  // trip: 3-arg integer formatter. Default barrier = N so all
  // position-mixing arrangements (all-forward through all-stack)
  // bind sig[0..2] to the same source-order args.
  reg({
    name: 'trip',
    forwardPrecedence: true,
    signatures: [
      {
        args: [TInteger, TInteger, TInteger],
        handler: (args) =>
          [newString(
            `${args[0]!.asInteger().toString()},${args[1]!.asInteger().toString()},${args[2]!.asInteger().toString()}`,
          )],
      },
    ],
  })

  // pair: mixed-barrier sig [Integer | Integer]. Forward fills
  // sig[0]; sig[1] must come from the stack. The handler formats
  // "args[0]:args[1]" so the binding is visible in the output.
  reg({
    name: 'pair',
    forwardPrecedence: true,
    signatures: [
      {
        args: [TInteger, TInteger],
        barrierPos: 1,
        handler: (args) =>
          [newString(`${args[0]!.asInteger().toString()}:${args[1]!.asInteger().toString()}`)],
      },
    ],
  })

  // length, first: list-aware test words for list.tsv.
  reg({
    name: 'length',
    forwardPrecedence: true,
    signatures: [
      {
        args: [TList],
        handler: (args) => [newInteger(BigInt(args[0]!.asList().length))],
      },
    ],
  })
  reg({
    name: 'first',
    forwardPrecedence: true,
    signatures: [
      {
        args: [TList],
        handler: (args) => {
          const lst = args[0]!.asList()
          if (lst.length === 0) return [newTypeLiteral(TNone)]
          return [lst[0]!]
        },
      },
    ],
  })

  // def: spec-subset code-body binding. Captures `def NAME body`
  // where NAME arrives as a Word token (no /q machinery in this
  // runner's tokenizer) and body is any value — typically a List
  // literal that becomes a callable code body. The handler pushes
  // the body onto the def stack under NAME.
  reg({
    name: 'def',
    forwardPrecedence: true,
    signatures: [
      {
        args: [TWord, TAny],
        noEvalArgs: new Set([1]),
        handler: (args, _ctx, _stk, registry) => {
          const w = args[0]!.asWord()
          registry.pushDef(w.name, args[1]!)
          return []
        },
      },
    ],
  })

  // fn: builds a function definition value from
  // `fn [ params ] [ returns ] [ body ]`. Each param is a single
  // Word token of the form `name:Type` (the spec tokenizer is
  // whitespace-only so we don't get a separate `:` token). The
  // handler parses each param, resolves the type via the type-name
  // table, and produces a TFunction Value the engine dispatches
  // when the surrounding `def` binding is later invoked.
  reg({
    name: 'fn',
    forwardPrecedence: true,
    signatures: [
      {
        args: [TList, TList, TList],
        noEvalArgs: new Set([0, 1, 2]),
        handler: (args) => {
          const paramsList = args[0]!.asList()
          const returnsList = args[1]!.asList()
          const body = args[2]!.asList()
          const params = paramsList.map((p) => parseFnParam(p))
          const returns = returnsList.map((r) => parseFnReturn(r))
          return [newFnDef({ params, returns, body })]
        },
      },
    ],
  })

  // Simple-value defs the def.tsv spec references. A word whose name
  // is in the def stack is substituted by its value before normal
  // dispatch, provided the value isn't an FnDef / ObjectType.
  r.pushDef('pi', newInteger(3n))
  r.pushDef('tau', newInteger(6n))
  r.pushDef('greeting', newString('hello'))
}

// ── Tokenizer ─────────────────────────────────────────────────────────────

interface TokenStream {
  s: string
  i: number
}

function tokenize(s: string): Value[] {
  const stream: TokenStream = { s, i: 0 }
  const result = readTokens(stream, null)
  if (stream.i < stream.s.length) {
    throw new Error(`tokenize: unexpected ']' at ${stream.i}`)
  }
  return result
}

/**
 * Read tokens from `stream` until end-of-string or the matching close
 * bracket `until` (the character `]` for list bodies). Returns the
 * collected Values. Lists nest recursively — `[ [ 1 ] 2 ]` becomes
 * a TList containing a TList of Integer(1) plus an Integer(2).
 */
function readTokens(stream: TokenStream, until: ']' | null): Value[] {
  const out: Value[] = []
  while (stream.i < stream.s.length) {
    while (stream.i < stream.s.length && (stream.s[stream.i] === ' ' || stream.s[stream.i] === '\t')) {
      stream.i++
    }
    if (stream.i >= stream.s.length) break

    if (stream.s[stream.i] === '"') {
      let j = stream.i + 1
      while (j < stream.s.length && stream.s[j] !== '"') j++
      if (j >= stream.s.length) throw new Error(`unterminated string at ${stream.i}`)
      out.push(newString(stream.s.slice(stream.i + 1, j)))
      stream.i = j + 1
      continue
    }

    let j = stream.i
    while (j < stream.s.length && stream.s[j] !== ' ' && stream.s[j] !== '\t') j++
    const tok = stream.s.slice(stream.i, j)
    stream.i = j

    if (tok === '[') {
      const elems = readTokens(stream, ']')
      out.push(newList(elems))
      continue
    }
    if (tok === ']') {
      if (until !== ']') {
        throw new Error(`tokenize: unmatched ']' at ${stream.i - 1}`)
      }
      return out
    }

    switch (tok) {
      case 'true':
        out.push(newBoolean(true))
        continue
      case 'false':
        out.push(newBoolean(false))
        continue
      case 'null':
        out.push(newTypeLiteral(TNone))
        continue
    }
    if (/^-?\d+$/.test(tok)) {
      out.push(newInteger(BigInt(tok)))
      continue
    }
    if (/^-?\d+\.\d+$/.test(tok)) {
      out.push(newDecimal(Number.parseFloat(tok)))
      continue
    }
    // /s and /f trailing modifiers — at the call site, force
    // stack-only or forward-only dispatch regardless of the sig's
    // declared barrierPos. Mirrors the lexer's handling of these
    // modifiers in the full parser.
    let name = tok
    let forceStack = false
    let forceForward = false
    if (name.endsWith('/s')) {
      forceStack = true
      name = name.slice(0, -2)
    } else if (name.endsWith('/f')) {
      forceForward = true
      name = name.slice(0, -2)
    }
    out.push(newWordWithModifiers(name, forceStack, forceForward))
  }
  if (until === ']') {
    throw new Error(`tokenize: unterminated list literal '['`)
  }
  return out
}

function newWordWithModifiers(name: string, forceStack: boolean, forceForward: boolean): Value {
  if (!forceStack && !forceForward) return newWord(name)
  const wi: WordInfo = { name }
  if (forceStack) wi.forceStack = true
  if (forceForward) wi.forceForward = true
  return new Value(TWord, wi)
}

/**
 * Parse one fn-param token. The spec tokenizer produces a Word per
 * whitespace-split chunk, so a typed param like `n:Integer` arrives
 * as a single Word with name `n:Integer`. Split on the first `:`,
 * resolve the type name, build an FnParam.
 */
function parseFnParam(v: Value): FnParam {
  if (!v.isWord()) {
    throw new Error(`fn: expected param Word, got ${v.toString()}`)
  }
  const w = v.asWord()
  const idx = w.name.indexOf(':')
  if (idx < 0) {
    throw new Error(`fn: param ${JSON.stringify(w.name)} missing ':TypeName' suffix`)
  }
  const name = w.name.slice(0, idx)
  const typeName = w.name.slice(idx + 1)
  const type = typeNameTable().get(typeName)
  if (!type) throw new Error(`fn: unknown type ${JSON.stringify(typeName)}`)
  return { name, type }
}

/** Parse one fn-return token (a bare TypeName Word). */
function parseFnReturn(v: Value) {
  if (!v.isWord()) throw new Error(`fn: expected return type Word`)
  const w = v.asWord()
  const type = typeNameTable().get(w.name)
  if (!type) throw new Error(`fn: unknown return type ${JSON.stringify(w.name)}`)
  return type
}

function renderStack(stack: Value[]): string {
  return stack.map(renderValue).join(' ')
}

function renderValue(v: Value): string {
  if (v.data === null) {
    if (v.vType.equal(TNone)) return 'null'
    return v.vType.toString()
  }
  if (v.vType.matches(TInteger)) return v.asInteger().toString()
  if (v.vType.matches(TDecimal)) return String(v.asDecimal())
  if (v.vType.matches(TString)) return JSON.stringify(v.asString())
  if (v.vType.matches(TBoolean)) return String(v.asBoolean())
  return v.toString()
}

// ── Test harness ─────────────────────────────────────────────────────────

interface SpecRow {
  lineNum: number
  input: string
  expected: string
}

function parseSpec(file: string): SpecRow[] {
  const text = fs.readFileSync(file, 'utf8')
  const rows: SpecRow[] = []
  text.split('\n').forEach((raw, idx) => {
    const lineNum = idx + 1
    const trimmed = raw.replace(/[ \t]+$/, '')
    if (trimmed === '' || trimmed.startsWith('#')) return
    const parts = trimmed.split('\t')
    if (parts.length < 2) {
      throw new Error(`${file}:L${lineNum}: malformed row, want at least input<TAB>expected`)
    }
    rows.push({
      lineNum,
      input: parts[0]!.trim(),
      expected: parts[1]!.trim(),
    })
  })
  return rows
}

function runRow(row: SpecRow): { ok: true; got: string } | { ok: false; err: string } {
  const r = new Registry()
  registerSpecWords(r)

  let values: Value[]
  try {
    values = tokenize(row.input)
  } catch (e) {
    return { ok: false, err: `tokenize: ${(e as Error).message}` }
  }

  try {
    const result = new Engine(r).run(values)
    return { ok: true, got: renderStack(result) }
  } catch (e) {
    if (e instanceof AqlError) {
      return { ok: false, err: e.message }
    }
    return { ok: false, err: `unexpected: ${(e as Error).message}` }
  }
}

const specFiles = fs
  .readdirSync(SPEC_DIR)
  .filter((f) => f.endsWith('.tsv'))
  .sort()

describe('spec', () => {
  for (const file of specFiles) {
    const fullPath = path.join(SPEC_DIR, file)
    const rows = parseSpec(fullPath)
    describe(file.replace(/\.tsv$/, ''), () => {
      for (const row of rows) {
        const name = `L${row.lineNum} ${row.input}`
        it(name, () => {
          const result = runRow(row)
          if (row.expected.startsWith('ERROR:')) {
            const want = row.expected.slice('ERROR:'.length)
            assert.ok(!result.ok, `expected error containing ${JSON.stringify(want)}`)
            if (!result.ok && want !== '') {
              assert.ok(
                result.err.toLowerCase().includes(want.toLowerCase()),
                `error ${JSON.stringify(result.err)} does not contain ${JSON.stringify(want)}`,
              )
            }
            return
          }
          assert.ok(result.ok, result.ok ? '' : `unexpected error: ${result.err}`)
          if (result.ok) {
            assert.equal(result.got, row.expected)
          }
        })
      }
    })
  }
})
