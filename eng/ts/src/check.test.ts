// Check-mode parity tests for the TypeScript engine. Runs the SAME
// corpus the Go engine-level check runner uses (eng/spec/check/*.tsv)
// and renders each row to the identical string (see renderCheck /
// test/go/engspec/engcheck_test.go::renderCheck). The TS engine must
// match the Go checker row-for-row.
//
// The fixtures here are a focused, return-annotated subset mirroring the
// engspec kernel fixtures the corpus exercises. In check mode the
// handlers are short-circuited (carrierResults synthesises the return
// carriers), so the handler bodies need only be present, not correct.
import { describe, it } from 'node:test'
import { strict as assert } from 'node:assert'
import * as fs from 'node:fs'
import * as path from 'node:path'
import { fileURLToPath } from 'node:url'

import {
  type CheckDiagnostic,
  Engine,
  type NativeFunc,
  Registry,
  TAny,
  TAtom,
  TBoolean,
  TInspect,
  TInteger,
  TList,
  TNumber,
  TScalar,
  TString,
  TType,
  Value,
  newCarrier,
  newFloat,
  newInteger,
  newList,
  newString,
  newTypeLiteral,
  newWord,
  typeNameTable,
} from './index.ts'

const __filename = fileURLToPath(import.meta.url)
const __dirname = path.dirname(__filename)
const SPEC_DIR = path.resolve(__dirname, '..', '..', 'spec', 'check')

const typeTable = typeNameTable()

// ── Fixtures ────────────────────────────────────────────────────────────
// Each word mirrors the engspec kernel fixture of the same name, but only
// the signature shape + return annotation matter (handlers are inert in
// check mode). A `stub` handler stands in for the runtime behaviour.
const stub = (): Value[] => []

function registerCheckWords(r: Registry): void {
  const reg = (fn: NativeFunc): void => r.registerNativeFunc(fn)
  const numberPair = [TNumber, TNumber]

  reg({ name: 'addq', forwardPrecedence: true, signatures: [{ args: numberPair, handler: stub, returns: [TNumber] }] })
  reg({ name: 'subq', forwardPrecedence: true, signatures: [{ args: numberPair, handler: stub, returns: [TNumber] }] })
  reg({ name: 'mulq', forwardPrecedence: true, signatures: [{ args: numberPair, handler: stub, returns: [TNumber] }] })
  reg({ name: 'negq', signatures: [{ args: [TNumber], barrierPos: 1, handler: stub, returns: [TNumber] }] })
  reg({
    name: 'concatq',
    forwardPrecedence: true,
    signatures: [{ args: [TString, TString], handler: stub, returns: [TString] }],
  })
  reg({ name: 'lengthq', forwardPrecedence: true, signatures: [{ args: [TList], handler: stub, returns: [TInteger] }] })
  reg({ name: 'firstq', forwardPrecedence: true, signatures: [{ args: [TList], handler: stub, returns: [TAny] }] })
  // pairq has a barrier at 1: sig[0] forward-eligible, sig[1] from stack.
  reg({ name: 'pairq', signatures: [{ args: [TInteger, TInteger], barrierPos: 1, handler: stub, returns: [TString] }] })
  reg({
    name: 'tripq',
    forwardPrecedence: true,
    signatures: [{ args: [TInteger, TInteger, TInteger], handler: stub, returns: [TString] }],
  })
  reg({ name: 'nilq', signatures: [{ args: [], barrierPos: 0, handler: stub, returns: [TString] }] })
  reg({
    name: 'is',
    forwardPrecedence: true,
    signatures: [{ args: [TAny, TAny], handler: stub, returns: [TBoolean] }],
  })
  reg({ name: 'typeof', forwardPrecedence: true, signatures: [{ args: [TAny], handler: stub, returns: [TType] }] })
  reg({
    name: 'quote',
    forwardPrecedence: true,
    signatures: [
      { args: [TAtom], quoteArgs: new Set([0]), handler: stub, returns: [TAtom] },
      { args: [TAny], noEvalArgs: new Set([0]), handler: stub, returns: [TAny] },
    ],
  })
  reg({
    name: 'inspect',
    forwardPrecedence: true,
    signatures: [
      { args: [TAtom], quoteArgs: new Set([0]), handler: stub, returns: [TInspect] },
      { args: [TAny], handler: stub, returns: [TInspect] },
    ],
  })
  reg({
    name: 'do',
    forwardPrecedence: true,
    signatures: [{ args: [TList], noEvalArgs: new Set([0]), handler: stub, returns: [TAny] }],
  })
  // make: the scalar coercion overload. ReturnsFreshInstance(0) — the
  // result carries the target type (args[0] is the type-literal target).
  reg({
    name: 'make',
    forwardPrecedence: true,
    signatures: [
      {
        args: [TScalar, TAny],
        typeArgs: new Set([0]),
        handler: stub,
        returnsFn: (args) => [newCarrier(args[0]!.vType)],
      },
    ],
  })
  // noretq: a word with NO return annotation — dispatch over it emits
  // missing_returns and yields a dynamic(Any) carrier.
  reg({ name: 'noretq', forwardPrecedence: true, signatures: [{ args: [TInteger], handler: stub }] })
}

