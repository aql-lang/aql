package langspec

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/aql-lang/aql/test/go/vary"
)

// TestVariationDifferential — the standing variation gate (plan WS3): a
// deterministic sample of passing corpus rows is re-embedded in every
// compile context of the shared transform table (test/go/vary) and each
// variant is classified through the dual pipeline AT TEST TIME (no checked-in
// generated corpus — the interpreter is the oracle, like the property
// fuzzer). The gate asserts:
//
//   - NO unpinned variant diverges (a divergence is a miscompile — a NEW one
//     always fails; the known do-unit registry-replay class is pinned in
//     varyKnownMiscompiles with a stale arm forcing graduation on fix);
//   - every compile refusal / island falls in a KNOWN reason bucket
//     (varyBucket) pinned in varyRefusalLedger — a new bucket is a new
//     frontier class that must be triaged (fix, or pin here + add a
//     representative row to lang/spec/frontier/);
//   - ledgered buckets are still observed (stale arm → delete the entry) —
//     enforced only at >= default breadth, since Sample() nests samples
//     (a bucket observed at the default stays observed at any larger one,
//     while a smaller sample legitimately misses some).
//
// Breadth is env-crankable for triage sweeps: AQL_VARY_SEEDS=<n> (0 = the
// whole corpus). The default is modest for CI wall-clock; the full sweep
// lives in `specgen -vary`.
func TestVariationDifferential(t *testing.T) {
	seeds, err := vary.LoadSeeds(filepath.Join("..", "..", "..", "lang", "spec"))
	if err != nil {
		t.Fatalf("seeds: %v", err)
	}
	n := defaultVarySeeds
	if s := os.Getenv("AQL_VARY_SEEDS"); s != "" {
		v, err := strconv.Atoi(s)
		if err != nil {
			t.Fatalf("AQL_VARY_SEEDS=%q: %v", s, err)
		}
		n = v
	}
	sample := vary.Sample(seeds, n)

	variants := vary.SweepSeeds(sample, nil)
	observed := map[string]int{}
	divergedSeen := map[string]bool{}
	counts := map[vary.Outcome]int{}
	skipped := 0
	for _, v := range variants {
		if v.Transform == "seed" {
			if v.Res.Outcome != vary.Pass {
				skipped++ // base not vary-eligible (fixture rows, corpus refusals — the ratchets own those)
			}
			continue
		}
		counts[v.Res.Outcome]++
		switch v.Res.Outcome {
		case vary.Diverged:
			if _, known := varyKnownMiscompiles[v.Src]; known {
				divergedSeen[v.Src] = true
				continue
			}
			t.Errorf("MISCOMPILE — variant diverges from the interpreter:\n  seed:      %s (%s:%d)\n  transform: %s\n  variant:   %s\n  %s\ntriage: shrink + fix, or pin in varyKnownMiscompiles AND add a frontier row (never leave a divergence unpinned)",
				v.Seed.Input, v.Seed.File, v.Seed.Line, v.Transform, v.Src, v.Res.Detail)
		case vary.Refused, vary.Islanded:
			bucket := varyBucket(v.Res.Detail)
			observed[bucket]++
			if _, ok := varyRefusalLedger[bucket]; !ok {
				t.Errorf("NEW refusal class from variation:\n  bucket:    %q\n  seed:      %s (%s:%d)\n  transform: %s\n  variant:   %s\n  detail:    %s\ntriage: widen the compiler, or pin the bucket in varyRefusalLedger AND add a representative row to lang/spec/frontier/",
					bucket, v.Seed.Input, v.Seed.File, v.Seed.Line, v.Transform, v.Src, v.Res.Detail)
			}
		}
	}
	if n >= defaultVarySeeds || n <= 0 {
		for bucket, why := range varyRefusalLedger {
			if observed[bucket] == 0 {
				t.Errorf("stale varyRefusalLedger bucket %q — no variant refuses in it any more; graduate it (delete the entry).\n  was red because: %s", bucket, why)
			}
		}
		for src, why := range varyKnownMiscompiles {
			if !divergedSeen[src] {
				t.Errorf("stale varyKnownMiscompiles pin — this variant no longer diverges; graduate it (delete the pin; also graduate the frontier-do-registry-replay rows if the class is fixed).\n  variant: %.120s…\n  was red because: %s", src, why)
			}
		}
	}
	t.Logf("variation census: %d seeds (%d skipped) → pass=%d refused=%d islanded=%d diverged-known=%d interp-reject=%d check-reject=%d; refusal buckets: %v",
		len(sample), skipped, counts[vary.Pass], counts[vary.Refused], counts[vary.Islanded],
		len(divergedSeen), counts[vary.InterpReject], counts[vary.CheckReject], observed)
}

