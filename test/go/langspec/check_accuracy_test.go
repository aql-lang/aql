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
	pinnedFalsePositives     = 0   // value rows the checker wrongly errors on — none. was 2 — −2 recursion.tsv:71,72 DYNAMIC-SCOPE references (a `g` reading the caller's param `n`; a body-local `def acc2` read across a recursive frame): AQL is dynamically scoped, so a fn body run at CALL time sees names bound in the dynamic call chain, which a lexical undefined-word analysis flags. RescueForwardRefDiagnostics now clears such a fn-body undefined_word iff a fn that BINDS the name (as a param or body-local def, recorded in CheckState.FnBinders) can actually REACH the reading fn through the call graph (CheckState.FnCallGraph, built from nested AnalyseFnBody entries including a recursive fn's self-edge) — the SOUND condition the reverted "bound anywhere in the pass" rescue (PR #209 review P1) lacked: a name merely bound by an unrelated fn that never calls the reader (`def f fn [[] [x]]  def g fn [[x:Integer] [1]]  f`) stays flagged (it errors at run). Params/captures are frame-lifetime so the answer is exact for them; a body-local binder is sound for the recursion idiom (the def precedes the reaching call), with a narrow documented residual when a local is bound only AFTER the reaching call. Pinned by TestDynamicScopeUndefinedRescue (both positives + the unreachable-binder negative). was 3 — −1 macro.tsv:24 hand-hygiene macro (`def g (gensym) … def unquote g …`): `gensym` lacked RunInCheckMode, so in the check pass it produced no result and the macro expansion spliced `def <empty>` → invalid_word_name. `gensym` is a pure per-registry counter mint, so it now runs in check mode: the expansion binds a real `tmp$G<n>` name, and the check-time / runtime counters stay in lockstep so the compiled expansion is byte-identical to the interpreted one (differential green). was 4 — −1 error.tsv:25 `do [1 div 0] convert Map e`: an integer div/mod by a STATICALLY-ZERO divisor raises at runtime, but the check-mode ReturnsFn produced a numeric carrier, so `do` typed a Number (not the caught Error) and `convert Map Number` failed no_signature. div/mod now model a static-zero-integer-divisor as DIVERGENCE (empty residual, like `raise`), so an enclosing `do` catches it as an Error carrier and the row types cleanly; Float division-by-zero is IEEE inf/nan and excluded. Soundness holds at 12. was 5 — −1 generics.tsv:63 `Pkg.Box of [Integer]`: a generic SCHEMA exported from a module and read back through `Pkg.Box` was carrier-stripped by toCarrier (which preserved FnDef / Disjunct / Module payloads but not *TypeSchemaInfo), so IsTypeSchema went false and `of` rejected it no_signature. toCarrier now preserves the schema payload (the parameters + body `of` instantiates), same as the other type-definition payloads. Soundness holds at 12, unflagged at 192. was 7 — −2 generic-fn body rows (generics-fn.tsv:48 `repeat` recursive generic, :54 `unbox` generic higher-order body): the declaration-time abstract check bound an UNCONSTRAINED type-parameter (`gen [T]`, `[b:T]`) to a STRICT carrier of the placeholder node, so a body op the bare `T` "couldn't justify" (`b dot value`, `1 add (repeat …)`) failed no_signature — the old §9.4 strict-generics stance. Since an unconstrained parameter is `extends Any`, a value of it is statically ANY type; bind a dynamic(Any) carrier so body ops match gradually (mirroring the `Any`-param path), deferring the real check to the per-call analysis with concrete args. Bounded (`T extends C`) params stay strict, and a genuine undefined word in a generic body is still flagged (design/GENERICS.10.md §9.4 revised; TestGenCheckDeclarationTimeBody updated). Soundness holds at 12, unflagged at 192. was 9 — −2 closure-return rows (def-node-binding.tsv:55 `def f (mk 7) f`, generics-fn.tsv:49 `def add5 (mka 5)`): a fn DECLARED to return a Function whose body produces a CONCRETE closure value (a real FnDefInfo — a returned lambda) was surfacing an ABSTRACT Function carrier from the declared-return substitution, which has no FnDefInfo — so `def f (mk 7)` bound a non-dispatchable value and the later `f` reference failed undefined_word. buildFnBodyReturnsFn now surfaces the concrete closure residual (cloned for a fresh ID, like getNodeReturns' concrete list/map fold) when the declared return is Function/FnDef and the aligned body value is a concrete fn; sound (the real closure is what runs) and the differential / prod / status gates stay green. was 11 — −2 module-minilang.tsv §? register-then-use rows (`mini count` / `mini dbl`): a fn dispatched through the mini-kind (module) path surfaces a GRADUAL dynamic(Any) body residual (a value read off a shapeless carrier — `m.n` over an abstract Map, or `cache get (src)`), which the __RC return-check (engine.go) compared with a raw `.Is(exp)` that ignores the Dynamic flag — so the fn wrongly failed its own declared-return check even though the identical body called directly passes. The return check now lets a dynamic residual optimistically conform in check mode (mirroring the parameter boundary, where a dynamic arg matches a concrete slot); sound because the compiled/interpreted RET re-checks the real runtime value (Dynamic is never set on a concrete value), and a CONCRETE return mismatch (Dynamic=false) still errors. was 12 — −1 generics-fn.tsv §8 `loose 1`: a FN whose declared return names a type parameter that no argument can infer (`def loose gen [T] fn [[x:Integer] [T] [x]]`) still RUNS — it returns whatever the body produces and the checker degrades the result to dynamic(Any) — unlike an uninferable `make` parameter, which cannot construct the instance and stays a hard error (generics.tsv:81). So the fn-return-only unbound_param is now a non-gating WARNING (a precision REPORT, matching the spec's "the checker reports the precision loss — Phase 5"), while `make`'s unbound_param keeps Error severity. was 23 (live 20, 3 headroom) — −8 across three sound check-mode fixes: (a) −6 error/case field-access rows: `do [raise …]` leaves an EMPTY static residual (raise produces no carrier), which doListReturnsFn modelled as a strict Carry<Any> — but `do` is the error-CATCHING word, so at runtime it surfaces the caught error as an Error VALUE; modelling the empty-residual case as an Error carrier lets a downstream `e dot code`/`.message`/`convert Map e` match the [_ Error] accessor sigs instead of failing no_signature (sound — a genuinely-empty body's empty runtime stack is still covered by a one-longer checked stack); (b) −1 module-emitlang register-then-use: EmitLang.register gained the check-mode ReturnsFn + identity-idempotent install that parselang/minilang already have, so `emit <name>` statically resolves the dynamically-registered emitter (the runtime double-register still errors); (c) −1 higher-order `scan [add] []`: a statically-EMPTY collection runs the body zero times, so scanReturnsFn now returns an empty-list carrier without analysing the never-executed body (which had reported no_signature on `add` over the empty element type). was 27 — −2 unpack module forms + −2 minilang register-then-use. A module's exports / a registered kind's fn are STATICALLY declared, so the checker resolves them (NOT a runtime-only effect — the same exports `import` already resolves in check). `unpack 'aql:mod'` / `unpack Export 'aql:mod'` gained RunInCheckMode + kept-concrete module-name strings, binding the exports unqualified so a later bare word resolves. `MiniLang.register <name> <fn>` gained an idempotent check-mode install (ReturnsFn mirroring parselang-register), so a later `mini <name>` resolves the kind. was 28 — −1 user-types.tsv: an abstract carrier already TAGGED as a predicate-refine (subset) type now satisfies that type's param nominally (depScalarUnifier.Match trusts the tag for an abstract carrier — the contract guarantee — and only runs the value-level predicate on a CONCRETE value), so a `Big`-returning fn flows into a `Big` param. was 31 — −3 class.tsv singleton types: `typeof` of a CONCRETE argument now returns the precise type literal in check mode (a concrete-gated ReturnsFn), so `def One (typeof (const 1))` gets a valid type body (the singleton) instead of a bare `Type` carrier the def-type validator rejected; `is` over it stays precise. A non-concrete arg keeps the bare Type carrier (blast radius off every runtime typeof). was 35 — −4 recursion.tsv mutual-recursion forward refs (isod↔isev): checkFlagsError now mirrors lang.(*AQL).Check by calling RescueForwardRefDiagnostics before reading diagnostics, so a fn-body reference to a sibling def below it (flagged undefined_word at eager body-analysis time, then rescued once the def exists) is no longer over-counted — the CLI never showed these. was 47 — −12 path-modifier.tsv: a dispatch modifier (usurp / stack-args / forward-args / force-arity — the `/u` `/s` `/f` `/N` desugarings) over a stored fn-ref read via dot-access (`m.a` where `m = {a:add/r}`) is statically dynamic(Any) because getNodeReturns cannot narrow a dispatch-bearing field; the handlers now return a gradual Function carrier in check mode for a non-concrete arg the real wrapper declined, instead of an illegal_ref. was 65 — −18 compare.tsv type-algebra over user types: `tor` (type union) modelled its check-mode result with a branch-merge JoinCarriers that mishandled the nil-Parent None literal and halted the tape (`(String tor None)`); it now mirrors TorHandler's disjunct (unionType). `enum [...]` gained a ReturnsFn that runs its pure handler so the result carries its DisjunctInfo members. And toCarrier now preserves a Disjunct/Enum value's DisjunctInfo (as it does FnDef/Module), so `def Maybe (String tor None)` / `def Color enum […]` are valid type bodies in check mode and `tcmp`/`is`/`typeof` over them keep their members. was 105 — −40 flex.tsv: `flex` / `node` declared the supertype `Returns: [TNode]`, so a FlexMap/FlexList result never matched the `Flex*` overload of a downstream `set`/`append`/`push`/`pop`/`sort`/`each` and failed no_signature; both now carry a ReturnsFn modelling the precise flex subtype from the input's node family (Map→FlexMap, List→FlexList, Xml→FlexXml), mirroring FlexDeepCopy. was 114 — −9 from the client-module gradual-dispatch fixes (design/CLIENT-FIXES-2026-06-24.md): a polymorphic word over a dynamic receiver no longer narrows the binding to one overload's slot (slice's String-vs-List data arg) nor commits to one overload's return (add's temporal vs String/Number over two fully-unknown operands), a None-vs-value branch merge is gradual (the `if (nd eq none) [build] [nd]` builder sentinel), and a dynamic arg to a declared concrete param is analysed at the param's type. MEASURED on the integrated branch (was a loose pin of 123 with lower live actual): net effect of Stage-3 module-parselang:23 landing on THIS branch's engine — the register-then-parse rows now check clean (parser installed at CHECK time via a check-mode ReturnsFn on parselang-register), but the body-bearing fn-VALUE dispatch fix (spliceFnValueCheckResult → buildFnBodyReturnsFn → AnalyseFnBody) emits body diagnostics on a few fn-value calls under generalised carrier args that the legacy inline-splice did not, so the live count settles at 114. This is PRECISION only (sound — TestSpecCompiledDifferential + TestCheckTypeSoundness both pass, every row still RUNS correctly), a documented tradeoff for clearing the refusal (refusals 5→4). Pin is the measured floor, below the prior loose 123.  Earlier chain: −5 module-parselang register-then-parse value rows (§1 L16-18, §2 L21-22): ParseLang.register now installs the parser at CHECK time (a check-mode ReturnsFn on parselang-register), so `parse <name>` resolves and the checker no longer wrongly flags those rows — Stage-3 module-parselang:23; was 120 — +3 module-parse register-then-parse rows: the checker can't see the runtime Parse.register, so `parse <name>` flags, exactly like the module-parselang register-then-parse rows; was 119 — +1 module-minilang §7 register-compiled-then-use row, like the MiniLang.register rows the checker can't see the runtime register for; was 114 — +5 module-parselang register-then-parse rows; was 132 — macro installs in check mode; was 122 — make returns a value carrier of the made type, not the type literal)
	pinnedUnflaggedErrorRows = 169 // ERROR rows the checker is silent on. was 166 — +3 module-minilang.tsv §mi2 runtime-only rows from the grammar-form rewrite (MiniLang.micron now takes a GRAMMAR, not a regexp): a gate token colliding with an EARLIER registration's (per-registry STATE — the builtin-leaf collision IS check-flagged from the map alone), a RULED grammar whose prefix gate claims a span the rules then reject (a +m runtime parse fact), and a builder consumed by MiniLang.micron passed to Parse.register afterwards (the parse_grammar_done seal is registry state). The other new §mi2 negatives ARE check-flagged via miniMicronLitReturns' lenient dry pass — the retired pattern-String form, a token-less grammar, a user-set options.tag, a builtin-leaf token collision, and an invalid @/…/ token regexp (the dry pass replays the scratch steps); the false-positive pin is untouched (still 0). was 163 — +3 module-minilang.tsv §mi2 runtime-only rows: the duplicate MiniLang.micron registration (per-registry STATE the checker cannot see), and the two builder-failure rows (a wrong-typed result and a make error inside the registered fn — both fire only when the +m literal parses in the shell and the builder runs). The three §mi2 SHAPE negatives (builtin kind / non-Micron kind / invalid pattern) ARE check-flagged via miniMicronLitReturns' lenient dry pass; the false-positive pin is untouched (still 0). was 162 — +1 module-parse.tsv §5 double-Parse.spec row (`Parse.register sp7 g  Parse.spec g {…}`): the parse_grammar_done seal is runtime BUILDER STATE — the grammar carrier is not concrete under analysis, so the checker cannot know it was already consumed, exactly like the parselang loop-register row pinned below. The other six §5 shape negatives ARE check-flagged (parseSpecReturns' map-decidable dry pass — unknown section, mistyped token/action/matcher/abnf/rule entries); the false-positive pin is untouched (still 0). was 160 — +2 micron.tsv §7 cross-kind cmp restriction rows (`(make Emailon …) cmp (make Pathon …)` / `cmp (make Urlon …)`): the [aql/incomparable] raise is the viaFamily verdict of the runtime Comparer walk (compareValuesClassified) over two FRESH-INSTANCE carriers, and the check-time incomparable diagnostic (OrderingReturnsFn) is deliberately gated to CONCRETE operands — the same abstract-carrier precision limit as the compare-restrict.tsv Bytes rows pinned below. The other 26 micron.tsv ERROR rows ARE check-flagged (CheckMicronConstruction on the scalar make sig, setMicronReturns' guaranteed-immutable diagnostic, getrMicronReturns' static-miss not_found); the false-positive pin is untouched (still 0). was 159 — +1 open-words.tsv §7b Point negative (`add p0 1` where p0 is a class instance built via an imported module's exported type): in check mode the module import binds gradually and the constructed instance is a dynamic carrier, so the add call matches optimistically instead of flagging no_signature. The same dispatch-over-dynamic-module-values precision limit as patrun.tsv:L40 / module-rand — corpus growth, not lost coverage; the false-positive pin is untouched (still 0). was 157 — +2 new compare-restrict.tsv Bytes ordering rows from main (`(convert Bytes "a") lt "b"` / `cmp 1`): the `convert Bytes` operand is an ABSTRACT Bytes carrier in check mode (convert does not concrete-fold), and the check-time incomparable diagnostic (OrderingReturnsFn) is deliberately gated to CONCRETE operands — judging abstract carriers' family disjointness statically needs a comparer-aware walk (user `behave` comparers can bridge families) and is future precision work, not a merge fix. Pure corpus growth, not lost coverage; the false-positive pin is untouched (still 0). was 172 — −15 from check-mode class/Resource construction validation (CheckMakeConstruction, core_make.go, wired via makeObjReturns on the [Ideal Map] make sig): a CONCRETE construction map is validated against the schema at check time — unknown field, missing non-defaulted field, and a concrete field value failing MakeClassFieldValue — with the byte-identical runtime messages. Value-dependent parts (a carrier map, a computed field value) stay with the runtime constructor; the false-positive pin is untouched (still 0). was 182 — −10 from the check-mode declared-return conformance check (checkBodyReturnConformance, core_helpers.go): a fn whose analysed body residual is PROVABLY DISJOINT from its declared return type (`def f fn [[a:Integer] [String] [a add 1]]` — Integer can never be a String) is now flagged at check time with the byte-identical runtime type_error text (returnTypeErrorText). Only impossibility flags: a value-dependent subset return (`[n add 1]` against `Big (Integer gt 10)`), a dynamic residual (gradual contract), a disjunct with any conforming alternative, a Function residual (the fn-value-call frontier), and a body whose analysis saw an undefined forward ref (mutual recursion) all stay with the runtime RET check — the false-positive pin is untouched (still 0). was 191 — net −9: the DSL register words (ParseLang / MiniLang / EmitLang) now surface an install failure from the check-mode ReturnsFn as a check diagnostic instead of discarding it, so a genuine double-register (or a collision with a built-in kind) is STATICALLY flagged — moving ~12 existing collision rows from unflagged→flagged. Offset by +3 new module-{emit,parse,mini}lang loop-register rows (`for 2 [register …]`): a loop re-executing the same register line is a genuine runtime double-register (idempotency now covers only the compiled check-install + VM-re-run double-exec, not runtime repeats), which is a RUNTIME-state fact the static checker cannot predict. was 192 — −1: modelling a static-zero-integer divisor as divergence (the div/mod ReturnsFn returning an empty residual, error.tsv:25 fix) also makes a bare integer div/mod-by-zero ERROR row statically visible — a downstream typed use of the (now absent) residual fails, so the checker flags a row it previously left to the runtime. Sound coverage gain, not a regression. was 189: +3 module-minilang §6 runtime-only errors: the `+hb/abc/` odd-length hex, `+bb/0102/` non-binary digit, and `+bb/0100110/` non-multiple-of-8 bit count all fail only when the hb/bb kind runs in the shell — a malformed hex/binary source is a runtime fact the static checker cannot predict, exactly like the §m / §jpq / §xp runtime-only mini_parse_error rows already pinned here. Pure coverage growth from the new hb/bb Bytes-literal kinds, not a checker regression; the false-positive pin is untouched (still 23). was 196: −7 from the ordering/connective check-mode soundness fix — concrete cross-family ordering (`0 cmp 'x'`, `lt 0 'x'`) now raises [aql/incomparable] at check time via OrderingReturnsFn, and a value-selecting connective over two concrete operands (`and 0 false`) concrete-folds to the EXACT selected operand type via foldOrJoin, so a downstream typed use (`add (and 0 false) 0`) now check-fails instead of being silently admitted; the false-positive pin is untouched (still 23). +4 module-debug runtime-only errors (Debug.assert's assertion_failure, Debug.todo's not_implemented, Debug.parse's parse_error, Debug.sig's unknown-word debug_error — each surfaces only when the word runs in the shell, like the other runtime-only rows here; the body-running words' bad-body rows are check-flagged via their ReturnsFn so they do NOT add here). was 192: +5 module-log.tsv runtime-only rows: the span-mismatch row (ending a non-active span is a runtime STATE check), the `Log.with-span … [raise …]` re-raise (a body error the checker cannot predict), the Log.register name collision (sink-exists fires only when the handler runs), and the Log.set-level / Log.set-format / Log.add-sink invalid-atom validation rows — all value/state-dependent like the other runtime-only rows here; measured floor after merge. +3 module-bin.tsv §11 runtime-only decode errors: `BinUtil.base64-decode '!!notbase64'` / `BinUtil.hex-decode 'xyz'` / `BinUtil.hex-decode '6'` raise a decode_error only when the codec runs in the shell — the malformedness of a base64/hex literal is a runtime fact the static checker cannot predict, exactly like the other runtime-only rows pinned here. Pure coverage growth from the new encoders, not a checker regression; the false-positive pin is untouched. +5 accessor.tsv runtime-only strict-miss errors: `getr`/`dotr` (and the `!.` sugar) raise on a missing map key, an out-of-bounds list index, a missing class field, or a missing Store key — all runtime facts the static checker cannot predict, exactly like the other runtime-only rows pinned here. The accessor matrix that added them is pure coverage growth, not a checker regression: the same rows error identically on the pre-split build, and the false-positive pin is untouched. +1 module-struct.tsv:78 runtime-only error exposed by the `tor` check-mode fix: `StructUtil.reify Shape {r:2.0}` over a union type `Shape = Circle tor Square` raises reify_error "a union target needs a $class key" only when reify runs in the shell — the static checker cannot know `{r:2.0}` lacks the discriminator. It was previously "flagged" ONLY by the spurious tor-halt the fix removed, exactly like the flex runtime-only rows below. +5 flex.tsv runtime-only errors exposed by the `flex` ReturnsFn fix: `set 3 99 (flex [1 2 3])` / `set 9 …` / `set 0 9 (flex [])` are out-of-bounds INDEX errors and `pop (flex [])` / `shift (flex [])` are empty-collection errors — all raise only when the op runs in the shell, which the static checker cannot predict. They were previously "flagged" ONLY as a side effect of the spurious no_signature the flex false-positive fix removed; the static checker correctly admits them now (the index/emptiness is a runtime fact), exactly like the other runtime-only rows pinned here. +2 module-parselang §4 runtime-only errors: with ParseLang.register now installing the parser at check time, `parse calc {file:…}` / `parse calc {nope:1}` resolve and dispatch cleanly statically — the parse_file_unsupported / parse_bad_source errors fire only when the registered parser resolves the source in the runtime shell, exactly like the §5 ini and §8 json/toml runtime-only source/syntax rows already pinned here — Stage-3 module-parselang:23; +5 module-parselang §9 aontu runtime-only errors: a unification conflict / kind mismatch / unresolvable reference / incomplete kind / missing-brace syntax error all surface only when the aontu parser runs in the shell, which the static checker cannot predict, exactly like the §8 json/toml syntax-error rows; +3 module-emitlang runtime-only emit_unsupported errors: a non-tabular csv shape (a bare Map / a scalar) and a TOML-unrepresentable scalar (None) surface only when the encoder walks the value in the runtime shell, which the static checker cannot predict — the encoder twin of the existing emitlang register/runtime rows; +8 module-emitlang runtime-only errors: emit_unknown_lang on an unbound namespace, emit_unsupported (toml top-level List, xml non-Xml top, ini deep nesting), and the four EmitLang.register validation rejections (bad name / bad signature / emit_ prefix / built-in collision) all resolve only when the emitter runs in the shell, which the static checker cannot predict — the emit twin of the module-parselang register-then-parse runtime rows; +3 module-parse §4 runtime-only errors: a malformed ABNF / a bad action ref / a built-in-kind collision all surface only when Parse.register or the parser runs in the shell, which the static checker cannot predict; +1 module-minilang §xp runtime-only error: a malformed XPath (`mini xp '//['`) fails only when the kind runs in the shell, which the static checker cannot predict — the XPath twin of the jp/jq runtime-error rows; +2 module-minilang §jpq runtime-only errors: a malformed jp/jq query fails only when the kind runs in the shell, which the static checker cannot predict; +3 module-minilang §m runtime-only errors: m's unknown-variable / division-by-zero / malformed-formula all surface only when the kind runs in the shell, which the static checker cannot predict; +2 module-parselang §8 syntax-error negatives: a malformed json/toml source fails only when the parser runs in the shell, which the static checker cannot predict; +2 module-parselang §5 ini runtime-only errors: parse_file_unsupported / parse_bad_source resolve the source in the runtime shell, so the static checker cannot predict them; +2 record.tsv §V runtime-only errors: a numeric record field now rejects a non-numeric (String/Boolean) source at make time, which the static checker does not predict; +1 patrun.tsv runtime-only error the checker can't predict statically — a non-Scalar pattern value to `add`; was 138 — fold-over-map result typing surfaces one more error; June 2026; was 136 — +3 module-minilang §7 runtime-only errors: mini_no_transducer / mini_bad_compiler / mini_bad_name; was 131 — +5 module-parselang runtime-only errors; was 132)
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
					if os.Getenv("AQL_LOG_UNFLAGGED") != "" {
						t.Logf("UNFLAGGED %s:L%d: %s", e.Name(), lineNum, input)
					}
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

