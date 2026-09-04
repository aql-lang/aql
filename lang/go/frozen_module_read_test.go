package lang

// Frozen module reads (the unit-bake freeze discipline): a CONCRETE
// module-scope binding read inside a fn/closure UNIT analysis bakes into the
// unit (a const, or splice-fired tokens) that re-runs on every call, while
// the interpreter re-resolves the name per call (the documented module-level
// dynamic semantics, lang/go/CLAUDE.md "Closures and Capture"). A later
// module-scope rebind therefore made the compiled program diverge:
//
//	def x 1  def f fn [[y:Integer] [Integer] [x add y]]  f 0  def x 2  f 0
//	interpreter: 1 2      compiled (before the fix): 1 1   ← MISCOMPILE
//
// The fix: NoteFrozenRead records such reads (engine.go stepWord), and
// NotifyNameRebound marks the program uncompilable on a later module-scope
// rebind — interpreter fallback, correct result. STORED-REF units (service
// handlers, spawn bodies) are exempt: their rebind safety is the precise
// per-ref poisoning (TestCompiledStoredHandlerFreezeRedefine), which keeps
// the rest of the program compiled.

import (
	"fmt"
	"strings"
	"testing"
)

// frozenReason is the latch's own text (compiler/go/emit.go, NotifyNameRebound's
// fn-unit arm). Each row below asserts it EXACTLY, and that is not decoration:
// MarkUncompilable is FIRST-REASON-WINS, so a row that only checks `reason != ""`
// passes whenever ANY other refusal fires first — including one introduced by the
// very change under review. Until 2026-09-04 nothing in the tree named this
// string, so the latch could have been deleted outright with these rows still
// green. Assert the text, or the pin is not a pin.
func frozenReason(name, bake string) string {
	return "module binding " + name + " rebound after a fn unit baked its " + bake
}

