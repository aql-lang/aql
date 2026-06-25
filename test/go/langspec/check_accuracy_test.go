// Check-accuracy ratchet (design/checker-accuracy-review.10.md §5).
//
// Runs `aql check` semantics (Registry.Check.Begin + a normal engine
// run) over every row of the production language spec at lang/spec/
// and counts two things:
//
//   - FALSE POSITIVES: rows whose expectation is a VALUE (the program
//     is correct and runs) but where the checker reports an
//     error-severity diagnostic or a hard error. Target: 0.
//   - UNFLAGGED ERROR ROWS: rows whose expectation is ERROR:* but
//     where the checker is silent. This will never reach 0 — many
//     spec errors are value-dependent (overflow, missing keys,
//     division by zero) and are the runtime's job — so the count is
//     a trend metric, not a target.
//
// Both counts are pinned and may only DECREASE (a ratchet): a change
// that pushes either count above its pin is a checker-accuracy
// regression and fails this test. When a fix lowers a count, lower
// the pin in the same commit so the gain is locked in.
package langspec

import (
	"bufio"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/aql-lang/aql/eng/go"
	"github.com/aql-lang/aql/eng/go/parser"
	"github.com/aql-lang/aql/lang/go/modules"
	"github.com/aql-lang/aql/lang/go/native"
	"github.com/aql-lang/aql/test/go/specrunner"
)

