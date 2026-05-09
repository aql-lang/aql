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
  TAny,
  TAtom,
  TBoolean,
  TInteger,
  TList,
  TString,
  TWord,
  typeNameTable,
} from './type.ts'
import {
  type FnDefInfo,
  type ForwardMarker,
  newBoolean,
  newForwardMarker,
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
        if (w.name === 'end') {
          this.stepEnd()
          continue
        }
        // If a pending forward marker is waiting for a Word-typed
        // arg, capture this word as data instead of executing it.
        // Mirrors aqleng/go/engine.go's hasPendingForwardExpectingWord
        // check at the top of stepWord — without this the engine would
        // dispatch the word and prematurely consume its forward args.
        if (this.pendingExpectsWord()) {
          this.stepLiteral()
          continue
        }
        this.stepWord(val)
      } else if (val.isForward()) {
        // Forward markers are passive — the pointer just walks past
        // them. They consume incoming literals via stepLiteral.
        this.pointer++
      } else {
        this.stepLiteral()
      }
    }

    // End-of-run drain: any residual eval-list on the stack runs its
    // contents and is replaced by the residual sub-stack as a list.
    this.autoEvalStack()

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
      if (top.vType.matches(TList) && top.isConcrete() && !top.quoted) {
        // Unquoted list → code body: splice its elements at the
        // pointer so they execute inline. A quoted list (set via
        // `quote`) is data and falls through to the literal-substitute
        // branch below. Mirrors aqleng/go/engine.go's def-sub
        // `!top.Quoted` check.
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

    // If the match has unresolved forward args (or any forward arg
    // that is still a Word that might be a function call), defer the
    // dispatch by replacing the function word with a ForwardMarker.
    // The engine then steps the original forward args; their values
    // (or, for sub-call results) flow back into the marker via
    // stepLiteral until the marker fires. Mirrors insertForward in
    // aqleng/go/engine.go.
    if (this.shouldDeferDispatch(result.args, result.forwardCount, result.sig)) {
      this.beginForward(name, result, fn)
      return
    }

    this.dispatch(result, name)
  }

  /**
   * Run a fully-resolved match: auto-evaluate any list args whose
   * sig position isn't NoEvalArgs-marked, then execute the handler
   * and splice the result over the consumed prefix + word + forward
   * range.
   */
  private dispatch(
    result: import('./match.ts').MatchResult,
    name: string,
  ): void {
    this.autoEvalArgs(result.args, result.sig)
    const handlerResult = result.sig.handler(result.args, null, [], this.registry)
    if (handlerResult instanceof Promise) {
      throw new AqlError(
        'unsupported',
        `async handlers are not supported in the TS port`,
        name,
      )
    }
    const out = handlerResult as Value[]

    const replaceFrom = this.pointer - result.prefixCount
    const replaceCount = result.prefixCount + 1 + result.forwardCount
    this.stack.splice(replaceFrom, replaceCount, ...out)
    // Position the pointer at the first result (or, for empty
    // outputs, whatever the splice left at this index). The next
    // iteration re-processes that slot — letting a pending outer
    // forward marker collect a Value-typed result via stepLiteral,
    // or just advancing past an immediate-dispatch result.
    this.pointer = replaceFrom
  }

  /**
   * Decide whether the match needs deferred forward collection. We
   * defer when an optimistically-matched forward arg is still a Word
   * that the engine will want to dispatch (e.g. a TAny slot grabbed
   * a function-name Word). When the matcher accepted Word-as-data
   * for a TWord/TAtom slot, we keep the immediate dispatch — those
   * slots intentionally capture names without executing them. Same
   * for /q-style slots (not yet ported); they'd be checked here too.
   */
  private shouldDeferDispatch(
    args: Value[],
    forwardCount: number,
    sig: Signature,
  ): boolean {
    for (let i = 0; i < forwardCount; i++) {
      const a = args[i]!
      if (!a.isWord()) continue
      const expected = sig.args[i]!
      // TWord/TAtom slots: the matcher kept the Word as data on
      // purpose; never defer here even if the name happens to match
      // a registered function (e.g. `quote dup`).
      if (expected.equal(TWord) || expected.equal(TAtom)) continue
      const w = a.asWord() as WordInfo
      if (this.registry.lookup(w.name)) return true
    }
    return false
  }

  /**
   * Insert a ForwardMarker in place of the function word and
   * advance past it. The optimistically-matched forward args remain
   * at their original positions on the stack; the engine will step
   * them as usual, and their post-evaluation values flow back into
   * the marker via stepLiteral.
   */
  private beginForward(
    name: string,
    result: import('./match.ts').MatchResult,
    _fn: FunctionEntry,
  ): void {
    // Stack args (sig positions [forwardCount..N-1]) stay where they
    // are below the pointer; we capture them in sig order for the
    // final dispatch.
    const stackArgs = result.args.slice(result.forwardCount)
    const marker: ForwardMarker = {
      funcName: name,
      sig: result.sig,
      expectedForward: result.forwardCount,
      collected: [],
      stackArgs,
    }
    // Replace the function word with the marker. Pointer stays —
    // the main loop's "isForward" branch will advance past it.
    this.stack[this.pointer] = newForwardMarker(marker)
    // Stack args (resolved before the pointer) need to be removed
    // here so they're not double-counted at completion time. Mirror
    // Go's insertForward which records prefix args separately.
    const replaceFrom = this.pointer - result.prefixCount
    this.stack.splice(replaceFrom, result.prefixCount)
    this.pointer = replaceFrom
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

  /**
   * Return true iff the nearest preceding ForwardMarker (within the
   * current paren scope) is waiting for a Word-typed arg at its
   * next collection slot. Mirrors aqleng/go/engine.go's
   * hasPendingForwardExpectingWord — the gate that lets `def NAME`
   * capture NAME as data even when it would otherwise dispatch.
   */
  private pendingExpectsWord(): boolean {
    const idx = this.findPendingMarker()
    if (idx < 0) return false
    const m = this.stack[idx]!.asForward()
    const nextIdx = m.collected.length
    if (nextIdx >= m.sig.args.length) return false
    const expected = m.sig.args[nextIdx]!
    // Only an EXPLICIT TWord/TAtom slot suppresses dispatch. TAny
    // also accepts Word values, but at TAny slots we still want the
    // engine to dispatch the word and feed its result back. Mirrors
    // aqleng/go/engine.go's hasPendingForwardExpectingWord which
    // checks `Equal(TWord)` and the /q flag, never TAny.matches(TWord).
    return expected.equal(TWord) || expected.equal(TAtom)
  }

  /**
   * Walk backward from the pointer, stopping at open-paren markers,
   * and return the index of the nearest unfilled ForwardMarker. -1
   * if none.
   */
  private findPendingMarker(): number {
    for (let i = this.pointer - 1; i >= 0; i--) {
      const v = this.stack[i]!
      if (v.isWord() && (v.asWord() as WordInfo).name === '(') return -1
      if (v.isForward()) {
        const m = v.asForward()
        if (m.collected.length < m.expectedForward) return i
        return -1
      }
    }
    return -1
  }

  /**
   * Handle a literal (or word-treated-as-literal) at the pointer.
   * If a pending marker can absorb it, collect; otherwise advance.
   */
  private stepLiteral(): void {
    const fwdIdx = this.findPendingMarker()
    if (fwdIdx < 0) {
      this.pointer++
      return
    }
    const m = this.stack[fwdIdx]!.asForward()
    const nextIdx = m.collected.length
    const expected = m.sig.args[nextIdx]!
    const val = this.stack[this.pointer]!

    // Type-check. Words at TWord/TAtom slots match directly; other
    // values check via sigTypeMatches.
    let matches: boolean
    if (val.isWord()) {
      // A Word can fill a TWord/TAtom slot, or a TAny slot (data).
      // For other slot types it must resolve via def-sub first; that
      // happens at stepWord, so reaching here with a Word means
      // either we want it as data (TWord/TAtom/TAny) or it's a
      // mismatch.
      matches = expected.equal(TWord) || expected.equal(TAtom) || expected.equal(TAny)
    } else {
      matches = val.vType.matches(expected)
    }
    if (!matches) {
      // Implicit end of forward collection — fail the dispatch.
      throw new AqlError(
        'signature_error',
        `forward arg ${nextIdx} type mismatch for ${m.funcName}: expected ${expected.toString()}, got ${val.toString()}`,
        m.funcName,
      )
    }

    // Collect: remove value from current position, append to marker.
    this.stack.splice(this.pointer, 1)
    m.collected.push(val)
    // pointer stays — it now points at what came after the value.

    if (m.collected.length === m.expectedForward) {
      this.completeForward(fwdIdx)
    }
  }

  /**
   * Handle the `end` keyword. If a forward marker is pending in the
   * current paren scope, complete it with whatever's been collected
   * so far (forward args + stack args from the original match) — the
   * collection short-circuits before all expected slots arrive.
   * Otherwise just remove the `end` token. Mirrors stepEnd in
   * aqleng/go/engine.go.
   */
  private stepEnd(): void {
    const fwdIdx = this.findPendingMarker()
    if (fwdIdx < 0) {
      // No pending forward — just drop the end token.
      this.stack.splice(this.pointer, 1)
      return
    }
    // Drop the end token first so the marker is no longer "pending"
    // when completeForward runs.
    this.stack.splice(this.pointer, 1)
    this.completeForwardPartial(fwdIdx)
  }

  /**
   * Variant of completeForward that fires a marker with fewer
   * collected forward args than expected. Used by stepEnd. The args
   * list is built from whatever's collected so far (any unfilled
   * forward slots stay missing, which the handler must tolerate).
   * For the spec subset this is rarely meaningful, but the path
   * keeps shape parity with Go's stepEnd → implicitEnd flow.
   */
  private completeForwardPartial(fwdIdx: number): void {
    const m = this.stack[fwdIdx]!.asForward()
    const args = [...m.collected, ...m.stackArgs]
    if (args.length < m.sig.args.length) {
      throw new AqlError(
        'signature_error',
        `${m.funcName}: 'end' before all forward args collected (have ${args.length}, need ${m.sig.args.length})`,
        m.funcName,
      )
    }
    this.fireMarker(fwdIdx, m, args)
  }

  /**
   * Build the full args list (forward then stack, in sig order) and
   * dispatch the marker's sig. The marker is replaced by the result.
   */
  private completeForward(fwdIdx: number): void {
    const m = this.stack[fwdIdx]!.asForward()
    const args = [...m.collected, ...m.stackArgs]
    this.fireMarker(fwdIdx, m, args)
  }

  /** Run the marker's handler with `args` and replace it with the result. */
  private fireMarker(fwdIdx: number, m: ForwardMarker, args: Value[]): void {
    this.autoEvalArgs(args, m.sig)
    const handlerResult = m.sig.handler(args, null, [], this.registry)
    if (handlerResult instanceof Promise) {
      throw new AqlError(
        'unsupported',
        `async handlers are not supported in the TS port`,
        m.funcName,
      )
    }
    const out = handlerResult as Value[]
    this.stack.splice(fwdIdx, 1, ...out)
    this.pointer = fwdIdx
  }

  /**
   * Auto-evaluate any TList args carrying `eval=true && !quoted`,
   * unless the sig declares NoEvalArgs for that position. Mirrors
   * aqleng/go/engine.go's pre-handler autoEvalList step in execMatch.
   * Mutates `args` in place.
   */
  private autoEvalArgs(args: Value[], sig: Signature): void {
    for (let i = 0; i < args.length; i++) {
      const a = args[i]!
      if (sig.noEvalArgs?.has(i)) continue
      if (!a.vType.matches(TList)) continue
      if (!a.eval || a.quoted) continue
      if (!a.isConcrete()) continue
      args[i] = this.autoEvalList(a)
    }
  }

  /**
   * Auto-evaluate a single TList: run a fresh sub-engine on its
   * elements and wrap the residual stack as a new (non-eval) list.
   * The result is data — clearing eval=true ensures it doesn't
   * recursively re-evaluate when consumed by an outer caller.
   */
  private autoEvalList(list: Value): Value {
    const elems = list.asList()
    const sub = new Engine(this.registry)
    const result = sub.run([...elems])
    return new Value(list.vType, result, { eval: false, quoted: false })
  }

  /**
   * End-of-Run pass: any TList still on the stack with eval=true and
   * !quoted gets auto-evaluated. Mirrors Go's autoEvalStack drain.
   */
  private autoEvalStack(): void {
    for (let i = 0; i < this.stack.length; i++) {
      const v = this.stack[i]!
      if (!v.vType.matches(TList)) continue
      if (!v.eval || v.quoted) continue
      if (!v.isConcrete()) continue
      this.stack[i] = this.autoEvalList(v)
    }
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
