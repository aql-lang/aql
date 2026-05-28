package shrink

import (
	"github.com/aql-lang/aql/eng/go/stackform"
)

// Outcome reports how an evaluation of a candidate StackForm
// resolved against the user's property predicate.
//
//   - Fail:    candidate is failure-preserving. Reducer accepts it
//     if it lowers cost.
//   - Pass:    property holds — candidate does NOT preserve the
//     failure. Reducer rejects.
//   - Invalid: candidate failed to evaluate at all (engine error,
//     missing word, type mismatch). Reducer rejects.
type Outcome int

const (
	Pass Outcome = iota
	Fail
	Invalid
)

// EvalFn is the caller-supplied function that evaluates a candidate
// StackForm and reports whether it preserves the original failure.
// For the PBT integration in aql:test, this closes over the property
// body and the seeded rand instance so each candidate reproduces the
// same generator/property pipeline.
type EvalFn func(*stackform.StackForm) Outcome

// Profile bundles the reducer's tuning knobs — how many greedy
// iterations to attempt before giving up, and which Policy to
// consult for cost weighting + word classification.
type Profile struct {
	// MaxSteps caps the outer greedy loop. Each step generates
	// candidates and accepts at most one (the lowest-cost
	// failure-preserving mutation). 200 steps is enough to
	// converge on most realistic counterexamples.
	MaxSteps int

	// Policy controls per-word transparency + cost weights. See
	// shrink/policy.go.
	Policy *Policy
}

// DefaultProfile returns sensible defaults: 200 max steps, the
// canonical DefaultPolicy.
func DefaultProfile() *Profile {
	return &Profile{
		MaxSteps: 200,
		Policy:   DefaultPolicy(),
	}
}

// Reduce minimises `initial` to the smallest StackForm (by
// ShrinkCost under the profile's Policy) for which `eval` still
// returns Fail. Pure greedy descent: at each step it generates
// candidates, sorts by cost, and accepts the first failure-
// preserving one that lowers cost. Stops when no candidate lowers
// cost OR MaxSteps is reached.
//
// Returns the smallest failing form found. If `initial` itself does
// NOT fail under `eval`, returns it unchanged — Reduce only shrinks
// known counterexamples.
//
// The algorithm mirrors design/aql_property_based_reduction_report.md
// §11 ("greedy failure-preserving reduction"). Phase-4 best-first
// search and exact small-program enumeration (report §15-§16) are
// out of scope here — see PBT-PLAN.0.md "Out of scope".
func Reduce(initial *stackform.StackForm, eval EvalFn, profile *Profile) *stackform.StackForm {
	if profile == nil {
		profile = DefaultProfile()
	}
	if initial == nil {
		return nil
	}
	// Reducer contract: only shrink genuine counterexamples. If the
	// caller hands us a form that doesn't fail, return it as-is.
	if eval(initial) != Fail {
		return initial
	}

	current := initial
	cost := ShrinkCost(current, profile.Policy)

	// Fingerprint set so a candidate already tried (and rejected or
	// accepted) isn't re-evaluated. Pretty serialisation is the
	// fingerprint — distinct forms have distinct pretty strings.
	seen := map[string]bool{stackform.Pretty(current): true}

	for step := 0; step < profile.MaxSteps; step++ {
		cands := generateCandidates(current, profile.Policy)
		sortByCost(cands, profile.Policy)

		accepted := false
		for _, cand := range cands {
			fp := stackform.Pretty(cand)
			if seen[fp] {
				continue
			}
			seen[fp] = true

			candCost := ShrinkCost(cand, profile.Policy)
			if candCost >= cost {
				// Not a reduction. Skip — sorted ascending so
				// nothing remaining can help either.
				break
			}

			if eval(cand) == Fail {
				current, cost = cand, candCost
				accepted = true
				break
			}
		}
		if !accepted {
			break
		}
	}
	return current
}

// sortByCost orders cands in-place ascending by ShrinkCost.
// Insertion sort is fine — candidate counts are small (O(N) where
// N = number of ops in the form).
func sortByCost(cands []*stackform.StackForm, policy *Policy) {
	for i := 1; i < len(cands); i++ {
		ci := ShrinkCost(cands[i], policy)
		j := i
		for j > 0 && ShrinkCost(cands[j-1], policy) > ci {
			cands[j], cands[j-1] = cands[j-1], cands[j]
			j--
		}
	}
}