// TestModuleReadRebindRefusesAndMatches — the miscompile pin: the rebind
// program refuses with the named reason and the fallback matches the
// interpreter's documented dynamic-read result.
func TestModuleReadRebindRefusesAndMatches(t *testing.T) {
	// Legacy refusal+fallback-parity contract: pins the one-release
	// BORU_COMPILE_FALLBACK=1 hatch behavior (Stage J flipped the default
	// to compile_refused; migrate this contract or retire it with the hatch).
	t.Setenv("BORU_COMPILE_FALLBACK", "1")
	// bound is the name the row rebinds and bake is the artifact the unit
	// froze — the refusal must name both, because the three bakes are
	// repaired by three different mechanisms.
	cases := []struct{ src, bound, bake string }{
		// The documented module-dynamic case (scalar read baked as a const).
		{`def x 1  def f fn [[y:Integer] [Integer] [x add y]]  f 0  def x 2  f 0`, "x", "value"},
		// The splice twin: the macro payload fires into the unit's tokens at
		// analysis time; a rebind would leave the old tokens frozen.
		{`def op (quote [1 add 2])  def f fn [[n:Integer] [Any] [do [n drop word op]]]  f 0  def op (quote [10 mul 4])  f 0`, "op", "value"},
		// The UNDEF twin. `undef` reaches the SAME NotifyNameRebound as `def`
		// (basic/go/native_definition.go), and until 2026-09-03 no case here
		// exercised it. Measured by suppressing the guard at emit.go and
		// rebuilding: the program compiles to `7 7` where the interpreter
		// raises `undefined_word` — the unit's baked const outlives the very
		// binding that produced it, so the compiled body cannot tell that `k`
		// stopped existing.
		{`def k 5  def f fn [[] [Integer] [k add 2]]  f  undef k  f`, "k", "value"},
		// The CROSS-FAMILY twin, and the one a live operand ALONE cannot fix.
		// Measured the same way: compiles to `7 7` where the interpreter
		// raises `type_error: f: return value 1: expected Integer, got
		// ProperString` over the value `'x2'`.
		//
		// What separates it from the first case is the EMITTED SHAPE. The unit
		// lowers to two `PUSH_CONST`s feeding a MONO `CALL_NATIVE add (Number,
		// Number)` — a signature chosen at compile time FROM the frozen value.
		// `OpCallNative` invokes that BAKED signature's handler directly
		// (eng/go/vm.go) and never rematches, so making the read live without
		// re-deciding the signature does not reproduce the interpreter at all:
		// the interpreter rematches to `add`'s String arm and concatenates,
		// and only f's declared `[Integer]` return catches it. Whatever
		// eventually makes this read live therefore owes a signature DECISION
		// as well as an operand, and this row is what refuses to let that step
		// supply the operand while keeping the stale selection.
		{`def k 5  def f fn [[] [Integer] [k add 2]]  f  def k "x"  f`, "k", "value"},
		// The TYPE twin, and the row that says the discipline is about
		// BINDINGS rather than about values. A type name lives in the SAME
		// single binding store (lang/go/CLAUDE.md "Registry Bindings"), so a
		// module-scope type read inside a unit freezes exactly as a value
		// read does — resolveOperand's bare-type-node arm returns a
		// typeOperand and the unit lowers to OpPushType carrying the node's
		// compile-time IDENTITY.
		//
		// Until 2026-09-04 BOTH halves of the discipline missed it. The read
		// never travels stepWord's simple-value substitution branch (the
		// type-literal arm returns first), so NoteFrozenRead was never
		// attempted; and `def`/`undef`'s capitalised arms returned before
		// every recorder notification, so NotifyNameRebound was never called
		// for a type name at all. Measured on the DEFAULT lane — plain `boru
		// run`, no flags, no -force-compile:
		//
		//	interpreted -> true false      compiled -> true true
		{`def T Integer  def f fn [[] [Boolean] [5 is T]]  f  def T String  f`, "T", "type"},
		// The type UNDEF twin, worse in the same way the value one is: an
		// ALIAS binding is not Minted, so undef retires no lattice node
		// (basic/go/native_definition.go's undef type arm) and the unit's
		// baked ID keeps resolving after its binding is gone.
		//
		//	interpreted -> raises undefined_word      compiled -> true true
		{`def T Integer  def f fn [[] [Boolean] [5 is T]]  f  undef T  f`, "T", "type"},
		// The CALL-TARGET family, and the plainest shape in this file: define
		// a helper, define a caller, call it, redefine the helper. That is the
		// documented late-binding half of NUR097 and it is what a REPL session
		// or a hot reload does, so it is not an edge case the corpus merely
		// happens to lack.
		//
		// The unit lowers to `TAIL_CALL_USER f1` naming a SPECIFIC compiled
		// unit. The bind twin replays `def-replace helper` correctly beside
		// it — verified by disassembly — and the call target does not move,
		// which is the §6.5 lesson exactly: the twins fix where the registry
		// is, and this gate defends what is in the bytecode. Measured on the
		// default lane before this arm existed:
		//
		//	interpreted -> 2 101      compiled -> 2 2
		{`def helper fn [[x:Integer] [Integer] [x add 1]]  def use fn [[x:Integer] [Integer] [helper x]]  ` +
			`use 1  def helper fn [[x:Integer] [Integer] [x add 100]]  use 1`, "helper", "call target"},
		// Transitively: the rebind is two call levels below the unit that
		// froze it. `1 9` interpreted, `1 1` compiled.
		{`def a fn [[][Integer][1]]  def b fn [[][Integer][a]]  def c fn [[][Integer][b]]  c  ` +
			`def a fn [[][Integer][9]]  c`, "a", "call target"},
		// The SIGNATURE-UNDEF twin. `undef g (fnsig …)` reaches UndefFnHandler,
		// which called UninstallFnSigs and returned — no recorder notification
		// of any kind, the same shape as the type arms. Compiled `1 1` where
		// the interpreter raises undefined_word.
		{`def g fn [[][Integer][1]]  def f fn [[] [Integer] [g]]  f  undef g (fnsig [[] [Integer]])  f`,
			"g", "call target"},
	}
	for _, c := range cases {
		src := c.src
		a, err := New()
		if err != nil {
			t.Fatalf("New: %v", err)
		}
		prog, reason, _, cerr := a.CompileCheck(src)
		if cerr != nil {
			t.Fatalf("CompileCheck(%q): %v", src, cerr)
		}
		if prog != nil {
			t.Errorf("%q: a module rebind after a unit baked the binding must refuse; compiled instead", src)
		} else if want := frozenReason(c.bound, c.bake); reason != want {
			t.Errorf("%q: refusal must be the frozen-read latch's own; want %q, got %q", src, want, reason)
		}
		gotC, compiled, errC, gotI, errI := runBothEngines(t, src)
		if compiled {
			t.Errorf("%q: expected the interpreter fallback", src)
		}
		if fmt.Sprint(gotC) != fmt.Sprint(gotI) || fmt.Sprint(errC) != fmt.Sprint(errI) {
			t.Errorf("%q: engine divergence: compiled=%v/%v interp=%v/%v", src, gotC, errC, gotI, errI)
		}
	}
}