// The ratchet pins. Lower them when a checker improvement lands;
// never raise them without a documented decision.
const (
	pinnedFalsePositives     = 23  // value rows the checker wrongly errors on. was 27 — −2 unpack module forms + −2 minilang register-then-use. A module's exports / a registered kind's fn are STATICALLY declared, so the checker resolves them (NOT a runtime-only effect — the same exports `import` already resolves in check). `unpack 'aql:mod'` / `unpack Export 'aql:mod'` gained RunInCheckMode + kept-concrete module-name strings, binding the exports unqualified so a later bare word resolves. `MiniLang.register <name> <fn>` gained an idempotent check-mode install (ReturnsFn mirroring parselang-register), so a later `mini <name>` resolves the kind. was 28 — −1 user-types.tsv: an abstract carrier already TAGGED as a predicate-refine (subset) type now satisfies that type's param nominally (depScalarUnifier.Match trusts the tag for an abstract carrier — the contract guarantee — and only runs the value-level predicate on a CONCRETE value), so a `Big`-returning fn flows into a `Big` param. was 31 — −3 class.tsv singleton types: `typeof` of a CONCRETE argument now returns the precise type literal in check mode (a concrete-gated ReturnsFn), so `def One (typeof (const 1))` gets a valid type body (the singleton) instead of a bare `Type` carrier the def-type validator rejected; `is` over it stays precise. A non-concrete arg keeps the bare Type carrier (blast radius off every runtime typeof). was 35 — −4 recursion.tsv mutual-recursion forward refs (isod↔isev): checkFlagsError now mirrors lang.(*AQL).Check by calling RescueForwardRefDiagnostics before reading diagnostics, so a fn-body reference to a sibling def below it (flagged undefined_word at eager body-analysis time, then rescued once the def exists) is no longer over-counted — the CLI never showed these. was 47 — −12 path-modifier.tsv: a dispatch modifier (usurp / stack-args / forward-args / force-arity — the `/u` `/s` `/f` `/N` desugarings) over a stored fn-ref read via dot-access (`m.a` where `m = {a:add/r}`) is statically dynamic(Any) because getNodeReturns cannot narrow a dispatch-bearing field; the handlers now return a gradual Function carrier in check mode for a non-concrete arg the real wrapper declined, instead of an illegal_ref. was 65 — −18 compare.tsv type-algebra over user types: `tor` (type union) modelled its check-mode result with a branch-merge JoinCarriers that mishandled the nil-Parent None literal and halted the tape (`(String tor None)`); it now mirrors TorHandler's disjunct (unionType). `enum [...]` gained a ReturnsFn that runs its pure handler so the result carries its DisjunctInfo members. And toCarrier now preserves a Disjunct/Enum value's DisjunctInfo (as it does FnDef/Module), so `def Maybe (String tor None)` / `def Color enum […]` are valid type bodies in check mode and `tcmp`/`is`/`typeof` over them keep their members. was 105 — −40 flex.tsv: `flex` / `node` declared the supertype `Returns: [TNode]`, so a FlexMap/FlexList result never matched the `Flex*` overload of a downstream `set`/`append`/`push`/`pop`/`sort`/`each` and failed no_signature; both now carry a ReturnsFn modelling the precise flex subtype from the input's node family (Map→FlexMap, List→FlexList, Xml→FlexXml), mirroring FlexDeepCopy. was 114 — −9 from the client-module gradual-dispatch fixes (design/CLIENT-FIXES-2026-06-24.md): a polymorphic word over a dynamic receiver no longer narrows the binding to one overload's slot (slice's String-vs-List data arg) nor commits to one overload's return (add's temporal vs String/Number over two fully-unknown operands), a None-vs-value branch merge is gradual (the `if (nd eq none) [build] [nd]` builder sentinel), and a dynamic arg to a declared concrete param is analysed at the param's type. MEASURED on the integrated branch (was a loose pin of 123 with lower live actual): net effect of Stage-3 module-parselang:23 landing on THIS branch's engine — the register-then-parse rows now check clean (parser installed at CHECK time via a check-mode ReturnsFn on parselang-register), but the body-bearing fn-VALUE dispatch fix (spliceFnValueCheckResult → buildFnBodyReturnsFn → AnalyseFnBody) emits body diagnostics on a few fn-value calls under generalised carrier args that the legacy inline-splice did not, so the live count settles at 114. This is PRECISION only (sound — TestSpecCompiledDifferential + TestCheckTypeSoundness both pass, every row still RUNS correctly), a documented tradeoff for clearing the refusal (refusals 5→4). Pin is the measured floor, below the prior loose 123.  Earlier chain: −5 module-parselang register-then-parse value rows (§1 L16-18, §2 L21-22): ParseLang.register now installs the parser at CHECK time (a check-mode ReturnsFn on parselang-register), so `parse <name>` resolves and the checker no longer wrongly flags those rows — Stage-3 module-parselang:23; was 120 — +3 module-parse register-then-parse rows: the checker can't see the runtime Parse.register, so `parse <name>` flags, exactly like the module-parselang register-then-parse rows; was 119 — +1 module-minilang §7 register-compiled-then-use row, like the MiniLang.register rows the checker can't see the runtime register for; was 114 — +5 module-parselang register-then-parse rows; was 132 — macro installs in check mode; was 122 — make returns a value carrier of the made type, not the type literal)
	pinnedUnflaggedErrorRows = 189 // ERROR rows the checker is silent on. MERGE of two independent additions (this branch's +5 module-log, main's +5 accessor) over the shared 183-pin tail; measured floor 189 (false positives held at 23 — no checker regression, pure runtime-only coverage growth). +5 module-log.tsv runtime-only rows: the span-mismatch row (ending a non-active span is a runtime STATE check), the `Log.with-span … [raise …]` re-raise (a body error the checker cannot predict), the Log.register name collision (sink-exists fires only when the handler runs), and the Log.set-level / Log.set-format / Log.add-sink invalid-atom validation rows — all value/state-dependent like the other runtime-only rows here. +5 accessor.tsv runtime-only strict-miss errors: `getr`/`dotr` (and the `!.` sugar) raise on a missing map key, an out-of-bounds list index, a missing class field, or a missing Store key — all runtime facts the static checker cannot predict, exactly like the other runtime-only rows pinned here. The accessor matrix that added them is pure coverage growth, not a checker regression: the same rows error identically on the pre-split build, and the false-positive pin is untouched. +1 module-struct.tsv:78 runtime-only error exposed by the `tor` check-mode fix: `StructUtil.reify Shape {r:2.0}` over a union type `Shape = Circle tor Square` raises reify_error "a union target needs a $class key" only when reify runs in the shell — the static checker cannot know `{r:2.0}` lacks the discriminator. It was previously "flagged" ONLY by the spurious tor-halt the fix removed, exactly like the flex runtime-only rows below. +5 flex.tsv runtime-only errors exposed by the `flex` ReturnsFn fix: `set 3 99 (flex [1 2 3])` / `set 9 …` / `set 0 9 (flex [])` are out-of-bounds INDEX errors and `pop (flex [])` / `shift (flex [])` are empty-collection errors — all raise only when the op runs in the shell, which the static checker cannot predict. They were previously "flagged" ONLY as a side effect of the spurious no_signature the flex false-positive fix removed; the static checker correctly admits them now (the index/emptiness is a runtime fact), exactly like the other runtime-only rows pinned here. +2 module-parselang §4 runtime-only errors: with ParseLang.register now installing the parser at check time, `parse calc {file:…}` / `parse calc {nope:1}` resolve and dispatch cleanly statically — the parse_file_unsupported / parse_bad_source errors fire only when the registered parser resolves the source in the runtime shell, exactly like the §5 ini and §8 json/toml runtime-only source/syntax rows already pinned here — Stage-3 module-parselang:23; +5 module-parselang §9 aontu runtime-only errors: a unification conflict / kind mismatch / unresolvable reference / incomplete kind / missing-brace syntax error all surface only when the aontu parser runs in the shell, which the static checker cannot predict, exactly like the §8 json/toml syntax-error rows; +3 module-emitlang runtime-only emit_unsupported errors: a non-tabular csv shape (a bare Map / a scalar) and a TOML-unrepresentable scalar (None) surface only when the encoder walks the value in the runtime shell, which the static checker cannot predict — the encoder twin of the existing emitlang register/runtime rows; +8 module-emitlang runtime-only errors: emit_unknown_lang on an unbound namespace, emit_unsupported (toml top-level List, xml non-Xml top, ini deep nesting), and the four EmitLang.register validation rejections (bad name / bad signature / emit_ prefix / built-in collision) all resolve only when the emitter runs in the shell, which the static checker cannot predict — the emit twin of the module-parselang register-then-parse runtime rows; +3 module-parse §4 runtime-only errors: a malformed ABNF / a bad action ref / a built-in-kind collision all surface only when Parse.register or the parser runs in the shell, which the static checker cannot predict; +1 module-minilang §xp runtime-only error: a malformed XPath (`mini xp '//['`) fails only when the kind runs in the shell, which the static checker cannot predict — the XPath twin of the jp/jq runtime-error rows; +2 module-minilang §jpq runtime-only errors: a malformed jp/jq query fails only when the kind runs in the shell, which the static checker cannot predict; +3 module-minilang §m runtime-only errors: m's unknown-variable / division-by-zero / malformed-formula all surface only when the kind runs in the shell, which the static checker cannot predict; +2 module-parselang §8 syntax-error negatives: a malformed json/toml source fails only when the parser runs in the shell, which the static checker cannot predict; +2 module-parselang §5 ini runtime-only errors: parse_file_unsupported / parse_bad_source resolve the source in the runtime shell, so the static checker cannot predict them; +2 record.tsv §V runtime-only errors: a numeric record field now rejects a non-numeric (String/Boolean) source at make time, which the static checker does not predict; +1 patrun.tsv runtime-only error the checker can't predict statically — a non-Scalar pattern value to `add`; was 138 — fold-over-map result typing surfaces one more error; June 2026; was 136 — +3 module-minilang §7 runtime-only errors: mini_no_transducer / mini_bad_compiler / mini_bad_name; was 131 — +5 module-parselang runtime-only errors; was 132)
)

