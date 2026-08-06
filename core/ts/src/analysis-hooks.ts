// AnalysisImpl — the check piece's hook table, owned by core.
//
// The TS twin of core/go/analysis_hooks.go. The core step loop must be able
// to model a dispatch's results under check mode, and to strip a program's
// inputs to carriers, WITHOUT naming a check symbol. Core declares the slots
// here with NAMED inactive defaults; the check piece replaces them at import
// time (eng/ts's check.ts calls installAnalysisImpl).
//
// The naming matters, per core/go/CLAUDE.md: "every slot has a NAMED inactive
// default pinned by a core-side test... an anonymous default replaced at init
// is unreachable and fails the merged ADR-008 gate." The same rule applies
// here — the defaults below are exported so a core-side test can call them
// directly, which is the only way they stay reachable once eng/ts installs the
// real implementations.
//
// Why these two and not more: core/go/analysis_hooks.go's table also carries
// FnConstructionPass, ReturnsFn, ZeroOutResiduals, MixedConform,
// ValueCarriesCarrier, AtUncaughtTopLevel and AddUnique. The TS port has not
// grown those behaviours yet (see check.ts's PARITY NOTE — fn-body analysis,
// the fixed-point step budget and disjunct partitioning are later phases), so
// adding empty slots for them would be inventing a surface neither side uses.
// They join this table when their behaviour lands.

import type { Value } from './value.ts'
import type { Signature } from './signature.ts'
import type { Registry } from './registry.ts'

/** The check-piece behaviours the core step loop offers work through. */
export interface AnalysisHooks {
  /**
   * Strip a program's input values to type-only carriers. The identity when
   * no check piece is linked.
   */
  stripToCarriers(input: Value[]): Value[]

  /**
   * Model a matched dispatch's results under check mode. Returns the carrier
   * values that stand in for the handler's real output. Empty when no check
   * piece is linked — a core-only build cannot meaningfully run analysis.
   */
  carrierResults(registry: Registry, word: string, sig: Signature, args: Value[]): Value[]
}

/** The NAMED inactive strip: analysis-less builds pass values through. */
export function inactiveStripToCarriers(input: Value[]): Value[] {
  return input
}

/** The NAMED inactive model: analysis-less builds produce no carriers. */
export function inactiveCarrierResults(
  _registry: Registry,
  _word: string,
  _sig: Signature,
  _args: Value[],
): Value[] {
  return []
}

/**
 * The live hook table. Mutable by design — the check piece installs over it at
 * import time, exactly as check/go's init replaces core/go's AnalysisImpl.
 */
export const AnalysisImpl: AnalysisHooks = {
  stripToCarriers: inactiveStripToCarriers,
  carrierResults: inactiveCarrierResults,
}

/**
 * installAnalysisImpl replaces the inactive defaults with the check piece's
 * real implementations. Called once, at check.ts import time.
 */
export function installAnalysisImpl(hooks: Partial<AnalysisHooks>): void {
  if (hooks.stripToCarriers !== undefined) AnalysisImpl.stripToCarriers = hooks.stripToCarriers
  if (hooks.carrierResults !== undefined) AnalysisImpl.carrierResults = hooks.carrierResults
}
