// CheckState — the per-pass static-analysis state bundle, owned by core.
//
// The TS twin of core/go/check_state.go. The STATE lives in core (the
// Registry hangs one off itself and the step loop reads `mode` on every
// dispatch); the analysis LOGIC — toCarrier, stripToCarriers, carrierResults —
// lives in the check piece and reaches core through AnalysisImpl. That split
// is core/go's, not an invention: core/go/check_state.go owns CheckState,
// CheckDiagnostic, checkCodeSeverity and DefaultCheckStepBudget, while
// check/go/carrier.go owns toCarrier / StripToCarriers / CarrierResults.
//
// PARITY NOTE (inherited from the eng/ts port this was cut from): this is the
// Phase-1 subset. Fn-body analysis, the step budget's fixed-point machinery
// and disjunct partitioning are later phases; core/go's CheckState carries
// more fields than this one does.

import type { EmitRecorder } from './emit-recorder.ts'

/** Severity classes for a diagnostic, mirroring eng CheckSeverity. */
export type CheckSeverity = 'error' | 'warning' | 'info'

/** One static-analysis finding. Mirrors eng CheckDiagnostic. */
export interface CheckDiagnostic {
  code: string
  detail: string
  word?: string
  severity: CheckSeverity
  /** Emitted during fn-body analysis (call-time code) — see forward-ref rescue. */
  fnBody?: boolean
}

// checkCodeSeverity maps a diagnostic code to its default severity,
// mirroring eng/go/registry.go::checkCodeSeverity.
const CHECK_CODE_SEVERITY: Record<string, CheckSeverity> = {
  no_signature: 'error',
  undefined_word: 'error',
  fn_body_error: 'error',
  branch_error: 'error',
  type_error: 'error',
  uncalled_function: 'error',
  constraint_violation: 'error',
  unbound_param: 'error',
  arity_mismatch: 'error',
  unreachable_signature: 'warning',
  partial_dispatch: 'warning',
  index_out_of_range: 'warning',
  missing_returns: 'warning',
  step_budget_exceeded: 'warning',
  body_error: 'warning',
  static_warning: 'warning',
  analysis_truncated: 'info',
  forward_strands_operand: 'info',
  mixed_form_call: 'info',
  speculative_forward_commit: 'info',
}

/** Default severity for a diagnostic code (info when unmapped). */
export function severityFor(code: string): CheckSeverity {
  return CHECK_CODE_SEVERITY[code] ?? 'info'
}

/** DefaultCheckStepBudget caps total check-mode steps across sub-engines. */
export const DEFAULT_CHECK_STEP_BUDGET = 500_000

/**
 * CheckState is the per-pass static-analysis state bundle, mirroring
 * eng/go/registry.go::CheckState (Phase-1 subset). It hangs off the
 * Registry; handlers consult `mode` to suppress side effects and the
 * engine routes dispatch through carrierResults while it is active.
 */
export class CheckState {
  mode = false
  diagnostics: CheckDiagnostic[] = []
  stepCount = 0
  /** -1 is the "unset" sentinel resolved to DEFAULT_CHECK_STEP_BUDGET. */
  stepBudget = -1
  budgetTripped = false
  /** Names installed via def during the pass (→ unused_def warnings). */
  defsInstalled = new Map<string, void>()
  /** Names referenced during the pass. */
  defsUsed = new Set<string>()
  /** Nesting depth of fn-body analysis (diagnostics tagged fnBody when > 0). */
  fnBodyDepth = 0
  /**
   * Keys (name#arity) of fn-body analyses currently running, so a
   * recursive self-call breaks the cycle instead of looping. Mirrors
   * CheckState.FnInflight (Phase-2 subset — no memoized summaries /
   * fixed-point refinement yet).
   */
  fnInflight = new Set<string>()

  /**
   * When > 0, error-level DISPATCH diagnostics (no_signature /
   * undefined_word) are dropped: the enclosing analysis runs a body under
   * args a real match already REJECTED (the best-fit recovery), so its
   * dispatch failures are cascade noise — the one honest diagnostic is the
   * recovery's own no_signature. Mirrors the Go engine's
   * SuppressBodyErrors discipline.
   */
  suppressBodyErrors = 0
  /**
   * When set, the check pass doubles as the bytecode recording pass:
   * each native dispatch is offered to emit.recordCall. Null for a plain
   * check. Mirrors CheckState.Emit. Recording is passive — it never
   * changes the carriers returned.
   */
  emit: EmitRecorder | undefined

  /** Deep copy the mutable analysis state for speculative passes. */
  clone(): CheckState {
    const cp = new CheckState()
    cp.mode = this.mode
    cp.diagnostics = this.diagnostics.map((d) => ({ ...d }))
    cp.stepCount = this.stepCount
    cp.stepBudget = this.stepBudget
    cp.budgetTripped = this.budgetTripped
    cp.defsInstalled = new Map(this.defsInstalled)
    cp.defsUsed = new Set(this.defsUsed)
    cp.fnBodyDepth = this.fnBodyDepth
    cp.fnInflight = new Set(this.fnInflight)
    cp.emit = this.emit
    return cp
  }

  /** Restore this object in place from a clone. */
  restoreFrom(snapshot: CheckState): void {
    this.mode = snapshot.mode
    this.diagnostics = snapshot.diagnostics.map((d) => ({ ...d }))
    this.stepCount = snapshot.stepCount
    this.stepBudget = snapshot.stepBudget
    this.budgetTripped = snapshot.budgetTripped
    this.defsInstalled = new Map(snapshot.defsInstalled)
    this.defsUsed = new Set(snapshot.defsUsed)
    this.fnBodyDepth = snapshot.fnBodyDepth
    this.fnInflight = new Set(snapshot.fnInflight)
    this.emit = snapshot.emit
  }

  /** Whether check mode is currently on. */
  isActive(): boolean {
    return this.mode
  }

  /** Enable check mode and reset per-pass state. Returns a disable fn. */
  begin(): () => void {
    this.mode = true
    this.diagnostics = []
    this.stepCount = 0
    this.budgetTripped = false
    this.defsInstalled = new Map()
    this.defsUsed = new Set()
    this.fnBodyDepth = 0
    this.fnInflight = new Set()
    return () => {
      this.mode = false
    }
  }

  /** Append a diagnostic, defaulting severity from its code. */
  addDiagnostic(d: Omit<CheckDiagnostic, 'severity'> & { severity?: CheckSeverity }): void {
    const diag: CheckDiagnostic = {
      ...d,
      severity: d.severity ?? severityFor(d.code),
    }
    if (
      this.suppressBodyErrors > 0 &&
      diag.severity === 'error' &&
      (diag.code === 'no_signature' || diag.code === 'undefined_word')
    ) {
      return
    }
    if (this.fnBodyDepth > 0) diag.fnBody = true
    this.diagnostics.push(diag)
  }

  /** Mark a name installed via def (for unused-def analysis). */
  recordDef(name: string): void {
    if (!this.isActive() || name === '' || name.startsWith('_')) return
    this.defsInstalled.set(name, undefined)
    this.defsUsed.delete(name)
  }

  /** Mark a name referenced during check mode. */
  recordUse(name: string): void {
    if (!this.isActive() || name === '') return
    this.defsUsed.add(name)
  }

  /** Emit unused_def warnings for installed-but-never-used names. */
  emitUnusedDefDiagnostics(): void {
    for (const name of this.defsInstalled.keys()) {
      if (this.defsUsed.has(name)) continue
      this.addDiagnostic({
        code: 'unused_def',
        detail: `def ${name} is never used`,
        word: name,
        severity: 'warning',
      })
    }
  }
}
