// Engine.run is the interpreter loop. It walks left-to-right through
// the input values, dispatching words via signature matching and
// passing literals through.
//
// PARITY GAP: the Go engine has step budgets, check-mode, trace,
// loop break/continue handling, paren pre-evaluation, mark/move
// continuation, args-stack, context-store push/pop, interpolated
// strings, parser-eval lists, and module sub-engines. The TS port
// here is the interpreter slice that the current TSV specs reach.
import { AqlError } from './error.js'
import { matchEntry } from './match.js'
import type { Registry } from './registry.js'
import {
  TBoolean,
  TInteger,
  TString,
  typeNameTable,
} from './type.js'
import {
  newBoolean,
  newTypeLiteral,
  Value,
  type WordInfo,
} from './value.js'

const STEP_LIMIT = 22222

export class Engine {
  readonly registry: Registry
  private stack: Value[] = []
  private pointer = 0

  constructor(registry: Registry) {
    this.registry = registry
  }

  /**
   * Run the input value sequence. Returns the residual stack.
   * Throws AqlError on undefined word, signature mismatch, etc.
   */
  run(input: Value[]): Value[] {
    this.stack = [...input]
    this.pointer = 0

    for (let step = 0; step < STEP_LIMIT; step++) {
      if (this.pointer >= this.stack.length) break

      const val = this.stack[this.pointer]!
      if (val.isWord()) {
        this.stepWord(val)
      } else {
        this.pointer++
      }
    }

    return this.stack
  }

  private stepWord(val: Value): void {
    const w = val.asWord() as WordInfo
    const name = w.name

    // Built-in keywords.
    if (name === 'true') {
      this.stack[this.pointer] = newBoolean(true)
      return
    }
    if (name === 'false') {
      this.stack[this.pointer] = newBoolean(false)
      return
    }
    const tn = typeNameTable().get(name)
    if (tn !== undefined) {
      this.stack[this.pointer] = newTypeLiteral(tn)
      return
    }

    // Simple-value def substitution.
    const top = this.registry.topOfDefStack(name)
    if (top !== undefined) {
      this.stack[this.pointer] = top
      // Don't advance: let the value go through literal-handling on
      // the next loop iteration so a pending forward can pick it up.
      return
    }

    const fn = this.registry.lookup(name)
    if (!fn) {
      throw new AqlError('undefined_word', `undefined word: ${name}`, name)
    }

    const result = matchEntry(fn, this.stack, this.pointer)
    if (!result) {
      throw new AqlError(
        'signature_error',
        `no matching signature for ${name}\n  = expected: ${name} (${describeExpected(fn)})\n  = stack: ${this.describeStack()}`,
        name,
      )
    }

    // Execute handler (always synchronous in this port).
    const handlerResult = result.sig.handler(result.args, null, [], this.registry)
    if (handlerResult instanceof Promise) {
      throw new AqlError(
        'unsupported',
        `async handlers are not supported in the TS port`,
        name,
      )
    }
    const out = handlerResult as Value[]

    // Splice: replace [prefix...word...forward] with handler output.
    const replaceFrom = this.pointer - result.prefixCount
    const replaceCount = result.prefixCount + 1 + result.forwardCount
    this.stack.splice(replaceFrom, replaceCount, ...out)
    this.pointer = replaceFrom + out.length
  }

  private describeStack(): string {
    return this.stack
      .map((v, i) => (i === this.pointer ? `>>>${v.toString()}<<<` : v.toString()))
      .join(' ')
  }
}

function describeExpected(fn: import('./registry.js').FunctionEntry): string {
  // Pick the first non-fallback signature for the error message.
  const sig = fn.signatures.find((s) => !s.fallback)
  if (!sig) return ''
  return sig.args.map((t) => t.toString()).join(', ')
}

// Suppress "imported but unused" in stricter setups where these are
// referenced only in match.ts. They're re-exported here for users
// that want a single import point.
export { TBoolean, TInteger, TString }