func TestCheckAccuracyRatchet(t *testing.T) {
	specDir := filepath.Join("..", "..", "..", "lang", "spec")
	entries, err := os.ReadDir(specDir)
	if err != nil {
		t.Fatalf("read %s: %v", specDir, err)
	}

	var falsePositives, unflagged, valueRows, errorRows int

	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".tsv") {
			continue
		}
		// bytecode-combinations.tsv is a compiled-vs-interpreter PARITY
		// fixture (validated by the differential / whole-corpus / spec
		// gates), not a checker-accuracy spec — its rows deliberately
		// exercise dynamic/island shapes the gradual checker widens, so
		// it must not move the false-positive / soundness baselines.
		if e.Name() == "bytecode-combinations.tsv" {
			continue
		}
		path := filepath.Join(specDir, e.Name())
		f, err := os.Open(path)
		if err != nil {
			t.Fatalf("open %s: %v", path, err)
		}
		scanner := bufio.NewScanner(f)
		lineNum := 0
		for scanner.Scan() {
			lineNum++
			line := strings.TrimRight(scanner.Text(), " \t")
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			parts := strings.Split(line, "\t")
			if len(parts) < 2 {
				continue // malformed rows are TestSpecProd's problem
			}
			input := strings.TrimSpace(parts[0])
			expected := strings.TrimSpace(parts[1])
			expectError := strings.HasPrefix(expected, "ERROR:")

			flagged := checkFlagsError(t, input)

			if expectError {
				errorRows++
				if !flagged {
					unflagged++
				}
				continue
			}
			valueRows++
			if flagged {
				falsePositives++
				t.Logf("FALSE POSITIVE %s:L%d: %s", e.Name(), lineNum, input)
			}
		}
		f.Close()
		if err := scanner.Err(); err != nil {
			t.Fatalf("scanner error in %s: %v", path, err)
		}
	}

	t.Logf("check-accuracy: %d/%d value rows falsely flagged; %d/%d error rows unflagged",
		falsePositives, valueRows, unflagged, errorRows)

	if falsePositives > pinnedFalsePositives {
		t.Errorf("false positives rose to %d (pin %d) — the checker now wrongly rejects correct spec rows",
			falsePositives, pinnedFalsePositives)
	} else if falsePositives < pinnedFalsePositives {
		t.Logf("false positives improved to %d — lower pinnedFalsePositives to lock it in", falsePositives)
	}
	if unflagged > pinnedUnflaggedErrorRows {
		t.Errorf("unflagged error rows rose to %d (pin %d) — the checker lost coverage",
			unflagged, pinnedUnflaggedErrorRows)
	} else if unflagged < pinnedUnflaggedErrorRows {
		t.Logf("unflagged error rows improved to %d — lower pinnedUnflaggedErrorRows to lock it in", unflagged)
	}
}

