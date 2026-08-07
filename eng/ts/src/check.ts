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
import { BoruType, TAny } from '@voxgig/borucore'
import type { Signature } from '@voxgig/borucore'
import { isSugar, newCarrier, newDynamicCarrier, Value } from '@voxgig/borucore'
import type { Registry } from '@voxgig/borucore'

// The analysis STATE lives in core (core/ts/src/check-state.ts, the twin of
// core/go/check_state.go); this piece owns the analysis LOGIC. Re-exported
// here so every existing consumer of `check.ts` keeps working unchanged.
import { installAnalysisImpl } from '@voxgig/borucore'
export {
  CheckState,
  DEFAULT_CHECK_STEP_BUDGET,
  severityFor,
} from '@voxgig/borucore'
export type { CheckDiagnostic, CheckSeverity } from '@voxgig/borucore'


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
  // Control / structural tokens are not values to strip. A sugar
  // marker MUST keep its SugarInfo payload — a stripped marker can
  // never lower and would bake as inert data in the compiled program
  // (the Go carrier-strip has the same exemption, eng/go/carrier.go).
  if (v.isWord() || v.isForward() || v.isMark() || v.isMove() || v.isParenExpr()) return v
  if (v.isInterpString() || v.isXmlInterp() || isSugar(v)) return v
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
  const mk = (t: BoruType): Value =>
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
  return newCarrier(common.length > 0 ? new BoruType(common) : TAny)
}


// Install this piece's implementations over core's NAMED inactive defaults —
// the TS twin of check/go's init replacing core/go's AnalysisImpl. Importing
// check.ts is what arms analysis; a core-only build runs the inactive
// defaults instead (pinned by core/ts/src/seams.test.ts).
installAnalysisImpl({ stripToCarriers, carrierResults })