// TestModuleReadNoRebindStillCompiles — the positive pair: the same fn over a
// NEVER-rebound module binding keeps compiling (the freeze is keyed on actual
// rebinds, not on "reads a module ref").
func TestModuleReadNoRebindStillCompiles(t *testing.T) {
	srcs := []string{
		`def x 1  def f fn [[y:Integer] [Integer] [x add y]]  f 0  f 10`,
		// The TYPE control, and the one that bounds the 2026-09-04 fix. The
		// new note fires on EVERY module-scope type read inside a unit, so
		// this row is what says the latch still needs a REBIND to trip: a
		// type read in a unit, called twice, must keep compiling.
		`def T Integer  def f fn [[] [Boolean] [5 is T]]  f  f`,
		// And the type read that is REBOUND but never read inside a unit —
		// the other side of the same bound. Nothing froze, so nothing refuses.
		`def T Integer  5 is T  def T String  5 is T`,
		// The CALL-TARGET controls, bounding the third arm the same way. A fn
		// called from a unit and never rebound keeps compiling —
		`def g fn [[][Integer][1]]  def f fn [[] [Integer] [g]]  f  f`,
		// so does SELF-recursion, which reads its own name inside its own unit
		// (the shape that would break every recursive fn if the note fired on
		// a name the analysis merely resolves) —
		`def fact fn [[n:Integer] [Integer] [if (n lte 1) [1] [n mul (fact (n sub 1))]]]  fact 5`,
		// and so does a rebind with no unit between it and the call.
		`def g fn [[][Integer][1]]  g  def g fn [[][Integer][2]]  g`,
	}
	for _, src := range srcs {
		a, err := New()
		if err != nil {
			t.Fatalf("New: %v", err)
		}
		prog, reason, _, cerr := a.CompileCheck(src)
		if cerr != nil || prog == nil {
			t.Fatalf("%q: must compile; reason=%q err=%v", src, reason, cerr)
		}
		gotC, compiled, errC, gotI, errI := runBothEngines(t, src)
		if !compiled || errC != nil || errI != nil || fmt.Sprint(gotC) != fmt.Sprint(gotI) {
			t.Errorf("%q: want compiled parity; compiled=%v gotC=%v errC=%v gotI=%v errI=%v",
				src, compiled, gotC, errC, gotI, errI)
		}
	}
}