// checkFlagsError runs one spec row in check mode against a fresh
// production registry (the same setup as runSpecProd) and reports
// whether the checker flags it: a parse failure, a hard run error, or
// any error-severity diagnostic.
func checkFlagsError(t *testing.T, input string) bool {
	t.Helper()
	values, err := parser.Parse(input)
	if err != nil {
		return true // parse errors are flagged by definition
	}
	reg, err := native.DefaultRegistry()
	if err != nil {
		t.Fatalf("registry: %v", err)
	}
	specrunner.RegisterQFixtures(reg)
	reg.SetParseFunc(parser.Parse)
	modules.InstallResolver(reg)
	native.SetHostClock(reg, specClock)

	done := reg.Check.Begin()
	_, runErr := native.NewTop(reg).Run(values)
	// Mirror lang.(*AQL).Check: a fn-body forward reference to a name defined
	// LATER in the same program (mutual recursion isod/isev, a body referencing
	// a sibling def below it) is flagged undefined_word at eager body-analysis
	// time, then rescued once the def exists. The real checker drops these
	// before reporting; the ratchet must too, or it over-counts forward-ref
	// false positives the CLI never shows.
	reg.RescueForwardRefDiagnostics()
	diags := reg.Check.Diagnostics
	done()

	if runErr != nil {
		return true
	}
	for _, d := range diags {
		if d.Severity == eng.SeverityError {
			return true
		}
	}
	return false
}

// ---- type-soundness differential (checker-accuracy-review.10.md §5,
// follow-on): for every value row that BOTH checks clean and runs,
// the runtime result's type must be covered by the checked carrier —
// `typeof(actual) ⊑ checked`. This is the assertion that catches
// wrong-TYPE checker bugs (A1, A4), which the value-pinning ratchet
// cannot see. Violations are pinned and may only decrease.

