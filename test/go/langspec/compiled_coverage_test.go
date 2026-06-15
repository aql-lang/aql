// Compiled-coverage ratchet (design plan: "Make the AQL bytecode VM fully
// independent of the interpreter at runtime", phase P0). The runtime-
// independence work drives every supported program to compile to bytecode
// that runs entirely in the VM, so that BOTH interpreter dependencies — the
// OpFallback island and the whole-program fallback — can be deleted.
//
// This test is the objective measure of that goal. It runs EVERY spec value
// row through CompileCheck and counts the rows that REFUSE (no Program, no
// check error), bucketed by a normalised refusal reason. The total refusal
// count is a downward ratchet: it must never rise above the recorded ceiling,
// and each phase (P2..P6) lowers the ceiling as a refusal category is
// eliminated. P7 (delete the fallback) is gated on this reaching ZERO.
//
// Rows that error during the check pass itself (parse errors, type-error
// rows) are NOT refusals — the program is statically invalid in both engines
// — and are reported separately, not counted against the ceiling.
package langspec

import (
	"bufio"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// refusalCeiling is the maximum number of spec value rows allowed to refuse
// compilation (CompileCheck returns a nil Program with no check error AND the
// row is not a statically-invalid check-diagnostics row). The runtime-
// independence phases lower it monotonically; it must reach 0 before the
// interpreter fallback can be deleted (plan P7). Never raise it.
const refusalCeiling = 407 // P0 651 -> P3 642 -> wide poly 618 -> P4 CALL_DYNAMIC (fn-value apply) 616 -> P5 multi/0-result calls 598 -> make carrier-identity (impure constructor fresh-id) 580 -> value-def locals (OpStoreLocal) 568 -> dup carrier-identity (distinct ids for repeated outputs) 565 -> predicate-type operand provenance (DepScalar const-bake) 555 -> if value-else arm 545 -> case clause compilation (desugar to if-chain) 542 -> multi-return / 0-return / anonymous-lambda fns (N-result fn units) 538 -> apply of a fn value (ReturnsIdentity makes the check engine re-step it) 527 -> unnamed-fn map/list members const-bakeable (method fields: m.f via poly get + CALL_DYNAMIC) 521 -> 0-value-then 2-arg if statement guard (if cond [raise]) 519 -> concrete-args dynamic-output core builtins bake CALL_NATIVE (unify et al) 514 -> module inner natives (StructUtil.* etc, dot-access) bake CALL_NATIVE via sub-registry sig verification 459 -> 3-arg operand shape: sig-0 result over const operands lowers via push+swap chain (setpath et al) 453 -> strict-disjunct straddle lowers OpCallNativePoly (5 is (tnot (Integer gt 0))) 445 -> object/class atom-keyed set (quoted-operand exemption, mutation-safe) 425 -> computed-else if (SWAP cond up, DROP else on taken path) 421 -> variadic-else statement-if (mismatched-arm merge marked variadic) 417 -> fn-value introspection (typeof/tcmp/teq/tand/tor/tnot read a baked fn-value const) 407 -> filter list-lambda compiles to a named-param closure (pair-Map input via InvokeBody, roadmap item 5a-1) 405 -> map lambdas (filter/each/fold/scan over {…}) compile to named-param closures (KeyVal input via InvokeBody, roadmap item 5a-2) 399 -> with-decimal body compiles to a closure run inside the pushed decimal context (roadmap item 7) 394 -> aql:query DSL (select/where/order/group/having/on/using clause lists + from/join table names bake as inert SQL-spec consts) compiles CALL_NATIVE (roadmap item 7) 374 -> reach inert lens paths + raise error-code atom bake as inert const operands (ParenExpr-bearing paths excluded, they need live scope) 368 -> structural type operands of BUILTIN type literals (`[1] is [Integer]`, `{a:Integer}`, type-algebra) bake as consts (canonical builtin pointers, no behave-staleness; user-type leaves stay refused) 353

// islandCeiling is the maximum number of compiled programs allowed to embed an
// interpreter island (OpFallback). Islands re-enter the interpreter sub-engine
// at run time, so this is the second downward ratchet toward run-time
// independence (plan): each phase that compiles an island shape natively lowers
// it, and it must reach 0 before the OpFallback machinery can be deleted (P7).
const islandCeiling = 9 // atom-keyed gets 102->36; fold-no-init + filter (dynamic-output) closures 36->29; P5 multi/0-result calls 29->26; case clause compilation (islanded cases now compile natively) 26->15; map-iteration quotation each/fold/scan over {…} compile to closures via InvokeBody (roadmap item 5b) 15->9; lower as more island shapes compile natively

// normaliseReason buckets a refusal reason into a stable category by
// stripping the row-specific tail (word names, counts), so the histogram is
// comparable across rows.
func normaliseReason(reason string) string {
	switch {
	case strings.HasPrefix(reason, "code-body word"):
		return "code-body word (NoEvalArgs)"
	case strings.HasPrefix(reason, "quoted-operand word"):
		return "quoted-operand word"
	case strings.HasPrefix(reason, "compile-time word"):
		return "compile-time word (RunInCheckMode)"
	case strings.HasPrefix(reason, "full-stack word"):
		return "full-stack word (depth/pick/roll)"
	case strings.HasPrefix(reason, "context-dependent word"):
		return "context-dependent word (args/__pa)"
	case strings.HasPrefix(reason, "user fn call"):
		return "user fn call (Stage 3)"
	case strings.HasPrefix(reason, "function-valued operand"):
		return "function-valued operand (Stage 3)"
	case strings.HasPrefix(reason, "function value reaches"):
		return "function value reaches word (Stage 3)"
	case strings.HasPrefix(reason, "anonymous function dispatch"):
		return "anonymous fn dispatch (Stage 3)"
	case strings.HasPrefix(reason, "dynamic input at"):
		return "dynamic input"
	case strings.HasPrefix(reason, "unannotated or opaque word"):
		return "dynamic/opaque output"
	case strings.HasPrefix(reason, "polymorphic dispatch"):
		return "polymorphic dispatch"
	case strings.Contains(reason, "surface-shape typed dispatch"),
		strings.Contains(reason, "unmatched dispatch recovered"):
		return "dispatch recovery (best guess)"
	case strings.Contains(reason, "returns ") && strings.Contains(reason, "values"):
		return "multi-result call"
	case strings.HasPrefix(reason, "dynamic value precedes residual"):
		return "fn-value-call boundary"
	case strings.Contains(reason, "operand of unknown provenance"),
		strings.Contains(reason, "of unknown provenance"):
		return "operand provenance"
	case strings.Contains(reason, "suppressed a runtime error"):
		return "suppressed runtime error"
	case strings.Contains(reason, "without exactly one declared return"),
		strings.Contains(reason, "body value count differs"):
		return "multi-return fn"
	case strings.HasPrefix(reason, "residual shape beyond Stage 1"),
		strings.Contains(reason, "residual value not statically materialisable"):
		return "residual lowering (Stage 1 limit)"
	case strings.HasPrefix(reason, "if:"):
		return "if-branch lowering"
	case strings.HasPrefix(reason, "stack discipline"):
		return "stack discipline (lowering)"
	case strings.HasPrefix(reason, "operand shape at"):
		return "operand shape (Stage 1 limit)"
	default:
		return "other: " + reason
	}
}

func TestCompiledCoverage(t *testing.T) {
	specDir := filepath.Join("..", "..", "..", "lang", "spec")
	entries, err := os.ReadDir(specDir)
	if err != nil {
		t.Fatalf("read %s: %v", specDir, err)
	}

	var rows, compiled, checkErr, refused, islanded int
	buckets := map[string]int{}

	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".tsv") {
			continue
		}
		path := filepath.Join(specDir, e.Name())
		f, err := os.Open(path)
		if err != nil {
			t.Fatalf("open %s: %v", path, err)
		}
		scanner := bufio.NewScanner(f)
		scanner.Buffer(make([]byte, 1024*1024), 1024*1024)
		for scanner.Scan() {
			line := strings.TrimRight(scanner.Text(), " \t")
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			parts := strings.Split(line, "\t")
			if len(parts) < 2 {
				continue
			}
			input := strings.TrimSpace(parts[0])
			rows++

			a := newDifferentialInstance(t)
			prog, reason, _, cerr := a.CompileCheck(input)
			switch {
			case cerr != nil, reason == "check diagnostics":
				// Statically invalid in both engines: a parse/check Go error,
				// or error-severity check diagnostics (a type-error row).
				// CompileCheck returns a nil Program for these with the row
				// genuinely erroring in the interpreter too — not a refusal.
				checkErr++
			case prog != nil:
				compiled++
				// An islanded program still re-enters the interpreter at run
				// time (OpFallback → sub-engine). Driving this to zero — by
				// compiling each island shape natively — is the run-time-
				// independence goal; track it as a downward ratchet.
				if strings.Contains(prog.Disassemble(), "FALLBACK") {
					islanded++
				}
			default:
				refused++
				buckets[normaliseReason(reason)]++
			}
		}
		f.Close()
		if err := scanner.Err(); err != nil {
			t.Fatalf("scanner error in %s: %v", path, err)
		}
	}

	// Histogram, most-frequent first.
	type kv struct {
		reason string
		n      int
	}
	hist := make([]kv, 0, len(buckets))
	for r, n := range buckets {
		hist = append(hist, kv{r, n})
	}
	sort.Slice(hist, func(i, j int) bool {
		if hist[i].n != hist[j].n {
			return hist[i].n > hist[j].n
		}
		return hist[i].reason < hist[j].reason
	})

	t.Logf("compiled coverage: %d rows — %d compiled (%d islanded), %d check-errors, %d refused",
		rows, compiled, islanded, checkErr, refused)
	for _, h := range hist {
		t.Logf("  refusal %4d  %s", h.n, h.reason)
	}

	if refused > refusalCeiling {
		t.Errorf("compile refusals %d exceed ceiling %d — coverage regressed", refused, refusalCeiling)
	}
	if islanded > islandCeiling {
		t.Errorf("islanded programs %d exceed ceiling %d — interpreter-island use regressed", islanded, islandCeiling)
	}
}