// TestStaticSpliceBodiesCompile — closure bodies containing `word` splices
// over compile-time-known payloads compile NATIVELY: the check engine fires
// the splice during body analysis (`word` is a non-emitting projection) and
// the post-splice dispatches record into the unit. Unblocked by the
// multi-out/out-of-order residual work; pinned here so the shapes never
// regress to "code-body word do (Stage 2)".
func TestStaticSpliceBodiesCompile(t *testing.T) {
	cases := []struct{ src, want string }{
		{`def xs (quote [1 add 2])  do [word xs]`, "[3]"},
		{`def op (quote [mul 2])  do [5 word op]`, "[10]"},
		{`def twice word [dup add]  do [5 twice]`, "[10]"},
		{`def op (quote [mul 2])  [1 2 3] each [word op]`, "[[2 4 6]]"},
		// a computed payload that CONST-FOLDS (literal index) composes.
		{`def i 0  def ops [quote [1 add 2]]  do [word (ops get i)]`, "[3]"},
	}
	for _, c := range cases {
		a, err := New()
		if err != nil {
			t.Fatalf("New: %v", err)
		}
		prog, reason, _, cerr := a.CompileCheck(c.src)
		if cerr != nil || prog == nil {
			t.Errorf("%q: splice body must compile; reason=%q err=%v", c.src, reason, cerr)
			continue
		}
		if strings.Contains(prog.Disassemble(), "FALLBACK") {
			t.Errorf("%q: must compile natively, not island", c.src)
		}
		gotC, compiled, errC, gotI, errI := runBothEngines(t, c.src)
		if !compiled || errC != nil || errI != nil ||
			fmt.Sprint(gotC) != fmt.Sprint(gotI) || fmt.Sprint(gotC) != c.want {
			t.Errorf("%q: want %s compiled with parity; compiled=%v gotC=%v errC=%v gotI=%v errI=%v",
				c.src, c.want, compiled, gotC, errC, gotI, errI)
		}
	}
	// A RUNTIME-computed payload (the def evaluates its list body) compiles
	// via the dyn-body backstop: the def's OpBindDynScope twin re-binds the
	// RUNTIME value (its computed source event promotes to a frame local
	// under DynEnv — the OpMakeList shape), and the body's sub-run splices
	// the live binding exactly as the interpreter does.
	src := `def xs [add 1 2]  do [word xs]`
	b, _ := New()
	prog, reason, _, _ := b.CompileCheck(src)
	if prog == nil {
		t.Errorf("%q: the dyn-body backstop must compile a computed splice payload; reason=%q", src, reason)
	}
	gotC, compiled, errC, gotI, errI := runBothEngines(t, src)
	if !compiled || errC != nil || errI != nil || fmt.Sprint(gotC) != fmt.Sprint(gotI) || fmt.Sprint(gotC) != "[3]" {
		t.Errorf("%q: want [3] compiled with parity: compiled=%v gotC=%v errC=%v gotI=%v errI=%v",
			src, compiled, gotC, errC, gotI, errI)
	}
}

// TestComputedEachBodyStaysRefused — a COMPUTED body on a non-CompileDynBody
// higher-order word (each) keeps the refusal with fallback parity: the island
// cannot bake a carrier code body (its tokens carry the island's program), and
// each declares no dyn-body backstop. Pins the island's non-bakeable
// NoEvalArgs decline.
func TestComputedEachBodyStaysRefused(t *testing.T) {
	// Legacy refusal+fallback-parity contract: pins the one-release
	// BORU_COMPILE_FALLBACK=1 hatch behavior (Stage J flipped the default
	// to compile_refused; migrate this contract or retire it with the hatch).
	t.Setenv("BORU_COMPILE_FALLBACK", "1")
	src := `def op (quote [mul 2])  def f fn [[b:List] [List] [[1 2 3] each b]]  f op`
	a, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	prog, reason, _, cerr := a.CompileCheck(src)
	if cerr != nil {
		t.Fatalf("CompileCheck(%q): %v", src, cerr)
	}
	if prog != nil || reason == "" {
		t.Errorf("%q: computed each body must refuse; got prog=%v reason=%q", src, prog != nil, reason)
	}
	gotC, compiled, errC, gotI, errI := runBothEngines(t, src)
	if compiled || errC != nil || errI != nil || fmt.Sprint(gotC) != fmt.Sprint(gotI) {
		t.Errorf("%q: fallback parity broke: compiled=%v gotC=%v errC=%v gotI=%v errI=%v",
			src, compiled, gotC, errC, gotI, errI)
	}
}