const pinnedTypeSoundnessViolations = 8 // was 12 — −4 loop residual-arity: a STATICALLY-COUNTED `for` leaves the SPREAD of its per-iteration residual on the stack (`for 3 ['x']` → three strings, `for [1 4] [7 8]` → six ints), but the checker modelled it as a single List carrier — a variadic APPROXIMATION the bytecode lowerer requires. forCarrierAnalyse now returns the exact spread (per-iteration residual repeated `count` times, bounded by loopSpreadResidualCap) in PLAIN-CHECK mode only (gated on !Emit.Active(), so the recording path keeps the List and the bytecode differential is untouched). Repeating the fixed-point join `count` times is a sound over-approximation — a break/continue loop runs FEWER iterations, and a shorter runtime stack is still covered by the longer checked one (control.tsv:20, recursion.tsv:63/64, and forward-barrier.tsv:23 whose else-less-if Disjunct now covers the aligned Integer). REMAINING 8 are precision/dynamic limits, not the loop cluster: forward-barrier.tsv:83 (an else-less-if's Disjunct residual corrupts the subsequent MathUtil.sqrt module-wrapper dispatch — needs concrete-condition folding, which needs comparison ops to fold concretes) and recursion.tsv:53 (a recursive fn declared `[]`-void whose body leaks a per-frame value — modelling the spread needs recursion unrolling) are the two hard stragglers; the other six (class.tsv:88 singleton widened by set, corpus-core.tsv:61 enum identity, generics.tsv:75 make-generic→Map, module-rand.tsv:37 + patrun.tsv:40 dynamic-dispatch containers, user-types.tsv:115 subset keeps base tag) are checker-MORE-precise-than-runtime or inherent dynamic-dispatch limits. was 13 — module-rand:38: a Rand.with-seed instance method dispatch (`def r (Rand.with-seed N)  r.<method> …`) was a residual-mismatch violation (shapeless `r` → method result Any vs the runtime's concrete value); with-seed's check-mode shaped-carrier ReturnsFn resolves the method wrapper's typed result, removing it (−1, sound). was 15 — IO.write now declares its true Returns (the target path / Stream handle), making the write/read spec rows sound (−2); was 14 — +1 patrun.tsv:L40, the dynamic dispatch of a value pulled out of a Patrun: `find` returns a dynamic Any (the matcher is a dynamic-dispatch container, so a stored value's static type is unknowable), so calling the found lambda leaves the checker residual [dynamic(Any) Map] while the runtime yields [Integer] — a precision limit surfacing as an apparent mismatch, not a checker soundness bug; was 15 — do [body] now reports the body's full residual stack (matching doListHandler) instead of only the last value; was 16 — pop/shift TList sigs now declare their true 2-value Returns ([TList, TAny]); was 22 — corrected six wrong aql:time-util inner-native Returns (weeks/days/until/since → CalDuration, tz-offset → String, total-ms → Float) to match their handlers; was 159 — typeCovered now applies the runtime `is Type` membership rule to type-as-value actuals (typeof / type-algebra / make-of-a-type / record shapes), which a raw actual.Parent.ConformsTo misjudged (a type literal's Parent is the DENOTED type's lattice parent), removing 127 spurious flags and exposing the genuine residue (wrong module-time Returns annotations, do/for/pop arity, dynamic method dispatch)

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

