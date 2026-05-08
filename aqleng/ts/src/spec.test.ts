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
  TWord,
  Value,
  type WordInfo,
  newBoolean,
  newDecimal,
  newInteger,
  newString,
  newTypeLiteral,
  newWord,
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

  // Simple-value defs the def.tsv spec references. A word whose name
  // is in the def stack is substituted by its value before normal
  // dispatch, provided the value isn't an FnDef / ObjectType.
  r.pushDef('pi', newInteger(3n))
  r.pushDef('tau', newInteger(6n))
  r.pushDef('greeting', newString('hello'))
}

// ── Tokenizer ─────────────────────────────────────────────────────────────

function tokenize(s: string): Value[] {
  const out: Value[] = []
  let i = 0
  while (i < s.length) {
    while (i < s.length && (s[i] === ' ' || s[i] === '\t')) i++
    if (i >= s.length) break

    if (s[i] === '"') {
      let j = i + 1
      while (j < s.length && s[j] !== '"') j++
      if (j >= s.length) throw new Error(`unterminated string at ${i}`)
      out.push(newString(s.slice(i + 1, j)))
      i = j + 1
      continue
    }

    let j = i
    while (j < s.length && s[j] !== ' ' && s[j] !== '\t') j++
    const tok = s.slice(i, j)
    i = j

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
  return out
}

function newWordWithModifiers(name: string, forceStack: boolean, forceForward: boolean): Value {
  if (!forceStack && !forceForward) return newWord(name)
  const wi: WordInfo = { name }
  if (forceStack) wi.forceStack = true
  if (forceForward) wi.forceForward = true
  return new Value(TWord, wi)
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
