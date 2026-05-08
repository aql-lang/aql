// Engine.run is the interpreter loop. It walks left-to-right through
// the input values, dispatching words via signature matching, passing
// literals through, and pre-evaluating paren groups so a function
// word's forward scan sees fully-resolved values.
//
// PARITY GAP: the Go engine has step budgets, check-mode, trace,
// loop break/continue handling, mark/move continuation, args-stack,
// context-store push/pop, interpolated strings, parser-eval lists,
// and module sub-engines. The TS port here is the interpreter slice
// that the current TSV specs reach.
import { AqlError } from './error.ts'
import { matchEntry } from './match.ts'
import type { Registry } from './registry.ts'
import {
  TBoolean,
  TInteger,
  TList,
  TString,
  TWord,
  typeNameTable,
} from './type.ts'
import {
  type FnDefInfo,
  newBoolean,
  newTypeLiteral,
  Value,
  type WordInfo,
} from './value.ts'
import type { FunctionEntry } from './registry.ts'
import type { Signature } from './signature.ts'

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
        const w = val.asWord() as WordInfo
        if (w.name === '(') {
          this.evalParenAt(this.pointer)
          continue
        }
        if (w.name === ')') {
          throw new AqlError('syntax_error', `unmatched ')'`, ')')
        }
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

    // Def-stack substitution. Three paths:
    //   1. FnDef (typed-param fn): match args against the synthesised
    //      sig from params, bind each param on the def stack, run the
    //      body in a sub-engine, splice the result back in place of
    //      the consumed word + prefix + forward args.
    //   2. List (code body): splice its elements at the pointer to
    //      execute inline against the current stack.
    //   3. Anything else (simple value): replace the word with the
    //      value and let the next iteration pick it up as a literal.
    const top = this.registry.topOfDefStack(name)
    if (top !== undefined) {
      if (top.isFnDef()) {
        this.dispatchFnDef(name, top.asFnDef())
        return
      }
      if (top.vType.matches(TList) && top.isConcrete()) {
        const elems = top.asList()
        this.stack.splice(this.pointer, 1, ...elems)
        // pointer stays — first body token executes next iteration.
        return
      }
      this.stack[this.pointer] = top
      // Don't advance: let the value go through literal-handling on
      // the next loop iteration so a pending forward can pick it up.
      return
    }

    const fn = this.registry.lookup(name)
    if (!fn) {
      throw new AqlError('undefined_word', `undefined word: ${name}`, name)
    }

    // Pre-evaluate any paren groups in the forward window so the
    // matcher sees concrete values. The window is bounded by the
    // function's largest forward-eligible arg count across all sigs.
    this.preEvalParens(fn.maxForwardArgs)

    const result = matchEntry(fn, this.stack, this.pointer, this.registry)
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

  /**
   * Evaluate the paren group whose `(` sits at `idx`. Finds the
   * matching `)` (accounting for nesting), runs a sub-engine on the
   * contents, and splices the results back in place of the
   * `(...)`. The pointer is positioned at the first result so the
   * outer interpreter can pick it up on the next iteration.
   */
  private evalParenAt(idx: number): void {
    const closeIdx = this.findMatchingClose(idx)
    if (closeIdx < 0) {
      throw new AqlError('syntax_error', `unmatched '('`, '(')
    }
    const inner = this.stack.slice(idx + 1, closeIdx)
    const sub = new Engine(this.registry)
    const results = sub.run(inner)
    this.stack.splice(idx, closeIdx - idx + 1, ...results)
    // Don't advance the pointer; the next iteration will process the
    // first spliced result.
  }

  /**
   * Find the index of the `)` that closes the `(` at `openIdx`.
   * Returns -1 if no matching close is found. Tracks paren depth so
   * nested groups don't break out prematurely.
   */
  private findMatchingClose(openIdx: number): number {
    let depth = 1
    for (let i = openIdx + 1; i < this.stack.length; i++) {
      const v = this.stack[i]!
      if (!v.isWord()) continue
      const name = (v.asWord() as WordInfo).name
      if (name === '(') depth++
      else if (name === ')') {
        depth--
        if (depth === 0) return i
      }
    }
    return -1
  }

  /**
   * Pre-evaluate paren groups in the forward window so matchSignature
   * sees concrete values. Scans from pointer+1 forward; for each `(`
   * encountered within the window, evaluates that paren in-place
   * (which splices its results back into the main stack). Stops at a
   * structural boundary or a registered function word.
   *
   * `maxFwd` is the upper bound on how many forward values we might
   * need to resolve — taken from FunctionEntry.maxForwardArgs.
   */
  private preEvalParens(maxFwd: number): void {
    if (maxFwd <= 0) return
    let resolved = 0
    let scanIdx = this.pointer + 1
    let guard = 0
    while (resolved < maxFwd && scanIdx < this.stack.length && guard < 2222) {
      guard++
      const tok = this.stack[scanIdx]!
      if (!tok.isWord()) {
        resolved++
        scanIdx++
        continue
      }
      const name = (tok.asWord() as WordInfo).name
      if (name === ')' || name === 'end') break
      if (name === '(') {
        const before = this.stack.length
        this.evalParenAt(scanIdx)
        const produced = this.stack.length - (before - 1) // closeIdx - openIdx + 1 was removed; results were inserted
        // After the splice the first result is at `scanIdx`. Each
        // produced value counts toward the resolved budget. If the
        // paren produced zero values, advance scanIdx to skip the
        // (now-empty) slot — but since we removed (openIdx..closeIdx)
        // and inserted N values, scanIdx still points at the first
        // result (or past it if N==0).
        if (produced <= 0) continue
        resolved += produced
        scanIdx += produced
        continue
      }
      // A registered function word in the forward window is a
      // boundary — leave it for the outer matcher to either consume
      // (e.g. if the sig accepts TWord) or reject. Simple-def words
      // count as one resolved value.
      if (this.registry.lookup(name)) break
      resolved++
      scanIdx++
    }
  }

  /**
   * Dispatch a fn-typed def at the pointer. Synthesises a Signature
   * from the FnDef's params (one overload, all-forward-eligible),
   * matches it against the current stack/forward window, binds each
   * param onto the def stack, runs the body in a sub-engine, then
   * splices the result back in place of the consumed prefix + word
   * + forward args. The bound params are popped after the sub-engine
   * returns so they don't leak into the surrounding scope.
   *
   * Mirrors aqleng/go/engine.go's FnDef dispatch path (the "stepWord
   * → execFnDefSig" arc) compressed into a single function.
   */
  private dispatchFnDef(name: string, info: FnDefInfo): void {
    // Pre-evaluate forward parens so the matcher sees concrete values.
    this.preEvalParens(info.params.length)

    const sig: Signature = {
      args: info.params.map((p) => p.type),
      barrierPos: info.params.length,
      handler: () => [],
    }
    const fakeEntry: FunctionEntry = {
      name,
      signatures: [sig],
      forwardPrecedence: true,
      maxForwardArgs: info.params.length,
    }
    const result = matchEntry(fakeEntry, this.stack, this.pointer, this.registry)
    if (!result) {
      throw new AqlError(
        'signature_error',
        `no matching signature for ${name}\n  = expected: ${name} (${describeExpected(fakeEntry)})\n  = stack: ${this.describeStack()}`,
        name,
      )
    }

    // Bind each param on the def stack so the body can reference it.
    for (let i = 0; i < info.params.length; i++) {
      this.registry.pushDef(info.params[i]!.name, result.args[i]!)
    }

    let bodyResult: Value[]
    try {
      const sub = new Engine(this.registry)
      bodyResult = sub.run([...info.body])
    } finally {
      // Pop in reverse to keep the def stack consistent with multiple
      // params sharing a name (rare but possible).
      for (let i = info.params.length - 1; i >= 0; i--) {
        this.registry.popDef(info.params[i]!.name)
      }
    }

    // Splice the result over the consumed range (prefix + word +
    // forward args), exactly like a native handler.
    const replaceFrom = this.pointer - result.prefixCount
    const replaceCount = result.prefixCount + 1 + result.forwardCount
    this.stack.splice(replaceFrom, replaceCount, ...bodyResult)
    this.pointer = replaceFrom + bodyResult.length
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
export { TBoolean, TInteger, TString, TWord, Value }