// ── Minimal tokenizer ───────────────────────────────────────────────────
// Covers exactly what the check corpus uses: words, integers, single/
// double-quoted strings, type-name → type literal, `true`, `[ … ]` lists,
// and `( … )` paren groups (emitted as bare `(`/`)` word tokens the
// engine's paren handling consumes).
function tokenize(src: string): Value[] {
  const s = src
  let i = 0
  const readList = (): Value[] => {
    const out: Value[] = []
    for (;;) {
      while (i < s.length && (s[i] === ' ' || s[i] === '\t')) i++
      if (i >= s.length) throw new Error('unterminated list')
      if (s[i] === ']') {
        i++
        return out
      }
      out.push(readOne())
    }
  }
  const readOne = (): Value => {
    while (i < s.length && (s[i] === ' ' || s[i] === '\t')) i++
    const c = s[i]!
    if (c === "'" || c === '"') {
      let j = i + 1
      while (j < s.length && s[j] !== c) j++
      const str = newString(s.slice(i + 1, j))
      i = j + 1
      return str
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
    while (j < s.length && !' \t[]()'.includes(s[j]!)) j++
    const tok = s.slice(i, j)
    i = j
    return atomic(tok)
  }
  const out: Value[] = []
  while (i < s.length) {
    while (i < s.length && (s[i] === ' ' || s[i] === '\t')) i++
    if (i >= s.length) break
    out.push(readOne())
  }
  return out
}

function atomic(tok: string): Value {
  if (tok === 'true' || tok === 'false') return new Value(TBoolean, tok === 'true')
  if (/^-?\d+$/.test(tok)) return newInteger(BigInt(tok))
  if (/^-?\d+\.\d+$/.test(tok)) return newFloat(Number.parseFloat(tok))
  const t = typeTable.get(tok)
  if (t !== undefined) return newTypeLiteral(t)
  return newWord(tok)
}

// ── Render + run ─────────────────────────────────────────────────────────
// renderCheck must match test/go/engspec/engcheck_test.go::renderCheck
// byte-for-byte: `<stack> :: <diag>…` with `!`/`~`/`?` severity sigils.
function renderCheck(stack: Value[], diags: CheckDiagnostic[]): string {
  const parts = stack.map((v) => (v.dynamic ? `dynamic(${v.vType.leaf()})` : v.vType.leaf()))
  let out = parts.join(' ')
  if (diags.length === 0) return out
  const ds = diags.map((d) => {
    const sigil = d.severity === 'error' ? '!' : d.severity === 'warning' ? '~' : '?'
    return sigil + d.code
  })
  return (out + ' :: ' + ds.join(' ')).trim()
}

function runCheckRow(input: string): string {
  const r = new Registry()
  registerCheckWords(r)
  const done = r.check.begin()
  try {
    const out = new Engine(r).run(tokenize(input))
    r.check.emitUnusedDefDiagnostics()
    return renderCheck(out, r.check.diagnostics)
  } finally {
    done()
  }
}

interface Row {
  lineNum: number
  input: string
  expected: string
}

function parseSpec(file: string): Row[] {
  const text = fs.readFileSync(file, 'utf8')
  const rows: Row[] = []
  const lines = text.split('\n')
  for (let n = 0; n < lines.length; n++) {
    const line = lines[n]!.replace(/[ \t]+$/, '')
    if (line === '' || line.startsWith('#')) continue
    const parts = line.split('\t')
    if (parts.length < 2) continue
    rows.push({ lineNum: n + 1, input: parts[0]!.trim(), expected: parts[1]!.trim() })
  }
  return rows
}

const specFiles = fs
  .readdirSync(SPEC_DIR)
  .filter((f) => f.endsWith('.tsv'))
  .sort()

describe('check', () => {
  for (const file of specFiles) {
    describe(file.replace(/\.tsv$/, ''), () => {
      for (const row of parseSpec(path.join(SPEC_DIR, file))) {
        it(`L${row.lineNum} ${row.input}`, () => {
          if (row.expected.startsWith('ERROR:')) {
            const want = row.expected.slice('ERROR:'.length)
            assert.throws(() => runCheckRow(row.input), (e: Error) =>
              want === '' || e.message.toLowerCase().includes(want.toLowerCase()),
            )
            return
          }
          assert.equal(runCheckRow(row.input), row.expected)
        })
      }
    })
  }
})