const pinnedTypeSoundnessViolations = 12 // was 13 — module-rand:38: a Rand.with-seed instance method dispatch (`def r (Rand.with-seed N)  r.<method> …`) was a residual-mismatch violation (shapeless `r` → method result Any vs the runtime's concrete value); with-seed's check-mode shaped-carrier ReturnsFn resolves the method wrapper's typed result, removing it (−1, sound). was 15 — IO.write now declares its true Returns (the target path / Stream handle), making the write/read spec rows sound (−2); was 14 — +1 patrun.tsv:L40, the dynamic dispatch of a value pulled out of a Patrun: `find` returns a dynamic Any (the matcher is a dynamic-dispatch container, so a stored value's static type is unknowable), so calling the found lambda leaves the checker residual [dynamic(Any) Map] while the runtime yields [Integer] — a precision limit surfacing as an apparent mismatch, not a checker soundness bug; was 15 — do [body] now reports the body's full residual stack (matching doListHandler) instead of only the last value; was 16 — pop/shift TList sigs now declare their true 2-value Returns ([TList, TAny]); was 22 — corrected six wrong aql:time-util inner-native Returns (weeks/days/until/since → CalDuration, tz-offset → String, total-ms → Float) to match their handlers; was 159 — typeCovered now applies the runtime `is Type` membership rule to type-as-value actuals (typeof / type-algebra / make-of-a-type / record shapes), which a raw actual.Parent.ConformsTo misjudged (a type literal's Parent is the DENOTED type's lattice parent), removing 127 spurious flags and exposing the genuine residue (wrong module-time Returns annotations, do/for/pop arity, dynamic method dispatch)

func TestCheckTypeSoundness(t *testing.T) {
	specDir := filepath.Join("..", "..", "..", "lang", "spec")
	entries, err := os.ReadDir(specDir)
	if err != nil {
		t.Fatalf("read %s: %v", specDir, err)
	}

	var violations, compared int

	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".tsv") {
			continue
		}
		// bytecode-combinations.tsv is a compiled-vs-interpreter PARITY
		// fixture (validated by the differential / whole-corpus / spec
		// gates), not a checker-accuracy spec — its rows deliberately
		// exercise dynamic/island shapes the gradual checker widens, so
		// it must not move the false-positive / soundness baselines.
		if e.Name() == "bytecode-combinations.tsv" {
			continue
		}
		path := filepath.Join(specDir, e.Name())
		f, err := os.Open(path)
		if err != nil {
			t.Fatalf("open %s: %v", path, err)
		}
		scanner := bufio.NewScanner(f)
		lineNum := 0
		for scanner.Scan() {
			lineNum++
			line := strings.TrimRight(scanner.Text(), " \t")
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			parts := strings.Split(line, "\t")
			if len(parts) < 2 {
				continue
			}
			input := strings.TrimSpace(parts[0])
			expected := strings.TrimSpace(parts[1])
			if strings.HasPrefix(expected, "ERROR:") {
				continue
			}

			checked, flagged := checkRow(t, input)
			if flagged {
				continue // counted by the FP ratchet, not here
			}
			actual, ok := runRow(t, input)
			if !ok {
				continue // runtime-environment rows (fixtures etc.)
			}
			compared++
			if !stackTypeCovered(checked, actual) {
				violations++
				t.Logf("TYPE UNSOUND %s:L%d: %s\n  checked=%s actual=%s",
					e.Name(), lineNum, input, stackTypes(checked), stackTypes(actual))
			}
		}
		f.Close()
		if err := scanner.Err(); err != nil {
			t.Fatalf("scanner error in %s: %v", path, err)
		}
	}

	t.Logf("type-soundness: %d violations across %d compared rows", violations, compared)
	if violations > pinnedTypeSoundnessViolations {
		t.Errorf("type-soundness violations rose to %d (pin %d)", violations, pinnedTypeSoundnessViolations)
	} else if violations < pinnedTypeSoundnessViolations {
		t.Logf("violations improved to %d — lower pinnedTypeSoundnessViolations to lock it in", violations)
	}
}