// varyBucket normalises a refusal/island detail into a stable ledger key:
// the vary-specific families first (runtime-bail details embed full rendered
// errors; the check-diagnostics sentinel and the do/for reasons are stable
// prefixes), then the census's normaliseReason for everything else.
func varyBucket(detail string) string {
	switch {
	case strings.HasPrefix(detail, "runtime bail:"):
		return "runtime bail (VM defer, interpreter fallback)"
	case strings.HasPrefix(detail, "check diagnostics"):
		return "check diagnostics (wrapped-context false positive)"
	case strings.HasPrefix(detail, "do: fallible multi-value body"):
		return "do: fallible multi-value body under a catch"
	case strings.HasPrefix(detail, "for: body nets multiple values"):
		return "for: body nets multiple values per iteration"
	case strings.HasPrefix(detail, "program embeds an OpFallback island"):
		return "islanded"
	default:
		return normaliseReason(detail)
	}
}

// defaultVarySeeds is the CI breadth: large enough to hold every ledgered
// bucket observable, small enough to keep the langspec wall-clock sane.
const defaultVarySeeds = 32

// varyRefusalLedger pins the refusal reason-buckets (varyBucket) that
// variation currently produces, with the same graduation contract as every
// expected-red ledger in the suite: a NEW bucket fails until triaged; a
// bucket no longer observed at default breadth fails as stale. The buckets
// are calibrated against the DEFAULT sample — editing the corpus can shift
// which seeds are sampled and legitimately graduate a bucket. Bootstrap
// census (2026-07-13, 32 seeds): pass=384 refused=42 islanded=0.
var varyRefusalLedger = map[string]string{
	"do: fallible multi-value body under a catch":        "plan Phase 5 (L-DO) — the frontier-do-catch.tsv class reached via the do-catch transform",
	"for: body nets multiple values per iteration":       "plan Phase 5 (net drivers) — the frontier-for-multi.tsv class reached via the for-body transform over multi-value seeds",
	"runtime bail (VM defer, interpreter fallback)":      "plan Phase 10 (bail census to zero) — module-body wrapping reaches VM defer sites; the fallback is sound but not runtime-independent",
	"check diagnostics (wrapped-context false positive)": "checker emits model-undermining diagnostics for a program the interpreter runs clean once re-embedded (for/fn wrapping of typed defs) — sound-but-lossy refusal",
	"dynamic/opaque output":                              "plan Phase 7 (module-fn param slots) — a module fn's dot-dispatch result is opaque inside fn/lambda units",
	"dynamic input":                                      "poly-extension to non-core words over dynamic carriers (plan Phase 5 leverage list)",
	"operand provenance":                                 "residual operand loses provenance across a wrapped context (plan Phase 4/5 accounting classes)",
	"residual lowering (Stage 1 limit)":                  "scheduling — the wrapped residual shape exceeds Stage 1's lowering (prefix-stack transform)",
	"stack discipline (lowering)":                        "scheduling — dirty-stack prefixes the lowerer cannot arrange (prefix-stack transform)",
}

// varyKnownMiscompiles pins the KNOWN divergences (the do-unit
// registry-replay class, minimal repro in
// lang/spec/frontier/frontier-do-registry-replay.tsv): a compiled do-body
// re-runs typed defs / registrations against the check pass's surviving
// registry state — type defs conflict at VM time, check-time registrations
// are invisible. Keyed by EXACT variant source (derived from the seed ×
// transform, so the pins track the transform table); the stale arm forces
// graduation when the class is fixed. A divergence NOT in this map always
// fails — new miscompiles are never silently ledgerable.
var varyKnownMiscompiles = func() map[string]string {
	wrap := map[string]func(string) string{}
	for _, tr := range vary.Transforms() {
		wrap[tr.Name] = tr.Wrap
	}
	m := map[string]string{}
	for _, s := range []struct{ seed, why string }{
		{`def A (Integer gt 10) def B (Integer lt 20) def x:(A tand B) 15 x`,
			"do-unit registry replay: predicate-type defs re-install (type-conflict Error vs 15)"},
		{`def Big (Integer gt 10) def W (Big tand (Integer lt 20)) 10 is W`,
			"do-unit registry replay: predicate-type defs re-install (type-conflict Error vs false)"},
		{`def Pos (Integer gt 0) def H gen [(T extends Pos)] refine Record [x:T] end H of [Pos]`,
			"do-unit registry replay: generic + refine defs re-install (type-conflict Error vs the record)"},
		{`import "aql:minilang"  +re/[a-z]+/ typeof`,
			"do-unit registry replay (mirror half): the check-time minilang kind registration is invisible at VM time (mini_unknown_lang vs Function)"},
		{`import "aql:parse"  import "aql:parselang"  def g Parse.grammar  Parse.spec g {abnf:[{src:'op = "inc" / "dec"' start:'op'}]}  Parse.action g '@op:o:INC' ([nd:Any] => [9])  Parse.register sp6 g  end  parse sp6 'inc'`,
			"do-unit registry replay (mirror half): the check-time ParseLang registration is invisible at VM time (parse_unknown_lang vs 9)"},
	} {
		m[wrap["do-body"](s.seed)] = s.why
		m[wrap["do-catch"](s.seed)] = s.why
	}
	return m
}()
