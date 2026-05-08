// Registry holds per-name function tables, def stacks, and the
// host-installed capability slot.
//
// PARITY NOTE: the Go Registry has many fields the spec subset
// doesn't exercise (NativeModResolver, ModuleInitFunc, ParseFunc,
// SDKCache, Manager, BaseDir, OnRegisterHook, Check state, args
// stack). Those are intentionally omitted here — adding them would
// inflate the port without helping the specs run. A full-parity
// version would mirror them one-for-one.
import type { NativeFunc, NativeSig, Signature } from './signature.ts'
import { sortSignatures } from './signature.ts'
import type { Value } from './value.ts'

/** A registered word with its overloaded signatures + dispatch flags. */
export interface FunctionEntry {
  name: string
  signatures: Signature[]
  forwardPrecedence: boolean
  /** Longest forward arg count across all sigs (used by stepWord). */
  maxForwardArgs: number
}

export class Registry {
  private functions = new Map<string, FunctionEntry>()
  private defStacks = new Map<string, Value[]>()
  private capabilities = new Map<string, unknown>()

  // ── Capabilities ──────────────────────────────────────────────────────

  /** Returns true iff a capability is registered under `name`. */
  hasCapability(name: string): boolean {
    return this.capabilities.has(name)
  }

  /**
   * Look up the value stored under `name`. Returns a tuple analogous
   * to Go's (value, ok) so a stored null/undefined value is
   * distinguishable from "no capability registered".
   */
  capability(name: string): [unknown, true] | [undefined, false] {
    if (!this.capabilities.has(name)) return [undefined, false]
    return [this.capabilities.get(name), true]
  }

  /**
   * Install or replace the capability value under `name`.
   *
   * Storing `null` or `undefined` is a real STORE — the capability is
   * present and its value is null/undefined. Use deleteCapability to
   * remove an entry. The previous version conflated the two; passing
   * a possibly-undefined argument silently became a delete instead of
   * an "install undefined" call.
   */
  setCapability(name: string, value: unknown): void {
    this.capabilities.set(name, value)
  }

  /** Remove the capability for `name`. Returns true if one was present. */
  deleteCapability(name: string): boolean {
    return this.capabilities.delete(name)
  }

  capabilityNames(): string[] {
    return [...this.capabilities.keys()]
  }

  // ── Function table ────────────────────────────────────────────────────

  /** Look up a registered word entry, or undefined if not registered. */
  lookup(name: string): FunctionEntry | undefined {
    return this.functions.get(name)
  }

  /** Register a NativeFunc — converts NativeSig → Signature and stores under name. */
  registerNativeFunc(fn: NativeFunc): void {
    const entry = this.upsert(fn.name, fn.forwardPrecedence ?? false)
    for (const ns of fn.signatures) {
      entry.signatures.push(this.toSignature(ns))
    }
    sortSignatures(entry.signatures)
    entry.maxForwardArgs = computeMaxForward(entry, fn.forwardPrecedence ?? false)
  }

  private upsert(name: string, forward: boolean): FunctionEntry {
    let entry = this.functions.get(name)
    if (!entry) {
      entry = { name, signatures: [], forwardPrecedence: forward, maxForwardArgs: 0 }
      this.functions.set(name, entry)
    } else {
      // Forward-precedence sticks once set.
      entry.forwardPrecedence = entry.forwardPrecedence || forward
    }
    return entry
  }

  private toSignature(ns: NativeSig): Signature {
    return {
      args: ns.args,
      handler: ns.handler,
      noEvalArgs: ns.noEvalArgs,
      fallback: ns.fallback,
    }
  }

  // ── Def stack ─────────────────────────────────────────────────────────
  pushDef(name: string, v: Value): void {
    let stack = this.defStacks.get(name)
    if (!stack) {
      stack = []
      this.defStacks.set(name, stack)
    }
    stack.push(v)
  }

  popDef(name: string): boolean {
    const stack = this.defStacks.get(name)
    if (!stack || stack.length === 0) return false
    stack.pop()
    if (stack.length === 0) this.defStacks.delete(name)
    return true
  }

  topOfDefStack(name: string): Value | undefined {
    const stack = this.defStacks.get(name)
    if (!stack || stack.length === 0) return undefined
    return stack[stack.length - 1]
  }

  hasDef(name: string): boolean {
    const stack = this.defStacks.get(name)
    return !!stack && stack.length > 0
  }

  defStackDepth(name: string): number {
    const stack = this.defStacks.get(name)
    return stack ? stack.length : 0
  }

  defNames(): string[] {
    return [...this.defStacks.keys()]
  }
}

function computeMaxForward(entry: FunctionEntry, forwardPrecedence: boolean): number {
  if (!forwardPrecedence) return 0
  let max = 0
  for (const sig of entry.signatures) {
    if (sig.args.length > max) max = sig.args.length
  }
  return max
}