// checkRow runs one row in check mode and returns the residual
// carrier stack plus whether the checker flagged it.
func checkRow(t *testing.T, input string) ([]eng.Value, bool) {
	t.Helper()
	values, err := parser.Parse(input)
	if err != nil {
		return nil, true
	}
	reg, err := native.DefaultRegistry()
	if err != nil {
		t.Fatalf("registry: %v", err)
	}
	specrunner.RegisterQFixtures(reg)
	reg.SetParseFunc(parser.Parse)
	modules.InstallResolver(reg)
	native.SetHostClock(reg, specClock)

	done := reg.Check.Begin()
	out, runErr := native.NewTop(reg).Run(values)
	diags := reg.Check.Diagnostics
	done()
	if runErr != nil {
		return nil, true
	}
	for _, d := range diags {
		if d.Severity == eng.SeverityError {
			return nil, true
		}
	}
	return out, false
}

// runRow executes one row at runtime; ok=false when the row needs an
// environment this harness doesn't provide (it errored at runtime
// although the spec expects a value — fixtures, network, files).
func runRow(t *testing.T, input string) ([]eng.Value, bool) {
	t.Helper()
	values, err := parser.Parse(input)
	if err != nil {
		return nil, false
	}
	reg, err := native.DefaultRegistry()
	if err != nil {
		t.Fatalf("registry: %v", err)
	}
	specrunner.RegisterQFixtures(reg)
	reg.SetParseFunc(parser.Parse)
	modules.InstallResolver(reg)
	native.SetHostClock(reg, specClock)
	out, runErr := native.NewTop(reg).Run(values)
	if runErr != nil {
		return nil, false
	}
	return out, true
}

// stackTypeCovered checks the runtime stack against the checked
// carrier stack position-for-position from the TOP. A checked stack
// may be longer (None padding from branch joins); a runtime stack
// longer than the checked one is a violation.
func stackTypeCovered(checked, actual []eng.Value) bool {
	if len(actual) > len(checked) {
		return false
	}
	for i := 0; i < len(actual); i++ {
		c := checked[len(checked)-1-i]
		a := actual[len(actual)-1-i]
		if !typeCovered(c, a) {
			return false
		}
	}
	return true
}

// typeCovered reports whether one checked carrier admits the runtime
// value's type.
func typeCovered(checked, actual eng.Value) bool {
	if checked.Dynamic {
		return true
	}
	if eng.IsDisjunct(checked) {
		di, err := eng.AsDisjunct(checked)
		if err != nil {
			return false
		}
		for _, alt := range di.Alternatives {
			if typeCovered(alt, actual) {
				return true
			}
		}
		return false
	}
	node := checked.Parent
	if eng.IsBareTypeNode(checked) {
		node = eng.ValueType(checked)
	}
	if node == nil {
		return false
	}
	// A type-as-value actual — the result of `typeof`, type algebra
	// (`exclude`/`extract`/`tor`), `make`-of-a-type, a record shape left
	// on the stack — is a VALUE THAT IS A TYPE. Its runtime membership is
	// the `is Type` rule (native_type.go), NOT actual.Parent.ConformsTo:
	// a type literal's Parent is the DENOTED type's lattice parent (the
	// `Integer` literal has Parent == Number, the `Resource` literal has
	// Parent == Object), so a raw Parent.ConformsTo misjudges every such
	// value as not-a-Type. The checker correctly types these as the
	// meta-type `Type`; cover them when the checked node admits `Type`
	// (checked == Type/Any), or — when the checker was precise enough to
	// produce a specific type literal — when the actual's denoted node
	// conforms to it.
	if actualIsTypeValue(actual) {
		if eng.TType.ConformsTo(node) {
			return true
		}
		if dn := eng.ValueType(actual); dn != nil && dn.ConformsTo(node) {
			return true
		}
	}
	return actual.Parent.ConformsTo(node)
}

