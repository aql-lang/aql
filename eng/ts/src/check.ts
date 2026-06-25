// Static type-checker (check mode), ported from eng/go/check.go +
// carrier.go. In check mode the engine runs the same dispatch/matching
// machinery, but concrete payloads are stripped to type-only *carriers*,
// signature handlers are short-circuited to produce carrier return
// values (declared via Signature.returns / returnsFn), and
// signature/return violations surface as diagnostics rather than hard
// errors.
//
// PARITY NOTE: this is the Phase-1 core — carriers, StripToCarriers,
// carrierResults (returnsFn → returns → missing_returns), gradual
// contagion, and the diagnostic store. Fn-body analysis, the step
// budget's fixed-point machinery, disjunct partitioning, and the
// bytecode recording pass land in later phases.
import type { EmitState } from './emit.ts'
import { AqlType, TAny } from './type.ts'
import type { Signature } from './signature.ts'
import { newCarrier, newDynamicCarrier, Value } from './value.ts'
import type { Registry } from './registry.ts'

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
   * When set, the check pass doubles as the bytecode recording pass:
   * each native dispatch is offered to emit.recordCall. Null for a plain
   * check. Mirrors CheckState.Emit. Recording is passive — it never
   * changes the carriers returned.
   */
  emit: EmitState | undefined

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

/**
 * toCarrier strips a concrete value to a type-only carrier. Structural
 * and control values pass through unchanged (lists/maps keep their
 * payload so positional matching still works; words/markers stay as
 * dispatch tokens; type literals stay literals). Everything else
 * collapses to a bare carrier of its type. Mirrors carrier.go::toCarrier
 * (Phase-1 subset).
 */
export function toCarrier(v: Value): Value {
  // Already a carrier — leave its modality intact.
  if (v.carrier) return v
  // Control / structural tokens are not values to strip.
  if (v.isWord() || v.isForward() || v.isMark() || v.isMove() || v.isParenExpr()) return v
  if (v.isInterpString() || v.isXmlInterp()) return v
  // Lists and maps keep their concrete payload — matching needs it.
  if (Array.isArray(v.data)) return v
  if (v.isMap() || v.isTypedList() || v.isTypedMap()) return v
  // Bare type literals (Data === null, not a carrier) stay literals so
  // type-argument slots and typeof still see them as types.
  if (v.data === null) return v
  // Fn definitions keep their body/params for closure analysis.
  if (v.isFnDef()) return v
  // Everything concrete else → a bare carrier of its type.
  return newCarrier(v.vType)
}

/** StripToCarriers maps every input value through toCarrier. */
export function stripToCarriers(input: Value[]): Value[] {
  return input.map(toCarrier)
}

/**
 * carrierResults produces the carrier return values for a matched
 * signature in check mode. Resolution order (mirrors
 * carrier.go::carrierResults, Phase-1 subset):
 *   1. sig.returnsFn(args, r) — custom check-mode logic; results coerced
 *      to carriers.
 *   2. sig.returns — one fresh carrier per declared type.
 *   3. neither → emit missing_returns and return a dynamic(Any) carrier.
 *
 * Gradual contagion: when any arg is a dynamic carrier, the produced
 * carriers are dynamic too (a statically-unknown input cannot yield a
 * statically-known output).
 */
export function carrierResults(
  registry: Registry,
  word: string,
  sig: Signature,
  args: Value[],
): Value[] {
  const contagious = args.some((a) => a.carrier && a.dynamic)
  // A declared return type of Any is statically unknown, so its carrier
  // is dynamic; a known type stays strict unless a dynamic operand makes
  // the whole result contagious. Mirrors carrierResults' Any handling.
  const mk = (t: AqlType): Value =>
    contagious || t.equal(TAny) ? newDynamicCarrier(t) : newCarrier(t)

  if (sig.returnsFn) {
    return sig.returnsFn(args, registry).map((v) => {
      const c = toCarrier(v)
      if (c.carrier && !c.dynamic && (contagious || c.vType.equal(TAny))) {
        return newDynamicCarrier(c.vType)
      }
      return c
    })
  }
  if (sig.returns) {
    return sig.returns.map(mk)
  }
  registry.check.addDiagnostic({
    code: 'missing_returns',
    detail: `${word} has no return-type annotation; result type is unknown`,
    word,
  })
  return [newDynamicCarrier(TAny)]
}

/**
 * joinCarriers merges two branch-arm result carriers into one. Mirrors
 * the lattice-join half of eng/go/carrier.go::JoinCarriers (Phase-4
 * subset — no disjunct construction yet): equal leaves keep the type;
 * a subtype/supertype pair widens to the supertype; otherwise the type
 * is the longest common lattice-path prefix (Scalar, Node, … or Any).
 * The result is always a STRICT carrier (never dynamic) so a branch's
 * merged result keeps stable provenance through carrierResults.
 *
 * The carrier type is informational for the compiler — the differential
 * gate compares concrete VM values — so a coarse join still verifies.
 */
export function joinCarriers(a: Value, b: Value): Value {
  const ta = a.vType
  const tb = b.vType
  if (ta.equal(tb)) return newCarrier(ta)
  if (ta.matches(tb)) return newCarrier(tb)
  if (tb.matches(ta)) return newCarrier(ta)
  // Longest common prefix of the two lattice paths.
  const common: string[] = []
  const n = Math.min(ta.parts.length, tb.parts.length)
  for (let i = 0; i < n; i++) {
    if (ta.parts[i] !== tb.parts[i]) break
    common.push(ta.parts[i]!)
  }
  return newCarrier(common.length > 0 ? new AqlType(common) : TAny)
}