const pinnedAnyFrontierRows = 345 // INFORMATIONAL ONLY (reported via t.Logf; the RATIO is gated below). was 303 — measured 354 on the post-merge corpus BEFORE the June-2026 checker-review work (the 303 was stale, like the COMPILED_STATUS snapshot), then −9 from the Tier-3 tand/DepScalar check-mode fixes (TandReturnsFn + toCarrier preserving DepScalarInfo): type-algebra meets and named-refinement values now keep their bounds instead of degrading to dynamic Any. was 255 — +48 the precision cost of the client-module gradual-dispatch fixes (design/CLIENT-FIXES-2026-06-24.md): a polymorphic word over a fully-unknown (dynamic-Any) operand now widens its result to the gradual union incl. Any (add of two unknowns), a None-vs-value branch merge is gradual, and a dynamic arg to a concrete param is analysed at the param's type — each loosens matching, ending more clean rows on a dynamic carrier rather than a committed concrete type. Sound and a net win: the GATED accuracy ratchet (false positives) improved 114→105 over the same corpus, and TestCheckTypeSoundness held at 12. was 254 — +1 error.tsv: a new `… error [ get code ]` regression row (the None-built raise message, bloom report §1) reads a generic error's `code` field, which is statically Any (the checker can't know the code) — sound. was 253 — +1 object.tsv:134 `(convert Map o) get a`: convert now models its result as a fresh VALUE carrier OF the target type (ReturnsFreshInstance, like make) instead of the bare target type LITERAL (ReturnsIdentity). The freeze yields a Map carrier whose element types are unknown, so the downstream `get a` soundly ends Any. Net improvement: the same convert fix moved 3 spec rows from errored→clean (denominator 2379->2382), eliminating the `no_signature` false positives on arithmetic over a `convert Float`ed scalar (voxgig-aql/bloom-filter regression report §2). was 254 — module-rand:38: a Rand.with-seed instance row that previously ended Any (shapeless `r` → `r.<method>` → Any) now resolves the method wrapper's typed result via with-seed's check-mode shaped carrier (−1, sound). Prior combined floor 254 = this branch's -5 concrete-list/map field fold (254->249) AND module-parselang:23's +5 register-then-parse rows (254->259), additive. was 249 — -5: getNodeReturns now folds a CONCRETE list/map field read to the stored CONTAINER value (cloned, fresh ID) rather than its bare type carrier, so `(m get "k")` over a concrete map yields a concrete list/map whose downstream `size` / index / element access types precisely instead of ending Any. was 246 — +8 module-parselang §9 aontu rows: `parse aontu` returns Any (like the rest of the parser family), so a parsed value accessed with get / typeof ends Any. was 245 — +1 from merging origin/main (denominator 2332->2341, +9 new value rows; the top-10 per-file breakdown is unchanged from the §5b state, so the +1 is a new main row outside the top files, not a regression from this branch). was 243 (actual 241 + headroom) — +4 module-test.tsv rows from the CheckState-ownership refactor (design/module-fn-checkstate-ownership.1.md §5b): module-preamble fn bodies (run-spec/run-cases/run-case) now run IN CHECK MODE under the parent pass, so their results are CARRIERS (Any) instead of being concrete-folded by real execution during the check pass. The pre-refactor precision was ill-gotten — it came from running side-effecting module-fn bodies for real during Check() (the same path that leaked test-record), which §5b+§5c correctly stop. §6 (compiling the framework code-body words) is expected to recover precision by giving those results real return types. was 229 — +14 module-parselang tabnas-family rows (json/jsonic/json5/jsonc/csv/toml/yaml/xml/zon/markdown/feed §6–§7, accessed with get); was 225 — +4 module-parselang ini rows (parse returns Any, so a parsed value accessed with get ends Any). pristine origin/main already measures 208 against the prior 194 pin — a pre-existing precision regression on main (the per-file breakdown here is identical to main except for patrun.tsv), NOT introduced by this branch; +17 patrun.tsv rows whose `find` returns a dynamic Any (the matcher is a dynamic-dispatch container, so a found value's static type is unknowable). was 194 — fold over a map narrows to the accumulator type (the list ReturnsFns are collection-agnostic); was 196; was 201 (list-index), 208 (struct-util), 217 (filter), 241 (case), 286 (item #4 field access); see TestCheckAnyFrontier

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
	// The Any-frontier COUNT is informational (pinning it turns every corpus
	// addition into pin churn), but its RATIO is gated: the fraction of clean
	// rows that end at an Any carrier must not grow past the ceiling below.
	// New corpus rows move both numerator and denominator, so ordinary corpus
	// growth passes; only a systemic precision REGRESSION (a fix that widens
	// results to Any across the board) trips it. Lower the ceiling as
	// precision fronts land; never raise it without a documented decision.
	t.Logf("any-frontier %d/%d (pin ref %d) — count informational, ratio gated", anyRows, valueRows, pinnedAnyFrontierRows)
	const anyFrontierRatioCeilingPct = 12 // measured 345/3050 ≈ 11.3% June 2026 (pre-review baseline was 354/3045 ≈ 11.6%), +0.7pt headroom
	if valueRows > 0 && anyRows*100 > valueRows*anyFrontierRatioCeilingPct {
		t.Errorf("any-frontier ratio %d/%d (%.1f%%) exceeds the %d%% ceiling — a systemic "+
			"precision regression widened results to Any; find the change that grew the "+
			"frontier instead of raising the ceiling",
			anyRows, valueRows, float64(anyRows)*100/float64(valueRows), anyFrontierRatioCeilingPct)
	}
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