// actualIsTypeValue mirrors the runtime `is Type` membership rule
// (native_type.go isHandler): a value is itself a type when it is a
// bare type node, a structural type body, an (explicit-map) record
// shape, or a Type/-rooted value (Function / Disjunct / Enum). Used by
// typeCovered so a `typeof`/type-algebra result is judged by its
// type-membership, not by its denoted type's lattice parent.
func actualIsTypeValue(v eng.Value) bool {
	return eng.IsBareTypeNode(v) || eng.IsTypeBody(v) ||
		eng.IsRecordShape(v) || v.Parent.ConformsTo(eng.TType)
}

func stackTypes(vs []eng.Value) string {
	parts := make([]string, len(vs))
	for i, v := range vs {
		parts[i] = v.Parent.Leaf()
		if v.Dynamic {
			parts[i] = "dynamic(" + parts[i] + ")"
		}
	}
	return "[" + strings.Join(parts, " ") + "]"
}

// ---- Any-frontier metric (the "narrow types instead of Any" goal):
// counts clean value rows whose residual carrier stack still contains an
// Any-bounded carrier — strict Carry<Any> or dynamic(Any), the two shapes
// where the checker gave up to Any instead of a narrow type. (A dynamic
// carrier with a real bound — dynamic(Integer) — is NOT counted; it has a
// useful type.) Pinned as a ratchet (only decreases) so return-annotation
// / ReturnsFn narrowing work is measured and an Any-widening regression is
// caught. Unlike type-soundness this is a PRECISION metric, not a
// soundness one: a high count is imprecise, not unsound.

const pinnedAnyFrontierRows = 303 // INFORMATIONAL ONLY (reported via t.Logf, not gated). was 255 — +48 the precision cost of the client-module gradual-dispatch fixes (design/CLIENT-FIXES-2026-06-24.md): a polymorphic word over a fully-unknown (dynamic-Any) operand now widens its result to the gradual union incl. Any (add of two unknowns), a None-vs-value branch merge is gradual, and a dynamic arg to a concrete param is analysed at the param's type — each loosens matching, ending more clean rows on a dynamic carrier rather than a committed concrete type. Sound and a net win: the GATED accuracy ratchet (false positives) improved 114→105 over the same corpus, and TestCheckTypeSoundness held at 12. was 254 — +1 error.tsv: a new `… error [ get code ]` regression row (the None-built raise message, bloom report §1) reads a generic error's `code` field, which is statically Any (the checker can't know the code) — sound. was 253 — +1 object.tsv:134 `(convert Map o) get a`: convert now models its result as a fresh VALUE carrier OF the target type (ReturnsFreshInstance, like make) instead of the bare target type LITERAL (ReturnsIdentity). The freeze yields a Map carrier whose element types are unknown, so the downstream `get a` soundly ends Any. Net improvement: the same convert fix moved 3 spec rows from errored→clean (denominator 2379->2382), eliminating the `no_signature` false positives on arithmetic over a `convert Float`ed scalar (voxgig-aql/bloom-filter regression report §2). was 254 — module-rand:38: a Rand.with-seed instance row that previously ended Any (shapeless `r` → `r.<method>` → Any) now resolves the method wrapper's typed result via with-seed's check-mode shaped carrier (−1, sound). Prior combined floor 254 = this branch's -5 concrete-list/map field fold (254->249) AND module-parselang:23's +5 register-then-parse rows (254->259), additive. was 249 — -5: getNodeReturns now folds a CONCRETE list/map field read to the stored CONTAINER value (cloned, fresh ID) rather than its bare type carrier, so `(m get "k")` over a concrete map yields a concrete list/map whose downstream `size` / index / element access types precisely instead of ending Any. was 246 — +8 module-parselang §9 aontu rows: `parse aontu` returns Any (like the rest of the parser family), so a parsed value accessed with get / typeof ends Any. was 245 — +1 from merging origin/main (denominator 2332->2341, +9 new value rows; the top-10 per-file breakdown is unchanged from the §5b state, so the +1 is a new main row outside the top files, not a regression from this branch). was 243 (actual 241 + headroom) — +4 module-test.tsv rows from the CheckState-ownership refactor (design/module-fn-checkstate-ownership.1.md §5b): module-preamble fn bodies (run-spec/run-cases/run-case) now run IN CHECK MODE under the parent pass, so their results are CARRIERS (Any) instead of being concrete-folded by real execution during the check pass. The pre-refactor precision was ill-gotten — it came from running side-effecting module-fn bodies for real during Check() (the same path that leaked test-record), which §5b+§5c correctly stop. §6 (compiling the framework code-body words) is expected to recover precision by giving those results real return types. was 229 — +14 module-parselang tabnas-family rows (json/jsonic/json5/jsonc/csv/toml/yaml/xml/zon/markdown/feed §6–§7, accessed with get); was 225 — +4 module-parselang ini rows (parse returns Any, so a parsed value accessed with get ends Any). pristine origin/main already measures 208 against the prior 194 pin — a pre-existing precision regression on main (the per-file breakdown here is identical to main except for patrun.tsv), NOT introduced by this branch; +17 patrun.tsv rows whose `find` returns a dynamic Any (the matcher is a dynamic-dispatch container, so a found value's static type is unknowable). was 194 — fold over a map narrows to the accumulator type (the list ReturnsFns are collection-agnostic); was 196; was 201 (list-index), 208 (struct-util), 217 (filter), 241 (case), 286 (item #4 field access); see TestCheckAnyFrontier

func TestCheckAnyFrontier(t *testing.T) {
	specDir := filepath.Join("..", "..", "..", "lang", "spec")
	entries, err := os.ReadDir(specDir)
	if err != nil {
		t.Fatalf("read %s: %v", specDir, err)
	}

	var anyRows, valueRows int
	byFile := map[string]int{}

	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".tsv") {
			continue
		}
		if e.Name() == "bytecode-combinations.tsv" {
			continue
		}
		f, err := os.Open(filepath.Join(specDir, e.Name()))
		if err != nil {
			t.Fatalf("open: %v", err)
		}
		scanner := bufio.NewScanner(f)
		for scanner.Scan() {
			line := strings.TrimRight(scanner.Text(), " \t")
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			parts := strings.Split(line, "\t")
			if len(parts) < 2 || strings.HasPrefix(strings.TrimSpace(parts[1]), "ERROR:") {
				continue
			}
			checked, flagged := checkRow(t, strings.TrimSpace(parts[0]))
			if flagged {
				continue
			}
			valueRows++
			if residualHasAnyFrontier(checked) {
				anyRows++
				byFile[e.Name()]++
			}
		}
		f.Close()
		if err := scanner.Err(); err != nil {
			t.Fatalf("scanner: %v", err)
		}
	}

	t.Logf("any-frontier: %d/%d clean value rows end with an Any carrier", anyRows, valueRows)
	for _, kv := range topFiles(byFile, 10) {
		t.Logf("  %3d  %s", kv.n, kv.name)
	}
	// The Any-frontier count is INFORMATIONAL, not a gate. It tracks how many
	// clean rows end at an Any carrier (a checker-precision surface), but pinning
	// it turns every corpus addition into pin churn. The hard checker gates that
	// remain are the ones that catch real defects: false positives on rows that
	// must pass, and type-soundness violations.
	t.Logf("any-frontier %d/%d (pin ref %d) — informational", anyRows, valueRows, pinnedAnyFrontierRows)
}

// residualHasAnyFrontier reports whether any carrier in a residual stack
// is Any-bounded (strict Any carrier or dynamic(Any)) — the "gave up to
// Any" shape the frontier metric counts.
func residualHasAnyFrontier(stk []eng.Value) bool {
	for _, v := range stk {
		if v.Parent != nil && v.Parent.Equal(eng.TAny) {
			return true
		}
	}
	return false
}

type fileCount struct {
	name string
	n    int
}

// topFiles returns the n spec files with the most Any-frontier rows,
// most first — a worklist of where narrowing would pay off most.
func topFiles(byFile map[string]int, n int) []fileCount {
	out := make([]fileCount, 0, len(byFile))
	for name, c := range byFile {
		out = append(out, fileCount{name, c})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].n != out[j].n {
			return out[i].n > out[j].n
		}
		return out[i].name < out[j].name
	})
	if len(out) > n {
		out = out[:n]
	}
	return out
}
