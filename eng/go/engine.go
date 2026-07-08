package eng

import (
	"fmt"
	"strconv"
	"strings"
)

// stackHeadroom is the extra capacity allocated beyond current need,
// so that most insert/splice operations avoid heap allocation.
const stackHeadroom = 8

// TraceCallback is called before each step of execution when tracing is enabled.
// It receives the step number, pointer position, full stack, and an annotation
// describing what happened on the previous step.
type TraceCallback func(step int, pointer int, stack []Value, note string)

// Engine is the AQL stack machine.
//
// Execution model: `tape` is a rewriting tape, not a LIFO stack. The
// input program is loaded onto it whole; `pointer` then walks forward
// and the value under the pointer dispatches (word → run, forward →
// advance, open-paren → group, ...). A dispatched word may splice
// values into the tape in place — consuming neighbours, inserting
// results. Execution ends when the pointer walks off the end; the
// residual tape is the result.
//
// The tape is a gap buffer (eng/go/tape.go): edits land at or near the
// pointer, and the gap riding the edit sites makes them O(edit size)
// instead of O(tail length) — the fix for the O(depth²) recursion cost
// measured in design/RECURSION-PERFORMANCE.10.md and benchmarked in
// design/TAPE-DATA-STRUCTURE.10.md.
type Engine struct {
	tape      *Tape
	pointer   int
	registry  *Registry
	trace     TraceCallback
	traceNote string          // annotation set during execution for the next trace call
	recorder  Recorder        // optional StackForm recorder; see stackform package
	stepLimit int             // hard cap on the Run loop; always positive, set by the New/NewTop constructors below
	marks     map[string]bool // active mark IDs (for mark/move control flow)
	source    string          // original source text for error reporting
	isTop     bool            // true for engines created via NewTop; an unhandled FlowCtrl at end-of-Run is an error here, propagates upward otherwise
	// elemEvalRecordable marks a SUB-engine spawned by autoEvalList/autoEvalMap
	// to evaluate CONTAINER ELEMENTS of a recordable container eval (top-level or
	// consumed): a map/list literal ELEMENT inside it is residual-evaluated
	// (consumed=false) yet its assembly is anchored by the enclosing recordable
	// container, so its OpMakeMap/OpMakeList may record — this is what lets a
	// nested spec literal (`def specs [ {…cases:[{…in:[(make …)]}…]…} … ]`, the
	// voxgig bloom class-instance spec list) resolve level by level instead of
	// breaking at the first map-in-list layer. The fn-body deferred-residual
	// hazard (a frame that already popped) cannot occur here: the enclosing
	// container eval runs in-frame at its own recordable site.
	elemEvalRecordable bool
	reuseTape          bool // when set, Run reloads the existing tape in place instead of allocating (the VM's reusable island engine)
	// flowUnwind marks a VM ISLAND engine: a break/continue that escapes the
	// island's tape (no enclosing loop there) must TEAR DOWN the island's live
	// spliced frames before returning — in the interpreter the frame and the
	// enclosing loop always share one tape (handleLoopBreak's unwindLiveFrames
	// runs at the loop), but the island boundary separates them, so the frame
	// cleanup (args pop, def truncation, capture teardown) would otherwise be
	// lost with the discarded tape. exitWithFlowCtrl then returns NO values —
	// the VM discards the signalled iteration's partials anyway (flowSignal) —
	// with the registry FlowCtrl flag left set for the VM to translate.
	flowUnwind bool
	// startAt is a one-shot start offset for the next Run: the leading
	// startAt input values are RESOLVED arguments (a callback's inputs, a
	// fn call's unnamed args) and enter as stack data below the pointer,
	// never re-stepped — the sub-engine twin of FrameOpenInfo.ArgSpan
	// (arguments are inert; design/ARG-SEMANTICS-UNIFICATION.0.md).
	// Consumed (zeroed) by Run so it cannot leak into a later reuse.
	startAt int
	// voidGroups records the candidate consumers of paren groups that
	// resolved to ZERO values in the current statement: the pending
	// word names sitting below such a group when it closed. A
	// following signature failure on one of those words is reported
	// as "argument expression produced no value" at the causing site
	// rather than as a generic mismatch — the blame-shift fix of
	// design/ERRORS.8.md §3 (VOXGIG B3). Cleared at every statement
	// boundary (stepEnd).
	voidGroups []string
}

// RecorderSkipper is an optional extension to Recorder. When a
// Recorder also implements RecorderSkipper, the engine can tell it
// to suppress the next N OnPushLit events — used by stepCloseParen
// (and other rewind paths) where the engine's main loop re-visits
// stack values that have already been emitted to the recorder via
// their original push event.
type RecorderSkipper interface {
	Skip(n int)
}

// Recorder receives events as the Engine executes a program. Used by
// the eng/go/stackform package to build a canonical strict-stack
// representation of a program (see design/PBT-PLAN.10.md and
// design/aql-bytecode-report.0.md). Nil by default; install via
// Engine.SetRecorder.
//
// Recorder is called at the two semantic actions that define a
// strict-stack form:
//
//   - OnPushLit fires when a source literal gets pushed onto the
//     stack at the current pointer. Handler return values that the
//     engine re-encounters at the pointer also trigger this — the
//     recorder is responsible for distinguishing source-literals
//     from handler-results via the `returns` count delivered by
//     OnCall (the next `returns` OnPushLit events are handler
//     outputs, not source data).
//   - OnCall fires AFTER a successful matchSignature dispatch and
//     handler invocation. `arity` is len(matched-sig.Args); `returns`
//     is the number of values the handler actually produced; `name`
//     is the word the caller invoked. Recorders typically suppress
//     the next `returns` OnPushLit events.
//
// Recorders that need to capture nested sub-programs (e.g. quoted
// list bodies passed as NoEvalArgs) install themselves on the
// sub-engine that runs them.
type Recorder interface {
	OnPushLit(v Value)
	OnCall(name string, arity, returns int)
}

// SetRecorder installs a Recorder on this engine. Pass nil to clear.
// Recorder events fire during Run(); see the Recorder interface
// for the semantics of each callback.
func (e *Engine) SetRecorder(r Recorder) { e.recorder = r }

// SetTrace installs a TraceCallback on this engine. Pass nil to clear.
// The callback fires once before every step of Run() with the step
// index, the pointer position, a snapshot of the tape, and the
// pending trace note. This is the seam debug tooling builds on to
// count steps, profile per-word cost, or drive interactive stepping
// (see design/DEBUG-MODULE.0.md §6.1); the existing RunTrace uses the
// same field internally. The cost is paid only when a callback is
// installed — a normal Run leaves e.trace nil and never snapshots.
func (e *Engine) SetTrace(t TraceCallback) { e.trace = t }

// Default step limits for the Run loop. Exposed as named constants so
// every Engine constructor names them explicitly — there is no
// "zero means default" sentinel on `stepLimit`; the field is always
// set to a positive value by the constructors below.
//
// These are runaway-program guards (a genuinely non-terminating program
// must fail rather than hang forever), NOT a budget real programs are
// expected to brush against. The old values (22222 / 2222) were tuned
// for the pre-gap-buffer engine, where deep work was quadratic and so
// self-limiting; they capped a plain `for` loop at ~3700 iterations.
// With the gap-buffer tape (design/TAPE-DATA-STRUCTURE.10.md) loops and
// tail recursion are linear, so the cap is raised to a generous ceiling.
//
// Calibration (measured): a `for` loop costs ~6 top-level steps per
// iteration, tail recursion ~24 per level; data-bulk words (each/sort/
// map ops) do their work inside handlers and cost O(1) top-level steps.
// 10M steps therefore admits ~1.6M loop iterations / ~400k recursion
// levels — far beyond any real workload — while a genuine infinite loop
// trips in a few seconds and within tens of MB of tape growth (a higher
// ceiling like 100M lets a runaway run ~100s and balloon to ~600MB
// before tripping, which is worse than the cure). When the cap IS hit it
// now raises an explicit `evaluation_limit` error (see the Run loop and
// evalParenGroupAt), never the old phantom "unmatched opening
// parenthesis". Callers that genuinely need more can raise the budget
// per engine.
const (
	DefaultStepLimit    = 10_000_000 // top-level engine cap
	DefaultSubStepLimit = 10_000_000 // sub-engine cap (autoEvalMap, CallAQL, etc.)

	// maxParenGroupSteps bounds a single paren-group evaluation in
	// evalParenGroupAt. Same role and ceiling as the Run-loop cap, for
	// the nested-evaluation path.
	maxParenGroupSteps = 10_000_000
)

// New creates an Engine with the given function registry.
// The returned engine uses the sub-engine step limit.
// Use NewTop for the top-level engine with a higher limit.
func New(registry *Registry) *Engine {
	return &Engine{registry: registry, stepLimit: DefaultSubStepLimit}
}

// NewTop creates a top-level Engine with the maximum step limit.
// isTop is set so an unhandled FlowCtrl signal at end-of-Run is reported
// as an error rather than propagating outward.
func NewTop(registry *Registry) *Engine {
	return &Engine{registry: registry, stepLimit: DefaultStepLimit, isTop: true}
}

// SetSource sets the original source text for error reporting.
// When set, AqlErrors include source extracts showing the error location.
func (e *Engine) SetSource(src string) {
	e.source = src
}

// effectiveSource returns the source text for error reporting.
// Prefers the engine's own source; falls back to the registry's.
func (e *Engine) effectiveSource() string {
	if e.source != "" {
		return e.source
	}
	return e.registry.Source
}

// reorderHint reports a "did you swap the arguments?" hint when the
// values the failed dispatch saw match one of fn's signatures under a
// non-identity PERMUTATION (decision DX report finding 2): the caller
// of `with-policy policy:String table:Map` who writes
// `with-policy table "collect"` gets the declared order spelled out
// instead of the forward-grouping hint, which points at parsing.
// Conservative: plain value sigs only (no quote/raw/form/type capture,
// no patterns), 2–4 args, first matching signature wins. Returns ""
// when no reorder explains the failure.
func (e *Engine) reorderHint(name string, fn *FnDefInfo) string {
	return reorderHintFor(name, fn, reorderCandidates(e.tape.Prefix(e.pointer)))
}

// reorderCandidates collects up to 4 plain values from the top of the
// stack (walking down, stopping at engine markers / words) — the tuple
// a failed STACK dispatch saw, in the assignment order matchSignature
// uses (top-first: sig[i] ↔ vals[i]).
func reorderCandidates(stack []Value) []Value {
	var vals []Value
	for i := len(stack) - 1; i >= 0 && len(vals) < 4; i-- {
		v := stack[i]
		if IsOpenParen(v) || IsForward(v) || IsWord(v) || IsEnd(v) ||
			v.Parent.ConformsTo(TMark) || v.Parent.ConformsTo(TMove) ||
			v.Parent.ConformsTo(TInternal) {
			break
		}
		vals = append(vals, v)
	}
	return vals
}

// reorderForwardCandidates collects up to 4 UNCLAIMED forward value
// tokens after the word — concrete literals only; words, parens, and
// markers stop the scan. The result is in SOURCE order, which is the
// assignment order the forward plan would have used (sig[i] ↔
// token[i]) — exactly the failing-tuple view reorderHintFor wants.
func reorderForwardCandidates(tape *Tape, pointer int) []Value {
	var written []Value
	for i := pointer + 1; i < tape.Len() && len(written) < 4; i++ {
		v := tape.At(i)
		if !IsConcrete(v) || IsWord(v) || IsParenExpr(v) || IsForward(v) ||
			IsOpenParen(v) || IsEnd(v) {
			break
		}
		written = append(written, v)
	}
	return written
}

// reorderHintFor is the shared probe behind the signature errors.
// written is the failing tuple in the ASSIGNMENT order the dispatch
// used (sig[i] ↔ written[i]): source order for forward tokens,
// top-first for stack values.
func reorderHintFor(name string, fn *FnDefInfo, written []Value) string {
	if fn == nil {
		return ""
	}
	for si := range fn.Signatures {
		sig := &fn.Signatures[si]
		n := sig.TotalArgs()
		if sig.Fallback || n < 2 || n > 4 || n != len(written) {
			continue
		}
		if len(sig.QuoteArgs) > 0 || len(sig.RawParens) > 0 ||
			len(sig.FormArgs) > 0 || len(sig.TypeArgs) > 0 || sig.Patterns != nil {
			continue
		}
		// The identity assignment must FAIL (otherwise this sig would
		// have matched) and some permutation must succeed.
		if assignsPositionally(written, sig, true) || !assignsPositionally(written, sig, false) {
			continue
		}
		got := make([]string, n)
		want := make([]string, n)
		params := make([]string, n)
		for i := 0; i < n; i++ {
			got[i] = ValueType(written[i]).Leaf()
			t := sigArgType(sig, i)
			want[i] = t.Leaf()
			if i < len(sig.Params) && sig.Params[i].Name != "" {
				params[i] = sig.Params[i].Name + ":" + t.Leaf()
			} else {
				params[i] = t.Leaf()
			}
		}
		return "no signature matches (" + strings.Join(got, ", ") +
			"); one exists for (" + strings.Join(want, ", ") +
			") — did you swap the arguments? expected: " + name + " " +
			strings.Join(params, " ")
	}
	return ""
}

// assignsPositionally reports whether written (caller order, =
// reversed sig order) satisfies sig either positionally (identity =
// true) or under SOME assignment of distinct written args to sig
// slots (identity = false; backtracking over n ≤ 4 slots).
func assignsPositionally(written []Value, sig *Signature, identity bool) bool {
	n := sig.TotalArgs()
	if identity {
		for i := 0; i < n; i++ {
			if !sigTypeMatches(written[i], sigArgType(sig, i)) {
				return false
			}
		}
		return true
	}
	used := make([]bool, n)
	var try func(slot int) bool
	try = func(slot int) bool {
		if slot == n {
			return true
		}
		t := sigArgType(sig, slot)
		for j := 0; j < n; j++ {
			if used[j] || !sigTypeMatches(written[j], t) {
				continue
			}
			used[j] = true
			if try(slot + 1) {
				return true
			}
			used[j] = false
		}
		return false
	}
	return try(0)
}

// sigError builds a detailed AqlError for a signature mismatch.
// It includes the word name, available signatures, and the actual
// types found on the stack near the word.
func (e *Engine) sigError(name string, fn *FnDefInfo, pos SrcPos) *AqlError {
	// A word starved by a VOID argument group (a parenthesised call in
	// its argument range that produced no value, recorded by
	// stepCloseParen) reports the causing expression, not the generic
	// mismatch (ERRORS.8.md §3).
	if verr := e.voidArgErrorFor(name, pos); verr != nil {
		return verr
	}
	detail := "no matching signature for " + name

	// Build hint with available signatures and actual stack types.
	var hint strings.Builder
	if fn != nil && len(fn.Signatures) > 0 {
		hint.WriteString("expected: " + name + " " + describeAllSigs(fn))
	}

	// Reorder hint: when the actual argument types match some declared
	// signature under a PERMUTATION, the arguments are almost certainly
	// swapped — say so, with the declared parameter order, and suppress
	// the forward-grouping hint, which would point at parsing (the
	// wrong fix). Decision DX report finding 2.
	reorder := e.reorderHint(name, fn)
	switch {
	case reorder != "":
		if hint.Len() > 0 {
			hint.WriteString("\n  = ")
		}
		hint.WriteString(reorder)
	case fn != nil && fn.HasForwardSigs():
		// Forward-precedence hint: when the word has forward-collecting
		// signatures, the most common cause of this error is that forward
		// collection ran into a following word (another call, a builtin)
		// before it could gather enough arguments — e.g. `inc inc 5` or
		// `f a g b`. The fix is PARENS — group the call so its result becomes
		// the argument; `end` / `;` only ends the statement and will NOT nest a
		// following word into a sub-call (a trailing `;` does not rescue
		// `print Decision.eval-cond c x`). Point at parens so they aren't left
		// to guess from a bare "no matching signature".
		if hint.Len() > 0 {
			hint.WriteString("\n  = ")
		}
		hint.WriteString("forward args for " + name +
			" may have run into the next word; group the call in parens so its " +
			"RESULT becomes the argument — (" + name + " …). `end` / `;` only ends " +
			"the statement — it does NOT turn a following word into a nested call.")
	}

	if e.tape.Len() > 0 {
		if hint.Len() > 0 {
			hint.WriteString("\n  = ")
		}
		hint.WriteString("stack: " + describeStackTypes(e.tape, e.pointer))
	}

	src := e.effectiveSource()
	return e.maybeAddFnShapeHint(makeAqlErrorAt("signature_error", detail, name, src, hint.String(), pos)).(*AqlError)
}

// isFnShapeTypedBindingContext reports whether the failing word is
// positioned at the body slot of a typed binding whose constraint is
// a function-shape type (TFnUndef).
//
// Deferred forward-arg dispatch works by inserting a Forward marker
// after the func word and letting the engine evaluate upcoming tokens
// normally; the marker holds the matched signature plus arg-count
// bookkeeping. When a fn dispatch fails inside that deferred
// collection, walking back through the stack finds the Forward marker
// for the outer collector. If that collector is `def` and its
// typed-name map (sitting at FuncIndex - CollectedArgs, since
// stepLiteral splices each collected arg in immediately before the
// func word) carries a fn-shape constraint, the failing dispatch is
// exactly the §7.2 "user wrote a fn name where they meant
// `(quote name)`" case.
func (e *Engine) isFnShapeTypedBindingContext() bool {
	if e.registry == nil || e.pointer == 0 {
		return false
	}
	for i := e.pointer - 1; i >= 0; i-- {
		if IsOpenParen(e.tape.At(i)) {
			return false
		}
		if !IsForward(e.tape.At(i)) {
			continue
		}
		fwd, _ := AsForward(e.tape.At(i))
		if fwd.FuncName != "def" || fwd.Sig == nil {
			return false
		}
		// def's typed-name sig is the only one with TMap at position 0.
		if fwd.Sig.TotalArgs() < 2 || !sigArgType(fwd.Sig, 0).Equal(TMap) {
			return false
		}
		// stepLiteral moves each collected forward arg to the slot
		// immediately before the func word, in collection order
		// (first-collected = deepest). So position 0's value sits at
		// FuncIndex - CollectedArgs, position 1 at FuncIndex - CollectedArgs + 1, etc.
		if fwd.CollectedArgs < 1 {
			return false
		}
		mapIdx := fwd.FuncIndex - fwd.CollectedArgs
		if mapIdx < 0 || mapIdx >= e.tape.Len() {
			return false
		}
		m, _ := AsMap(e.tape.At(mapIdx))
		if m == nil || m.Len() != 1 {
			return false
		}
		constraint, _ := m.Get(m.Keys()[0])
		if IsWord(constraint) {
			cw, _ := AsWord(constraint)
			if tv, ok := e.registry.ResolveTypedName(cw.Name); ok {
				constraint = tv
			}
		}
		return constraint.Parent.Equal(TFnUndef)
	}
	return false
}

// pendingForwardFunc returns the name of the nearest enclosing
// forward-collecting word (the function currently gathering arguments at
// the pointer), or "" if there is none before an open-paren barrier.
// Used to tailor error hints to the collecting context (e.g. a bare
// undefined word being collected as `def`'s body).
func (e *Engine) pendingForwardFunc() string {
	for i := e.pointer - 1; i >= 0; i-- {
		if IsOpenParen(e.tape.At(i)) {
			return ""
		}
		if IsForward(e.tape.At(i)) {
			fwd, _ := AsForward(e.tape.At(i))
			return fwd.FuncName
		}
	}
	return ""
}

// insufficientArgsError builds a detailed AqlError for forward argument
// collection failure (not enough arguments after the word).
func (e *Engine) insufficientArgsError(name string, expected int, pos SrcPos) *AqlError {
	detail := fmt.Sprintf("insufficient arguments for %s (expected %d forward args)", name, expected)
	hint := "stack: " + describeStackTypes(e.tape, e.pointer)
	src := e.effectiveSource()
	return makeAqlErrorAt("signature_error", detail, name, src, hint, pos)
}

// undefinedWordHint tailors the undefined_word hint to the two known
// causes worth pointing at:
//   - `def name foo` where foo is undefined: the user almost
//     certainly meant to bind foo's VALUE, not the bare word. `def`
//     does not auto-quote its body, so a bare word is dispatched (and
//     errors when undefined). Point at the fixes: `(foo)` to
//     evaluate, or `foo/q` to bind the name.
//   - The undefined name was pending next to a paren group that
//     produced NO value (`def r (returns-nothing …)` — the def
//     silently never bound, and this reference is the blame-shifted
//     victim). Name the real cause (ERRORS.8.md §3, VOXGIG B3).
func (e *Engine) undefinedWordHint(name string) string {
	if e.pendingForwardFunc() == "def" {
		return "did you mean `def … (" + name + ")` to bind its value, " +
			"or `def … " + name + "/q` to bind the name itself?"
	}
	for _, vg := range e.voidGroups {
		if vg == name {
			return "an earlier expression produced no value to bind to '" + name +
				"' — the def never happened; give that expression a return value"
		}
	}
	return ""
}

// voidArgErrorFor reports the §3 "argument expression produced no
// value" error when the failing word was a candidate consumer of a
// paren group that resolved to ZERO values in the current statement;
// nil otherwise so the caller falls through to its generic error.
// `def` gets the tailored message (with the bound name read from the
// nearest collected atom below the pointer), since a void value
// expression is its classic blame-shift shape (VOXGIG B3:
// `def r (returns-nothing 1)`).
func (e *Engine) voidArgErrorFor(name string, pos SrcPos) *AqlError {
	matched := false
	for _, vg := range e.voidGroups {
		if vg == name {
			matched = true
			break
		}
	}
	if !matched {
		return nil
	}
	src := e.effectiveSource()
	if name == "def" {
		n := "<name>"
		for i := e.pointer - 1; i >= 0; i-- {
			if IsOpenParen(e.tape.At(i)) {
				break
			}
			if IsAtom(e.tape.At(i)) {
				n, _ = AsAtom(e.tape.At(i))
				break
			}
		}
		return makeAqlErrorAt("def_error",
			"def: expression produced no value to bind to '"+n+"'",
			"def", src,
			"hint: the called word returns nothing — call it without def, or give it a return value",
			pos)
	}
	return makeAqlErrorAt("no_value_error",
		"argument expression produced no value for "+name,
		name, src,
		"hint: a parenthesised argument evaluated to nothing — give it a return value, or drop it from the call",
		pos)
}

// stampResultPos stamps pos onto handler-produced values that lack a source
// position: ReturnCheck markers (so a return-type error built later knows the
// call site) and freshly-constructed Function/FnDef values (so an anonymous
// `fn`/`afn` value carries its construction site for downstream errors). Only
// zero-Pos entries are touched, so values that already carry a position — a
// stored fn passed through, a literal — are left alone.
func stampResultPos(vals []Value, pos SrcPos) {
	if pos.Row == 0 {
		return
	}
	for i := range vals {
		if vals[i].Pos.Row != 0 {
			continue
		}
		switch {
		case IsReturnCheck(vals[i]):
			if rc, err := AsReturnCheck(vals[i]); err == nil && rc.Pos.Row == 0 {
				rc.Pos = pos
				vals[i] = NewReturnCheck(rc)
			}
		case vals[i].Parent.Equal(TFunction) || vals[i].Parent.Equal(TFnDef):
			vals[i].Pos = pos
		}
	}
}

// stampErrPos attaches the currently-dispatched word's position to a
// handler-produced AqlError that has none. A handler raises its error while
// the engine is executing a specific word (at the pointer), so that word's
// position is the genuine location of the failure — no text-search guess.
// Errors that already carry a position, and non-AqlError errors, are left
// untouched.
func (e *Engine) stampErrPos(err error) error {
	ae, ok := err.(*AqlError)
	if !ok {
		return err
	}
	if ae.Row == 0 {
		pos := e.currentPos()
		if pos.Row != 0 {
			ae.Row, ae.Col = pos.Row, pos.Col
			if pos.Src != "" {
				ae.Src = pos.Src
			}
			// The position comes from THIS engine's tokens, so this
			// engine's source is the text it points into — attach it
			// (with the originating file) so an error raised inside an
			// imported module renders with the module's excerpt and
			// filename rather than a bare position (decision DX report
			// finding 4).
			if ae.fullSource == "" {
				ae.fullSource = e.effectiveSource()
			}
		}
	}
	if ae.File == "" && ae.Row != 0 && e.registry != nil {
		ae.File = e.registry.BaseFile
	}
	return err
}

// returnCountError builds a detailed AqlError for wrong number of return
// values. The detail text is shared with the VM via returnCountErrorText.
func (e *Engine) returnCountError(funcName string, expected, got int, pos SrcPos) *AqlError {
	src := e.effectiveSource()
	return makeAqlErrorAt("type_error", returnCountErrorText(funcName, expected, got), funcName, src, "", pos)
}

// validateReturnTypes checks the top nret residual values (results[extra:])
// against a ReturnCheck's declared return types, returning the first mismatch
// as an AqlError (nil when all conform). Extracted from stepCloseParen so that
// hot path stays under the cyclomatic-complexity gate.
//
// Uses the membership predicate v.Is(exp) — the SAME question the parameter
// boundary asks (sigTypeMatches → v.Is) — so a type's Behavior governs both
// ends symmetrically: a predicate refine runs its predicate on the way out
// (subset semantics), a bare refine stays nominal (newtype), and
// builtins/objects are unchanged (v.Is ≡ v.Parent.ConformsTo on concrete
// values). See design/REFINE-NEWTYPE-VS-SUBSET.10.md.
func (e *Engine) validateReturnTypes(rc ReturnCheckInfo, results []Value, extra int) error {
	for k, exp := range rc.Returns {
		got := results[extra+k]
		// A GRADUAL (dynamic) residual optimistically conforms to any declared
		// return in check mode — the checker genuinely does not know the type
		// (e.g. a value read off a shapeless carrier, `m.n` over an abstract
		// Map), so gating here would be a false positive. This mirrors the
		// parameter boundary, where a dynamic arg matches a concrete slot, and
		// stays sound because the compiled/interpreted RET re-checks the real
		// runtime value (Dynamic is never set on a concrete runtime value).
		// Without this, a fn dispatched through a path that surfaces a dynamic
		// body residual (module / mini-kind dispatch) wrongly fails its own
		// declared-return check while the identical body called directly passes.
		if got.Dynamic && e.registry != nil && e.registry.Check.IsActive() {
			continue
		}
		if !got.Is(exp) {
			return e.returnTypeError(rc.FuncName, k+1, exp, got, rc.Pos)
		}
	}
	return nil
}

// returnTypeError builds a detailed AqlError for a return type mismatch. The
// detail/hint text is shared with the VM via returnTypeErrorText.
func (e *Engine) returnTypeError(funcName string, index int, expected *Type, got Value, pos SrcPos) *AqlError {
	detail, hint := returnTypeErrorText(funcName, index, expected, got)
	src := e.effectiveSource()
	return makeAqlErrorAt("type_error", detail, funcName, src, hint, pos)
}

// currentPos returns the source position of the value at the pointer — the
// token currently being processed — or the unknown SrcPos when the pointer
// is out of range. Engine-side error builders use it so an error is located
// at the token that triggered it.
func (e *Engine) currentPos() SrcPos {
	if e.pointer >= 0 && e.pointer < e.tape.Len() {
		return e.tape.At(e.pointer).Pos
	}
	return SrcPos{}
}

// syntaxError builds a detailed AqlError for a syntax error.
func (e *Engine) syntaxError(msg, token string) *AqlError {
	src := e.effectiveSource()
	return makeAqlErrorAt("syntax_error", msg, token, src, "", e.currentPos())
}

// runtimeError builds a detailed AqlError for a runtime error.
func (e *Engine) runtimeError(code, detail, word, hint string) *AqlError {
	src := e.effectiveSource()
	return makeAqlErrorAt(code, detail, word, src, hint, e.currentPos())
}

// evalLimitError reports that evaluation hit the step-count guard before
// the program finished — the explicit, honest diagnosis for a runaway
// (non-terminating or pathologically deep) program, replacing the
// phantom "unmatched opening parenthesis" the old silent break produced.
func (e *Engine) evalLimitError(limit int) *AqlError {
	return e.runtimeError("evaluation_limit",
		fmt.Sprintf("evaluation exceeded the step limit of %d — the program ran too long (an infinite loop or unbounded recursion?)", limit),
		"",
		"if this is a legitimately long computation, raise the limit via the engine's step budget; otherwise check for a loop or recursion that never terminates")
}

// tapeExhaustedError reports that the tape hit its growth ceiling — the
// loud failure for unbounded consumption (a runaway splicing onto the
// tape without bound). Distinct from evalLimitError, which is the
// step-count (CPU) guard; this is the memory guard.
func (e *Engine) tapeExhaustedError() *AqlError {
	return e.runtimeError("tape_exhausted",
		fmt.Sprintf("evaluation tape exhausted its growth ceiling of %d entries — the program consumed unbounded space (an infinite loop or unbounded recursion?)", e.tape.MaxCap()),
		"",
		"raise the tape size via options (initial size / grow count / growth factor) for a legitimately large program; otherwise check for a loop or recursion that never terminates")
}

// tapeWarn forwards a tape capacity warning to the registry's error
// writer. Wired as the Tape's warn sink so 90/95/99% crossings surface
// without the tape importing io.
func (e *Engine) tapeWarn(msg string) {
	if e.registry != nil && e.registry.ErrOutput != nil {
		fmt.Fprintf(e.registry.ErrOutput, "aql: warning: %s\n", msg)
	}
}

// traceSigStr formats a signature as "name(type, type) prec=N" for trace annotations.
func traceSigStr(name string, sig *Signature) string {
	args := make([]string, sig.TotalArgs())
	for i := range args {
		args[i] = sigArgType(sig, i).String()
	}
	return name + "(" + strings.Join(args, ", ") + ")"
}

// Run executes the input values through the stack machine and returns
// the residual stack — there is no single "result value". After the
// pointer walks off the end, end-of-input cleanup runs (resolve
// pending forwards, reject stray `(`, strip mark/move markers,
// auto-evaluate leftover lists/maps); whatever Values remain are
// returned. Callers decide the shape they want: take [0] for a single
// value, keep the slice for a list, or splice it back into a parent
// tape.
// resolveAtomReferents walks a loaded program and stamps each atom that has
// no referent yet with a snapshot of what its name is currently bound to,
// when such a binding exists. It recurses into list payloads so quoted
// atoms nested in code/data lists are covered. Atom identity is unaffected
// (the referent is metadata; see AtomPayload). Names with no current binding
// are left unresolved — that is the honest "bound only at runtime" case.
func resolveAtomReferents(r *Registry, vals []Value) {
	for i := range vals {
		switch data := vals[i].Data.(type) {
		case AtomPayload:
			if data.Referent != nil {
				continue
			}
			if bound, ok := r.Defs.Top(data.Name); ok {
				vals[i] = SetAtomReferent(vals[i], bound)
			}
		case ListPayload:
			// FlexListData is deliberately not handled: this walk runs
			// on loaded program tapes, and flex nodes exist only at
			// runtime — they can never appear here.
			resolveAtomReferents(r, data.Elems)
		}
	}
}

func (e *Engine) Run(input []Value) (result []Value, runErr error) {
	// Count this interpreter activation against the registry so a compiled
	// RunProgram can see (at its entry) that an interpreter run is in flight.
	// Balanced on every exit path, including panic unwind.
	e.registry.enterInterpRun()
	defer e.registry.exitInterpRun()

	// Track this engine as the current one for on-demand stack
	// introspection (Debug.stack). Defer-balanced like enterInterpRun.
	e.registry.pushEngine(e)
	defer e.registry.popEngine()

	// Last-resort panic guard at the top-level engine boundary. A bug in
	// any handler or in the step loop should surface to the user as a
	// clean AQL error, never as a goroutine stack trace. Only the
	// outermost (NewTop) engine recovers; sub-engines let the panic
	// propagate so it unwinds to here with the original stack intact for
	// the debug detail. Errors returned normally are untouched.
	if e.isTop {
		defer func() {
			if rec := recover(); rec != nil {
				result = nil
				runErr = makeAqlErrorAt("internal_error",
					fmt.Sprintf("internal engine error: %v", rec),
					"", e.effectiveSource(),
					"this is a bug in AQL; please report it", e.currentPos())
			}
		}()
	}

	// Truth-over-symptom guard: once any tape edit hit the growth
	// ceiling (and was dropped to avoid an out-of-bounds write), every
	// later behaviour runs on a TRUNCATED tape — downstream errors are
	// phantoms of the drop (a starved ReturnCheck's "expected 1 return
	// value(s), got 0", the genre the recursion pins documented), and a
	// "successful" completion may have silently lost results. Whatever
	// this Run was about to report, the truthful diagnosis is
	// tape_exhausted.
	defer func() {
		if e.tape != nil && e.tape.Exhausted() {
			result = nil
			runErr = e.tapeExhaustedError()
		}
	}()

	// Push a scoped context Store whose prototype is the parent context.
	parent := e.registry.Contexts.Top()
	e.registry.Contexts.Push(parent)
	defer e.registry.Contexts.Pop()

	// In static type-check mode, convert concrete literal values to
	// carriers before execution. The same dispatch/matching machinery
	// then runs over carrier values; execMatch short-circuits handler
	// calls to push carrier return values declared on the signature.
	if e.registry.Check.IsActive() {
		es := e.registry.Check.Recorder()
		es.bindRegistry(e.registry) // back-pointer for returned-closure compilation
		pre := input
		input = StripToCarriers(input)
		es.RememberStrippedOriginals(pre, input)
	}

	// Post-parse referent resolution: stamp each /q-style atom in the
	// program with a snapshot of what its name refers to, for any name
	// already bound when the program loads (pre-installed module/global
	// defs). Names bound only later, during execution, stay unresolved here —
	// the `quote` word captures those at quote time instead. Top engine only
	// (sub-engines run fragments already walked) and not in check mode (the
	// program has been stripped to carriers). Stamping happens on a private
	// copy of the input BEFORE the tape is built, so the caller's slice is
	// never mutated and the tape constructor owns a fully-prepared program.
	prog := input
	if e.isTop && !e.registry.Check.IsActive() {
		prog = make([]Value, len(input))
		copy(prog, input)
		resolveAtomReferents(e.registry, prog)
	}

	// Load the program onto the gap-buffer tape with bounded growth
	// (NewTapeWith copies, so later splices never touch the caller's input
	// slice). The tape grows at most N times by factor M and then fails
	// loudly — see TapeConfig / the tape_exhausted check below. The
	// registry's TapeConfig (zero value = defaults) flows through
	// unchanged so resolve() applies the initial-size floor.
	// A reusable island engine reloads its tape in place (no per-island
	// allocation); a hot fallback island in a loop would otherwise spin
	// up a fresh tape every execution. Falls back to a fresh tape when
	// the existing buffer is too small. Per-run scratch state is cleared
	// so nothing leaks across reuses.
	if !(e.reuseTape && e.tape != nil && e.tape.Reload(prog)) {
		e.tape = NewTapeWith(prog, e.registry.TapeConfig, e.tapeWarn)
	}
	// Per-run scratch state is cleared on EVERY entry (not only the
	// Reload branch) so a reused engine whose previous tape was too
	// small for this program — the fresh-tape fallback above — cannot
	// leak marks/void-group state from its prior run.
	e.marks = nil
	e.voidGroups = nil
	e.traceNote = ""
	e.pointer = e.consumeStartAt()

	// stepLimit is always set by the constructors (New / NewTop); the
	// defensive check that used to substitute a default if the field
	// was zero was load-bearing for callers that built Engine{}
	// directly, but no longer — the constructors are the only entry.
	limit := e.stepLimit
	completed := false
	for step := 0; step < limit; step++ {
		// Memory guard FIRST: a previous edit hit the tape's growth
		// ceiling (and was dropped to avoid an out-of-bounds write).
		// This must precede the completion check — a dropped FINAL
		// splice leaves the pointer past the truncated tape, which is
		// indistinguishable from normal completion and used to return
		// success with the results silently gone.
		if e.tape.Exhausted() {
			return nil, e.tapeExhaustedError()
		}
		if e.pointer >= e.tape.Len() {
			completed = true
			break
		}

		// Check-mode global step budget: abort the whole run
		// gracefully once exceeded. Emits one diagnostic and
		// then short-circuits every subsequent sub-engine too.
		if e.registry.Check.IsActive() {
			// -1 is the "unset" sentinel; resolve to the
			// project default. A literal 0 is honored as
			// "abort immediately" rather than treated as a
			// magic "use default."
			budget := e.registry.Check.StepBudget
			if budget == -1 {
				budget = DefaultCheckStepBudget
			}
			e.registry.Check.StepCount++
			if e.registry.Check.StepCount > budget {
				if !e.registry.Check.BudgetTripped {
					e.registry.Check.BudgetTripped = true
					e.registry.Check.AddDiagnostic(CheckDiagnostic{
						Code:   "step_budget_exceeded",
						Detail: fmt.Sprintf("check mode aborted: step budget of %d exceeded", budget),
					})
				}
				// The check-mode budget is a deliberate, already-reported
				// stop — not the runtime step-limit exhaustion below. Mark
				// it complete so the drain proceeds normally.
				completed = true
				break
			}
		}

		val := e.tape.At(e.pointer)

		if e.trace != nil {
			snapshot := e.tape.Snapshot()
			note := e.traceNote
			e.traceNote = ""
			e.trace(step, e.pointer, snapshot, note)
		}

		switch {
		case IsWord(val):
			if err := e.stepWord(val); err != nil {
				return nil, err
			}

		case IsForward(val):
			e.pointer++

		case IsOpenParen(val):
			e.stepPastOpenParen(val)

		case IsCloseParen(val):
			if err := e.stepCloseParen(); err != nil {
				return nil, err
			}

		case IsEnd(val):
			if err := e.stepEnd(); err != nil {
				return nil, err
			}

		case IsParenExpr(val):
			// A word-context ParenExpr (paren-nesting work, Step 2 —
			// design/PAREN-REPRESENTATION.9.md): expand it back to its
			// OpenParen … CloseParen marker span in place and let the
			// existing in-place collapse machinery evaluate it on THIS
			// engine. That keeps exact parity with the former marker
			// representation (recorder-transparent, same stack/registry
			// semantics) — the four contracts hold because markers already
			// honor them: errors propagate, defs leak, the OpenParen is a
			// stack barrier, and results flow out. Do not advance — the
			// OpenParen now sits at the pointer.
			//
			// Step 4: a codequote-captured ParenExpr (Quoted) is data, and a
			// raw-capture pending forward (pendingForwardWantsRawParen) wants
			// the paren collected as-is — both route through stepLiteral
			// (push data / collect the forward arg) rather than expanding.
			if val.Quoted || e.pendingForwardWantsRawParen() {
				if err := e.stepLiteral(); err != nil {
					return nil, err
				}
			} else {
				items, _ := AsParenExpr(val)
				e.tape.Splice(e.pointer, 1, expandParenExpr(items)...)
			}

		case IsReach(val):
			// A parsed Reach (dot-access node, m.a.b — Eval=true) evaluates
			// by lowering to its get/getr chain in place, exactly like the
			// ParenExpr it replaced. An inert reach (Eval=false, from `reach`)
			// or a codequote'd one (Quoted) is data — left via stepLiteral.
			if isEvalReach(val) && !e.pendingForwardWantsRawParen() {
				info, _ := AsReach(val)
				e.tape.Splice(e.pointer, 1, expandReach(info)...)
			} else {
				if err := e.stepLiteral(); err != nil {
					return nil, err
				}
			}

		case IsInterpString(val):
			result, err := e.evalInterpString(val)
			if err != nil {
				return nil, err
			}
			// Replace with the evaluated string but do NOT advance the
			// pointer. The resulting string value needs to go through
			// stepLiteral so forward collection works correctly.
			e.tape.Set(e.pointer, result)

		case IsXmlInterp(val):
			// Interpolated XML literal: evaluate the skeleton in place to
			// a concrete Node/Xml, then re-step it as the value (no pointer
			// advance) so forward collection sees a Node/Xml — mirrors the
			// IsInterpString case above.
			result, err := e.evalXmlInterp(val)
			if err != nil {
				return nil, err
			}
			e.tape.Set(e.pointer, result)

		case IsMark(val):
			e.stepMark(val)

		case IsMove(val):
			if err := e.stepMove(val); err != nil {
				return nil, err
			}

		case IsReturnCheck(val):
			e.pointer++

		case IsDefCleanup(val):
			e.stepDefCleanup(val)
			e.pointer++

		default:
			if val.Parent == nil && val.Behavior == nil {
				return nil, e.runtimeError("halt", fmt.Sprintf("undefined stack entry at position %d", e.pointer), "", "")
			}
			if err := e.stepLiteral(); err != nil {
				return nil, err
			}
		}

		// Flow-control signal raised during the step (by a break/
		// continue handler or by a sub-engine sharing this registry).
		// Try to resolve locally; if no enclosing loop is on this
		// tape, leave the flag set and bail out of the loop so an
		// outer Run frame can catch it.
		if e.registry.FlowCtrl != FlowNone {
			if e.handleFlowCtrl() {
				continue
			}
			return e.exitWithFlowCtrl()
		}
	}

	// If the loop exited naturally (pointer walked off the end) with a
	// signal still set, fall through to the same handler.
	if e.registry.FlowCtrl != FlowNone {
		return e.exitWithFlowCtrl()
	}

	// The loop ran out of step budget before the program finished. Report
	// it explicitly: the program was cut off mid-evaluation, so the drain
	// below would otherwise see a half-processed tape (e.g. an open paren
	// the run never reached) and blame a phantom "unmatched opening
	// parenthesis". This is the honest diagnosis instead.
	if !completed {
		return nil, e.evalLimitError(limit)
	}

	// Implicit end-of-input: resolve any pending forwards from the stack.
	if err := e.resolveOrphanedForwards(); err != nil {
		return nil, err
	}

	for i := 0; i < e.tape.Len(); i++ {
		if IsOpenParen(e.tape.At(i)) {
			return nil, e.syntaxError("unmatched opening parenthesis", "(")
		}
	}

	// Remove any leftover marks and moves from the stack.
	e.cleanMarks()

	// Auto-evaluate unquoted lists and maps on the final stack.
	// Lists are evaluated as sub-programs: [1 add 2] → [3].
	// Maps have their values evaluated recursively.
	// Values marked Quoted (by the quote word) are left as-is.
	if err := e.autoEvalStack(); err != nil {
		return nil, err
	}

	// Orphan GenSpec residue (generics plan D1/D2): a `gen [...]`
	// whose spec no constructor consumed leaves placeholder type
	// bindings behind — pop them and report the gen loudly. TOP-level
	// engines only: sub-engine Runs (paren groups, list/map arg
	// auto-evaluation) legitimately execute while a spec is pending
	// for the enclosing constructor — `refine Record [value:T]`
	// auto-evaluates its list arg in a sub-engine BETWEEN gen and the
	// refine handler.
	if e.isTop {
		if spec := e.registry.TakePendingGen(); spec != nil {
			PopGenBindings(e.registry, spec)
			if !e.registry.Check.IsActive() {
				return nil, makeAqlError("gen_without_constructor",
					"gen: parameter spec was not consumed by a type constructor",
					"gen", e.effectiveSource(),
					"hint: follow gen [...] with refine Record [...], class {...}, fnsig [...], or fn [...]")
			}
			// Check mode is lenient (no error), but the compiled stream
			// elides gen. Record a TERMINAL trap so a compiled program raises
			// the byte-identical gen_without_constructor error at this point
			// (orphan gen errors at end-of-run, exactly where this fires)
			// instead of refusing; if the trap can't be recorded (nested), keep
			// the blanket-refusal flag so the program falls back.
			if !e.registry.Check.Recorder().RecordTrap("gen_without_constructor",
				"gen: parameter spec was not consumed by a type constructor", "gen",
				"hint: follow gen [...] with refine Record [...], class {...}, fnsig [...], or fn [...]",
				e.currentPos()) {
				e.registry.Check.SuppressedRuntimeError = true
			}
		}
	}

	// Runtime uncalled-function residue (ERRORS.8.md §5, VOXGIG T1): a
	// named Function value placed by a FAILED dispatch that nothing
	// ever consumed. Higher-order uses consume the value, so they never
	// reach here; only at the top level — where no consumer can exist
	// anymore — does the residue become an error, with the original
	// call-site span. The same bug check mode names uncalled_function.
	if e.isTop {
		for i := 0; i < e.tape.Len(); i++ {
			v := e.tape.At(i)
			if v.FailedDispatch && (v.Parent.Equal(TFnDef) || v.Parent.Equal(TFunction)) {
				name := ""
				if info, ok := v.Data.(FnDefInfo); ok {
					name = info.Name
				}
				if e.registry.Check.IsActive() {
					// Check-mode equivalent: a FailedDispatch fn value still on
					// the stack at the top-level drain was never consumed (a
					// following higher-order word would have taken it). Flag it
					// now — the genuine wrong-order namespace-call footgun — as
					// the diagnostic, not a hard error.
					e.registry.Check.AddDiagnostic(CheckDiagnostic{
						Code:   "uncalled_function",
						Detail: "call to '" + name + "' matched no signature and was left on the stack as data",
						Word:   name,
						Row:    v.Pos.Row,
						Col:    v.Pos.Col,
					})
					continue
				}
				return nil, makeAqlErrorAt("uncalled_function",
					"call to '"+name+"' matched no signature and was left on the stack as data",
					name, e.effectiveSource(),
					"hint: check the call's argument types and arity — or use "+name+"/r to push the function as a value deliberately",
					v.Pos)
			}
		}
	}

	// Drain any Undefined-Atom values left on the stack. Outside check
	// mode `stepWord` errors on undefined words so this loop is a
	// no-op. Under CheckMode `stepWord` already emitted the diagnostic
	// at the source token; here we only need to replace any dangling
	// Undefined atoms with `Any` carriers so the residual stack stays
	// type-clean for downstream consumers of CheckResult.Stack.
	e.drainUndefinedAtoms()

	return e.reconcileTopResidual(e.tape.TakeAll()), nil
}

// drainUndefinedAtoms replaces dangling Undefined atoms with Any carriers
// (check mode only — outside check mode stepWord errors on undefined words,
// so this is a no-op). Each carrier remembers which NAME the atom carried:
// an analysis-time-undefined read inside a fn body can still be a
// DYNAMIC-SCOPE reference bound by a live frame at run time (a recursive
// base case reading the previous frame's body-local — recursion.tsv:71);
// resolveOperand's dynScopeRescue lowers it to a runtime lookup iff a
// binder fn reaches the reader.
func (e *Engine) drainUndefinedAtoms() {
	for i := 0; i < e.tape.Len(); i++ {
		und := e.tape.At(i)
		if !und.Undefined || !e.registry.Check.IsActive() {
			continue
		}
		c := NewCarrier(TAny)
		if a, aerr := AsAtom(und); aerr == nil && a != "" {
			e.registry.Check.Recorder().NoteDefRead(c.ID, a)
		}
		e.tape.Set(i, c)
	}
}

// reconcileTopResidual reconciles the top-level program residual the same
// way fn-body summaries are (carrier.go stripZeroOutResiduals at
// StartFnCompile finish): a trailing 0-output statement guard — `if cond
// [printstr …] [printstr …]` — registers a phantom None carrier but
// produces 0 runtime values, so the residual must skip it. Recording-active
// only; the uncompilable case nets 0 at the source (if2/if3ReturnsFn).
func (e *Engine) reconcileTopResidual(out []Value) []Value {
	if e.isTop && e.registry.Check.IsActive() {
		return stripZeroOutResiduals(e.registry, out)
	}
	return out
}

// resolveOrphanedForwards handles end-of-input by resolving pending forwards.
func (e *Engine) resolveOrphanedForwards() error {
	for attempt := 0; attempt < 222; attempt++ {
		fwdIdx := -1
		for i := 0; i < e.tape.Len(); i++ {
			if IsForward(e.tape.At(i)) {
				fwdIdx = i
				break
			}
		}
		if fwdIdx < 0 {
			return nil
		}

		fwd, _ := AsForward(e.tape.At(fwdIdx))
		funcIdx := fwd.FuncIndex
		collectedCount := fwd.CollectedArgs
		stackArgCount := fwd.StackArgs

		// Remove the forward marker.
		e.tape.Remove(fwdIdx)
		if fwdIdx < funcIdx {
			funcIdx--
		}

		// Try stack match or create curry list.
		e.curryOrStack(funcIdx, collectedCount, stackArgCount)

		// Retry from the current pointer position.
		for step := 0; step < 100; step++ {
			if e.pointer >= e.tape.Len() {
				break
			}
			val := e.tape.At(e.pointer)
			switch {
			case IsWord(val):
				if err := e.stepWord(val); err != nil {
					return err
				}
			case IsCloseParen(val):
				if err := e.stepCloseParen(); err != nil {
					return err
				}
			case IsEnd(val):
				if err := e.stepEnd(); err != nil {
					return err
				}
			case IsForward(val):
				e.pointer++
			case IsOpenParen(val):
				e.pointer++
			default:
				if err := e.stepLiteral(); err != nil {
					return err
				}
			}
			// Propagate any flow-control signal raised by the
			// step; the outer Run frame will resolve it.
			if e.registry.FlowCtrl != FlowNone {
				return nil
			}
		}
	}
	return nil
}

// rawParenForward reports whether any of fn's signatures captures a forward
// ParenExpr RAW at sig position pos (RawParens[pos]). See
// design/PAREN-REPRESENTATION.9.md Step 4.
func rawParenForward(fn *FnDefInfo, pos int) bool {
	if fn == nil {
		return false
	}
	for i := range fn.Signatures {
		if fn.Signatures[i].RawParens != nil && fn.Signatures[i].RawParens[pos] {
			return true
		}
	}
	return false
}

// rawFormForward reports whether any of fn's signatures captures the forward
// operand at sig position pos as a raw FORM (FormArgs[pos]) — the macro
// raw-capture mode. Like rawParenForward, it gates preEvalParens so a paren /
// reach at that position is left unevaluated. See design/MACROS-PHASE1.10.md §3.
func rawFormForward(fn *FnDefInfo, pos int) bool {
	if fn == nil {
		return false
	}
	for i := range fn.Signatures {
		if fn.Signatures[i].FormArgs != nil && fn.Signatures[i].FormArgs[pos] {
			return true
		}
	}
	return false
}

// bindsReferent reports whether the named word is a BINDER whose operand
// slot copies a word's binding rather than its expansion. `def y xs` with
// xs splice-bound must rebind the marker itself — the new name ALIASES the
// splice — because expanding there would lose the referent (and code-bearing
// splices already alias on def by skipping expansion, so this keeps data and
// code splices consistent at binders). This is the one deliberate exception
// to the `f w ≡ f (w)` equivalence; write `def y (xs)` to force expansion.
// def is frozen (reserved_word), so the name is a reliable identity.
func bindsReferent(name string) bool {
	return name == "def"
}

// capturesForward reports whether any of fn's signatures captures the
// forward operand at sig position pos STRUCTURALLY — /q word-name capture
// (QuoteArgs), raw paren capture (RawParens), macro form capture (FormArgs),
// or type-literal capture (TypeArgs) — rather than as an ordinary value.
// Gates the splice-word paren expansion in resolveForwardArgs: capture slots
// take the token itself (the word's name, the raw form) regardless of any
// def binding, so the `f w ≡ f (w)` equivalence deliberately does not apply
// there (`quote vs` captures the NAME vs even when vs is splice-bound —
// word-splice spec §3).
func capturesForward(fn *FnDefInfo, pos int) bool {
	if fn == nil {
		return false
	}
	for i := range fn.Signatures {
		sig := &fn.Signatures[i]
		if sig.QuoteArgs != nil && sig.QuoteArgs[pos] {
			return true
		}
		if sigRawSlot(sig, pos) {
			return true
		}
	}
	return false
}

// pendingForwardWantsRawParen reports whether the nearest enclosing pending
// Forward (collecting args at the pointer) was matched to a signature that
// captures a ParenExpr raw. When true, a ParenExpr reached during forward
// collection is collected as data (via stepLiteral) rather than expanded.
// Stops at an OpenParen barrier. See Step 4.
func (e *Engine) pendingForwardWantsRawParen() bool {
	for i := e.pointer - 1; i >= 0; i-- {
		if IsOpenParen(e.tape.At(i)) {
			return false
		}
		if IsForward(e.tape.At(i)) {
			fwd, _ := AsForward(e.tape.At(i))
			// RawParens or FormArgs (macro raw capture) both want a forward
			// ParenExpr / Reach left raw at the pointer rather than evaluated
			// during collection.
			return fwd.Sig != nil && (len(fwd.Sig.RawParens) > 0 || len(fwd.Sig.FormArgs) > 0)
		}
	}
	return false
}

// resolveForwardArgs implements structure-first, lazy forward-argument
// resolution (design/LAZY-ARG-RESOLUTION.10.md). It replaces the former
// eager `preEvalParens(MaxForwardArgs)` scan, which evaluated EVERY forward
// paren group up to the highest-arity overload's needs before any signature
// was chosen — the cause of the `import "mod" (expr)` hazard (gotcha N1),
// where the trailing paren was evaluated before `import` installed the
// namespace it referenced.
//
// The scan is identical to the old one in every respect EXCEPT one: before
// evaluating a forward paren group, it checks whether any still-viable
// overload actually consumes a forward argument at that position. A
// signature is pruned from the viable set only when a concrete forward
// value (a literal, or a previously-evaluated group's result) DEFINITELY
// cannot satisfy a non-raw, non-Any slot the signature consumes — exactly
// the rejection `matchSignature` would make on the same value. So the
// signature `matchSignature` ultimately selects is never pruned, its
// claimed groups are always evaluated, and the only behavioural change is
// that a group NO surviving overload consumes is left raw (an OpenParen
// boundary that `matchSignature` stops at) rather than speculatively run.
//
// For `import "aql:string-util" (StringUtil.indexof ...)`: the leading
// String literal prunes the viable set to `[String]` (arity 1); at position
// 1 the paren is consumed by no viable overload, so it is left raw — import
// selects `[String]`, installs the namespace, and the paren then runs as an
// ordinary trailing statement.
func (e *Engine) resolveForwardArgs(fn *FnDefInfo, w WordInfo) error {
	// Forward-eligible signatures paired with their effective barrier
	// (the /s and /f modifiers override the declared BarrierPos, mirroring
	// matchSignature's forwardLimit computation).
	type viableSig struct {
		sig     *Signature
		barrier int
	}
	viable := make([]viableSig, 0, len(fn.Signatures))
	maxBarrier := 0
	for si := range fn.Signatures {
		sig := &fn.Signatures[si]
		if sig.Fallback {
			continue
		}
		barrier := sig.BarrierPos
		switch {
		case w.ForceStack:
			barrier = 0
		case w.ForceForward:
			barrier = sig.TotalArgs()
		}
		if barrier > 0 {
			viable = append(viable, viableSig{sig, barrier})
			if barrier > maxBarrier {
				maxBarrier = barrier
			}
		}
	}
	if maxBarrier <= 0 {
		return nil
	}

	// viableConsumes reports whether any still-viable signature collects a
	// forward argument at position pos (i.e. pos is within its barrier).
	viableConsumes := func(pos int) bool {
		for _, vs := range viable {
			if pos < vs.barrier {
				return true
			}
		}
		return false
	}

	// pruneViable drops every signature that a concrete forward value at
	// position pos definitely rules out (parity with matchSignature's
	// per-position rejection). Raw/Form/TypeArg slots and Any slots are
	// never used to prune (conservative — keep the signature viable).
	pruneViable := func(pos int, v Value) {
		kept := viable[:0]
		for _, vs := range viable {
			keep := true
			if pos < vs.barrier && !sigRawSlot(vs.sig, pos) {
				if et := sigArgType(vs.sig, pos); !et.Equal(TAny) && !sigArgMatches(vs.sig, pos, v) {
					keep = false
				}
			}
			if keep {
				kept = append(kept, vs)
			}
		}
		viable = kept
	}

	pos := 0
	scanIdx := e.pointer + 1
	for pos < maxBarrier && scanIdx < e.tape.Len() {
		tok := e.tape.At(scanIdx)

		// Boundary conditions: stop scanning.
		if IsForward(tok) || tok.Parent.ConformsTo(TMark) || tok.Parent.ConformsTo(TMove) ||
			tok.Parent.ConformsTo(TInternal) || tok.Parent.ConformsTo(TReturnCheck) {
			break
		}

		// Boundary tokens: end / ) stop the scan.
		if IsEnd(tok) || IsCloseParen(tok) {
			break
		}

		// Open paren: a forward group of unknown type.
		if IsOpenParen(tok) {
			// Structure-first gate: evaluate ONLY if some still-viable
			// overload consumes a forward argument at this position.
			// Otherwise no signature wants a value here — leave the
			// paren raw so matchSignature treats it as a boundary.
			if !viableConsumes(pos) {
				break
			}
			if err := e.evalParenGroupAt(scanIdx); err != nil {
				return err
			}
			// Flow-control raised inside the paren: let the outer Run
			// frame resolve it (parity with the former preEvalParens).
			if e.registry.FlowCtrl != FlowNone {
				return nil
			}
			// The paren collapsed to its result value(s) at scanIdx; count
			// it as one resolved position and advance, exactly as the
			// former scan did. (The result's runtime type is not used to
			// prune further: a group can collapse to zero or many values,
			// so we keep the conservative one-slot accounting.)
			pos++
			scanIdx++
			continue
		}

		// Interpolated template string: an expression, not a value — its
		// type is only knowable after evaluation (always a String).
		// Treated like a paren group: when a still-viable overload
		// consumes this position, evaluate it in place so a typed slot
		// (`raise` msg:String, `add`'s Scalar overload) sees the String
		// it will actually receive. Left raw, the token's internal
		// InterpString type would prune every typed signature and a
		// `raise `bad: ${x}`` mis-dispatched to the 0-arg fallback.
		if IsInterpString(tok) {
			if !viableConsumes(pos) {
				break
			}
			result, err := e.evalInterpString(tok)
			if err != nil {
				return err
			}
			result.Pos = tok.Pos
			e.tape.Set(scanIdx, result)
			pruneViable(pos, result)
			pos++
			scanIdx++
			continue
		}

		// Interpolated XML literal: same as InterpString — its type
		// (Node/Xml) is only knowable after evaluation, so evaluate in
		// place when a viable overload consumes this position, then prune.
		if IsXmlInterp(tok) {
			if !viableConsumes(pos) {
				break
			}
			result, err := e.evalXmlInterp(tok)
			if err != nil {
				return err
			}
			result.Pos = tok.Pos
			e.tape.Set(scanIdx, result)
			pruneViable(pos, result)
			pos++
			scanIdx++
			continue
		}

		// Paren expression value (paren-nesting Step 3): expand it back to
		// its OpenParen … CloseParen marker span in place, then re-process
		// — the IsOpenParen branch above collapses it on THIS engine. See
		// design/PAREN-REPRESENTATION.9.md Step 3.
		if IsParenExpr(tok) {
			// Step 4: a quote-captured ParenExpr (already Quoted) or a
			// raw-capture forward position is left unevaluated so the
			// matched sig captures the paren as code.
			if tok.Quoted || rawParenForward(fn, pos) || rawFormForward(fn, pos) {
				pos++
				scanIdx++
				continue
			}
			peItems, _ := AsParenExpr(tok)
			e.tape.Splice(scanIdx, 1, expandParenExpr(peItems)...)
			continue
		}

		// A Reach in the forward window evaluates like a ParenExpr (Reach
		// Phase B): expand to its lowered get-chain marker span in place,
		// then re-process. Quoted/raw-capture reaches are left for the
		// matched sig (parity with the ParenExpr branch above).
		if IsReach(tok) {
			if !isEvalReach(tok) || rawParenForward(fn, pos) || rawFormForward(fn, pos) {
				pos++
				scanIdx++
				continue
			}
			info, _ := AsReach(tok)
			e.tape.Splice(scanIdx, 1, expandReach(info)...)
			continue
		}

		// A word def-bound to a DATA __SP splice marker occupies its
		// forward position as the paren group (w) — the `f w ≡ f (w)`
		// equivalence: a plain (non-function) def-bound word expands into
		// the token stream wherever it stands. Rewriting the token to
		// ParenExpr([w]) and reprocessing routes it through the ParenExpr/
		// OpenParen branches above, so evaluation gating, multi-value
		// collapse, and raw-capture handling are byte-identical to a
		// written (w). Two exemptions: structural-capture slots (/q takes
		// the word's NAME, form/raw/type slots take the raw token — see
		// capturesForward), code-bearing splices (Forth-style macros
		// that must run against the live stack — see spliceIsData), and
		// binder operands (`def y xs` rebinds the MARKER so y aliases the
		// splice — see bindsReferent).
		if IsWord(tok) && !bindsReferent(fn.Name) && !capturesForward(fn, pos) {
			if wi, werr := AsWord(tok); werr == nil {
				if top, ok := e.registry.Defs.Top(wi.Name); ok && IsSplice(top) {
					if info, serr := AsSplice(top); serr == nil && spliceIsData(info) {
						pe := NewParenExpr([]Value{tok})
						pe.Pos = tok.Pos
						e.tape.Set(scanIdx, pe)
						continue
					}
				}
			}
		}

		// A registered FUNCTION word in the forward window that the collecting
		// word does NOT capture is the NEXT dispatch — the runtime's "another
		// function word is a barrier" rule (commitBarrierForward) stops forward
		// collection here, and the pre-evaluation scan must stop too. The
		// former scan counted the word as one resolved forward position and
		// kept going, so a LATER group was pre-evaluated ACROSS the barrier
		// once the COLLECTING word's own max arity (maxBarrier) reached past
		// it. With a heterogeneous-arity overload — e.g. a 3-arg `add` —
		// `(g) add (g) add (g)` evaluated the third group before the first add
		// ran; the recorded events then put both later operands on the
		// simulated stack and the operand layout refused "not adjacent on
		// top". Stop so each dispatch pre-evaluates only the groups IT
		// collects, in source order. Lookup mirrors commitBarrierForward's own
		// function-word test. The capturesForward guard preserves a word the
		// collecting sig takes STRUCTURALLY as an operand (a /q name like
		// `undef foo`, a raw/form/type slot) — there the function word is the
		// argument, not a barrier, and the scan must walk past it.
		if IsWord(tok) && !capturesForward(fn, pos) {
			if wi, werr := AsWord(tok); werr == nil && e.registry.Lookup(wi.Name) != nil {
				break
			}
		}

		// Non-group token. A concrete literal carries a final type that
		// matchSignature tests identically, so it is sound to prune the
		// viable set on it. Words and other non-concrete tokens are left
		// un-pruned (their matchSignature treatment is contextual) but are
		// still counted as one resolved position — so, exactly like the
		// former scan, groups beyond a NON-FUNCTION word remain reachable.
		if mt, kind := e.staticForwardType(tok); kind == fwdValue {
			pruneViable(pos, mt)
		}
		pos++
		scanIdx++
	}
	return nil
}

// sigRawSlot reports whether signature sig captures position pos structurally
// (raw paren / macro form / type-literal slot) rather than by ordinary value
// type — positions where a concrete-value type-prune would be unsound.
func sigRawSlot(sig *Signature, pos int) bool {
	return (sig.RawParens != nil && sig.RawParens[pos]) ||
		(sig.FormArgs != nil && sig.FormArgs[pos]) ||
		(sig.TypeArgs != nil && sig.TypeArgs[pos])
}

// fwdKind classifies a forward token by what it presents to signature
// matching WITHOUT evaluation.
type fwdKind int

const (
	fwdValue    fwdKind = iota // a token whose match-value/type is knowable now
	fwdGroup                   // an unevaluated paren/expr/reach group, type unknown
	fwdBoundary                // stops or is transparent to forward matching
)

// staticForwardType classifies a forward token by what it presents to
// signature matching WITHOUT evaluation, returning a match-value (for
// fwdValue) plus the classification.
//
// Only a concrete LITERAL value yields fwdValue: its type is final and
// matchSignature type-tests it identically (its literal-arg branch calls the
// same sigArgMatches), so it is sound to prune the viable set on it. A WORD
// is deliberately NOT a prunable value — matchSignature's treatment of a word
// is contextual (a `/q` QuoteArgs slot captures it as an Atom *name*
// regardless of any current binding; a word bound to an FnDef can still match
// a type-name slot via fall-through). Pruning on a word's resolved binding
// would diverge from the binder, so a word is classified as fwdBoundary:
// counted as one resolved position with the viable set left intact, exactly
// as the former eager scan treated it.
func (e *Engine) staticForwardType(tok Value) (match Value, kind fwdKind) {
	if IsOpenParen(tok) || IsParenExpr(tok) || IsReach(tok) {
		return Value{}, fwdGroup
	}
	if IsWord(tok) {
		return Value{}, fwdBoundary
	}
	if IsConcrete(tok) {
		return tok, fwdValue
	}
	// Type literals, carriers, markers: not a prunable concrete value.
	return Value{}, fwdBoundary
}

// evalParenGroupAt collapses the forward paren group at stack index scanIdx
// in place, evaluating its contents on this engine and leaving the result
// value(s) at scanIdx. Extracted verbatim from the former preEvalParens
// OpenParen branch; e.pointer is saved and restored. A flow-control signal
// raised inside the paren is left on the registry for the outer Run frame.
func (e *Engine) evalParenGroupAt(scanIdx int) error {
	savedPointer := e.pointer

	// Check mode: snapshot the group's inner tokens before evaluation
	// reduces them. If the group collapses to a single Boolean
	// carrier, the tokens are attached as a GuardFactInfo payload so
	// guard narrowing can recover the `x is T` structure from the
	// canonical paren condition form (checker-accuracy-review.10.md A3).
	var guardToks []Value
	var groupSpan, lenBefore int
	if e.registry.Check.IsActive() {
		gdepth := 0
		for i := scanIdx; i < e.tape.Len(); i++ {
			v := e.tape.At(i)
			if IsOpenParen(v) {
				gdepth++
				if i > scanIdx {
					guardToks = append(guardToks, v)
				}
				continue
			}
			if IsCloseParen(v) {
				gdepth--
				if gdepth == 0 {
					groupSpan = i - scanIdx + 1
					break
				}
				guardToks = append(guardToks, v)
				continue
			}
			if i > scanIdx {
				guardToks = append(guardToks, v)
			}
		}
		lenBefore = e.tape.Len()
	}

	e.pointer = scanIdx

	// Advance past the OpenParen marker.
	if err := e.stepOpenParen(); err != nil {
		e.pointer = savedPointer
		return err
	}

	// Step through contents until we reach the matching ")". Track paren
	// depth so inner parens are processed without prematurely breaking on
	// their ")" tokens.
	depth := 1
	for limit := 0; limit < maxParenGroupSteps && depth > 0; limit++ {
		if e.pointer >= e.tape.Len() {
			break
		}
		v := e.tape.At(e.pointer)

		if IsOpenParen(v) {
			depth++
			e.pointer++
			continue
		}
		if IsCloseParen(v) {
			depth--
			if err := e.stepCloseParen(); err != nil {
				e.pointer = savedPointer
				return err
			}
			if depth == 0 {
				break
			}
			continue
		}

		// Normal evaluation inside paren.
		switch {
		case IsWord(v):
			if err := e.stepWord(v); err != nil {
				e.pointer = savedPointer
				return err
			}
		case IsEnd(v):
			if err := e.stepEnd(); err != nil {
				e.pointer = savedPointer
				return err
			}
		case IsMark(v):
			e.stepMark(v)
		case IsMove(v):
			if err := e.stepMove(v); err != nil {
				e.pointer = savedPointer
				return err
			}
		case IsForward(v):
			e.pointer++
		case IsReturnCheck(v):
			e.pointer++
		case IsDefCleanup(v):
			e.stepDefCleanup(v)
			e.pointer++
		case IsInterpString(v):
			// Mirror the main loop: evaluate the template in place and
			// re-step the resulting string (no pointer advance), so a
			// paren-wrapped template — `raise (`bad: ${x}`)` — collapses
			// to a String rather than a raw InterpString token.
			result, err := e.evalInterpString(v)
			if err != nil {
				e.pointer = savedPointer
				return err
			}
			result.Pos = v.Pos
			e.tape.Set(e.pointer, result)
		case IsXmlInterp(v):
			// Mirror the InterpString case: a paren-wrapped interpolated
			// XML literal — `f (<p>${x}</p>)` — collapses to a Node/Xml.
			result, err := e.evalXmlInterp(v)
			if err != nil {
				e.pointer = savedPointer
				return err
			}
			result.Pos = v.Pos
			e.tape.Set(e.pointer, result)
		default:
			if err := e.stepLiteral(); err != nil {
				e.pointer = savedPointer
				return err
			}
		}
		// Propagate any flow-control signal raised by the step; the outer
		// Run frame will resolve it.
		if e.registry.FlowCtrl != FlowNone {
			e.pointer = savedPointer
			return nil
		}
	}

	// Budget exhausted before the group closed (depth still open with tape
	// left to process). Report it explicitly rather than silently
	// returning a half-evaluated group, which would later surface as a
	// phantom "unmatched opening parenthesis" at the top-level drain.
	if depth > 0 && e.pointer < e.tape.Len() {
		e.pointer = savedPointer
		return e.evalLimitError(maxParenGroupSteps)
	}

	// Guard-fact attachment (A3): the group reduced to exactly one
	// value and it is a Boolean carrier — keep the original tokens on
	// it for guard narrowing. Booleans never carry another payload at
	// this point (ChildTypeInfo is List/Map-only), so the write is
	// non-destructive.
	if groupSpan > 0 && len(guardToks) >= 3 {
		nResults := e.tape.Len() - lenBefore + groupSpan
		if nResults == 1 && scanIdx < e.tape.Len() {
			res := e.tape.At(scanIdx)
			if res.Carrier && !res.Dynamic && res.Parent.ConformsTo(TBoolean) {
				res.Data = GuardFactInfo{Toks: guardToks}
				e.tape.Set(scanIdx, res)
			}
		}
	}

	e.pointer = savedPointer
	return nil
}

// stepWord handles a word (function reference) at the current pointer.
//
// The reserved tape-syntactic tokens — `(`, `)`, `end` — never reach
// here: the parser emits them as typed OpenParen / CloseParen / End
// values, and the Run-loop switch dispatches them directly. stepWord
// therefore deals only with regular named words.
// stepWordUsurp handles the /u (ForceUsurp) word modifier: resolve the
// name to its bound Function value and wrap it so its signature
// argument order is reversed (usurped a b c ≡ f c b a). Like /r, /u is
// legal only for function words. /u alone dispatches the wrapper
// immediately (like a bare word); /ur (combined with /r) leaves the
// wrapper on the stack as inert data.
func (e *Engine) stepWordUsurp(val Value, w WordInfo) error {
	v, ok := ResolveUsurp(e.registry, w.Name)
	if !ok {
		if e.registry != nil && e.registry.Check.IsActive() {
			e.registry.Check.AddDiagnostic(CheckDiagnostic{
				Code:   "undefined_word",
				Detail: "undefined word: " + w.Name,
				Word:   w.Name,
				Row:    val.Pos.Row,
				Col:    val.Pos.Col,
			})
			placeholder := NewAtom(w.Name)
			placeholder.Pos = val.Pos
			placeholder.Undefined = true
			e.tape.Set(e.pointer, placeholder)
			return e.stepLiteral()
		}
		return &AqlError{
			Code:       "undefined_word",
			Detail:     "undefined word: " + w.Name,
			Src:        w.Name,
			Row:        val.Pos.Row,
			Col:        val.Pos.Col,
			fullSource: e.effectiveSource(),
		}
	}
	// /u may usurp only function words — a non-fn binding has no
	// signature to reverse. ResolveUsurp returns the raw binding in
	// that case so the IsFunctionRef check below raises illegal_ref.
	if !IsFunctionRef(v) {
		detail := "/u requires a function word: " + w.Name + " is bound to " + v.Parent.String()
		if e.registry != nil && e.registry.Check.IsActive() {
			e.registry.Check.AddDiagnostic(CheckDiagnostic{
				Code:   "illegal_ref",
				Detail: detail,
				Word:   w.Name,
				Row:    val.Pos.Row,
				Col:    val.Pos.Col,
			})
			// Check mode is lenient (the illegal_ref diagnostic is advisory), but
			// the interpreter raises illegal_ref here at runtime. Record a TERMINAL
			// trap so a compiled program raises the byte-identical error in place
			// instead of refusing on the downstream Undefined placeholder. Only a
			// top-level trap is recordable; a nested /u keeps the placeholder path
			// and refuses (falls back) as before.
			e.registry.Check.Recorder().RecordTrap("illegal_ref", detail, w.Name, "", e.currentPos())
			placeholder := NewAtom(w.Name)
			placeholder.Pos = val.Pos
			placeholder.Undefined = true
			e.tape.Set(e.pointer, placeholder)
			return e.stepLiteral()
		}
		return &AqlError{
			Code:       "illegal_ref",
			Detail:     detail,
			Src:        w.Name,
			Row:        val.Pos.Row,
			Col:        val.Pos.Col,
			fullSource: e.effectiveSource(),
		}
	}
	v.Pos = val.Pos
	if w.ForceRef {
		// /ur: leave the usurped wrapper as inert data (mirrors /r) — it
		// still dispatches if args follow or it is later stepped. As
		// DATA it is a legitimate arrival for a pending forward, so no
		// barrier commit here.
		e.tape.Set(e.pointer, v)
		if e.recorder != nil && isRecordableLiteral(v) {
			e.recorder.OnPushLit(v)
		}
		e.pointer++
		return nil
	}
	// /u alone dispatches the wrapper like a bare function word — a
	// statement boundary. Commit the nearest parked forward that can
	// already fire BEFORE rewriting the token (mirroring the bare-word
	// hook in stepWord); without this, the /u path bypassed the
	// barrier and `if (c) [t] sub/u 10 3` swallowed the usurped
	// wrapper as a phantom else instead of running it. On commit the
	// loop re-steps this /u word with no forward pending.
	if e.commitBarrierForward() {
		return nil
	}
	e.tape.Set(e.pointer, v)
	// Dispatch the unquoted wrapper now: stepLiteral routes it through
	// execFnDefLiteral, which forward-collects any trailing args.
	return e.stepLiteral()
}

// stepWordRef handles the /r modifier (the ForceRef branch of stepWord):
// resolve the name to its bound Function value as data, with no argument
// collection or dispatch. Extracted from stepWord (mirroring stepWordUsurp)
// so the dispatch hub stays under the cyclomatic-complexity bound; the body
// is a verbatim move — behaviour is unchanged.
func (e *Engine) stepWordRef(val Value, w WordInfo) error {
	v, ok := ResolveRef(e.registry, w.Name)
	if !ok {
		if e.registry != nil && e.registry.Check.IsActive() {
			e.registry.Check.AddDiagnostic(CheckDiagnostic{
				Code:   "undefined_word",
				Detail: "undefined word: " + w.Name,
				Word:   w.Name,
				Row:    val.Pos.Row,
				Col:    val.Pos.Col,
			})
			placeholder := NewAtom(w.Name)
			placeholder.Pos = val.Pos
			placeholder.Undefined = true
			e.tape.Set(e.pointer, placeholder)
			return e.stepLiteral()
		}
		return &AqlError{
			Code:       "undefined_word",
			Detail:     "undefined word: " + w.Name,
			Src:        w.Name,
			Row:        val.Pos.Row,
			Col:        val.Pos.Col,
			fullSource: e.effectiveSource(),
		}
	}
	// /r may reference only function words. A non-fn binding (plain
	// value, type body) has no call/value asymmetry for /r to break,
	// so referencing it is illegal.
	if !IsFunctionRef(v) {
		detail := "/r requires a function word: " + w.Name + " is bound to " + v.Parent.String()
		if e.registry != nil && e.registry.Check.IsActive() {
			e.registry.Check.AddDiagnostic(CheckDiagnostic{
				Code:   "illegal_ref",
				Detail: detail,
				Word:   w.Name,
				Row:    val.Pos.Row,
				Col:    val.Pos.Col,
			})
			// Check mode is lenient (the illegal_ref diagnostic is advisory), but
			// the interpreter raises illegal_ref here at runtime. Record a TERMINAL
			// trap so a compiled program raises the byte-identical error in place
			// instead of refusing on the downstream Undefined placeholder. Only a
			// top-level trap is recordable; a nested /r keeps the placeholder path
			// and refuses (falls back) as before.
			e.registry.Check.Recorder().RecordTrap("illegal_ref", detail, w.Name, "", e.currentPos())
			placeholder := NewAtom(w.Name)
			placeholder.Pos = val.Pos
			placeholder.Undefined = true
			e.tape.Set(e.pointer, placeholder)
			return e.stepLiteral()
		}
		return &AqlError{
			Code:       "illegal_ref",
			Detail:     detail,
			Src:        w.Name,
			Row:        val.Pos.Row,
			Col:        val.Pos.Col,
			fullSource: e.effectiveSource(),
		}
	}
	v.Pos = val.Pos
	e.tape.Set(e.pointer, v)
	// (The use is recorded inside ResolveRef, covering this `/r` path, the
	// `ref` word, and export-map reference values alike.)
	// `/r` resolves the name to its bound value and ADVANCES the
	// pointer, exactly like pushing a literal — it does NOT dispatch a
	// resolved function (that is what a bare word does). The value
	// stays a plain Function, so it still dispatches when later stepped
	// (e.g. `get` for `pkg.fn` / `m.fn arg`, or a bare word).
	if e.recorder != nil && isRecordableLiteral(v) {
		e.recorder.OnPushLit(v)
	}
	e.pointer++
	return nil
}

func (e *Engine) stepWord(val Value) error {
	w, _ := AsWord(val)

	// /u modifier — see stepWordUsurp. Handled before the /r branch
	// so the /ur combo usurps rather than plain-referencing.
	if w.ForceUsurp {
		return e.stepWordUsurp(val, w)
	}

	// /r modifier: resolve the name to its bound Function value as data,
	// with no argument collection or dispatch. The FnDef binding comes
	// back as an (unquoted) Function value that sits on the stack like any
	// other piece of data — exactly the case `ref` exists to enable. /r is
	// legal only for function words; a non-fn binding raises illegal_ref.
	if w.ForceRef {
		return e.stepWordRef(val, w)
	}

	// If there is a pending forward whose next slot is /q-marked
	// (QuoteArgs), capture this Word as data (converted to an Atom
	// further down the pipeline) rather than executing it. This is the
	// general word-capture mechanism: def, undef, type, untype, quote,
	// inspect, and similar words all declare /q on their name slot, so
	// `undef foo` works even when foo is currently defined. See
	// signature.go §1.5 on /q.
	if e.hasPendingForwardQuoteArg() {
		return e.stepLiteral()
	}

	// If a pending forward's next slot is FormArgs (macro raw capture),
	// collect this Word as a raw Word — do NOT execute it, and (unlike
	// QuoteArgs) do NOT coerce it to an Atom. stepLiteral collects the value
	// at the pointer; the Word matches the macro's Any-typed slot, so no
	// conversion fires. See design/MACROS-PHASE1.10.md §3.
	if e.hasPendingForwardFormArg() {
		return e.stepLiteral()
	}

	// If a pending forward expects TFunction, resolve this word to a
	// function reference value rather than executing it. The word must
	// have a FnDef entry in DefStacks.
	if e.hasPendingForwardExpectingFunction() {
		// Wrap the aggregate dispatch view so the reference carries every
		// overload of the name (across stacked defs), not just the topmost
		// entry's own sigs.
		if fnDef := e.registry.Lookup(w.Name); fnDef != nil {
			e.tape.Set(e.pointer, NewFunction(*fnDef))
			return e.stepLiteral()
		}
		// Not a def fn — fall through to normal execution.
	}

	// Named user-defined types take priority over DefStacks: type
	// bindings stack independently from def bindings, and a shadow-
	// then-reveal pattern (`type Foo Integer; type Foo fn […]`)
	// would otherwise see the legacy InstallDef mirror in DefStacks
	// instead of the active fn-type binding. Pushed with Quoted=true
	// for fn-shape types so execFnDefLiteral treats them as data.
	//
	// Word-capture cases (untype Foo, etc.) are intercepted earlier
	// via hasPendingForwardExpectingWord — by the time we reach this
	// priority block, no /q-Atom or Word slot is waiting for the
	// name. The pushed value flows through stepLiteral so a pending
	// forward can still consume it (e.g. `Color` as the value side
	// of an export map entry).
	if e.registry != nil {
		if tv, ok := e.registry.TopTypeBody(w.Name); ok {
			push := tv
			push.Pos = val.Pos
			// A fn-shape type body (a predicate type, e.g. `def Bbd
			// fn […]`) is pushed Quoted so stepLiteral's Function gate
			// leaves it as DATA: type-defining functions participate
			// in type operations (`def p:Bbd v`, `v is Bbd`, `inspect
			// Bbd`) and are never free-standing calls. Without the
			// mark, value dispatch — which collects forward args like
			// a word — would run the predicate on whatever follows
			// (`Bbd "c"`). Collection-time consumers are unaffected:
			// the forward planner resolves type names itself (see the
			// TopTypeBody block in the matchSignature scan).
			if _, isFn := push.Data.(FnDefInfo); isFn {
				push.Quoted = true
			}
			e.tape.Set(e.pointer, push)
			return e.stepLiteral()
		}
	}

	// Simple value def: substitute the word with its value directly,
	// bypassing function dispatch entirely. FnDefInfo and ClassTypeInfo
	// entries are not simple values — they go through normal Lookup.
	if top, ok := e.registry.Defs.Top(w.Name); ok {
		switch top.Data.(type) {
		case FnDefInfo, *ClassTypeInfo:
			// Not a simple value — fall through to Lookup.
		default:
			// f w ≡ f (w): a word bound to a DATA __SP splice marker that
			// is being collected by a pending forward expands as the paren
			// group (w) instead of substituting the raw marker — the
			// payload's values arrive from a barrier-protected segment (a
			// multi-value splice fills several slots, exactly as the
			// written form does; without this the marker itself would be
			// collected into an Any slot or implicit-end a typed one). The
			// /q and FormArgs intercepts above this block keep their
			// word-capture semantics; a raw-paren-capturing forward
			// collects the wrapped (w) as code via stepLiteral's existing
			// ParenExpr gate; code-bearing splices keep their live-stack
			// macro semantics (spliceIsData); and a pending BINDER
			// collects the raw marker so `def y xs` rebinds it — the new
			// name aliases the splice (bindsReferent). Inside the group
			// the OpenParen barrier hides the pending forward, so the
			// re-stepped word takes the standalone splice-fire path — no
			// recursion (and recordUse fires on that inner step).
			if info, serr := AsSplice(top); serr == nil && spliceIsData(info) {
				if fwdIdx := e.pendingForwardIdx(); fwdIdx >= 0 {
					if fwd, ferr := AsForward(e.tape.At(fwdIdx)); ferr == nil && !bindsReferent(fwd.FuncName) {
						pe := NewParenExpr([]Value{val})
						pe.Pos = val.Pos
						e.tape.Set(e.pointer, pe)
						return e.stepLiteral()
					}
				}
			}
			// Record the substitution as a "use" for unused-def
			// tracking in check mode.
			e.registry.Check.recordUse(w.Name)
			// Remember which NAME produced this value (compile passes): if the
			// value later has no compiled home in a fn unit, the read was a
			// dynamic-scope reference and lowers to a runtime name lookup
			// (resolveOperand's dynScopeRescue).
			e.registry.Check.Recorder().NoteDefRead(top.ID, w.Name)
			// A def'd word binds a VALUE: push it as-is. Lists bind like
			// maps — `def xs [1,2,3]` makes `xs` the list value, evaluated
			// at def time (so `size xs` → 3). To splice a list's elements
			// onto the stack (the old implicit behaviour / Forth-style
			// macros) use the explicit `def name word [list]` form, whose
			// __SP marker is handled in stepLiteral.
			if top.Dynamic && e.registry.Check.IsActive() {
				// Tag the gradual value with its binding so a typed use
				// downstream narrows the binding (narrowing-through-use).
				top.DynFrom = w.Name
			}
			e.tape.Set(e.pointer, top)
			return e.stepLiteral()
		}
	}

	fn := e.registry.Lookup(w.Name)
	if fn != nil {
		// User-code dispatch — record the name as "used" for
		// unused-def analysis in check mode.
		e.registry.Check.recordUse(w.Name)
		if err := e.policyGateWord(w.Name); err != nil {
			return err
		}
		// Statement boundary: a function word beginning its own dispatch
		// is a forward-collection barrier (the argument-order rule), so
		// first commit the nearest ARRIVAL-parked forward that can
		// already fire — otherwise an optional trailing slot (an
		// else-less `if`'s else) swallows this statement's result, or
		// this statement runs before a guard's then-branch does. See
		// commitBarrierForward.
		if e.commitBarrierForward() {
			return nil
		}
		// Macro dispatch (design/MACROS-PHASE1.10.md §5): a macro word is
		// applied to its raw operands ahead on the tape — BEFORE preEvalParens
		// (1228) or any forward collection, so operands arrive as code. The
		// word sits at e.pointer; execMacro replaces `mac operand…` with the
		// spliced expansion and lets the __SP marker re-step the result.
		if fn.Macro {
			return e.execMacro(e.pointer, fn)
		}
	}

	if fn == nil {
		if w.Name == "true" {
			e.tape.Set(e.pointer, NewBoolean(true))
			return nil
		}
		if w.Name == "false" {
			e.tape.Set(e.pointer, NewBoolean(false))
			return nil
		}
		if w.Name == "none" {
			e.tape.Set(e.pointer, NewNone())
			return nil
		}
		if w.Name == "null" {
			e.tape.Set(e.pointer, NewAtom("null"))
			return nil
		}
		if t, ok := typeNames[w.Name]; ok {
			e.tape.Set(e.pointer, NewTypeLiteral(t))
			return nil
		}
		if t, ok := ResolveTypePath(w.Name); ok {
			e.tape.Set(e.pointer, NewTypeLiteral(t))
			return nil
		}
		// (r.Types resolution lives in the priority block at the
		// top of stepWord — before DefStacks substitution — so a
		// named user-defined type is never reached here.)
		// Strict rule: an undefined word at the pointer is an error.
		// Names that need to be values must be quoted explicitly (`quote
		// foo` or a literal atom) or land at a /q-quoted argument
		// position, where forward collection captures the word as an
		// Atom before it ever reaches stepWord.
		//
		// In CheckMode the engine emits a diagnostic and continues with
		// an `Atom{Undefined:true}` so static analysis can keep going.
		// The diagnostic is recorded HERE rather than at end-of-Run
		// because the placeholder atom can be consumed by a downstream
		// operation (e.g. a checkModeAssumeSig for `add`) and never
		// reach the result stack — recording at the source guarantees
		// every undefined word produces exactly one diagnostic.
		if !e.registry.Check.IsActive() {
			hint := e.undefinedWordHint(w.Name)
			return &AqlError{
				Code:       "undefined_word",
				Detail:     "undefined word: " + w.Name,
				Src:        w.Name,
				Row:        val.Pos.Row,
				Col:        val.Pos.Col,
				Hint:       hint,
				fullSource: e.effectiveSource(),
			}
		}
		e.registry.Check.AddDiagnostic(CheckDiagnostic{
			Code:   "undefined_word",
			Detail: "undefined word: " + w.Name,
			Word:   w.Name,
			Row:    val.Pos.Row,
			Col:    val.Pos.Col,
		})
		v := NewAtom(w.Name)
		v.Pos = val.Pos
		v.Undefined = true
		e.tape.Set(e.pointer, v)
		return nil
	}

	// Pre-evaluate paren expressions in the forward scan range so that
	// matchSignature sees fully resolved values (rule 1.5). Only needed
	// when at least one signature wants forward args (BarrierPos > 0)
	// and the call hasn't been forced to stack mode via /s; or when /f
	// explicitly forces forward collection.
	if (fn.HasForwardSigs() && !w.ForceStack) || w.ForceForward {
		if err := e.resolveForwardArgs(fn, w); err != nil {
			return err
		}
	}

	// Unified signature matching: one path for all words.
	resolved := e.effectiveResolved()
	sig, positions, specAt := e.matchSignature(fn, w, resolved)

	// Retry fallback for words with forward-collecting sigs: when
	// nearest-first matching fails, retry with deepest-first
	// (ForceStack). Handles CallAQL sub-engines where FnDef args are
	// placed in deepest-first order on the input stack.
	if sig == nil && fn.HasForwardSigs() && !w.ForceStack {
		wDeep := w
		wDeep.ForceStack = true
		sig, positions, specAt = e.matchSignature(fn, wDeep, resolved)
	}

	// In check mode, if matchSignature fell through to the 0-arg /
	// Fallback handler because no typed signature matched (but
	// typed signatures exist), treat it as an unmatched call and go
	// through the assume-sig recovery path so the user gets a
	// diagnostic with the typed sig's Returns/ReturnsFn synthesis.
	if sig != nil && sig.Fallback && e.registry.Check.IsActive() {
		hasTyped := false
		for i := range fn.Signatures {
			if !fn.Signatures[i].Fallback {
				hasTyped = true
				break
			}
		}
		if hasTyped {
			sig = nil
		}
	}

	// Run-mode reorder probe (decision DX report finding 2): the
	// fallback is about to raise "no matching signature" — but when
	// the UNCLAIMED forward tokens (the plan pruned every typed sig,
	// so they were never collected) match a real signature under a
	// permutation, the arguments are swapped. Raise the dedicated
	// hint instead of dispatching the fallback's generic error.
	if sig != nil && sig.Fallback && !e.registry.Check.IsActive() {
		hint := reorderHintFor(w.Name, fn, reorderForwardCandidates(e.tape, e.pointer))
		if hint == "" {
			// Stack-form swap: the misordered values are already on
			// the stack below the word.
			hint = reorderHintFor(w.Name, fn, reorderCandidates(e.tape.Prefix(e.pointer)))
		}
		if hint != "" {
			return makeAqlErrorAt("signature_error",
				"no matching signature for "+w.Name, w.Name,
				e.effectiveSource(), hint, val.Pos)
		}
	}

	if sig == nil {
		// In check mode, a missing signature is a soft diagnostic
		// rather than a hard error: pick the first-ranked candidate,
		// synthesise carrier return values from it, and splice them
		// in place of the word + up to N adjacent arg slots.
		// We bypass insertForward here because forward collection
		// would re-trigger sigTypeMatches and loop indefinitely.
		if e.registry.Check.IsActive() && len(fn.Signatures) > 0 {
			// S2 (design/SURFACES.10.md): a required operation called on
			// a SURFACE-typed carrier types via the contract's shape
			// (Self := the surface node) — the contract guarantees the
			// operation for every member, so this is a correct typing,
			// not a degrade; no diagnostic.
			if handled, herr := e.checkModeSurfaceShape(w, val.Pos); handled {
				return herr
			}
			return e.checkModeAssumeSig(w, fn, &fn.Signatures[0], val.Pos)
		}
		return e.sigError(w.Name, fn, val.Pos)
	}

	// Count forward vs stack args from positions.
	fwdCount := 0
	stkCount := 0
	for _, pos := range positions {
		if pos > e.pointer {
			fwdCount++
		} else {
			stkCount++
		}
	}

	// Check-mode forward-greediness advisories (forward-strand + mixed-form).
	// Extracted from stepWord to keep this hot dispatch path under the
	// cyclomatic-complexity gate; diagnostics only — no effect on execution
	// or dispatch. See design/FORWARD-STRAND-ADVISORY.10.md.
	e.checkMixedFormAdvisories(w, sig, positions, val.Pos, fwdCount, stkCount)

	// Compile-mode stranded-member-fn guard (design/EDGE-SPEC-FINDINGS.0.md §2):
	// a parked user-fn value surfaced from a container read (`m.double`) AUTO-
	// APPLIES the moment a value lands on it in the interpreter. A dispatch that
	// instead consumes that value while the fn sits stranded just below it
	// (`m.double 21 eq 42` — `eq` grabs 21, and the fn is then applied WRONGLY at
	// the residual tail) diverges. Refuse so the body falls back. The
	// statement-tail apply (`m.double 21`, nothing dispatches above the fn) is
	// unaffected — its residual [fn, 21] lowers to the correct trailing apply.
	e.refuseStrandedMemberFn(positions)

	// Forward collection needed: defer execution.
	if fwdCount > 0 {
		e.traceNote = "forward→ " + traceSigStr(w.Name, sig)
		return e.insertForward(w, sig, fwdCount, stkCount, specAt)
	}

	// Compile-mode forward-collection drift guard
	// (design/EDGE-SPEC-FINDINGS.0.md §1). A forward-eligible word that matched
	// ALL-STACK (fwdCount==0) under a DYNAMIC top-of-stack operand — while a
	// concrete forward token sits right after it — diverges: the interpreter,
	// seeing the operand's CONCRETE runtime value, forward-collects that token
	// into the narrower overload (`add 1` → 7+1=8), whereas the check-mode match
	// (the dynamic carrier blocks that overload's stack slot) reached PAST the
	// dynamic value to a deeper leading residual (`add` over [dyn, 5] → 5+7).
	// The residual-window operand accounting across the reified-error / island
	// boundary is unsound, so refuse in compile mode and fall back. The
	// interpreter and plain check mode are untouched (recorder inactive).
	e.refuseForwardStackDrift(sig, positions)

	// Immediate execution: read args from recorded positions.
	match := &MatchResult{Sig: sig, Positions: positions, Name: w.Name}
	if stkCount > 0 {
		match.Args = make([]Value, stkCount)
		for i, pos := range positions {
			match.Args[i] = e.tape.At(pos)
		}
	}
	e.traceNote = "stack " + traceSigStr(w.Name, sig)
	return e.execMatch(match)
}

// checkMixedFormAdvisories emits the two check-mode forward-greediness
// advisories when a word forward-collects an argument AND also takes a stack
// argument (a swap-form dispatch): the forward-strand advisory (the
// `1 2 add 3 mul → 5` surprise — `add` grabs the forward `3` and strands the
// `1`) and the mixed-form-call advisory (a 3+-arg call taking operand(s) from a
// PRECEDING expression while forward-collecting binds differently from the
// all-forward reading — `(cond) if [a] [b]`). Both are advisory only (info
// severity), emitted in check mode, never gating. The mixed-form arm fires only
// when the deepest stack-bound slot is Any-typed (the genuine footgun, if3's
// {Any,Any,Any}); a concretely-typed receiver-first idiom (`xs set 0 v`,
// `s slice j e`) binds correctly and stays quiet — without that gate the
// advisory over-fired ~190 false info across the voxgig-aql libraries. Extracted
// from stepWord so the hot dispatch path stays under the cyclomatic-complexity
// gate. See design/FORWARD-STRAND-ADVISORY.10.md, ERRORS.8.md §6.2.
func (e *Engine) checkMixedFormAdvisories(w WordInfo, sig *Signature, positions []int, pos SrcPos, fwdCount, stkCount int) {
	if !(e.registry.Check.IsActive() && fwdCount > 0 && stkCount > 0) {
		return
	}
	e.checkForwardStrandsOperand(w, sig, positions, pos)
	if sig.TotalArgs() >= 3 && mixedFormStackSlotAny(e, sig, positions) {
		e.registry.Check.AddDiagnostic(CheckDiagnostic{
			Code: "mixed_form_call",
			Detail: w.Name + " takes " + strconv.Itoa(stkCount) + " argument(s) from the stack while forward-collecting " +
				strconv.Itoa(fwdCount) + " — the mixed form binds differently from the all-forward form; " +
				"prefer " + w.Name + " arg1 arg2 … or group explicitly",
			Word: w.Name,
			Row:  pos.Row,
			Col:  pos.Col,
		})
	}
}

// mixedFormStackSlotAny reports whether the deepest stack-bound sig slot of a
// mixed-form call is Any-typed. The mixed_form_call advisory fires only when it
// is: an Any slot has no type discipline, so a value stacked into it (the
// `(cond) if [a] [b]` footgun, if3's {Any,Any,Any}) silently misbinds — the
// case the advisory exists to flag. A concretely-typed deepest slot (the
// receiver-first `set`/`slice`/user-fn-typed-last idiom) binds correctly, so
// the advisory must stay quiet there. Mirrors checkForwardStrandsOperand's
// deepest-stack walk and its Any-slot bail.
func mixedFormStackSlotAny(e *Engine, sig *Signature, positions []int) bool {
	minStack := -1
	minSigPos := -1
	for sp, p := range positions {
		if p < e.pointer && (minStack == -1 || p < minStack) {
			minStack = p
			minSigPos = sp
		}
	}
	if minSigPos < 0 {
		return false
	}
	t := sigArgType(sig, minSigPos)
	return t == nil || t.Equal(TAny)
}

// checkForwardStrandsOperand implements the "forward greediness" advisory
// (check mode only). Preconditions (checked by the caller): the dispatch is
// mixed — it forward-collected ≥1 arg AND took ≥1 stack arg.
//
// It flags the case where a SIBLING operand is stranded: a value sitting on
// the stack just below the deepest stack arg the word consumed, in the same
// scope, whose type matches that consumed slot — i.e. an operand the word
// could equally have taken from the stack. The sibling-type test is what
// separates the genuine `1 2 add 3` gotcha (a stranded Number under `add`)
// from a deliberately-kept value of an unrelated type left by an earlier
// statement.
func (e *Engine) checkForwardStrandsOperand(w WordInfo, sig *Signature, positions []int, pos SrcPos) {
	// Deepest stack position the word consumed, and its sig slot.
	minStack := -1
	minSigPos := -1
	for sp, p := range positions {
		if p < e.pointer && (minStack == -1 || p < minStack) {
			minStack = p
			minSigPos = sp
		}
	}
	if minStack <= 0 || minSigPos < 0 {
		return
	}
	slotType := sigArgType(sig, minSigPos)
	// An Any-typed slot matches everything, so it cannot tell a sibling
	// operand from unrelated residue — too weak to be a reliable signal.
	if slotType == nil || slotType.Equal(TAny) {
		return
	}

	// Scan downward from just below the consumed stack args, stopping at the
	// nearest scope boundary. The first data value found is "stranded".
	for i := minStack - 1; i >= 0; i-- {
		v := e.tape.At(i)
		if IsOpenParen(v) || IsCloseParen(v) || IsForward(v) || IsDefCleanup(v) ||
			v.Parent.ConformsTo(TMark) || v.Parent.ConformsTo(TMove) ||
			v.Parent.ConformsTo(TInternal) || v.Parent.ConformsTo(TReturnCheck) {
			return // scope boundary: nothing stranded in this scope
		}
		if !IsConcrete(v) && !IsBareTypeNode(v) {
			continue // structural residue, keep scanning
		}
		// Sibling-operand heuristic: only flag a stranded value the word
		// could itself have consumed (same type as the slot it just took).
		if v.Is(slotType) {
			e.registry.Check.AddDiagnostic(CheckDiagnostic{
				Code: "forward_strands_operand",
				Detail: w.Name + " collected a forward argument while a " +
					slotType.String() + " operand was left unconsumed on the stack — " +
					"it may be stranded; group the intended operands, e.g. (… " + w.Name + " …)",
				Word: w.Name,
				Row:  pos.Row,
				Col:  pos.Col,
			})
		}
		return // only the nearest value below matters
	}
}

// refuseForwardStackDrift refuses (compile mode only) a dispatch whose
// check-mode operand match would DIVERGE from the interpreter's runtime
// forward collection — the reified-error / island residual accounting of
// design/EDGE-SPEC-FINDINGS.0.md §1. Preconditions (checked by the caller):
// the dispatch matched ALL-STACK (fwdCount==0). It refuses when ALL of:
//
//   - the recorder is active (a real compile pass — never plain check / run);
//   - the sig is forward-eligible (BarrierPos != 0, not a full-stack word), so
//     the trailing token is a forward CANDIDATE, not a separate residual;
//   - the TOP-OF-STACK matched arg (sig[0], the highest-position operand) is
//     DYNAMIC. That is the operand whose unknown static type BLOCKED the
//     narrower forward overload — forcing check mode to a catch-all all-stack
//     match — while at run time its concrete value would let the word
//     forward-collect instead. If a concrete value is on top, the all-stack
//     match is what the interpreter takes too (`get key dyn`, `force-arity 2
//     dynfn`), so there is no drift;
//   - a DEEPER matched arg is NON-dynamic — the leading residual the all-stack
//     match reached PAST (the `5` of `5 do … error … add 1`). Its runtime
//     value stays put under the word's real result;
//   - the token immediately after the word is a single ATOMIC LITERAL forward
//     operand (a concrete scalar/atom or a bare type node) — the operand the
//     interpreter forward-collects once the dynamic value is concrete. A
//     CloseParen / ParenExpr / structural token is NOT a forward operand (the
//     `eq`/`and` comparison residuals that end on `)` compile faithfully
//     all-stack), so those are excluded.
//
// Without the top-is-dynamic and trailing-token gates a genuine all-stack
// dynamic dispatch (`get key dyn`, `dyn 5 add`) would be refused although it
// compiles faithfully, so both gates are load-bearing.
func (e *Engine) refuseForwardStackDrift(sig *Signature, positions []int) {
	es := e.registry.Check.Recorder()
	if !es.active() || sig == nil || sig.BarrierPos == 0 || sig.fullStack() || len(positions) < 2 {
		return
	}
	// Find the top-of-stack matched arg (highest tape position) and whether any
	// deeper matched arg is non-dynamic.
	topPos, deeperConcrete := -1, false
	for _, p := range positions {
		if p < 0 || p >= e.tape.Len() {
			return
		}
		if p > topPos {
			topPos = p
		}
	}
	for _, p := range positions {
		if p != topPos && !e.tape.At(p).Dynamic {
			deeperConcrete = true
		}
	}
	if !e.tape.At(topPos).Dynamic || !deeperConcrete {
		return
	}
	nxt := e.pointer + 1
	if nxt >= e.tape.Len() {
		return
	}
	if forwardLiteralOperand(e.tape.At(nxt)) {
		es.MarkUncompilable("forward operand accounting across a dynamic/island residual (Stage 3)")
	}
}

// refuseStrandedMemberFn refuses (compile mode only) a dispatch that consumes a
// stack operand while a parked FUNCTION VALUE sits directly beneath it — the
// mid-expression member-fn-apply divergence of design/EDGE-SPEC-FINDINGS.0.md
// §2. The interpreter auto-applies a surfaced member fn (`m.double`) to the
// value that lands on it, so `m.double 21 eq 42` runs `(m.double 21)` → 42
// BEFORE `eq`; the compiler instead lets `eq` consume `21` and applies the
// stranded fn at the residual tail (to the wrong value). The bare statement-tail
// apply `m.double 21` never reaches here — nothing dispatches above the fn — so
// it keeps compiling. No-op outside a compile pass (recorder inactive).
func (e *Engine) refuseStrandedMemberFn(positions []int) {
	es := e.registry.Check.Recorder()
	if !es.active() {
		return
	}
	// Deepest stack operand this dispatch consumes.
	minStack := -1
	for _, p := range positions {
		if p >= 0 && p < e.pointer && (minStack == -1 || p < minStack) {
			minStack = p
		}
	}
	if minStack <= 0 {
		return
	}
	// Walk to the FIRST data value directly below the consumed operands (skipping
	// pure structural markers), stopping at a scope / statement boundary. Only an
	// IMMEDIATELY-adjacent parked fn is the stolen-arg hazard: a fn buried under
	// other residual values (a previous statement's leftover) is not stranded
	// against THIS dispatch's operand, so the scan stops at the first value
	// regardless of what it is.
	for i := minStack - 1; i >= 0; i-- {
		v := e.tape.At(i)
		if IsOpenParen(v) || IsCloseParen(v) || IsForward(v) || IsEnd(v) || IsDefCleanup(v) {
			return // scope / statement boundary
		}
		if v.Parent.ConformsTo(TMark) || v.Parent.ConformsTo(TMove) ||
			v.Parent.ConformsTo(TInternal) || v.Parent.ConformsTo(TReturnCheck) {
			continue // pure structural marker between the operand and its neighbour
		}
		// First data value directly below the operand. Only a CONTAINER MEMBER
		// read is the hazard this guard owns (design/EDGE-SPEC-FINDINGS.0.md §2):
		// its result is a checker-typed dynamic(Any) whose PROVENANCE (memberFnRead)
		// marks it as a fn-valued member surfaced by a get-family read. A bare
		// Function/FnDef value here (a `c/r` param ref, a factory closure) is a
		// DIFFERENT boundary (M2a `apply`, the residual leading/trailing apply) with
		// its own handling — do NOT claim it, or those refuse with the wrong reason.
		if es.memberFnRead(v.ID) {
			es.MarkUncompilable("member fn value auto-applies mid-expression (fn-value-call boundary, Stage 3)")
		}
		return
	}
}

// forwardLiteralOperand reports whether t is a single ATOMIC LITERAL that
// forward collection would grab as one operand: a concrete scalar/atom or a
// bare type node. It deliberately excludes structural tokens (paren markers,
// End, Forward, DefCleanup, Mark/Move/Internal/ReturnCheck) and ParenExpr
// data — none of which a forward-collecting word treats as a lone literal
// operand, and all of which showed up as false positives in the drift sweep
// (an `eq`/`and` comparison residual ending on `)` compiles faithfully
// all-stack). The structural exclusions mirror checkForwardStrandsOperand's
// scope-boundary set (a CloseParen marker's Parent quirkily conforms to
// TScalar, so the type test below is not sufficient on its own). Used only by
// refuseForwardStackDrift.
func forwardLiteralOperand(t Value) bool {
	if IsOpenParen(t) || IsCloseParen(t) || IsForward(t) || IsEnd(t) ||
		IsDefCleanup(t) || IsParenExpr(t) ||
		t.Parent.ConformsTo(TMark) || t.Parent.ConformsTo(TMove) ||
		t.Parent.ConformsTo(TInternal) || t.Parent.ConformsTo(TReturnCheck) {
		return false
	}
	if IsBareTypeNode(t) {
		return true
	}
	if !IsConcrete(t) || t.Parent == nil {
		return false
	}
	return t.Parent.ConformsTo(TScalar) || t.Parent.ConformsTo(TAtom)
}

// execMatch executes a matched signature, splicing args and results.
func (e *Engine) execMatch(match *MatchResult) error {
	n := match.Sig.TotalArgs()

	// Use recorded positions if available, otherwise derive from stack.
	indices := match.Positions
	if len(indices) == 0 && n > 0 {
		indices = e.resolvedIndicesBefore(n)
	}
	// Sort indices ascending for splice operations.
	sortedIndices := make([]int, len(indices))
	copy(sortedIndices, indices)
	for i := 1; i < len(sortedIndices); i++ {
		for j := i; j > 0 && sortedIndices[j] < sortedIndices[j-1]; j-- {
			sortedIndices[j], sortedIndices[j-1] = sortedIndices[j-1], sortedIndices[j]
		}
	}

	// Process consumed arguments:
	// - Maps with Eval=true: auto-evaluate their values now, so word
	//   handlers receive resolved data (e.g. {base:hex} → {base:atom(hex)}).
	// - Lists with Eval=true: auto-evaluate their contents now, so word
	//   handlers receive resolved data (e.g. [c1 c2] → [map1, map2]).
	//   Lists at QuoteArgs positions are NOT evaluated (code bodies for
	//   def, if, for, do, etc.).
	//
	// A pending gen spec is SUSPENDED for the duration: it belongs to
	// the word being dispatched (the constructor following `gen
	// [...]`), not to constructors nested inside its arguments — a
	// record field like `f:(fnsig [[T] [T]])` must build a plain
	// fn-shape, not steal the spec.
	restoreGen := e.registry.SuspendPendingGen()
	defer restoreGen()
	// For fn-body dispatches, snapshot the binding-mutation counter
	// before arg auto-evaluation: the TCO eligibility gate declines
	// eager teardown when the auto-eval below touched any binding.
	var defMutsBefore int64
	if match.Sig.fnFrame() != nil {
		defMutsBefore = e.registry.Defs.Mutations()
	}
	for i := range match.Args {
		if match.Args[i].Eval && !match.Args[i].Quoted {
			if match.Args[i].Parent.Equal(TMap) &&
				match.Args[i].Data != nil && !IsTypedMap(match.Args[i]) && !IsRecordType(match.Args[i]) && !IsOptionsType(match.Args[i]) {
				// NoEvalMapArgs (separate from the list-only
				// NoEvalArgs) suppresses map auto-evaluation at this
				// slot. Used by def's typed-name sig so a Word at the
				// type position arrives raw — important when the type
				// is a fn that's also a registered callable.
				noEval := match.Sig.NoEvalMapArgs != nil && match.Sig.NoEvalMapArgs[i]
				if !noEval {
					// A `make` construction body is DATA: its computed shared-mutable
					// values record as per-run OpMakeMap events rather than baking as
					// aliasable consts (a class/refine SCHEMA body keeps folding its
					// field defaults). Keyed on the dispatched word.
					evaluated, err := e.autoEvalMap(match.Args[i], match.Name == "make", true)
					if err != nil {
						return err
					}
					match.Args[i] = evaluated
				}
			} else if match.Args[i].Parent.Equal(TList) &&
				match.Args[i].Data != nil && !IsTypedList(match.Args[i]) && !IsTableType(match.Args[i]) {
				// NoEvalArgs suppresses list auto-evaluation for code-body
				// positions (def body, if branches, for body, etc.).
				noEval := match.Sig.NoEvalArgs != nil && match.Sig.NoEvalArgs[i]
				if noEval {
					// Check-mode use-scan: a code body (a Test.test / each /
					// fold / do / generator quotation) is NOT evaluated here,
					// so a name referenced ONLY inside it would never reach
					// recordUse and would be falsely flagged unused_def — the
					// "opaque body" FP (a shared fixture value, a generator's
					// charset, a test helper). Walk the body's bare words and
					// record each as a use. Sound for unused-def: a name a
					// runnable body references is a genuine use. Walk the
					// ELEMENTS (the body list itself may be marked quoted,
					// which WalkBodyWords would skip).
					if e.registry.Check.IsActive() {
						if lst, lerr := AsList(match.Args[i]); lerr == nil {
							WalkBodyWords(lst.Slice(), func(w WordInfo, _ Value) {
								e.registry.Check.recordUse(w.Name)
							})
						}
					}
				} else {
					// Bare words never degrade to data: a list element
					// that fails to evaluate (an undefined name, or a
					// valid name dispatched with the wrong arity) is an
					// error, not a silent fallback to its literal word.
					// Use `foo/q` for an atom, `quote […]` for a literal
					// word list / quotation.
					evaluated, err := e.autoEvalList(match.Args[i], true)
					if err != nil {
						return err
					}
					match.Args[i] = evaluated
				}
			}
		}
		match.Args[i].Eval = false
		match.Args[i].Undefined = false
	}
	// Arg evaluation done — the dispatched word's handler is the
	// intended consumer of any suspended gen spec.
	restoreGen()

	// Static type-check mode: skip the handler, splice carrier results
	// derived from Signature.ReturnsFn / Signature.Returns. The rest of
	// the dispatch machinery (positions, splicing, forward resolution)
	// is shared with normal execution, so runtime and checker stay in
	// parity.
	//
	// Signatures marked RunInCheckMode opt out of this intercept —
	// used by words whose side effects (def, undef, fn, type, …)
	// are prerequisites for subsequent analysis.
	if e.registry.Check.IsActive() && !match.Sig.runInCheckMode() {
		// The dispatch name: the word at the pointer, or — for a
		// VALUE dispatch (a module wrapper's trivial-delegation
		// short-circuit steps the Function literal, not a Word) — the
		// match's own name. A true anonymous lambda has both empty,
		// which is what the emit pass keys its fn-value refusal on.
		name := match.Name
		var pos SrcPos
		if e.pointer < e.tape.Len() && IsWord(e.tape.At(e.pointer)) {
			pos = e.tape.At(e.pointer).Pos
			if w, err := AsWord(e.tape.At(e.pointer)); err == nil {
				name = w.Name
			}
		}

		// FullStack signatures in check mode: if a CheckFullStackFn
		// is declared, it receives the preserved carrier stack
		// (below args) and returns the complete replacement for
		// base..end (matching the runtime FullStack path). end
		// covers both the word itself and any forward-collected
		// arg positions so the splice consumes every token the
		// call actually bound.
		if match.Sig.fullStack() && match.Sig.checkFullStackFn() != nil {
			base := 0
			for i := e.pointer - 1; i >= 0; i-- {
				if IsOpenParen(e.tape.At(i)) {
					base = i + 1
					break
				}
			}
			end := e.pointer
			for _, p := range sortedIndices {
				if p > end {
					end = p
				}
			}
			preserved := e.resolvedStackBeforeFrom(base, sortedIndices)
			results := match.Sig.checkFullStackFn()(match.Args, preserved, e.registry)
			e.tape.Splice(base, end+1-base, results...)
			e.pointer = base
			return nil
		}

		// Paren-tail lookahead: is the token right after this call's consumed
		// range a CloseParen? If so the result is the enclosing group's value
		// (consumed), which lets carrierResults safely model the mixed-arity
		// gradual arity under a real compile (a `set` over a dynamic receiver
		// bound by `def`). A statement-position call (next token is a word that
		// could collect the result) gets tailConsumed=false and the faithful
		// 0-arity stands.
		callEnd := e.pointer
		for _, p := range sortedIndices {
			if p > callEnd {
				callEnd = p
			}
		}
		tailConsumed := callEnd+1 < e.tape.Len() && IsCloseParen(e.tape.At(callEnd+1))
		results := carrierResults(e.registry, name, match.Sig, match.Args, pos, match.Reg, tailConsumed)
		return e.spliceMatchResults(match, sortedIndices, n, results)
	}

	// Compute context (cheap O(1) call).
	ctx := e.registry.Contexts.TopData()

	var fullStack []Value
	if match.Sig.fullStack() {
		// Find the nearest open-paren barrier so that FullStack handlers
		// only replace within the current paren scope, not below it.
		base := 0
		for i := e.pointer - 1; i >= 0; i-- {
			if IsOpenParen(e.tape.At(i)) {
				base = i + 1
				break
			}
		}
		// Collect the full resolved stack before the pointer (from base),
		// excluding the matched args and forwards.
		fullStack = e.resolvedStackBeforeFrom(base, sortedIndices)
		results, err := match.Sig.dispatchHandler()(match.Args, ctx, fullStack, e.registry)
		if err != nil {
			return e.stampErrPos(err)
		}
		if e.recorder != nil {
			e.recorder.OnCall(match.Name, n, len(results))
		}
		// FullStack handler returns the complete replacement for
		// everything from base through the pointer (inclusive).
		e.tape.Splice(base, e.pointer+1-base, results...)
		e.pointer = base
		return nil
	}

	// Tail calls (design/TCO-STAGED.10.md): an AQL fn-body dispatch
	// (Sig.FnFrame non-nil — natives skip on the nil check) sitting in
	// tail position of an enclosing fn frame is counted; when the
	// eligibility gate passes (no binding mutations during arg
	// auto-eval, plain teardown names, no generics, kill switch off)
	// the enclosing frame's teardown runs eagerly, before the handler
	// pushes the callee's per-call state — replacement by ordering,
	// for self AND mutual tail calls alike. Clean frames whose
	// ReturnCheck the callee's own check subsumes (returnsConform)
	// take FULL replacement: the callee's frame tokens replace the
	// caller's entire frame region after the handler returns, keeping
	// tape and stacks O(1) across the chain. Values-below frames and
	// non-conforming returns take the shell variant — the marker run
	// is deleted but the caller's shell (parens + ReturnCheck) stays,
	// so leftover values and the caller's return contract behave
	// exactly as under nesting. A declined call nests as before —
	// correctness never depends on firing.
	var fullReplace *frameTailScan
	if match.Sig.fnFrame() != nil {
		if scan, ok := e.probeTailCall(sortedIndices, n); ok {
			e.registry.TCO.Detected++
			if e.tcoEligible(scan, match.Sig, defMutsBefore) {
				if scan.ValuesBelow || !e.returnsConform(scan, match.Sig) {
					if err := e.elideTailFrame(scan); err != nil {
						return err
					}
					e.registry.TCO.Elided++
				} else {
					// State teardown now; the frame-region splice
					// happens below, once the handler has produced
					// the replacement tokens. The handler edits no
					// tape, so the scan's indices stay valid.
					if err := e.teardownFrameState(scan); err != nil {
						return err
					}
					fullReplace = &scan
				}
			}
		}
	}

	results, err := match.Sig.dispatchHandler()(match.Args, ctx, nil, e.registry)
	if err != nil {
		return e.stampErrPos(e.maybeAddFnShapeHint(err))
	}
	if e.recorder != nil {
		e.recorder.OnCall(match.Name, n, len(results))
	}

	// Stamp handler-produced ReturnCheck markers and fresh Function/FnDef
	// values that lack a position with the call-site word's position, so a
	// return-type error (named-fn body or anonymous fn value) points at the
	// call/construction rather than the last textual occurrence of the name.
	if e.pointer >= 0 && e.pointer < e.tape.Len() {
		stampResultPos(results, e.tape.At(e.pointer).Pos)
	}

	// Full frame replacement: the callee's frame (the handler result,
	// a complete `( body… tail )` carrying its own ReturnCheck)
	// replaces the caller's entire frame region. The pointer lands on
	// the new frame's open paren — the same re-step position a normal
	// splice would give relative to the spliced tokens. Fn-body sigs
	// never set ParkResult, so skipping that block is moot.
	if fullReplace != nil {
		e.tape.Splice(fullReplace.FrameOpen, fullReplace.CloseIdx+1-fullReplace.FrameOpen, results...)
		e.pointer = fullReplace.FrameOpen
		e.registry.TCO.Replaced++
		return nil
	}

	if err := e.spliceMatchResults(match, sortedIndices, n, results); err != nil {
		return err
	}
	// ParkResult words (notably `ref`) leave their result as inert data at
	// the call site rather than re-stepping it: advance the pointer past the
	// spliced result so an unquoted Function value does NOT auto-dispatch
	// here (matching the `/r` word-suffix). The value still dispatches when
	// re-stepped elsewhere — retrieved from a map, unwrapped from a paren.
	if match.Sig.parkResult() {
		e.pointer += len(results)
	}
	return nil
}

// maybeAddFnShapeHint wraps a signature_error from a fn-dispatch
// failure with a §7.2 hint when the caller is `def`'s typed-binding
// body slot expecting a fn-shape value. Without this, the user sees
// a confusing "no matching signature for double" error pointing at
// double's call site rather than the typed-binding context that
// caused the fn to be invoked at all.
func (e *Engine) maybeAddFnShapeHint(err error) error {
	if err == nil {
		return nil
	}
	aqlErr, ok := err.(*AqlError)
	if !ok || aqlErr.Code != "signature_error" {
		return err
	}
	if !e.isFnShapeTypedBindingContext() {
		return err
	}
	hint := "this is a typed-binding context expecting a function value — did you mean `" + aqlErr.Src + "/q`?"
	if aqlErr.Hint != "" {
		aqlErr.Hint = aqlErr.Hint + "\n  = " + hint
	} else {
		aqlErr.Hint = hint
	}
	return aqlErr
}

// spliceMatchResults replaces the word and its matched args on the
// stack with the supplied results. Shared between handler execution
// and carrier-based check-mode execution so both paths stay in parity.
func (e *Engine) spliceMatchResults(match *MatchResult, sortedIndices []int, n int, results []Value) error {
	if len(sortedIndices) == n && n > 0 {
		firstArgIdx := sortedIndices[0]

		// Compact: slide non-skip elements over skip elements in
		// [firstArgIdx..pointer] to preserve internal forwards.
		skipSet := make(map[int]bool, n+1)
		for _, idx := range sortedIndices {
			skipSet[idx] = true
		}
		skipSet[e.pointer] = true // skip the word itself

		dst := firstArgIdx
		for i := firstArgIdx; i <= e.pointer; i++ {
			if !skipSet[i] {
				e.tape.Set(dst, e.tape.At(i))
				dst++
			}
		}
		// Splice out the compacted garbage, insert results.
		e.tape.Splice(dst, e.pointer+1-dst, results...)
		e.pointer = firstArgIdx
	} else if n == 0 {
		// No args, just replace the word with results.
		e.tape.Splice(e.pointer, 1, results...)
		// Pointer stays at same position to re-examine results.
	} else {
		// Fallback: simple contiguous splice.
		argStart := e.pointer - n
		if argStart < 0 {
			argStart = 0
		}
		e.tape.Splice(argStart, e.pointer+1-argStart, results...)
		e.pointer = argStart
	}

	return nil
}

// rearrangeForForward reorders the N = stackArgs + forwardArgs resolved
// values before the current pointer to match what the unified matcher
// (post §1.4) will read with ForceStack on retry: stack-top-down is sig
// order. The first-collected forward arg is the canonical sig[0], so it
// has to sit on top of the stack. The pre-existing prefix args stay
// where they were (their order maps to sig[fwdCount..N-1] under the
// same top-down read).
//
// Before: [..., stk_0, stk_1, ..., stk_{S-1}, fwd_0, fwd_1, ..., fwd_{F-1}, WORD]
// After:  [..., stk_0, stk_1, ..., stk_{S-1}, fwd_{F-1}, ..., fwd_1, fwd_0, WORD]
//
// Under the post-rearrange layout, stack-top-down = [fwd_0, fwd_1, …,
// fwd_{F-1}, stk_{S-1}, stk_{S-2}, …, stk_0], which the unified
// matcher reads as sig[0..N-1] in order.
func (e *Engine) rearrangeForForward(stackArgs, forwardArgs int) {
	total := stackArgs + forwardArgs
	if total == 0 {
		return
	}

	indices := e.resolvedIndicesBefore(total)
	if len(indices) < total {
		return
	}

	// Extract values in current order.
	values := make([]Value, total)
	for i, idx := range indices {
		values[i] = e.tape.At(idx)
	}

	// Reorder: stack args stay in source order; forward args go after
	// them in REVERSED collection order so fwd_0 sits at the top.
	reordered := make([]Value, total)
	for i := 0; i < stackArgs; i++ {
		reordered[i] = values[i]
	}
	for i := 0; i < forwardArgs; i++ {
		reordered[stackArgs+i] = values[total-1-i]
	}

	// Write back.
	for i, idx := range indices {
		e.tape.Set(idx, reordered[i])
	}
}

// resolvedIndicesBefore returns the indices of the last n resolved values
// before the current pointer, stopping at open-paren barriers.
func (e *Engine) resolvedIndicesBefore(n int) []int {
	var indices []int
	for i := e.pointer - 1; i >= 0 && len(indices) < n; i-- {
		if IsOpenParen(e.tape.At(i)) {
			break
		}
		if IsForward(e.tape.At(i)) || IsMark(e.tape.At(i)) || IsMove(e.tape.At(i)) {
			continue
		}
		indices = append(indices, i)
	}
	// Reverse so indices are in stack order (ascending).
	for i, j := 0, len(indices)-1; i < j; i, j = i+1, j-1 {
		indices[i], indices[j] = indices[j], indices[i]
	}
	return indices
}

// resolvedStackBeforeFrom returns all resolved values from position 'from'
// up to the pointer, excluding forwards, open-parens, marks, moves,
// and the matched arg indices.
func (e *Engine) resolvedStackBeforeFrom(from int, excludeIndices []int) []Value {
	exclude := make(map[int]bool, len(excludeIndices))
	for _, idx := range excludeIndices {
		exclude[idx] = true
	}
	var stack []Value
	for i := from; i < e.pointer; i++ {
		if exclude[i] || IsForward(e.tape.At(i)) || IsOpenParen(e.tape.At(i)) || IsMark(e.tape.At(i)) || IsMove(e.tape.At(i)) {
			continue
		}
		stack = append(stack, e.tape.At(i))
	}
	return stack
}

// insertForward records a deferred dispatch by placing a Forward
// marker after the func word on the stack. The marker carries the
// matched signature plus arg-count bookkeeping; subsequent literals
// are routed into its arg slots until ExpectedArgs is reached, at
// which point the marker triggers handler execution.
// forceStackWord rewrites the word at idx into its force-stack form
// (ForceStack=true) while preserving the source position. Used at the
// forward-collection completion points where the word must re-dispatch
// against the stack; callers set e.pointer themselves as needed.
func (e *Engine) forceStackWord(idx int, w WordInfo) {
	pos := e.tape.At(idx).Pos
	nw := NewWordModified(w.Name, w.ArgCount, true, false)
	nw.Pos = pos // preserve source position across force-stack rewrite
	e.tape.Set(idx, nw)
}

func (e *Engine) insertForward(w WordInfo, sig *Signature, forwardNeeded, stackArgs, specAt int) error {
	var pos SrcPos
	if e.pointer >= 0 && e.pointer < e.tape.Len() {
		pos = e.tape.At(e.pointer).Pos
	}
	fwd := NewForward(ForwardInfo{
		FuncName:     w.Name,
		ExpectedArgs: forwardNeeded,
		StackArgs:    stackArgs,
		FuncIndex:    e.pointer,
		Sig:          sig,
		Pos:          pos,
		// The plan's stop condition, threaded from matchSignature:
		// specAt >= 0 means slot specAt was planned from a word that
		// will DISPATCH rather than arrive (see ForwardInfo docs).
		Speculative:   specAt >= 0,
		SpeculativeAt: max(specAt, 0),
	})

	e.tape.Insert(e.pointer+1, fwd)

	e.pointer += 2
	return nil
}

// stepLiteral handles a resolved (non-word, non-forward) value at the pointer.
// spliceExpand returns the stack entries an __SP marker contributes when it
// reaches the pointer. A plain (non-typed) list splices its top-level
// elements; every other value — scalars, maps, typed lists, tables — splices
// as a single entry. The data is returned verbatim (unevaluated); the engine
// re-steps over it after splicing.
func spliceExpand(data Value) []Value {
	if data.Parent.Equal(TList) && data.Data != nil && !IsTypedList(data) && !IsTableType(data) {
		elems, _ := AsList(data)
		out := make([]Value, elems.Len())
		copy(out, elems.Slice())
		return out
	}
	return []Value{data}
}

// spliceIsData reports whether an __SP marker's payload expands to plain
// VALUES only — no words, paren groups, reaches, or interpolated strings at
// the top level (the level `word` splices; nested lists stay values). A data
// splice contributes the same values in any context, so the `f w ≡ f (w)`
// paren expansion of a splice-bound word in a forward-argument position is
// sound for it. A code-bearing splice is a Forth-style macro that runs
// against the LIVE stack — paren isolation would change its meaning (`def
// inc word [1 add]  1 inc inc inc` needs each `add` to see the caller's
// values) — so it stays on the existing standalone-fire / boundary-word
// paths; group explicitly (`f (p)`) to use its result as an operand.
func spliceIsData(info SpliceInfo) bool {
	for _, el := range spliceExpand(info.Data) {
		if IsWord(el) || IsParenExpr(el) || IsReach(el) || IsInterpString(el) || IsSplice(el) {
			return false
		}
	}
	return true
}

// pendingForwardIdx returns the stack index of the nearest pending Forward
// below the pointer, stopping at an OpenParen barrier; -1 when none. This is
// the "is the value at the pointer being collected?" probe shared by
// stepLiteral's collection dispatch and the splice-word paren expansion in
// stepWord's def-substitution branch.
func (e *Engine) pendingForwardIdx() int {
	for i := e.pointer - 1; i >= 0; i-- {
		if IsOpenParen(e.tape.At(i)) {
			return -1
		}
		if IsForward(e.tape.At(i)) {
			return i
		}
	}
	return -1
}

func (e *Engine) stepLiteral() error {
	valIdx := e.pointer

	// A ParenExpr reaching stepLiteral (nested inside a collapsing paren
	// span, where the in-place collapse loops in preEvalParens and
	// stepCloseParen fall through to here) is expanded to its marker span
	// in place and re-stepped — the surrounding loop's OpenParen handling
	// then collapses it on this engine. Without this, a nested ParenExpr
	// would be pushed unevaluated. See design/PAREN-REPRESENTATION.9.md
	// (paren-nesting Steps 2/3).
	// Step 4: a Quoted ParenExpr (codequote-captured data) and a ParenExpr
	// being collected by a raw-capture pending forward are NOT expanded —
	// they fall through to normal literal handling (pushed as data /
	// collected as the forward arg). Otherwise expand and re-step (the
	// nested-paren case).
	if IsParenExpr(e.tape.At(valIdx)) && !e.tape.At(valIdx).Quoted && !e.pendingForwardWantsRawParen() {
		items, _ := AsParenExpr(e.tape.At(valIdx))
		e.tape.Splice(valIdx, 1, expandParenExpr(items)...)
		return nil
	}
	// A Reach reaching stepLiteral (nested in a collapsing span, or a
	// collected list/map element) lowers to its get-chain in place, like a
	// ParenExpr (Reach Phase B). Quoted/raw-pending reaches fall through.
	if isEvalReach(e.tape.At(valIdx)) && !e.pendingForwardWantsRawParen() {
		info, _ := AsReach(e.tape.At(valIdx))
		e.tape.Splice(valIdx, 1, expandReach(info)...)
		return nil
	}

	// Look backwards for the nearest forward entry, stopping at open-paren barriers.
	fwdIdx := e.pendingForwardIdx()

	if fwdIdx < 0 {
		// __SP (splice) marker: replace it, unevaluated, with its payload
		// and re-step. A plain list contributes its top-level elements;
		// any other value contributes itself. The spliced content runs
		// against the live stack (Forth-style macro). No pending forward
		// means it is being processed as a standalone stack entry — the
		// only place a splice should fire (as an arg it is collected by
		// value via the TAny match in the collection branch below).
		if IsSplice(e.tape.At(valIdx)) {
			info, _ := AsSplice(e.tape.At(valIdx))
			e.tape.Splice(valIdx, 1, spliceExpand(info.Data)...)
			return nil
		}
		// A dispatch-modifier marker reaching the pointer standalone means
		// the preceding value was NOT a pending function (so execFnDefLiteral
		// never consumed it) — e.g. `(1 add 2)/s`. The modifier is a no-op on
		// a non-function result: drop the marker.
		if IsDispatchMod(e.tape.At(valIdx)) {
			e.tape.Remove(valIdx)
			return nil
		}
		// Shaped-instance-method model (Stage M2c, method_shape.go): a DYNAMIC
		// method-read carrier at the pointer — where the interpreter would
		// auto-dispatch the concrete member — models that dispatch on the
		// compile pass and records a guarded mid-stream OpCallDynMethod.
		// Declines (leaving today's paths untouched) outside a live compile
		// pass and for any window/match shape it cannot prove.
		if e.registry.Check.IsActive() && e.tryShapedMethodDispatch(valIdx) {
			return nil
		}
		// General dynamic-fn-value dispatch (method_shape.go): a DYNAMIC carrier
		// whose bound is Function-bearing (a typed-patrun `find` result) followed
		// by an inert forward window collapses to one dynamic(Any) on the
		// plain-check surface, clearing the arg-stranding the compiled path lowers
		// via resolveDynamicApply. Declines outside plain check and for any
		// non-callable bound or non-inert window.
		if e.registry.Check.IsActive() && e.tryDynamicFnValueDispatch(valIdx) {
			return nil
		}
		// If the value is a FnDef/TFunction, execute it. Quoted function
		// values are treated as data (not executed).
		val := e.tape.At(valIdx)
		if (val.Parent.Equal(TFnDef) || val.Parent.Equal(TFunction)) &&
			val.Data != nil && !val.Quoted {
			if _, ok := val.Data.(FnDefInfo); ok {
				return e.execFnDefLiteral(valIdx)
			}
		}
		// Record the literal-push event for any installed Recorder.
		// Skip engine-internal control values (markers, the recorded
		// FnDef-as-data above is handled by OnCall when it dispatches).
		if e.recorder != nil && isRecordableLiteral(val) {
			e.recorder.OnPushLit(val)
		}
		e.pointer++
		return nil
	}

	fwd, _ := AsForward(e.tape.At(fwdIdx))
	funcIdx := fwd.FuncIndex

	// Check if the value matches the next expected arg positionally.
	// Once matchSignature has chosen a signature, args are collected in
	// order — no permutation or sig switching is permitted.
	//
	// When a /q-marked TAtom slot accepts a Word, convert the Word to
	// an Atom in place so the eventual handler sees a uniform Atom
	// value rather than having to polymorphically extract a name from
	// either shape.
	if fwd.CollectedArgs < fwd.ExpectedArgs {
		val := e.tape.At(valIdx)
		nextIdx := fwd.CollectedArgs
		matches := sigArgMatches(fwd.Sig, nextIdx, val)
		if !matches && fwd.Sig.QuoteArgs != nil && fwd.Sig.QuoteArgs[nextIdx] &&
			val.Parent.Equal(TWord) && TAtom.ConformsTo(sigArgType(fwd.Sig, nextIdx)) {
			w, _ := AsWord(val)
			atom := NewAtom(w.Name)
			atom.Pos = val.Pos // preserve source position across /q Word→Atom conversion
			e.tape.Set(valIdx, atom)
			matches = true
		}
		if !matches {
			// Type mismatch — implicit end: resolve forward from stack.
			return e.implicitEnd(fwdIdx)
		}
	}

	// Remove the value from its current position.
	val := e.tape.At(valIdx)
	e.tape.Remove(valIdx)

	// After removal, adjust indices if valIdx was before them.
	if valIdx < funcIdx {
		funcIdx--
	}
	if valIdx < fwdIdx {
		fwdIdx--
	}

	// Insert the forward value right before the function word. Forward values
	// are appended in collection order (first collected = deepest).
	// After collection the stack is:
	// [..., stack_args..., fwd0, fwd1, ..., func_word]
	// The rearrangeForForward call at completion time reorders to:
	// [..., fwd0, fwd1, ..., stack_reversed..., func_word]
	insertIdx := funcIdx

	e.tape.Insert(insertIdx, val)

	funcIdx++
	fwdIdx++

	fwd.CollectedArgs++
	fwd.FuncIndex = funcIdx

	e.traceNote = fmt.Sprintf("collect %s %d/%d",
		fwd.FuncName, fwd.CollectedArgs, fwd.ExpectedArgs)

	if fwd.CollectedArgs >= fwd.ExpectedArgs {
		// All forward args collected. Remove forward, force stack, retry.
		e.tape.Remove(fwdIdx)
		// Adjust funcIdx if forward was before it (shouldn't normally happen).
		if fwdIdx < funcIdx {
			funcIdx--
		}

		if funcIdx < e.tape.Len() && IsWord(e.tape.At(funcIdx)) {
			w, _ := AsWord(e.tape.At(funcIdx))
			e.forceStackWord(funcIdx, w)
		}

		// Rearrange values for forward-first matching: forward args at
		// the deep end (sigArgs[0..F-1]), stack args reversed after them.
		e.pointer = funcIdx
		e.rearrangeForForward(fwd.StackArgs, fwd.CollectedArgs)

		// Recorder hook: emit OnPushLit for each forward-collected
		// arg now that they're laid out on the stack in the order
		// the upcoming stack-only re-dispatch will see (deepest at
		// funcIdx-F, top just below WORD at funcIdx-1). Emitting
		// deepest-first matches the replay push order — a Flatten +
		// re-run will rebuild the same stack shape.
		//
		// Stack args fired OnPushLit earlier when their source
		// literals reached the pointer (via the bare-stack branch
		// in stepLiteral). Only the forward args need this hook.
		if e.recorder != nil && fwd.CollectedArgs > 0 {
			start := funcIdx - fwd.CollectedArgs
			for i := start; i < funcIdx; i++ {
				if i >= 0 && i < e.tape.Len() {
					v := e.tape.At(i)
					if isRecordableLiteral(v) {
						e.recorder.OnPushLit(v)
					}
				}
			}
		}
	} else {
		e.tape.Set(fwdIdx, NewForward(fwd))
		e.pointer = fwdIdx + 1
	}

	return nil
}

// execFnDefLiteral handles a FnDef or TFunction value that has landed on the
// stack without a pending forward. It tries to match the function's signatures
// against preceding resolved stack values and, if a match is found, executes
// autoEvalStack walks the final stack and auto-evaluates lists and maps
// that were created by the parser (Eval=true) and not explicitly quoted.
// Runtime-created values (from word handlers, def bodies, etc.) are
// not auto-evaluated. This is called at the end of Run().
func (e *Engine) autoEvalStack() error {
	for i := 0; i < e.tape.Len(); i++ {
		val := e.tape.At(i)
		if !val.Eval || val.Quoted {
			continue
		}
		if val.Parent.Equal(TList) && val.Data != nil && !IsTypedList(val) && !IsTableType(val) {
			result, err := e.autoEvalList(val, false)
			if err != nil {
				return err
			}
			e.tape.Set(i, result)
		} else if val.Parent.Equal(TMap) && val.Data != nil && !IsTypedMap(val) && !IsRecordType(val) && !IsOptionsType(val) {
			result, err := e.autoEvalMap(val, false, false)
			if err != nil {
				return err
			}
			e.tape.Set(i, result)
		}
	}
	return nil
}

// autoEvalList evaluates the contents of a plain list in a sub-engine,
// returning a new list containing the results. For example, [1 add 2] → [3].
// consumed marks the list as a word/fn ARGUMENT being auto-evaluated (execMatch /
// execFnDefSig), as opposed to the end-of-Run residual eval (autoEvalStack).
func (e *Engine) autoEvalList(val Value, consumed bool) (Value, error) {
	elems, _ := AsList(val)
	if elems.Len() == 0 {
		return val, nil
	}
	input := make([]Value, elems.Len())
	copy(input, elems.Slice())
	result, err := runPooledSub(e.registry, input, e.isTop || consumed || e.elemEvalRecordable)
	if err != nil {
		return Value{}, err
	}
	out := NewList(result)
	// In RECORDING mode a list whose elements are COMPUTED (an event carrier, not
	// plain data — `[1 add 2]`, `[1 (2 add 3) 4]`) cannot bake as an inert const,
	// so record it as an OpMakeList assembly of the evaluated elements; otherwise
	// the list is an unresolvable residual and the program falls back. A
	// fully-literal list (`[1 2 3]`) stays inert and bakes as a pooled const.
	if e.registry.Check.IsActive() {
		if es := e.registry.Check.Recorder(); es.Armed() && !isInertConst(out) {
			switch {
			case e.isTop:
				// Top-level (frames==1): the canonical case, evaluated once.
				es.RecordMakeList(e.registry, result, out, val.Pos)
			case consumed || e.elemEvalRecordable:
				// A CONSUMED computed list ARG inside a fn body / closure
				// (`make Array [i 99]`, `f [j (g x)]`): the interpreter auto-evaluates
				// it against the LIVE def stack here, so its element locals/params
				// resolve (i → its frame slot) exactly as recordMakeListInner re-pushes
				// them per call — OpMakeList re-assembles from operands per run, sound
				// in a nested frame (the RecordMakeMap caller already relies on this).
				// The end-of-Run RESIDUAL eval (consumed=false) is NOT recorded here:
				// a fn body returning a bare-word list (`[y y]`) raises undefined_word
				// at run time (the residual sub-engine lacks the body's def-locals), so
				// baking OpMakeList would diverge — it stays unresolved and falls back.
				// A stateful generator never reaches here: its body is a NoEval arg,
				// so execMatch does not auto-evaluate it as a data list.
				es.recordMakeListInner(e.registry, result, out, val.Pos)
			}
		}
	}
	return out, nil
}

// evalInterpString evaluates an interpolated string by running each
// expression part in a sub-engine, converting results to strings, and
// concatenating everything into a single string value.
func (e *Engine) evalInterpString(val Value) (Value, error) {
	parts, err := AsInterpString(val)
	if err != nil || parts == nil {
		return NewString(""), nil
	}
	s, dynamic, holes, holesOK, err := e.evalInterpParts(parts)
	if err != nil {
		return Value{}, err
	}
	if dynamic {
		// A `${expr}` part evaluated to a CARRIER — its value is only known at run
		// time (a fn param, or any native result, which check mode synthesises as a
		// carrier). The interpolated string is therefore NOT a compile-time
		// constant. When recording, lower it to OpInterp: the hole expressions
		// already recorded their own dispatch events, and OpInterp pops their
		// results and rebuilds the string at run time (byte-identical to
		// evalInterpParts). The static result is a String carrier either way; on
		// the rare unlowerable shape (a hole producing 0 or >1 values) refuse and
		// fall back. Mirrors RecordMakeMap — re-assembled per run, no const bake.
		out := NewCarrier(TString)
		if es := e.registry.Check.Recorder(); es.active() {
			if !holesOK || !es.RecordInterp(parts, holes, out, val.Pos) {
				es.MarkUncompilable("interpolated string with a runtime-computed part")
			}
		}
		return out, nil
	}
	return NewString(s), nil
}

// evalInterpParts evaluates a list of InterpParts (literal segments plus
// `${expr}` holes), running each expression part in a sub-engine,
// stringifying its results, and concatenating into one string. Shared by
// evalInterpString and the XML-attribute / text evaluation in
// evalXmlInterp.
//
// holes carries the per-hole single result value in source order, and holesOK
// reports that EVERY hole produced exactly one value (and no flow-control
// signal interrupted the walk) — the precondition evalInterpString needs to
// lower the template to OpInterp (one operand-stack value per hole). When a
// hole yields zero or several values, holesOK is false and holes is unused
// (the caller refuses compilation and falls back); the string itself is still
// built faithfully.
func (e *Engine) evalInterpParts(parts []InterpPart) (s string, dynamic bool, holes []Value, holesOK bool, err error) {
	var buf strings.Builder
	holesOK = true
	for _, part := range parts {
		if part.Expr == nil {
			buf.WriteString(part.Lit)
			continue
		}
		result, runErr := runPooledSub(e.registry, part.Expr, false)
		if runErr != nil {
			return "", dynamic, nil, false, runErr
		}
		if len(result) == 1 {
			holes = append(holes, result[0])
		} else {
			// A hole with 0 or >1 results cannot map to one stack slot for
			// OpInterp; the string is still built below, but the template is
			// not lowerable (the caller falls back).
			holesOK = false
		}
		for _, r := range result {
			// A CHECK-MODE CARRIER part means the value is only known at
			// runtime — flag it so the caller does not bake the carrier's render
			// as a constant string. ValToString of a carrier yields a type tag
			// ("dynamic(Any)"), which the runtime never produces. Test for the
			// carrier flag specifically, NOT !IsConcrete: at run time there are
			// no carriers, but there ARE legitimate non-concrete values — None
			// and bare type literals (e.g. `${typeof x}`) — which stringify to
			// real text ("None", "Integer") and must not collapse the whole
			// interpolation to a String carrier.
			if r.Carrier {
				dynamic = true
			}
			buf.WriteString(ValToString(r))
		}
		// If the expression raised a flow-control signal, stop
		// evaluating further parts. The outer Run loop will catch
		// the flag and unwind. Continuing would call sub.Run with
		// a stale flag still set and could produce observable
		// side effects from later parts.
		if e.registry.FlowCtrl != FlowNone {
			holesOK = false
			break
		}
	}
	return buf.String(), dynamic, holes, holesOK, nil
}

// evalXmlInterp evaluates an interpolated XML literal skeleton (Word/__XI)
// to a concrete Node/Xml value, running each embedded `${expr}` against
// the live registry — the structural analogue of evalInterpString. See
// design/XML-LITERAL.0.md §4.
func (e *Engine) evalXmlInterp(val Value) (Value, error) {
	tmpl, err := AsXmlInterp(val)
	if err != nil {
		return Value{}, err
	}
	result, dynamic, err := e.buildXmlFromTmpl(tmpl)
	if err != nil {
		return Value{}, err
	}
	if dynamic {
		// A `${expr}` hole evaluated to a NON-CONCRETE value — a carrier that
		// exists only during static analysis (a fn param, or a Node/Xml const,
		// which strips to a type-only carrier in record mode). The interpolated
		// element therefore depends on runtime data and is NOT a compile-time
		// constant: baking the check-mode tree would freeze a wrong child (a
		// carrier renders as its type tag, e.g. "Xml") and diverge from the
		// interpreter, which builds the real tree at run time. When recording,
		// refuse — the VM has no XML-interpolation op, so the program falls back
		// to the interpreter (mirrors evalInterpString). At run time there are no
		// carriers, so this never fires and the concrete element below is returned.
		if es := e.registry.Check.Recorder(); es.active() {
			es.MarkUncompilable("interpolated XML with a runtime-computed part")
			return NewCarrier(TXml), nil
		}
	}
	return result, nil
}

// buildXmlFromTmpl recursively evaluates an XmlTmpl into a Node/Xml value.
// Attribute values evaluate via evalInterpParts; children evaluate per
// XmlCren kind — a literal becomes a text node, a nested template recurses,
// and an expression hole splices per the design's rule (a List contributes
// each element, a Node/Xml is one child element, any other value becomes a
// text node). Adjacent text is merged so a `hello ${name}` run yields one
// text node rather than two.
//
// The returned dynamic flag is true when any hole (an attribute part, a child
// expression, or a nested template) evaluated to a NON-CONCRETE value — a
// carrier seen only under static analysis. evalXmlInterp uses it to refuse
// const-folding while recording (the InterpString contract).
func (e *Engine) buildXmlFromTmpl(t XmlTmpl) (Value, bool, error) {
	dynamic := false
	attr := NewOrderedMap()
	for _, a := range t.Attr {
		s, dyn, _, _, err := e.evalInterpParts(a.Parts)
		if err != nil {
			return Value{}, false, err
		}
		if dyn {
			dynamic = true
		}
		attr.Set(a.Name, NewString(s))
		if e.registry.FlowCtrl != FlowNone {
			return NewXmlElement(t.Tag, attr, nil), dynamic, nil
		}
	}

	var cren []Value
	addText := func(s string) {
		if s == "" {
			return
		}
		if n := len(cren); n > 0 {
			if sp, ok := cren[n-1].Data.(StrPayload); ok {
				cren[n-1] = NewString(sp.S + s)
				return
			}
		}
		cren = append(cren, NewString(s))
	}
	// addOne adds a single evaluated value as one child node: an XML
	// element (immutable Node/Xml OR mutable FlexXml) becomes a child
	// element, any other value becomes a text node. A non-concrete result
	// (a carrier under static analysis) marks the build dynamic — its
	// ValToString render ("Xml", "dynamic(Any)") is a check-mode artefact
	// the runtime never produces, so the element must not const-fold.
	addOne := func(r Value) {
		if IsXmlValue(r) {
			cren = append(cren, r)
			return
		}
		if !IsConcrete(r) {
			dynamic = true
		}
		addText(ValToString(r))
	}
	// addChild applies the ${...} child-hole splice rule: a List
	// contributes each of its TOP-LEVEL elements (one level only — a
	// nested list element is kept as a single value, not flattened, the
	// same as `append [..]` on a FlexXml); any other value is one child.
	addChild := func(r Value) {
		if IsConcrete(r) && r.Parent.ConformsTo(TList) {
			if rl, err := AsList(r); err == nil {
				for i := 0; i < rl.Len(); i++ {
					addOne(rl.Get(i))
				}
				return
			}
		}
		addOne(r)
	}

	for _, c := range t.Cren {
		switch c.Kind {
		case XmlCrenLit:
			addText(c.Lit)
		case XmlCrenChild:
			if c.Child == nil {
				continue
			}
			child, dyn, err := e.buildXmlFromTmpl(*c.Child)
			if err != nil {
				return Value{}, false, err
			}
			if dyn {
				dynamic = true
			}
			cren = append(cren, child)
		case XmlCrenExpr:
			results, err := runPooledSub(e.registry, c.Expr, false)
			if err != nil {
				return Value{}, false, err
			}
			for _, r := range results {
				addChild(r)
			}
			if e.registry.FlowCtrl != FlowNone {
				return NewXmlElement(t.Tag, attr, cren), dynamic, nil
			}
		}
	}
	return NewXmlElement(t.Tag, attr, cren), dynamic, nil
}

// expandParenExpr returns a ParenExpr's tokens wrapped in OpenParen …
// CloseParen markers — the span the existing in-place collapse machinery
// (stepCloseParen / preEvalParens) evaluates. Used to expand a word-context
// ParenExpr value back to markers on encounter (paren-nesting Steps 2/3),
// keeping exact parity with the former marker representation. See
// design/PAREN-REPRESENTATION.9.md.
func expandParenExpr(items []Value) []Value {
	span := make([]Value, 0, len(items)+2)
	span = append(span, NewOpenParen())
	span = append(span, items...)
	span = append(span, NewCloseParen())
	return span
}

// lowerReach turns a Reach into its equivalent get/getr chain tokens:
// `recv get k1 getr k2 …`. A computed segment's key becomes a ParenExpr so
// the chain evaluates it before get/getr consumes it. This is the Stage-1
// evaluator (design/REACH.10.md §4): the chain is identical to the former
// dot-access ParenExpr, so wrapping it with expandParenExpr and running it
// in place reproduces exact get/getr semantics.
func lowerReach(info ReachInfo) []Value {
	out := make([]Value, 0, len(info.Receiver)+len(info.Segments)*2)
	out = append(out, info.Receiver...)
	for _, seg := range info.Segments {
		if seg.Getr {
			out = append(out, NewWord("dotr"))
		} else {
			out = append(out, NewWord("dot"))
		}
		if seg.Computed {
			out = append(out, NewParenExpr(seg.KeyExpr))
		} else {
			out = append(out, seg.KeyLit)
		}
	}
	return out
}

// isEvalReach reports whether v is a Reach that should auto-evaluate now:
// a parsed dot-access (Eval=true) that has not been quote-captured. An inert
// constructor-built reach (Eval=false, from `reach …`) or a codequote'd one
// (Quoted) is data and is left alone.
func isEvalReach(v Value) bool {
	if !IsReach(v) || v.Quoted {
		return false
	}
	info, _ := AsReach(v)
	return info.Eval
}

// expandReach returns the marker span an Eval Reach evaluates to — its
// lowered get-chain wrapped in paren markers, run in place exactly like a
// ParenExpr (Stage-1 lowering).
func expandReach(info ReachInfo) []Value {
	return expandParenExpr(lowerReach(info))
}

// ApplyReach evaluates a Reach against a concrete receiver value — the lens
// "get" (design/REACH.10.md §7). The reach's own Receiver tokens are ignored
// (a receiverless lens has none); recv becomes the base and the segments
// (get/getr, literal/computed, in order) walk from it via the same Stage-1
// lowering bare m.a.b uses, so getr strictness and computed-key evaluation
// are identical. It is the primitive behind the `apply` word and the
// receiverless-reach-as-Function higher-order behaviour.
func ApplyReach(r *Registry, info ReachInfo, recv Value) (Value, error) {
	toks := lowerReach(ReachInfo{Receiver: []Value{recv}, Segments: info.Segments})
	res, err := runPooledSub(r, expandParenExpr(toks), false)
	if err != nil {
		return Value{}, err
	}
	if len(res) == 0 {
		return Value{}, fmt.Errorf("apply: reach produced no value")
	}
	return res[len(res)-1], nil
}

// evalParenExprResults evaluates a ParenExpr's tokens in a sub-engine and
// returns its result value(s). Used by autoEvalMap for paren values in map
// (data) context, where a single result value is collected for a key. It
// shares the registry (defs leak) and propagates errors (a paren is not an
// error boundary, unlike `do`).
func (e *Engine) evalParenExprResults(items []Value) ([]Value, error) {
	return runPooledSub(e.registry, expandParenExpr(items), false)
}

// autoEvalMap evaluates each value in a plain map using a sub-engine.
// Word values resolve directly; lists auto-evaluate via autoEvalStack:
//
//	{r:rv}        → {r:10}      (word evaluated to its def'd value)
//	{x:[1 add 2]} → {x:[3]}     (list evaluated, stays as list)
//	{a:[1,2]}     → {a:[1,2]}   (literal list unchanged)
//	{x:"hello"}   → {x:"hello"} (strings pass through unchanged)
//
// consumed reports whether the map is being evaluated as an in-frame CONSUMED
// argument (a word/fn-def arg) rather than a DEFERRED residual (autoEvalStack
// at end of run). Only a consumed map may record an OpMakeMap event: a deferred
// residual is evaluated LATE, after its enclosing fn frame has popped, so its
// value bindings (a fn param) are gone — recording it in-frame would diverge
// from the interpreter (which errors / re-binds at the later time).
func (e *Engine) autoEvalMap(val Value, dataMap, consumed bool) (Value, error) {
	m, _ := AsMutableMap(val)
	out := NewOrderedMap()
	if m.Implicit {
		out.Implicit = true
	}

	// Computed keys: evaluate key expressions at runtime.
	var ckSet map[string]bool
	if m.Meta != nil {
		ckSet, _ = m.Meta["ck"].(map[string]bool)
	}

	for _, key := range m.Keys() {
		v, _ := m.Get(key)
		resolvedKey := key

		// Computed key: evaluate the key text as AQL code to get
		// the actual string key. E.g., {[a]:1} with def a 'x' → {x:1}
		if ckSet[key] {
			keyResult, err := runPooledSub(e.registry, []Value{NewWord(key)}, false)
			if err != nil {
				return Value{}, fmt.Errorf("computed key [%s]: %w", key, err)
			}
			if len(keyResult) == 1 {
				if keyResult[0].Parent.ConformsTo(TString) {
					resolvedKey, _ = AsString(keyResult[0])
				} else if IsAtom(keyResult[0]) {
					resolvedKey, _ = AsAtom(keyResult[0])
				} else {
					resolvedKey = ValToString(keyResult[0])
				}
			}
		}

		// Interpolated string: evaluate inline.
		if IsInterpString(v) {
			result, err := e.evalInterpString(v)
			if err != nil {
				return Value{}, err
			}
			out.Set(resolvedKey, result)
			continue
		}

		// Interpolated XML literal as a map value: evaluate inline to a
		// Node/Xml, mirroring the InterpString case.
		if IsXmlInterp(v) {
			result, err := e.evalXmlInterp(v)
			if err != nil {
				return Value{}, err
			}
			out.Set(resolvedKey, result)
			continue
		}

		// Paren expression: evaluate items as an isolated sub-expression
		// (shared via evalParenExprResults with the main-stack path).
		if IsParenExpr(v) {
			items, _ := AsParenExpr(v)
			// CHECK-MODE const-fold: a computed container value (a class field
			// default like (make Foo 1), or a data-map (1 add 2)) evaluated
			// abstractly leaves a recorded event the container then swallows
			// ("unconsumed call results"), so the program refuses. When the
			// expression is DETERMINISTIC it is a compile-time constant — fold
			// it to its concrete value so the container bakes as a const. The
			// downstream const-bake gate (typeBodyConstOK for a schema default,
			// isInertConst for a data map) decides mutation-safety, so an
			// instance still bakes only where `make` copies it per instance.
			// ONLY at the TOP frame: a container inside a fn
			// body / for body / closure (`for 3 [{a: (3 mul i)} get a]`) is
			// RE-EVALUATED per call or iteration, often with a different binding
			// (the loop iterator `i`). The fold's determinism check (two equal
			// concrete evals) does NOT catch that — `i` is stable WITHIN the fold —
			// so freezing the value would replicate it across iterations. Those
			// keep refusing and fall back (mirrors the OpMakeList gate).
			// The expression must also not REFERENCE a CARRIER binding (a def-local
			// bound to a computed value, `def v0 (0 add 3) ... {a: (5 mul v0)}`): the
			// concrete fold coerces the carrier (e.g. to 0) and freezes a WRONG value
			// (the determinism check sees the same coerced 0 twice). exprRefsCarrier
			// catches that; a user TYPE binding (Carrier=false) still folds.
			topFrame := e.registry.Check.Recorder().topFrameOnly()
			if e.registry.Check.IsActive() && topFrame && !e.exprRefsCarrier(items) {
				if folded, ok := e.constFoldContainerVal(items); ok {
					// Bake the computed value as a const EXCEPT in a `make`
					// construction body (dataMap) when the value is shared-mutable: a
					// data-map instance is stored VERBATIM by make (MakeClassFieldValue),
					// so a baked const would alias across runs. Leave it to the recording
					// eval below — its make event records, and RecordMakeMap re-assembles
					// the map per run. A SCHEMA default (dataMap=false) still folds + bakes
					// as a template that make's FreshenDefault copies, unchanged.
					if (!dataMap && !e.elemEvalRecordable) || !containsSharedMutable(folded) {
						out.Set(resolvedKey, folded)
						continue
					}
				}
			}
			result, err := e.evalParenExprResults(items)
			if err != nil {
				return Value{}, err
			}
			if len(result) == 1 {
				out.Set(resolvedKey, result[0])
			} else if len(result) > 1 {
				out.Set(resolvedKey, NewList(result))
			}
			continue
		}

		// Reach map value (e.g. {x: m.a}) — evaluate its lowered get-chain
		// as an isolated sub-expression, like a ParenExpr (Reach Phase B).
		if isEvalReach(v) {
			info, _ := AsReach(v)
			result, err := e.evalParenExprResults(lowerReach(info))
			if err != nil {
				return Value{}, err
			}
			if len(result) == 1 {
				out.Set(resolvedKey, result[0])
			} else if len(result) > 1 {
				out.Set(resolvedKey, NewList(result))
			}
			continue
		}

		// CHECK-MODE const-fold for a bare value — the mirror of the ParenExpr
		// fold above for the un-parenthesised form. A bare word that is a 0-arg
		// fn auto-fires as a map value (`{a:g}` → `{a:42}`, exactly like the
		// parenthesised `{a:(g)}`); evaluated abstractly in the sub-engine below
		// it leaves a carrier of unknown provenance and the map refuses. When the
		// value is DETERMINISTIC at the top frame and references no carrier
		// binding, fold it to its concrete result (identical to the sub-engine
		// eval the interpreter runs) so the map bakes as a const. Same gating and
		// mutation-safety screen as the ParenExpr branch.
		if e.registry.Check.IsActive() &&
			e.registry.Check.Recorder().topFrameOnly() && !e.exprRefsCarrier([]Value{v}) {
			if folded, ok := e.constFoldContainerVal([]Value{v}); ok {
				if (!dataMap && !e.elemEvalRecordable) || !containsSharedMutable(folded) {
					out.Set(resolvedKey, folded)
					continue
				}
			}
		}

		// Evaluate each value in a pooled sub-engine.
		result, err := runPooledSub(e.registry, []Value{v},
			e.isTop || consumed || e.elemEvalRecordable)
		if err != nil {
			return Value{}, err
		}
		if len(result) == 1 {
			out.Set(resolvedKey, result[0])
		} else if len(result) > 1 {
			out.Set(resolvedKey, NewList(result))
		}
		// RECORDING mode: a LIST-valued entry (`{n:[expr]}` — the `do {map}` idiom,
		// where each value is a code list left AS a list) is a COMPUTED list whose
		// elements have provenance (their dispatches recorded above) but whose list
		// WRAPPER was never recorded — RecordMakeList's top-frame guard refuses a
		// fn-body list. Record the OpMakeList HERE, inline, right after this value's
		// dispatches, so the assembly is interleaved with the per-value events in
		// stack order (`get_n, wrap_n, get_m, wrap_m, …`) — wrapping in RecordMakeMap
		// afterward would find the next value's result on top. Sound (and gated like
		// the enclosing OpMakeMap) only when the map is CONSUMED in-frame: OpMakeList
		// re-assembles from its operands per run, so it never freezes a per-call
		// binding. recordMakeListInner declines (leaving es untouched) for an
		// unresolvable / stateful / type-pattern element, so the map then falls back.
		if (consumed || e.elemEvalRecordable) && e.registry.Check.IsActive() {
			if es := e.registry.Check.Recorder(); es.Armed() {
				if lv, _ := out.Get(resolvedKey); lv.Parent.Equal(TList) && !isInertConst(lv) {
					if lp, isList := lv.Data.(ListPayload); isList {
						es.recordMakeListInner(e.registry, lp.Elems, lv, lv.Pos)
					}
				}
			}
		}
	}
	res := NewMap(out)
	// RECORDING mode: a `make` construction body whose values are COMPUTED (an
	// event carrier, not bakeable as an inert const) records an OpMakeMap assembly
	// of the evaluated values, so the map resolves to a real per-run event the
	// outer make consumes — otherwise it is an unresolvable residual and the
	// program falls back. A fully-literal / const-folded map stays inert and bakes
	// as a pooled const. dataMap is set ONLY for make's construction body (a
	// CONSUMED arg, match.Name == "make" at the execMatch call site), which make
	// evaluates IN the current scope before constructing — so it records even
	// inside a fn body / branch arm (the OpMakeMap re-assembles per call from its
	// operands, matching make's per-call evaluation). It is never a deferred
	// residual, so the late-rebinding hazard that gates plain residual lists/maps
	// does not apply, and the top-frame restriction is unnecessary here.
	// A COMPUTED (non-inert) map literal records an OpMakeMap assembly so its
	// result is a real per-run event the rest of the program can consume — not
	// just make's construction body (dataMap), but ANY computed map literal,
	// including one returned as a fn body result or bound to a value-def local
	// (`def m {x:(a 1 add)} … m`). The const case (a fully-literal map) stays
	// inert and bakes as a pooled const (isInertConst), so it never reaches here.
	// OpMakeMap re-assembles a FRESH map from its operands on every run, so it is
	// sound inside a fn body / branch arm / loop body (no freezing, never a
	// deferred-residual late-rebind hazard) — the same property that made the
	// dataMap case safe without a top-frame restriction. RecordMakeMap leaves es
	// untouched and returns false when a value has no compiled home, so the map
	// stays an unresolvable residual and the program falls back faithfully.
	// Gated on `consumed`: a DEFERRED residual (end-of-run autoEvalStack) is
	// evaluated after its frame pops, so recording it in-frame would diverge.
	if (consumed || e.elemEvalRecordable) && e.registry.Check.IsActive() {
		if es := e.registry.Check.Recorder(); es.Armed() && !isInertConst(res) {
			keys := out.Keys()
			vals := make([]Value, len(keys))
			for i, k := range keys {
				vals[i], _ = out.Get(k)
			}
			es.RecordMakeMap(e.registry, keys, vals, out.Implicit, res, val.Pos)
		}
	}
	return res, nil
}

// exprRefsCarrier reports whether a folded container expression references a
// def-bound name whose current value is a CARRIER — a computed value or a loop
// iterator, abstract at check time. Folding such an expression runs the handler
// against the carrier, which coerces (e.g. AsInteger -> 0), so the fold freezes
// a wrong constant. A user TYPE binding (a type literal, Carrier=false) or a
// concrete literal binding is NOT a carrier and still folds. Walks nested paren-
// exprs, lists, and map values; builtins (not in Defs) are ignored.
func (e *Engine) exprRefsCarrier(items []Value) bool {
	r := e.registry
	found := false
	var walk func(vs []Value)
	walk = func(vs []Value) {
		for _, v := range vs {
			if found {
				return
			}
			if IsWord(v) {
				if w, err := AsWord(v); err == nil {
					if bound, ok := r.Defs.Top(w.Name); ok && bound.Carrier {
						found = true
						return
					}
				}
			}
			if IsParenExpr(v) {
				if toks, err := AsParenExpr(v); err == nil {
					walk(toks)
				}
				continue
			}
			// A Reach can hide a carrier in its receiver or a computed key
			// (`m.(k)` with a def-local carrier `k`): recurse into both so the
			// fold declines rather than baking the carrier's check-time value.
			if IsReach(v) {
				if ri, err := AsReach(v); err == nil {
					walk(ri.Receiver)
					for i := range ri.Segments {
						if ri.Segments[i].Computed {
							walk(ri.Segments[i].KeyExpr)
						}
					}
				}
				continue
			}
			if lst, err := AsList(v); err == nil && !lst.IsNil() {
				walk(lst.Slice())
				continue
			}
			if mp, err := AsMap(v); err == nil && mp != nil {
				for _, k := range mp.Keys() {
					mv, _ := mp.Get(k)
					walk([]Value{mv})
				}
			}
		}
	}
	walk(items)
	return found
}

// exprHasEffect reports whether items contain a word whose evaluation performs a
// side effect that the const-fold (which re-runs the expression off the emit
// path) must not double or strand: the effectful core word `print`, or a
// module-bound word fronting an IO/Net/etc. call. The walk mirrors
// exprRefsCarrier's structural recursion.
func (e *Engine) exprHasEffect(items []Value) bool {
	r := e.registry
	found := false
	var walk func(vs []Value)
	walk = func(vs []Value) {
		for _, v := range vs {
			if found {
				return
			}
			if IsWord(v) {
				if w, err := AsWord(v); err == nil {
					if w.Name == "print" {
						found = true
						return
					}
					if bound, ok := r.Defs.Top(w.Name); ok && isModuleFamilyValue(bound) {
						found = true
						return
					}
				}
			}
			if IsParenExpr(v) {
				if toks, err := AsParenExpr(v); err == nil {
					walk(toks)
				}
				continue
			}
			if IsReach(v) {
				if ri, err := AsReach(v); err == nil {
					walk(ri.Receiver)
					for i := range ri.Segments {
						if ri.Segments[i].Computed {
							walk(ri.Segments[i].KeyExpr)
						}
					}
				}
				continue
			}
			if lst, err := AsList(v); err == nil && !lst.IsNil() {
				walk(lst.Slice())
				continue
			}
			if mp, err := AsMap(v); err == nil && mp != nil {
				for _, k := range mp.Keys() {
					mv, _ := mp.Get(k)
					walk([]Value{mv})
				}
			}
		}
	}
	walk(items)
	return found
}

// constFoldContainerVal evaluates a container's computed value (a class field
// default or a data-map paren-expr) CONCRETELY at check time and returns the
// constant, so the container bakes as a const instead of recording an event the
// container swallows. It only folds a DETERMINISTIC expression: the items are
// evaluated twice in a throwaway non-recording sub-engine, and the fold is taken
// only when both runs yield the SAME single deeply-concrete value — so a
// clock/rand/mutation-bearing default (whose two runs differ) is left to the
// normal recording path and stays uncompiled rather than freezing a runtime
// value. A reference to a check-mode value binding evaluates to a carrier (not
// concrete) and so does not fold. Mutation-safety of the folded value is the
// downstream const-bake gate's job (a mutable instance bakes only as a schema
// default that make copies, never as a data-map member).
func (e *Engine) constFoldContainerVal(items []Value) (Value, bool) {
	// The fold runs the expression CONCRETELY (twice). That is only sound for a
	// pure computation: an effectful word (`print`, a module IO/Net call) would
	// perform its effect at compile time — and double it, across the two runs —
	// while the compiled program just pushes the folded constant. Decline the
	// fold for such an expression and let it record normally instead.
	if e.exprHasEffect(items) {
		return Value{}, false
	}
	one, ok := e.concreteEvalOnce(items)
	if !ok {
		return Value{}, false
	}
	two, ok := e.concreteEvalOnce(items)
	if !ok || !constFoldAgrees(one, two) {
		return Value{}, false
	}
	return one, true
}

// concreteEvalOnce runs items in a throwaway sub-engine with check mode OFF (so
// the result is a real value, not a carrier, and nothing is recorded into the
// parent's emit state) and returns the single concrete residual. The def stack
// is snapshotted and restored so a stray binding cannot leak into the compile.
func (e *Engine) concreteEvalOnce(items []Value) (Value, bool) {
	r := e.registry
	snap := r.Defs.Snapshot()
	prev := r.Check.Mode
	r.Check.Mode = false
	res, err := runPooledSub(r, append([]Value(nil), items...), false)
	r.Check.Mode = prev
	r.Defs.Restore(snap)
	if err != nil || len(res) != 1 || !IsConcrete(res[0]) {
		return Value{}, false
	}
	return res[0], true
}

// the function. If the FnDef carries a captured Registry (closure from a
// module), execution happens in a sub-engine using that registry so that
// module-internal words are available. Otherwise, body tokens are spliced
// into the current engine's stack.
func (e *Engine) execFnDefLiteral(valIdx int) error {
	val := e.tape.At(valIdx)
	fnDef, ok := val.Data.(FnDefInfo)
	if !ok {
		e.pointer++
		return nil
	}

	// A macro FnDef reaching here is a VALUE, not an application — e.g. the
	// `(macro …)` result being bound by `def`, or a parked macro. It must
	// stay as DATA: a macro is applied only by name (the stepWord branch
	// captures its raw operands before collection). The anonymous-0-arg
	// short-circuit below also returns macros as data. (Applying a macro is
	// never a stack-value dispatch — design/MACROS-PHASE1.10.md §5, D4.)

	// Resolve the dispatchable signatures. A self-contained Function value
	// (an anonymous closure, or a fn defined in THIS registry) is a STABLE
	// handle: we compile and use its OWN signatures rather than a fresh
	// registry lookup, so `undef foo; def foo …` doesn't change the meaning
	// of a previously-captured value. compileFnDef normalises and barrier-
	// resolves the authored sigs (it is idempotent on already-compiled
	// ones) and attaches the body-runner for anonymous fns.
	//
	// A value carrying a FOREIGN sub-registry (a module wrapper, or an AQL
	// fn defined inside a module preamble) is NOT a stable own-sig handle:
	// it must resolve the real (inner) definition in that sub-registry, so
	// it falls through to the name-lookup branch below. Anonymous closures
	// keep their own sigs even when they captured a module registry.
	var fn *FnDefInfo
	foreignReg := fnDef.Registry != nil && fnDef.Registry != e.registry
	if len(fnDef.Signatures) > 0 && (fnDef.Anonymous || !foreignReg) {
		reg := fnDef.Registry
		if reg == nil {
			reg = e.registry
		}
		fn = compileFnDef(reg, fnDef)
	}
	if fn == nil && fnDef.Name != "" {
		reg := fnDef.Registry
		if reg == nil {
			reg = e.registry
		}
		fn = reg.Lookup(fnDef.Name)
	}
	if fn == nil && len(fnDef.Signatures) > 0 {
		// An UNNAMED value from a foreign sub-registry — e.g. an
		// anonymous `fn` literal placed in a module export map. There
		// is nothing to look up, so its authored sigs are the only
		// description there is: compile and dispatch on them. The
		// body still runs with module scope — execFnDefSig /
		// execFnDefSigStackMatch receive fnDef.Registry, and the
		// sub-registry branch below handles handler-bearing matches.
		fn = compileFnDef(fnDef.Registry, fnDef)
	}
	if fn == nil {
		e.pointer++
		return nil
	}

	w := WordInfo{Name: fnDef.Name, ArgCount: -1}

	// A `/r` or `/q` modifier on a paren / dotted-path result is emitted by
	// the parser as a Word/__DM marker right after the group (/u /s /f /N
	// are the usurp / stack-args / forward-args / force-arity words). Peek
	// and consume it: it leaves the function inert (data).
	if valIdx+1 < e.tape.Len() {
		if _, ok := AsDispatchMod(e.tape.At(valIdx + 1)); ok {
			e.tape.Remove(valIdx + 1)
			v := e.tape.At(valIdx)
			v.Quoted = true
			e.tape.Set(valIdx, v)
			e.pointer++
			return nil
		}
	}

	// Pre-evaluate paren expressions in the forward scan range so that
	// matchSignature sees fully resolved values. Mirrors stepWord's
	// pre-eval pass — the unified rule says Function values at the
	// pointer dispatch with the same forward+stack matching as words.
	if (fn.HasForwardSigs() && !w.ForceStack) || w.ForceForward {
		if err := e.resolveForwardArgs(fn, w); err != nil {
			return err
		}
	}

	resolved := e.effectiveResolved()
	sig, positions, specAt := e.matchSignature(fn, w, resolved)

	// Retry fallback for words with forward-collecting sigs: when
	// nearest-first matching fails, retry with deepest-first
	// (ForceStack). Mirrors stepWord's CallAQL-input recovery.
	if sig == nil && fn.HasForwardSigs() && !w.ForceStack {
		wDeep := w
		wDeep.ForceStack = true
		sig, positions, specAt = e.matchSignature(fn, wDeep, resolved)
	}

	// Function-value dispatch does NOT fire Fallback sigs. Fallback
	// handlers (installed by InstallFnDef as 0-arg catch-alls) exist
	// to raise a clean "no matching signature for X" error when a
	// *bare word* arrives without args. For a Function value sitting
	// at the pointer, the right behavior is to leave it on the stack
	// as data — the user explicitly captured it and may consume it
	// later.
	if sig != nil && sig.Fallback {
		sig = nil
	}

	// Count forward vs stack positions of the matched sig (nil-safe:
	// positions is empty when sig == nil).
	fwdCount := 0
	for _, pos := range positions {
		if pos > e.pointer {
			fwdCount++
		}
	}

	// Fall through to FnSig-based pure-stack matching when
	// matchSignature finds nothing — this preserves the legacy
	// anonymous-fn-on-stack dispatch for AQL fns whose Sigs carry
	// named params. The same path runs when matched but the sig
	// has no Go Handler AND this isn't an `afn`-produced lambda:
	// predicate-type FnDefs landing bare are intentionally inert.
	//
	// EXCEPT when the handler-less match includes FORWARD positions:
	// a non-anonymous `fn` value at the pointer (typically a `get`
	// result — `m.f 5`, `Mod.word arg`) must collect its forward args
	// like any word (the unified rule above). It proceeds to the
	// insertForward branch below, which parks the future tokens and
	// re-processes this value with all args on the stack — landing
	// back here with fwdCount == 0 and dispatching via the stack
	// path. Macro values are excluded: they stay on the legacy path
	// (data — applied only by name; MACROS-PHASE1.10.md §5).
	if sig == nil || (sig.dispatchHandler() == nil && !fnDef.Anonymous && (fwdCount == 0 || fnDef.Macro)) {
		return e.execFnDefSigStackMatch(valIdx, fnDef, resolved)
	}

	// Anonymous lambdas (afn / =>) are VALUES that auto-dispatch only
	// when args are actually available (forward tokens, or stack args
	// for the swap form). A 0-arg lambda sitting alone on the stack
	// has positions=[] AND no forward — it's just data, let downstream
	// consumers (def, a stored map entry, call) take it as-is rather
	// than auto-invoking. This is what makes `def f ([] => [body])`
	// bind f to the Function value instead of to the body's result.
	// Macro values are likewise data here — a `(macro …)` result must bind
	// to its name, not auto-expand (it expands only via the named stepWord
	// branch). See design/MACROS-PHASE1.10.md §5.
	if (fnDef.Anonymous || fnDef.Macro) && fwdCount == 0 && len(positions) == 0 {
		e.pointer++
		return nil
	}

	// Forward-collecting match: defer dispatch until the remaining
	// tokens have been consumed. When the Forward marker completes,
	// the engine re-processes the Function value with all args on
	// the stack — which routes through this same execFnDefLiteral
	// entry. This branch runs whether the sig has a Go Handler
	// (registered native) or only an AQL body (anonymous FnDef from
	// `afn` / `=>`); in both cases matchSignature found valid
	// positions and we need the forward args on the stack before
	// the body / handler runs.
	if fwdCount > 0 {
		stkCount := len(positions) - fwdCount
		return e.insertForward(w, sig, fwdCount, stkCount, specAt)
	}

	// All args resolved on the stack. Anonymous FnDefs (no Go
	// Handler) take the legacy stack-match path, which splices the
	// body via execFnDefSig and binds named params via def-stack.
	if sig.dispatchHandler() == nil {
		return e.execFnDefSigStackMatch(valIdx, fnDef, resolved)
	}

	// Module closures: the FnDef carries a captured sub-registry
	// (Registry != e.registry). Two cases:
	//
	//  1. **Trivial-delegation wrapper** — the wrapper FnSig body is
	//     a single Word referencing the inner native of the same name
	//     (e.g. rand.string's wrapper body `[Word(rand-string)]`).
	//     matchSignature already found and matched that inner native;
	//     its handler is `sig.dispatchHandler()`. Direct-call it via execMatch
	//     so args flow straight through in sig order — no body
	//     execution, no token splicing, no push reordering.
	//
	//  2. **AQL fn defined inside a module preamble** (e.g.
	//     decision.cond, defined via `def cond fn […]` in the
	//     module's AQL source). The body references module-private
	//     types/words that only resolve in fnDef.Registry, so we must
	//     run it in that registry via CallAQL. These fns use NAMED
	//     params, so CallAQL's named-param binding (InstallDef by
	//     name) sidesteps any unnamed-param push ordering issues.
	//
	// See design/SIG-ORDER-REFACTOR.10.md for the architecture history.
	//
	// Only AQL-BODIED definitions take the sub-registry path: a trivial-
	// delegation wrapper (Body=[Word(inner)]) or a module-preamble fn (real
	// Body). A reference to a Go NATIVE living in the sub-registry carries
	// Body-less sigs and a real Go Handler — it must dispatch straight
	// through execMatch below, exactly like any other native, so we require
	// a body-bearing own sig before entering this branch.
	if fnDef.Registry != nil && fnDef.Registry != e.registry {
		ownSigs := fnDef.OwnSigs()
		var wrapperSig *FnSig
		// Select the own sig CORRESPONDING TO THE MATCHED sig — same
		// param names and types, not merely the same arity. With two
		// same-arity overloads (e.g. a generic `[rhs:T op:String
		// lhs:T]` sig and its `[rhs:Any op:String lhs:Any]` catch-all)
		// the old arity-only pick ran the FIRST body with the OTHER
		// sig's matched args — the exact body/sig mis-pairing the
		// per-sig handler attachment exists to prevent. Arity remains
		// the fallback when no exact correspondence is found.
		var arityPick *FnSig
		for i := range ownSigs {
			if len(ownSigs[i].body()) == 0 {
				continue // native ref sig — not a wrapper/preamble body
			}
			if wrapperSig == nil {
				wrapperSig = &ownSigs[i] // last resort: first body-bearing sig
			}
			if len(ownSigs[i].Params) != len(positions) {
				continue
			}
			if arityPick == nil {
				arityPick = &ownSigs[i]
			}
			if sig != nil && sigParamsCorrespond(ownSigs[i].Params, sig.Params) {
				arityPick = &ownSigs[i]
				break
			}
		}
		if arityPick != nil {
			wrapperSig = arityPick
		}
		if wrapperSig != nil {
			// Surface a module-export dispatch in the trace so a profiler
			// (Debug.profile) can attribute it: a dotted call like
			// `Math.sqrt` otherwise shows only the `get` expansion, never the
			// function it resolves to. Gated on a module origin so ordinary
			// user-fn and bare-word traces are unaffected.
			if fnDef.Module != "" {
				e.traceNote = "call " + fnDef.Name
			}
			// ONE dispatch path, no exceptions: a module wrapper — trivial-delegation
			// OR a real AQL body — dispatches through execMatch, exactly like a named
			// fn and a bare-word call. So a fn body is ANALYSED/COMPILED the SAME way
			// regardless of how the fn was reached: a param-slot unit via the matched
			// sig's ReturnsFn, NOT a separate def-stack CallAQL run that left Function
			// params slot-less and unreachable to a closure capture (the sort
			// comp-capture leaf — a fundamental dispatch-path divergence). match.Reg
			// carries the sub-registry so the body's module-private words resolve there
			// (the inner native's poly re-match, and the body's own scope);
			// shareCheckState routes the body's unit-compile into the MAIN program's
			// emit so RecordUserCall references it (else "user fn call (Stage 3)"). A
			// no-op outside check mode — interpret runs the matched handler in the
			// sub-registry it was installed in, exactly as before.
			if sig.dispatchHandler() != nil {
				match := &MatchResult{Sig: sig, Positions: positions, Name: fnDef.Name, Reg: fnDef.Registry}
				if len(positions) > 0 {
					match.Args = make([]Value, len(positions))
					for i, pos := range positions {
						match.Args[i] = e.tape.At(pos)
					}
				}
				restoreCheck := e.shareCheckState(fnDef.Registry)
				err := e.execMatch(match)
				restoreCheck()
				return err
			}
			// Degenerate: a wrapper sig with no body-runner handler — fall through.
			args := make([]Value, len(positions))
			for i, pos := range positions {
				args[i] = e.tape.At(pos)
			}
			return e.execFnDefSig(valIdx, wrapperSig, args, fnDef.Registry)
		}
	}

	// Pure-stack match: dispatch via execMatch the same way a bare
	// word with no forward args would. A NATIVE REFERENCE into a module
	// sub-registry (`assert-equal/r` in an export map — body-less sigs, a
	// real Go handler, so the wrapper branch above deliberately skips it)
	// still carries its owning registry on match.Reg, exactly like the
	// trivial-delegation dispatch: the recorder's poly re-match
	// (tryRecordPoly) validates the matched sig against THAT registry's
	// own binding and the VM re-matches over it (PolyRef.Reg). Without it
	// a dynamic-operand dispatch of a module native refused as "dynamic
	// input at <word>" although the runtime re-match over the sub-registry
	// IS the interpreter's dispatch. Interpretation is unchanged — Reg is
	// read only at the check-mode carrierResults seam (execMatch:2625).
	match := &MatchResult{Sig: sig, Positions: positions, Name: fnDef.Name}
	if fnDef.Registry != nil && fnDef.Registry != e.registry {
		match.Reg = fnDef.Registry
	}
	if len(positions) > 0 {
		match.Args = make([]Value, len(positions))
		for i, pos := range positions {
			match.Args[i] = e.tape.At(pos)
		}
	}
	return e.execMatch(match)
}

// sigParamsCorrespond reports whether an authored FnSig's params and
// a compiled Signature's params describe the same overload: same
// count, same names, same declared types. Used by execFnDefLiteral's
// sub-registry path to run the body belonging to the signature
// matchSignature actually selected.
func sigParamsCorrespond(a, b []FnParam) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].Name != b[i].Name {
			return false
		}
		at, bt := a[i].Type, b[i].Type
		if (at == nil) != (bt == nil) {
			return false
		}
		if at != nil && !at.Equal(bt) {
			return false
		}
	}
	return true
}

// isRecordableLiteral reports whether a stepLiteral value should be
// surfaced to a Recorder. Control-flow markers and engine-internal
// shapes (Forward/Mark/Move/ReturnCheck/DefCleanup/OpenParen/CloseParen/End/Internal)
// are NOT user-visible literals — they're scaffolding around the
// real data, and a stack-form recording wants only the data.
func isRecordableLiteral(v Value) bool {
	if v.Parent == nil {
		return false
	}
	switch {
	case IsForward(v), IsOpenParen(v), IsCloseParen(v), IsEnd(v):
		return false
	case v.Parent.ConformsTo(TMark), v.Parent.ConformsTo(TMove),
		v.Parent.ConformsTo(TReturnCheck), v.Parent.ConformsTo(TInternal):
		return false
	}
	return true
}

// trivialDelegationTarget reports the inner native name a wrapper FnSig
// purely delegates to — body of the form `[Word(inner)]` with all-
// unnamed Params — and whether the sig has that shape at all. Unlike
// isTrivialDelegationBody it does NOT require the inner name to equal
// the wrapper's own name, so it also recognises a wrapper rebound under
// a different name (`def w pkg.word`, `unpack [word] pkg`): the body
// word still names the original inner native to look up in the
// sub-registry. See InstallDef's module-wrapper rebinding branch.
func trivialDelegationTarget(sig *FnSig) (string, bool) {
	if len(sig.body()) != 1 {
		return "", false
	}
	for _, p := range sig.Params {
		if p.Name != "" {
			return "", false
		}
	}
	if !IsWord(sig.body()[0]) {
		return "", false
	}
	w, err := AsWord(sig.body()[0])
	if err != nil {
		return "", false
	}
	return w.Name, true
}

// upcomingArgs returns the value/literal tokens that follow valIdx up to
// the next statement or group boundary (End / CloseParen / Mark / Move)
// — the forward args a function-value call had available to collect.
// Forward-collection markers are skipped; everything before the first
// boundary is a candidate arg. Lets a failed forward-form call
// (`Pkg.fn a b`) be recognised as a call, not a bare value reference.
func (e *Engine) upcomingArgs(valIdx int) []Value {
	var out []Value
	for i := valIdx + 1; i < e.tape.Len(); i++ {
		v := e.tape.At(i)
		switch {
		case IsForward(v):
			continue
		case IsMark(v) || IsMove(v) || IsCloseParen(v) || IsEnd(v):
			return out
		default:
			out = append(out, v)
		}
	}
	return out
}

// execFnDefSigStackMatch is the legacy pure-stack dispatch path for
// AQL-defined functions whose signatures carry named params. Used as a
// fallback when matchSignature's aggregate match returns nothing.
func (e *Engine) execFnDefSigStackMatch(valIdx int, fnDef FnDefInfo, resolved []Value) error {
	resolvedIdx := e.resolvedIndicesBefore(len(resolved))
	checkMode := e.registry != nil && e.registry.Check.Mode && fnDef.Anonymous
	// A NON-anonymous body-bearing fn VALUE reached as a CALL while the
	// BYTECODE EMITTER is active (a `fn` literal resolved from a map / module
	// export, e.g. `ParseLang.parse_calc 'x' {}`) is dispatched like a named
	// user fn: through buildFnBodyReturnsFn (spliceFnValueCheckResult), which
	// arms the body compile so the per-call `__pa` tail is captured inside its
	// own CALL_USER unit instead of leaking into the top-level residual.
	//
	// Gated on an ACTIVE emit state so it only changes the COMPILE path, never
	// the pure type-check pass: buildFnBodyReturnsFn runs AnalyseFnBody, which
	// can emit body diagnostics under generalised carrier args that the legacy
	// inline-splice path (execFnDefSig) did not — the check-accuracy ratchet
	// pins that pure-check behaviour. Excludes foreign-sub-registry fns (their
	// body must run via CallAQL in that registry — the execFnDefLiteral
	// sub-registry branch handles them) and macros.
	checkFnValue := e.registry != nil && e.registry.Check.Mode && !fnDef.Anonymous && !fnDef.Macro &&
		(fnDef.Registry == nil || fnDef.Registry == e.registry) &&
		e.registry.Check.Recorder().active()
	ownSigs := fnDef.OwnSigs()
	for i := range ownSigs {
		sig := &ownSigs[i]
		nArgs := len(sig.Params)
		if nArgs == 0 {
			if checkMode {
				return e.spliceAnonCheckResult(valIdx, 0, sig, nil, fnDef.Captured)
			}
			if checkFnValue && len(sig.body()) > 0 {
				return e.spliceFnValueCheckResult(valIdx, 0, fnDef, sig, nil)
			}
			return e.execFnDefSig(valIdx, sig, nil, fnDef.Registry)
		}
		if len(resolved) < nArgs {
			continue
		}

		hasNamed := false
		for _, p := range sig.Params {
			if p.Name != "" {
				hasNamed = true
				break
			}
		}

		match := true
		if hasNamed {
			for j, p := range sig.Params {
				ri := len(resolved) - 1 - j
				if !sigTypeMatches(resolved[ri], p.Type) {
					match = false
					break
				}
				if p.Pattern != nil {
					pat := *p.Pattern
					if pat.Parent.Equal(TMap) && resolved[ri].Parent.Equal(TMap) &&
						pat.Data != nil && resolved[ri].Data != nil &&
						!IsOptionsType(pat) {
						if !OpenUnifyMap(pat, resolved[ri]) {
							match = false
							break
						}
					} else {
						if _, uOk := Unify(resolved[ri], pat); !uOk {
							match = false
							break
						}
					}
				}
			}
			if match {
				args := make([]Value, nArgs)
				for j := 0; j < nArgs; j++ {
					ri := len(resolvedIdx) - 1 - j
					args[j] = e.tape.At(resolvedIdx[ri])
				}
				if checkMode {
					return e.spliceAnonCheckResult(valIdx, nArgs, sig, args, fnDef.Captured)
				}
				if checkFnValue && len(sig.body()) > 0 {
					return e.spliceFnValueCheckResult(valIdx, nArgs, fnDef, sig, args)
				}
				return e.execFnDefSig(valIdx, sig, args, fnDef.Registry)
			}
		} else {
			candidate := resolved[len(resolved)-nArgs:]
			for j, p := range sig.Params {
				if !sigTypeMatches(candidate[j], p.Type) {
					match = false
					break
				}
				if p.Pattern != nil {
					pat := *p.Pattern
					if pat.Parent.Equal(TMap) && candidate[j].Parent.Equal(TMap) &&
						pat.Data != nil && candidate[j].Data != nil &&
						!IsOptionsType(pat) {
						if !OpenUnifyMap(pat, candidate[j]) {
							match = false
							break
						}
					} else {
						if _, uOk := Unify(candidate[j], pat); !uOk {
							match = false
							break
						}
					}
				}
			}
			if match {
				args := make([]Value, nArgs)
				startIdx := len(resolvedIdx) - nArgs
				for j := 0; j < nArgs; j++ {
					args[j] = e.tape.At(resolvedIdx[startIdx+j])
				}
				if checkMode {
					return e.spliceAnonCheckResult(valIdx, nArgs, sig, args, fnDef.Captured)
				}
				if checkFnValue && len(sig.body()) > 0 {
					return e.spliceFnValueCheckResult(valIdx, nArgs, fnDef, sig, args)
				}
				return e.execFnDefSig(valIdx, sig, args, fnDef.Registry)
			}
		}
	}

	// A NAMED function reached as a call — args on the stack
	// (swap/prefix form) or upcoming forward tokens (`Pkg.fn a b`) — that
	// matched no signature is the silent-dispatch footgun: the value is
	// left on the stack as data with no error (DX-report T1/B1). Guards
	// keep the detection precise: named (not an anonymous lambda value),
	// not explicitly inert (`/r` / `quote` set Quoted), and at least one
	// candidate arg available — so a bare function-as-value reference
	// with no args is left alone.
	//
	// Check mode diagnoses it immediately (uncalled_function). At
	// runtime the value MAY still be a legitimate higher-order operand
	// (`filter f xs`), so it is only marked here; the top-level
	// end-of-Run drain raises uncalled_function if nothing consumed it
	// (ERRORS.8.md §5 option 2) — check and runtime name the same bug
	// the same way.
	if e.registry != nil &&
		fnDef.Name != "" && !fnDef.Anonymous &&
		valIdx < e.tape.Len() && !e.tape.At(valIdx).Quoted {
		candidates := append(append([]Value{}, resolved...), e.upcomingArgs(valIdx)...)
		if len(candidates) > 0 {
			// Mark the value FailedDispatch and DEFER the diagnostic to the
			// end-of-run drain — in check mode too, not just at runtime. A
			// FailedDispatch fn value that a FOLLOWING higher-order word
			// consumes is NOT a bug: the documented `(Sort.by-number
			// Sort.reverse)` comparator composition leaves by-number as data
			// for reverse to take. Emitting eagerly here flagged that as a
			// false uncalled_function; deferring lets the drain flag only a
			// value that survives UNCONSUMED — exactly the runtime contract,
			// preserving the wrong-order true-positive while clearing the
			// composition FP.
			fv := e.tape.At(valIdx)
			fv.FailedDispatch = true
			// Borrow a span from the nearest argument when the FnDef value
			// itself carries none, so the end-of-run report can point
			// somewhere real.
			if fv.Pos.Row == 0 {
				for _, c := range candidates {
					if c.Pos.Row > 0 {
						fv.Pos = c.Pos
						break
					}
				}
			}
			e.tape.Set(valIdx, fv)
		}
	}

	e.pointer++
	return nil
}

// spliceAnonCheckResult runs AnalyseFnBody on an anonymous FnDef in
// check mode and splices the residual carrier stack as the dispatch
// result. This bypasses the body splice + ReturnCheck path that named
// fns use: an anonymous lambda's static Returns is the conservative
// [Any], and AnalyseFnBody recovers the real return type for downstream
// type propagation.
func (e *Engine) spliceAnonCheckResult(valIdx, nArgs int, sig *FnSig, args []Value, captures []CapturedBinding) error {
	paramNames := make([]string, len(sig.Params))
	for i, p := range sig.Params {
		paramNames[i] = p.Name
	}
	result := AnalyseFnBody(e.registry, "", paramNames, sig.body(), args, captures, sig.Returns)
	if len(result) == 0 {
		result = []Value{NewCarrier(TAny)}
	}
	e.spliceFnCheckTail(valIdx, nArgs, result)
	return nil
}

// spliceFnValueCheckResult is the check-mode dispatch for a NON-anonymous
// body-bearing fn VALUE (a `fn` literal resolved from a map/module export and
// then CALLED, e.g. `ParseLang.parse_calc 'x' {}`). Unlike a NAMED user fn
// (which dispatches through stepWord → its registered ReturnsFn) and unlike an
// anonymous lambda (spliceAnonCheckResult, analysis-only), a called fn value
// previously fell through to execFnDefSig, whose inline body splice leaks the
// per-call `__pa` (Args/FnBaseline pop) token into the TOP-LEVEL residual —
// refused by the emitter as "context-dependent word __pa". Routing through
// buildFnBodyReturnsFn ARMS the body analysis via StartFnCompile, so the body
// (with its `__pa` tail) is captured INSIDE its own CALL_USER unit and the
// call site records a CALL_USER — identical to the named-fn path. See
// design/aql-bytecode-stage3-inlining-plan.0.md "THE shared crux:
// body-bearing fn-VALUE dispatch (__pa)".
func (e *Engine) spliceFnValueCheckResult(valIdx, nArgs int, fnDef FnDefInfo, sig *FnSig, args []Value) error {
	returns := buildFnBodyReturnsFn(e.registry, fnDef.Name, *sig, fnDef)
	result := returns(args, e.registry)
	if len(result) == 0 && len(sig.Returns) > 0 {
		// A declared-return fn that produced no carrier (the body unit
		// declined to compile) degrades to one carrier per declared return so
		// downstream provenance refuses and the program falls back faithfully.
		result = make([]Value, len(sig.Returns))
		for i, t := range sig.Returns {
			result[i] = NewCarrier(t)
		}
	}
	e.spliceFnCheckTail(valIdx, nArgs, result)
	return nil
}

// spliceFnCheckTail removes the consumed args + the fn-value literal from the
// tape and splices the check-mode result carriers in their place. Shared by
// spliceAnonCheckResult and spliceFnValueCheckResult so the two check-mode
// fn-value paths cannot diverge in their stack discipline.
func (e *Engine) spliceFnCheckTail(valIdx, nArgs int, result []Value) {
	indices := e.resolvedIndicesBefore(nArgs)
	if len(indices) == nArgs && nArgs > 0 {
		firstArgIdx := indices[0]
		skipSet := make(map[int]bool, nArgs+1)
		for _, idx := range indices {
			skipSet[idx] = true
		}
		skipSet[valIdx] = true
		dst := firstArgIdx
		for i := firstArgIdx; i <= valIdx; i++ {
			if !skipSet[i] {
				e.tape.Set(dst, e.tape.At(i))
				dst++
			}
		}
		e.tape.Splice(dst, valIdx+1-dst, result...)
		e.pointer = firstArgIdx
	} else if nArgs == 0 {
		e.tape.Splice(valIdx, 1, result...)
	} else {
		argStart := valIdx - nArgs
		if argStart < 0 {
			argStart = 0
		}
		e.tape.Splice(argStart, valIdx+1-argStart, result...)
		e.pointer = argStart
	}
}

// compileFnDef produces the compiled-dispatch view of a constructed
// Function value (an afn closure or a captured FnDef) whose Signatures are
// still in authored form (named params + body, unresolved BarrierPos, no
// handler). It normalises each signature in place — resolving the BarrierPos
// sentinel, attaching handlers, sorting into dispatch order — and computes
// MaxForwardArgs. It is idempotent on already-compiled signatures.
//
// This centralisation is the seam the function-model consolidation builds
// on: every site that needs a dispatchable FnDefInfo from authored sigs
// routes through here rather than constructing the struct inline.
//
// Each compiled Signature is given a Go Handler (the shared AQL body-
// runner, buildFnBodyHandler) and a check-mode ReturnsFn
// (buildFnBodyReturnsFn), so a Function value dispatches through the
// uniform execMatch path exactly like a registered native — no
// handler-less fallback. Handlers are attached per-sig BEFORE
// SortSignatures so each handler stays paired with its own signature
// (the sort reorders sigs, so attaching by post-sort index would
// mis-pair them).
//
// r is the registry the body runs in: captures are already snapshotted
// into fnDef.Captured, and free body words resolve dynamically against r.
//
// Handlers are attached ONLY for anonymous fns (afn / `=>` lambdas and
// the closures they return). A non-anonymous bare FnDef value —
// notably a predicate-type FnDef sitting on the stack — must stay inert
// data (no auto-dispatch): leaving its Handler nil routes it through the
// legacy stack-match fall-through in execFnDefLiteral, which is the
// behaviour `def Bbd …; Bbd "c"` relies on (the FnDef and "c" remain on
// the stack rather than the predicate firing). See execFnDefLiteral's
// "predicate-type FnDefs landing bare are intentionally inert" guard.
func compileFnDef(r *Registry, fnDef FnDefInfo) *FnDefInfo {
	out := make([]Signature, len(fnDef.Signatures))
	for i := range fnDef.Signatures {
		sig := fnDef.Signatures[i]
		barrier := sig.BarrierPos
		if barrier == BarrierAllForward {
			barrier = len(sig.Params)
		}
		// Keep the full authored sig (Params with names, Returns, Body,
		// NoEval*) and layer the compiled dispatch fields on top, so a
		// constructed Function value stays a single full-fidelity slice.
		compiled := sig
		compiled.Params = append([]FnParam(nil), sig.Params...)
		compiled.BarrierPos = barrier
		if fnDef.Anonymous {
			meta := &FnFrameMeta{
				Name:         fnDef.Name,
				HasGen:       fnDef.Gen != nil,
				InstallNames: fnInstallNames(sig, fnDef.Captured),
			}
			compiled.Impl = &AQLImpl{
				Body:     sig.body(),
				FnFrame:  meta,
				dispatch: buildFnBodyHandler(r, fnDef.Name, sig, fnDef, meta),
			}
			compiled.ReturnsFn = buildFnBodyReturnsFn(r, fnDef.Name, sig, fnDef)
		}
		normalizeSig(&compiled)
		out[i] = compiled
	}
	SortSignatures(out)
	return &FnDefInfo{
		Name:           fnDef.Name,
		Signatures:     out,
		MaxForwardArgs: calcMaxForwardArgs(out),
		Registry:       fnDef.Registry,
		Anonymous:      fnDef.Anonymous,
		Captured:       fnDef.Captured,
		MiniKind:       fnDef.MiniKind,
	}
}

// shareCheckState lets a module sub-registry's fn body run IN CHECK MODE under
// the parent compile pass. When the calling engine is in check mode and the body
// executes in a DIFFERENT (captured module) registry, it points that registry's
// Check at the parent's for the duration of the call, so mode, Emit recording,
// the memo / in-flight maps and the step/budget counters are shared — while word
// resolution still uses the module registry's own Defs/Types (untouched). It
// returns a restore function (a no-op when no sharing applies). The shared memo
// keys stay disjoint across the boundary via the per-registry scopeID prefix
// (§5a), so a module fn and a parent fn of the same name cannot alias. See
// design/module-fn-checkstate-ownership.1.md §5b.
func (e *Engine) shareCheckState(capturedReg *Registry) func() {
	return shareCheckStateFrom(capturedReg, e.registry)
}

// shareCheckStateFrom is the registry-level mechanism behind shareCheckState:
// it points owner's Check at caller's for the duration (restore via the
// returned func), no-op when the registries coincide, either is nil, or the
// caller is not in check mode. Split out so the MERGED-WORD seam can share at
// the ReturnsFn boundary itself (buildFnBodyReturnsFn — Stage M1,
// design/STAGE3-INLINING-DESIGN-ROUND.0.md §5): a transplanted word-extension
// sig dispatches as a BARE word on the importer's engine, where no
// execFnDefLiteral wrapper exists to share around the call, and the sig's
// owning registry is known only to the ReturnsFn closure (the transplant
// clone's FnDefInfo.Registry is deliberately nil — see TransplantExtension).
// Idempotent under nesting: when an enclosing dispatch already shared, the
// swap is pointer-equal and the restore puts back the same shared state.
func shareCheckStateFrom(owner, caller *Registry) func() {
	if owner == nil || caller == nil || owner == caller || !caller.Check.IsActive() {
		return func() {}
	}
	saved := owner.Check
	owner.Check = caller.Check
	return func() { owner.Check = saved }
}

// execFnDefSig executes a matched FnDef signature. If capturedReg is non-nil
// (module closure), execution uses CallAQL on that registry. Otherwise, body
// tokens are spliced into the current engine's stack.
func (e *Engine) execFnDefSig(valIdx int, sig *FnSig, args []Value, capturedReg *Registry) error {
	nArgs := len(sig.Params)
	indices := e.resolvedIndicesBefore(nArgs)

	// Capture the call-site position (the fn value being invoked) before the
	// stack is mutated, so a return-type error can point at the call.
	var callPos SrcPos
	if valIdx >= 0 && valIdx < e.tape.Len() {
		callPos = e.tape.At(valIdx).Pos
	}

	// Auto-evaluate consumed arguments with Eval=true so FnDef handlers
	// receive resolved data. Maps: {base:hex} → {base:atom(hex)}.
	// Lists: [c1 c2] → [map1, map2].
	//
	// NoEvalArgs / NoEvalMapArgs on the sig suppresses auto-eval at
	// specific positions — required for module wrappers that take
	// quoted code bodies (e.g. rand.list-of [gen] N) so the body
	// reaches the inner handler intact rather than being sub-Run'd.
	// Mirrors the NoEvalArgs handling in execMatch (lines 977-1002).
	for i := range args {
		if args[i].Eval && !args[i].Quoted {
			if args[i].Parent.Equal(TMap) &&
				args[i].Data != nil && !IsTypedMap(args[i]) && !IsRecordType(args[i]) && !IsOptionsType(args[i]) {
				noEval := sig.NoEvalMapArgs != nil && sig.NoEvalMapArgs[i]
				if !noEval {
					// This FnDef/module-wrapper path never dispatches the core `make`
					// word (a core native goes through execMatch), so its map args are
					// not construction bodies — keep the const-fold path.
					evaluated, err := e.autoEvalMap(args[i], false, true)
					if err != nil {
						return err
					}
					args[i] = evaluated
				}
			} else if args[i].Parent.Equal(TList) &&
				args[i].Data != nil && !IsTypedList(args[i]) && !IsTableType(args[i]) {
				noEval := sig.NoEvalArgs != nil && sig.NoEvalArgs[i]
				if !noEval {
					// Bare words never degrade to data — propagate the
					// auto-eval failure (see execMatch).
					evaluated, err := e.autoEvalList(args[i], true)
					if err != nil {
						return err
					}
					args[i] = evaluated
				}
			}
		}
		args[i].Eval = false
		args[i].Undefined = false
	}

	if capturedReg != nil && (capturedReg != e.registry || e.registry.Lookup("__pa") == nil) {
		// Execute in the captured module's registry via CallAQL.
		// Pass the FnDef's lexical captures so the body sees them as
		// defs (alongside the module-registry's own bindings).
		//
		// Same-registry values fall through to the splice branch below: a
		// module-fn VALUE applied inside its own module (a callback passed
		// back in) needs no registry hop, so it takes the same main-tape
		// frame path a named fn call does — one fewer sub-engine per call,
		// and flow-control/def-cleanup semantics identical to named
		// dispatch (the TCO-STAGED Stage-5 residual flip; boundary rows in
		// lang/spec/module-fnvalue-boundary.tsv). Gated on the frame
		// protocol being EXECUTABLE: the splice tail's `__pa` word is
		// registered by the language layer, so a bare kernel registry (an
		// eng-only embedder, the kernel test harnesses) keeps the CallAQL
		// path, whose per-call cleanup is Go-side and needs no words.
		var captures []CapturedBinding
		if valIdx < e.tape.Len() {
			if fd, ok := e.tape.At(valIdx).Data.(FnDefInfo); ok {
				captures = fd.Captured
			}
		}
		restoreCheck := e.shareCheckState(capturedReg)
		result, err := capturedReg.CallAQL(sig, args, captures)
		restoreCheck()
		if err != nil {
			return err
		}
		// Splice: remove consumed args + FnDef, insert results.
		if len(indices) == nArgs && nArgs > 0 {
			firstArgIdx := indices[0]
			skipSet := make(map[int]bool, nArgs+1)
			for _, idx := range indices {
				skipSet[idx] = true
			}
			skipSet[valIdx] = true
			dst := firstArgIdx
			for i := firstArgIdx; i <= valIdx; i++ {
				if !skipSet[i] {
					e.tape.Set(dst, e.tape.At(i))
					dst++
				}
			}
			e.tape.Splice(dst, valIdx+1-dst, result...)
			e.pointer = firstArgIdx
		} else if nArgs == 0 {
			e.tape.Splice(valIdx, 1, result...)
		} else {
			argStart := valIdx - nArgs
			if argStart < 0 {
				argStart = 0
			}
			e.tape.Splice(argStart, valIdx+1-argStart, result...)
			e.pointer = argStart
		}
		return nil
	}

	// No captured registry — splice body tokens into the current stack.
	var tokens []Value
	tokens = append(tokens, NewFrameOpen(fnValueFrameMeta))

	// Push the fn-entry baseline before installing anything. Inner
	// fn/afn constructions inside this body consult TopFnBaseline
	// to identify enclosing-fn-local bindings. Paired with __pa
	// below, which pops the baseline.
	e.registry.PushFnBaseline(e.registry.Defs.Snapshot())

	argsCopy := make([]Value, len(args))
	copy(argsCopy, args)
	if err := e.registry.Args.Push(NewList(argsCopy)); err != nil {
		e.registry.PopFnBaseline()
		return err
	}

	// Lexical captures from the FnDefInfo that produced this dispatch.
	// Pulled from the stack value at valIdx since execFnDefSig's signature
	// doesn't carry the FnDefInfo directly. Install before params so
	// params shadow same-named captures (innermost wins).
	var captures []CapturedBinding
	if valIdx < e.tape.Len() {
		if fd, ok := e.tape.At(valIdx).Data.(FnDefInfo); ok {
			captures = fd.Captured
		}
	}
	var names []string
	for _, cb := range captures {
		InstallFrameBinding(e.registry, cb.Name, cb.Value)
		names = append(names, cb.Name)
	}

	// args in top-first sig order (matchSignature convention).
	// Named params bind by name; unnamed params push to body tokens in
	// i-order. No reordering — same convention as InstallFnDef and
	// CallAQL. See design/SIG-ORDER-REFACTOR.10.md.
	unnamedCount := 0
	for i, p := range sig.Params {
		if p.Name != "" {
			InstallFrameBinding(e.registry, p.Name, args[i])
			names = append(names, p.Name)
		} else {
			tokens = append(tokens, args[i])
			unnamedCount++
		}
	}
	// Stamp the resolved-argument span on the frame open so the step
	// loop skips the unnamed args (arguments are inert —
	// FrameOpenInfo.ArgSpan; same stamp as buildFnBodyHandler).
	if unnamedCount > 0 {
		tokens[0] = NewFrameOpenSpan(fnValueFrameMeta, unnamedCount)
	}
	// Snapshot AFTER captures+params so the tail's DefCleanup tears
	// down only body-local defs — the same placement as
	// buildFnBodyHandler. (This tail historically omitted the
	// DefCleanup marker; it is synthesized by the shared
	// AppendFrameTail now, so the two splice paths cannot diverge.)
	defSnapshot := e.registry.Defs.Snapshot()

	body := make([]Value, len(sig.body()))
	copy(body, sig.body())
	tokens = append(tokens, body...)

	tokens = AppendFrameTail(tokens, FrameTailSpec{
		Registry:     e.registry,
		Snapshot:     defSnapshot,
		Names:        names,
		Returns:      sig.Returns,
		UnnamedCount: unnamedCount,
		FuncName:     "<fn>",
		Pos:          callPos,
	})
	tokens = append(tokens, NewCloseParen())

	if len(indices) == nArgs && nArgs > 0 {
		firstArgIdx := indices[0]
		skipSet := make(map[int]bool, nArgs+1)
		for _, idx := range indices {
			skipSet[idx] = true
		}
		skipSet[valIdx] = true
		dst := firstArgIdx
		for i := firstArgIdx; i <= valIdx; i++ {
			if !skipSet[i] {
				e.tape.Set(dst, e.tape.At(i))
				dst++
			}
		}
		e.tape.Splice(dst, valIdx+1-dst, tokens...)
		e.pointer = firstArgIdx
	} else if nArgs == 0 {
		e.tape.Splice(valIdx, 1, tokens...)
	} else {
		argStart := valIdx - nArgs
		if argStart < 0 {
			argStart = 0
		}
		e.tape.Splice(argStart, valIdx+1-argStart, tokens...)
		e.pointer = argStart
	}

	return nil
}

// implicitEnd resolves a forward early when a type mismatch occurs.
func (e *Engine) implicitEnd(fwdIdx int) error {
	fwd, _ := AsForward(e.tape.At(fwdIdx))
	funcIdx := fwd.FuncIndex
	collectedCount := fwd.CollectedArgs
	stackArgCount := fwd.StackArgs

	e.tape.Remove(fwdIdx)
	if fwdIdx < funcIdx {
		funcIdx--
	}

	e.curryOrStack(funcIdx, collectedCount, stackArgCount)
	return nil
}

// policyGateWord consults the engine scope's word policy before a
// registered word dispatches. Skips check mode (static analysis should
// see every word so type-checking remains meaningful) and engine
// markers (`__`-prefixed, used by internal lowering — never directly
// addressable from user code).
func (e *Engine) policyGateWord(name string) error {
	if e.registry.Check.IsActive() || isInternalMarker(name) {
		return nil
	}
	if wc := LookupWordChecker(e.registry); wc != nil {
		return wc.CheckWord(name)
	}
	return nil
}

// commitBarrierForward applies the argument-order rule's "another
// function word is a barrier" to a Forward that is parked waiting for
// ARRIVALS (its args came from paren results rather than the token
// walk, so the walk-time barrier check never saw the boundary). Called
// from stepWord when a function word is about to begin its own
// dispatch: if the nearest pending forward in the current paren scope
// can already fire with the args it holds — the same dispatch an
// explicit `end` would trigger — it is committed NOW, and the current
// word re-steps afterwards.
//
// This is what makes an else-less guard fire before the next statement
// runs: in `if (bad) [raise …] def q (10 div n)`, the parked `if`
// previously kept waiting for a possible else while `def` ran first —
// so the div-by-zero pre-empted the guard's raise, and any
// value-producing statement could be swallowed into the else slot. A
// pending forward that CANNOT yet fire keeps waiting, since its
// missing args may be the very results the stepping word produces
// (`add 1 def x 5 x` still binds x and completes add).
//
// Returns true when a forward was committed; the caller must return
// to the engine loop (the pointer has moved to the committed word).
func (e *Engine) commitBarrierForward() bool {
	// Nearest pending forward, stopping at open-paren scope barriers —
	// the same scan stepEnd performs.
	fwdIdx := -1
	for i := e.pointer - 1; i >= 0; i-- {
		if IsOpenParen(e.tape.At(i)) {
			break
		}
		if IsForward(e.tape.At(i)) {
			fwdIdx = i
			break
		}
	}
	if fwdIdx < 0 {
		return false
	}

	fwd, _ := AsForward(e.tape.At(fwdIdx))
	funcIdx := fwd.FuncIndex
	claimed := fwd.CollectedArgs + fwd.StackArgs
	if claimed == 0 {
		// Nothing collected yet — no smaller-arity dispatch to commit.
		return false
	}
	if funcIdx < 0 || funcIdx >= e.tape.Len() || !IsWord(e.tape.At(funcIdx)) {
		return false
	}
	w, _ := AsWord(e.tape.At(funcIdx))
	fn := e.registry.Lookup(w.Name)
	if fn == nil {
		return false
	}

	// Probe: would the parked word dispatch with ONLY its claimed args
	// (collected forward + claimed stack)? Deliberately tighter than a
	// whole-scope match — an implicit commit must not reach below the
	// args the word actually claimed. The claimed region sits directly
	// below the word: stack args first, then collected forward args in
	// arrival order (first-collected deepest) — which IS sig order, the
	// layout MatchSignature reads bottom-first.
	start := funcIdx - claimed
	if start < 0 {
		return false
	}
	var resolved []Value
	for i := start; i < funcIdx; i++ {
		v := e.tape.At(i)
		if IsForward(v) || IsOpenParen(v) || IsMark(v) || IsMove(v) {
			continue
		}
		resolved = append(resolved, v)
	}
	if len(resolved) == 0 {
		return false
	}
	testW := WordInfo{Name: w.Name, ArgCount: -1, ForceStack: true}
	m := MatchSignature(fn.Signatures, resolved, testW)
	if m == nil || m.Sig == nil {
		return false
	}
	// The commit must consume EXACTLY the claimed args through a real
	// overload. AQL-bodied fns carry a synthetic 0-arg Fallback in the
	// aggregate dispatch table (it exists to raise a clean
	// "no matching signature" error); matching it here would commit a
	// waiting word to its own failure — `g 1 def x 5 x` must keep
	// waiting for def's result when g has only a 2-arg overload, not
	// error at the boundary. Likewise a shorter real overload must not
	// fire while claimed args would be stranded.
	if m.Sig.Fallback || m.Sig.TotalArgs() != len(resolved) {
		return false
	}

	// The commit is correct; in check mode additionally leave a
	// non-gating note when the parked plan was SPECULATIVE — the
	// else-less-guard shape, where the planner had filled a trailing
	// slot from the very word now acting as the boundary. Emitted
	// before the mechanics below so e.pointer still names the
	// boundary word.
	e.noteSpeculativeBarrierCommit(fwd)

	// Commit exactly like the collection-complete path (stepLiteral's
	// CollectedArgs >= ExpectedArgs branch): drop the marker, force the
	// word to stack mode, and rearrange so the first-collected forward
	// arg sits on top — the engine matcher's top-first read then sees
	// the args in sig order. (NOT curryOrStack: its rearrange is gated
	// on StackArgs > 0, so a purely-arrival forward would re-dispatch
	// with its collected args reversed.)
	e.tape.Remove(fwdIdx)
	if fwdIdx < funcIdx {
		funcIdx--
	}
	if funcIdx < e.tape.Len() && IsWord(e.tape.At(funcIdx)) {
		e.forceStackWord(funcIdx, w)
	}
	e.pointer = funcIdx
	e.rearrangeForForward(fwd.StackArgs, fwd.CollectedArgs)
	return true
}

// noteSpeculativeBarrierCommit emits the speculative_forward_commit
// advisory (check mode only, info severity, never gating): a parked
// word committed at a statement boundary AND its plan had filled a
// forward slot with a dispatching word — the else-less-guard shape.
// The commit resolves it correctly; the note exists because the
// source reads ambiguously, and an explicit `[]` else (or `end`)
// states the intent. Structurally silent on the `def name fn […]`
// idiom: there the smaller-arity probe FAILS, no commit happens, and
// this helper is never reached.
func (e *Engine) noteSpeculativeBarrierCommit(fwd ForwardInfo) {
	if e.registry == nil || !e.registry.Check.IsActive() || !fwd.Speculative {
		return
	}
	detail := fwd.FuncName + " committed with " +
		strconv.Itoa(fwd.CollectedArgs+fwd.StackArgs) + " argument(s) at a statement boundary"
	if e.pointer >= 0 && e.pointer < e.tape.Len() {
		if bw, err := AsWord(e.tape.At(e.pointer)); err == nil {
			detail += " (`" + bw.Name + "`)"
		}
	}
	detail += " — its trailing slot " + strconv.Itoa(fwd.SpeculativeAt) +
		" was planned from the following word; an explicit `[]` else or `end` makes the intent loud"
	e.registry.Check.AddDiagnostic(CheckDiagnostic{
		Code:   "speculative_forward_commit",
		Detail: detail,
		Word:   fwd.FuncName,
		Row:    fwd.Pos.Row,
		Col:    fwd.Pos.Col,
	})
}

// stepEnd handles the "end" keyword.
func (e *Engine) stepEnd() error {
	// Statement boundary: void-group records do not blame failures
	// across statements (ERRORS.8.md §3).
	e.voidGroups = e.voidGroups[:0]
	endIdx := e.pointer

	// Find nearest pending forward, stopping at open-paren barriers.
	fwdIdx := -1
	for i := endIdx - 1; i >= 0; i-- {
		if IsOpenParen(e.tape.At(i)) {
			break
		}
		if IsForward(e.tape.At(i)) {
			fwdIdx = i
			break
		}
	}

	if fwdIdx < 0 {
		e.tape.Remove(endIdx)
		return nil
	}

	fwd, _ := AsForward(e.tape.At(fwdIdx))
	funcIdx := fwd.FuncIndex

	// Remove forward and end from the stack.
	// Remove higher index first to preserve lower indices.
	if endIdx > fwdIdx {
		e.tape.Remove(endIdx)
		e.tape.Remove(fwdIdx)
		if fwdIdx < funcIdx {
			funcIdx-- // forward removal
		}
		// end was already removed (endIdx > fwdIdx), endIdx > funcIdx always
	} else {
		e.tape.Remove(fwdIdx)
		newEndIdx := endIdx
		if fwdIdx < endIdx {
			newEndIdx--
		}
		e.tape.Remove(newEndIdx)
		if fwdIdx < funcIdx {
			funcIdx--
		}
		if newEndIdx < funcIdx {
			funcIdx--
		}
	}

	e.curryOrStack(funcIdx, fwd.CollectedArgs, fwd.StackArgs)
	return nil
}

// stepMark records the mark's ID in the marks hash table and advances.
// stepDefCleanup removes defs that were created during fn body execution.
// The DefCleanupInfo carries a snapshot of DefStacks lengths taken before
// the body ran. Any defs added since are popped via UninstallDef.
func (e *Engine) stepDefCleanup(val Value) {
	info, _ := AsDefCleanup(val)
	reg := info.Registry
	for _, name := range reg.Defs.Names() {
		prevLen := info.Snapshot[name] // 0 for names not in snapshot
		for reg.Defs.Depth(name) > prevLen {
			UninstallDef(reg, name)
		}
	}
}

func (e *Engine) stepMark(val Value) {
	info, _ := AsMark(val)
	if e.marks == nil {
		e.marks = make(map[string]bool)
	}
	e.marks[info.ID] = true
	e.traceNote = "mark " + info.ID
	e.pointer++
}

// stepMove jumps the pointer back to the corresponding mark, replaying the
// original body. Both the mark and the move are removed from the stack after
// the jump to prevent infinite loops. If the target mark is not found, an
// error is returned using the move's reason metadata.
//
// When the move carries a ForCont (for-loop continuation), stepMoveCont is
// called instead of the basic one-shot replay.
func (e *Engine) stepMove(val Value) error {
	info, _ := AsMove(val)
	moveIdx := e.pointer

	if e.marks == nil || !e.marks[info.To] {
		return e.runtimeError("move_error", fmt.Sprintf("mark %q not found (%s)", info.To, info.Reason), info.To, "")
	}

	// Scan the stack to find the mark's current position.
	markIdx := -1
	for i := 0; i < e.tape.Len(); i++ {
		_as2, _ := AsMark(e.tape.At(i))
		if IsMark(e.tape.At(i)) && _as2.ID == info.To {
			markIdx = i
			break
		}
	}
	if markIdx < 0 {
		// Mark was removed from the stack (e.g. by a for-loop controller
		// signalling loop completion). Remove this orphaned move quietly.
		delete(e.marks, info.To)
		e.tape.Remove(e.pointer)
		e.traceNote = fmt.Sprintf("move orphan %s", info.To)
		return nil
	}

	// Delegate to continuation handler for for-loops.
	if info.Cont != nil {
		return e.stepMoveCont(markIdx, moveIdx, info)
	}

	// Delegate to if-statement continuation handler.
	if info.IfCont != nil {
		return e.stepMoveIf(markIdx, moveIdx, info)
	}

	// Get the saved body from the mark.
	markInfo, _ := AsMark(e.tape.At(markIdx))

	// Remove from hash table.
	delete(e.marks, info.To)

	// Replace everything from mark through move (inclusive) with the body copy.
	body := make([]Value, len(markInfo.Body))
	copy(body, markInfo.Body)
	e.tape.Splice(markIdx, moveIdx-markIdx+1, body...)

	e.traceNote = fmt.Sprintf("move→mark %s", info.To)

	// Set pointer to where the mark was (now the start of the replayed body).
	e.pointer = markIdx
	return nil
}

// stepMoveCont handles a for-loop continuation move. It collects this
// iteration's results, advances the iterator, and either splices in a new
// mark+body+move for the next iteration or finalizes the loop.
func (e *Engine) stepMoveCont(markIdx, moveIdx int, info MoveInfo) error {
	cont := info.Cont

	// Collect resolved values between mark and move (this iteration's output).
	for j := markIdx + 1; j < moveIdx; j++ {
		cont.Results = append(cont.Results, e.tape.At(j))
	}

	// Advance iterator.
	cont.Current += cont.Step

	// Check if more iterations remain.
	moreIterations := (cont.Step > 0 && cont.Current < cont.End) ||
		(cont.Step < 0 && cont.Current > cont.End)

	if moreIterations {
		// Update iterator: uninstall old value, install new one.
		// This keeps the DefStacks depth at 1 throughout the loop.
		UninstallDef(cont.Registry, cont.IterName)
		InstallDef(cont.Registry, cont.IterName, NewInteger(cont.Current))

		// Generate new mark ID.
		id := NextMarkID()

		// Build replacement: mark + body + move.
		body := cont.Body
		tokens := make([]Value, 0, len(body)+2)
		tokens = append(tokens, NewMark(id, body...))
		bodyCopy := make([]Value, len(body))
		copy(bodyCopy, body)
		tokens = append(tokens, bodyCopy...)
		tokens = append(tokens, NewMoveCont(id, info.Reason, cont))

		// Remove old mark ID, register new one.
		delete(e.marks, info.To)
		e.tape.Splice(markIdx, moveIdx-markIdx+1, tokens...)
		if e.marks == nil {
			e.marks = make(map[string]bool)
		}
		e.marks[id] = true

		// Set pointer to the new mark so stepMark processes it.
		e.pointer = markIdx
		e.traceNote = fmt.Sprintf("for next %s i=%d", id, cont.Current)
		return nil
	}

	// Done — uninstall iterator, splice in accumulated results.
	UninstallDef(cont.Registry, cont.IterName)
	delete(e.marks, info.To)
	e.tape.Splice(markIdx, moveIdx-markIdx+1, cont.Results...)
	e.pointer = markIdx
	e.traceNote = "for done"
	return nil
}

// stepMoveIf handles an if-statement continuation move. It collects the
// condition result (all resolved values between mark and move), evaluates
// the last value for truthiness, and splices in the chosen branch.
func (e *Engine) stepMoveIf(markIdx, moveIdx int, info MoveInfo) error {
	ifCont := info.IfCont

	// Collect condition results between mark and move.
	var condResult Value
	for j := markIdx + 1; j < moveIdx; j++ {
		condResult = e.tape.At(j)
	}

	// Remove mark from hash table.
	delete(e.marks, info.To)

	// Check if condition produced a value.
	if condResult.Parent == nil {
		e.tape.Splice(markIdx, moveIdx-markIdx+1)
		e.pointer = markIdx
		return e.runtimeError("runtime_error", "if: condition produced no value", "if", "")
	}

	// Evaluate truthiness and choose branch.
	cond := CoerceBoolean(condResult)

	var branch []Value
	if cond {
		branch = ifCont.Then
	} else {
		branch = ifCont.Else
	}

	// Splice chosen branch (or nothing) in place of mark+condition+move.
	e.tape.Splice(markIdx, moveIdx-markIdx+1, branch...)
	e.pointer = markIdx
	e.traceNote = fmt.Sprintf("if %v", cond)
	return nil
}

// handleFlowCtrl dispatches the active flow-control signal to the
// matching tape-level resolver. Returns true if the signal was consumed
// (the resolver found an enclosing loop on this tape and rewrote it),
// false if no resolver was applicable and the flag should bubble up.
//
// On a true return, FlowCtrl has been cleared. On false, it is left
// set for an outer Run frame to handle.
func (e *Engine) handleFlowCtrl() bool {
	var handled bool
	switch e.registry.FlowCtrl {
	case FlowBreak:
		handled = e.handleLoopBreak()
	case FlowContinue:
		handled = e.handleLoopContinue()
	}
	if handled {
		e.registry.FlowCtrl = FlowNone
	}
	return handled
}

// exitWithFlowCtrl returns from Run when a flow-control signal could
// not be resolved on this tape. For a top-level engine this is the
// "break/continue outside loop" error path; for a sub-engine, the flag
// stays set on the shared registry and the residual tape is returned
// cleanly so an outer Run can resolve it.
func (e *Engine) exitWithFlowCtrl() ([]Value, error) {
	if e.isTop {
		ctrl := e.registry.FlowCtrl
		e.registry.FlowCtrl = FlowNone
		return nil, e.runtimeError("flow_error", fmt.Sprintf("%s outside loop", ctrl), ctrl.String(), "")
	}
	if e.flowUnwind {
		// A VM island: no outer TAPE exists to adopt the residual — tear down
		// the live spliced frames (their registry state: args stack, body-local
		// defs, captures) and return nothing; the VM translates the signal.
		e.unwindLiveFrames(0, e.tape.Len())
		e.tape.TakeAll()
		return nil, nil
	}
	return e.tape.TakeAll(), nil
}

// handleLoopBreak resolves a FlowBreak signal by finding the nearest
// enclosing for-loop (move with continuation) on this tape and
// terminating it. Returns true if a loop was found and rewritten,
// false if no enclosing loop was on the tape.
func (e *Engine) handleLoopBreak() bool {
	// Scan forward from current pointer for a move with continuation.
	for i := e.pointer; i < e.tape.Len(); i++ {
		if IsMove(e.tape.At(i)) {
			info, _ := AsMove(e.tape.At(i))
			if info.Cont != nil {
				// Found the for-loop's move. Find its mark.
				markIdx := -1
				for j := 0; j < i; j++ {
					_as3, _ := AsMark(e.tape.At(j))
					if IsMark(e.tape.At(j)) && _as3.ID == info.To {
						markIdx = j
						break
					}
				}
				if markIdx < 0 {
					delete(e.marks, info.To)
					continue
				}

				// Unwind any fn frame the break is escaping — its cleanup
				// tail is about to be discarded with the loop region and
				// would otherwise leak the per-call stacks (fn_frame.go).
				e.unwindLiveFrames(markIdx, i)

				// Uninstall iterator, splice in accumulated results.
				UninstallDef(info.Cont.Registry, info.Cont.IterName)
				delete(e.marks, info.To)
				e.tape.Splice(markIdx, i-markIdx+1, info.Cont.Results...)
				e.pointer = markIdx
				return true
			}
		}
	}
	return false
}

// handleLoopContinue resolves a FlowContinue signal by finding the
// nearest enclosing for-loop and advancing to the next iteration
// (discarding the current iteration's partial results). Returns true
// if a loop was found, false if no enclosing loop was on the tape.
func (e *Engine) handleLoopContinue() bool {
	// Scan forward from current pointer for a move with continuation.
	for i := e.pointer; i < e.tape.Len(); i++ {
		if IsMove(e.tape.At(i)) {
			info, _ := AsMove(e.tape.At(i))
			if info.Cont != nil {
				// Found the for-loop's move. Find its mark.
				markIdx := -1
				for j := 0; j < i; j++ {
					_as4, _ := AsMark(e.tape.At(j))
					if IsMark(e.tape.At(j)) && _as4.ID == info.To {
						markIdx = j
						break
					}
				}
				if markIdx < 0 {
					delete(e.marks, info.To)
					continue
				}

				// Unwind any fn frame the continue is escaping — its cleanup
				// tail is about to be discarded with the iteration region and
				// would otherwise leak the per-call stacks (fn_frame.go).
				e.unwindLiveFrames(markIdx, i)

				// Remove values between mark and move (discard partial results).
				if i-markIdx > 1 {
					e.tape.Splice(markIdx+1, i-markIdx-1)
					// Recalculate move position.
					i = markIdx + 1
				}
				// Set pointer to the move so stepMove fires next.
				e.pointer = i
				return true
			}
		}
	}
	return false
}

// cleanMarks removes any leftover mark and move entries from the stack.
func (e *Engine) cleanMarks() {
	i := 0
	for i < e.tape.Len() {
		if IsMark(e.tape.At(i)) || IsMove(e.tape.At(i)) {
			e.tape.Remove(i)
		} else {
			i++
		}
	}
	e.marks = nil
}

// stepOpenParen replaces the "(" word with an open-paren marker.
func (e *Engine) stepOpenParen() error {
	e.tape.Set(e.pointer, NewOpenParen())
	e.pointer++
	return nil
}

// consumeStartAt resolves the one-shot resolved-argument prefix for the
// Run that is starting: the leading startAt input values are call-site-
// resolved arguments, so stepping starts after them and they enter as
// stack data (see the startAt field doc). Zeroes the field so it cannot
// leak into a later reuse of a pooled engine.
func (e *Engine) consumeStartAt() int {
	start := 0
	if e.startAt > 0 && e.startAt <= e.tape.Len() {
		start = e.startAt
	}
	e.startAt = 0
	return start
}

// stepPastOpenParen advances the pointer over an open paren at the
// pointer. A fn frame's open paren carries the span of unnamed arguments
// spliced at the frame head; they were resolved at the call site, so the
// pointer skips them and they enter as stack data, never re-stepped — a
// Function value or __SP marker argument must not fire on placement
// (arguments are inert; design/ARG-SEMANTICS-UNIFICATION.0.md).
func (e *Engine) stepPastOpenParen(val Value) {
	e.pointer++
	if info, ok := val.Data.(FrameOpenInfo); ok && info.ArgSpan > 0 {
		e.pointer += info.ArgSpan
	}
}

// stepCloseParen handles the ")" word. It resolves any pending forwards
// inside the paren scope via implicit end, then collapses the sub-expression.
func (e *Engine) stepCloseParen() error {
	closeIdx := e.pointer

	openIdx := -1
	for i := closeIdx - 1; i >= 0; i-- {
		if IsOpenParen(e.tape.At(i)) {
			openIdx = i
			break
		}
	}

	if openIdx < 0 {
		return e.syntaxError("unmatched closing parenthesis", ")")
	}

	// Resolve any forwards inside the paren scope via implicit end.
	// We loop because resolving a forward may cause re-evaluation.
	for attempt := 0; attempt < 222; attempt++ {
		hasFwd := false
		for i := openIdx + 1; i < closeIdx; i++ {
			if IsForward(e.tape.At(i)) {
				hasFwd = true
				fwd, _ := AsForward(e.tape.At(i))
				funcIdx := fwd.FuncIndex
				collectedCount := fwd.CollectedArgs
				stackArgCount := fwd.StackArgs

				// Remove the forward.
				e.tape.Remove(i)
				if i < funcIdx {
					funcIdx--
				}

				// Try stack match or create curry list.
				e.curryOrStack(funcIdx, collectedCount, stackArgCount)

				// Recalculate closeIdx after potential stack changes.
				closeIdx = e.findCloseParenAfter(openIdx)
				if closeIdx < 0 {
					return e.syntaxError("unmatched closing parenthesis", ")")
				}

				// Re-evaluate from current pointer up to closeIdx.
				for e.pointer < closeIdx {
					val := e.tape.At(e.pointer)
					switch {
					case IsWord(val):
						if err := e.stepWord(val); err != nil {
							return err
						}
						// Recalculate closeIdx: stack may have changed.
						closeIdx = e.findCloseParenAfter(openIdx)
						if closeIdx < 0 {
							return e.syntaxError("unmatched closing parenthesis", ")")
						}
					case IsCloseParen(val):
						if err := e.stepCloseParen(); err != nil {
							return err
						}
						closeIdx = e.findCloseParenAfter(openIdx)
						if closeIdx < 0 {
							return e.syntaxError("unmatched closing parenthesis", ")")
						}
					case IsEnd(val):
						if err := e.stepEnd(); err != nil {
							return err
						}
						closeIdx = e.findCloseParenAfter(openIdx)
						if closeIdx < 0 {
							return e.syntaxError("unmatched closing parenthesis", ")")
						}
					case IsForward(val):
						e.pointer++
					case IsOpenParen(val):
						e.pointer++
					case IsReturnCheck(val):
						e.pointer++
					case IsDefCleanup(val):
						e.stepDefCleanup(val)
						e.pointer++
					default:
						if err := e.stepLiteral(); err != nil {
							return err
						}
						closeIdx = e.findCloseParenAfter(openIdx)
						if closeIdx < 0 {
							return e.syntaxError("unmatched closing parenthesis", ")")
						}
					}
					// Propagate any flow-control signal raised by
					// the step; the outer Run frame will resolve it.
					if e.registry.FlowCtrl != FlowNone {
						return nil
					}
				}
				break // restart the outer loop to check for more forwards
			}
		}
		if !hasFwd {
			break
		}
	}

	// Check for any remaining orphaned forwards.
	for i := openIdx + 1; i < closeIdx; i++ {
		if IsForward(e.tape.At(i)) {
			fwd, _ := AsForward(e.tape.At(i))
			if verr := e.voidArgErrorFor(fwd.FuncName, fwd.Pos); verr != nil {
				return verr
			}
			return e.insufficientArgsError(fwd.FuncName, fwd.ExpectedArgs, fwd.Pos)
		}
	}

	// A group that resolved to ZERO values is recorded together with
	// its candidate consumers — the pending words below it on the
	// stack — for the blame-shift shape of ERRORS.8.md §3 (VOXGIG B3):
	// a void call in an argument position starves the consuming word,
	// which then fails LATER with a generic error at an innocent site.
	// Collection may legitimately resume past a void group
	// (`add () 5 6` → 11), so nothing errors here; the record speaks
	// only if one of those candidates raises a signature failure in
	// the same statement (sigError / insufficientArgsError consult it
	// via voidArgErrorFor).
	if closeIdx == openIdx+1 {
		for i := openIdx - 1; i >= 0; i-- {
			v := e.tape.At(i)
			if IsOpenParen(v) {
				break
			}
			switch {
			case IsWord(v):
				w, _ := AsWord(v)
				e.voidGroups = append(e.voidGroups, w.Name)
			case IsForward(v):
				fwd, _ := AsForward(v)
				e.voidGroups = append(e.voidGroups, fwd.FuncName)
			}
		}
	}

	// Remove any surviving def-cleanup markers.
	for i := openIdx + 1; i < closeIdx; i++ {
		if IsDefCleanup(e.tape.At(i)) {
			e.stepDefCleanup(e.tape.At(i))
			e.tape.Remove(i)
			closeIdx--
			i--
		}
	}

	// Check for return type validation.
	for i := openIdx + 1; i < closeIdx; i++ {
		if IsReturnCheck(e.tape.At(i)) {
			rc, _ := AsReturnCheck(e.tape.At(i))
			e.tape.Remove(i)
			closeIdx--

			// Collect resolved values in scope.
			var results []Value
			for j := openIdx + 1; j < closeIdx; j++ {
				results = append(results, e.tape.At(j))
			}

			// Unconsumed unnamed args sit at the bottom of the scope,
			// body results sit at the top. Allow extra values up to the
			// number of unnamed params that were pushed before the body.
			nret := len(rc.Returns)
			if len(results) < nret {
				return e.returnCountError(rc.FuncName, nret, len(results), rc.Pos)
			}
			extra := len(results) - nret
			if extra > rc.UnnamedCount {
				return e.returnCountError(rc.FuncName, nret, len(results)-rc.UnnamedCount, rc.Pos)
			}

			// Validate the top nret values match declared return types.
			if err := e.validateReturnTypes(rc, results, extra); err != nil {
				return err
			}

			// Discard unconsumed unnamed args from the bottom of the scope.
			for j := 0; j < extra; j++ {
				e.tape.Remove(openIdx + 1)
				closeIdx--
			}
			break
		}
	}

	// Check-mode fn-value-call boundary guard. A paren whose net contents are
	// a leading DYNAMIC value followed by >=1 more value is a deferred fn-value
	// application bounded by THIS paren — `(m.g 3)`, where m.g is a dynamic
	// method-field get the checker cannot dispatch in place (it stays a value
	// and flows to the residual's OpCallDynamic). That residual reconciliation
	// is paren-UNAWARE: if the paren result is then consumed by a trailing op,
	// the op reorders ahead of the apply and the value is applied to the wrong
	// argument — `((m.g 3) add 1)` compiled m.g(3 add 1)=8 instead of
	// (m.g 3) add 1=7. The interpreter dispatches the concrete fn AT the paren,
	// so refuse here and let the faithful interpreter fallback run it.
	if es := e.registry.Check.Recorder(); es.active() {
		first, count, lastIdx := -1, 0, -1
		for i := openIdx + 1; i < closeIdx; i++ {
			if isRecordableLiteral(e.tape.At(i)) {
				if count == 0 && e.tape.At(i).Dynamic {
					first = i
				}
				count++
				lastIdx = i
			}
		}
		// Classify the paren's fn-value-call boundary. The TRAILING case is checked
		// FIRST: when the LAST value is a concrete Function applied to the preceding
		// args, it is the sound paren-bounded apply (`(prev key comp)`, or
		// `((arr get x) key comp)` where an ARG is dynamic but comp is the fn) — even
		// if an arg is dynamic, the dynamic is an ARGUMENT, not the applied fn, so the
		// leading-dynamic reorder hazard does not apply. Only when the last value is
		// NOT the fn does a LEADING dynamic value mean the dynamic IS the fn being
		// applied before its args (`(m.g 3)`, `((m.g 3) add 1)`) — the unsound
		// paren-unaware reorder the residual reconciliation cannot reproduce.
		last := Value{}
		if lastIdx >= 0 {
			last = e.tape.At(lastIdx)
		}
		switch {
		case count >= 2 && lastIdx >= 0 && !last.Dynamic && isFnValueResidual(last):
			// TRAILING fn-value apply (`(a b comp)`): record it as an EVENT producing
			// ONE carrier and COLLAPSE the [args…, fn] tape residual to that carrier —
			// exactly as the interpreter's paren auto-dispatch nets one result. The
			// event seats like any computed result (a def-local `def c (a b comp)`, an
			// `if` operand, a list member, the body residual), so a comparator apply
			// bound to a local compiles, not ONLY the body's trailing residual. On
			// refusal (an unresolvable operand or a nested unapplied fn arg) the residual
			// is left intact and the body-residual lowering (RegisterTrailingApply) still
			// handles the trailing-residual case soundly.
			var argVals []Value
			var argIdxs []int
			for i := openIdx + 1; i < closeIdx; i++ {
				if isRecordableLiteral(e.tape.At(i)) && i != lastIdx {
					argVals = append(argVals, e.tape.At(i))
					argIdxs = append(argIdxs, i)
				}
			}
			out := NewCarrier(TAny)
			out.ID = GenerateID(IDPrefixForType(TAny))
			out.Pos = last.Pos
			if es.RecordDynApply(argVals, last, out, last.Pos) {
				e.tape.Set(lastIdx, out)
				for j := len(argIdxs) - 1; j >= 0; j-- {
					e.tape.Remove(argIdxs[j])
					closeIdx--
				}
			} else {
				es.RegisterTrailingApply(last.ID, count-1)
			}
		case first >= 0 && count >= 2:
			es.MarkUncompilable("fn-value application bounded by a paren (dynamic value precedes args)")
		}
	}

	// Remove the close paren (higher index first) and open paren.
	// The values between them are already in place.
	e.tape.Remove(closeIdx)
	e.tape.Remove(openIdx)

	// Recorder hook: the values that survived inside the paren will
	// be re-encountered by the main loop after we set pointer back
	// to openIdx (below). They were already emitted to the recorder
	// during the in-paren execution (via stepLiteral or via execMatch
	// for handler results), so tell a SkipRecorder to ignore the
	// next round.
	if skipper, ok := e.recorder.(RecorderSkipper); ok && e.recorder != nil {
		survived := 0
		// Contents now sit at [openIdx .. closeIdx-2] inclusive.
		// (closeIdx was the position of the CloseParen BEFORE the
		// pair removals; both removals happened above so the
		// surviving content occupies indices openIdx through
		// closeIdx-2 in the new stack — same as [openIdx, closeIdx-1)
		// in the original stack indices, minus 1 for the removed
		// OpenParen.)
		end := closeIdx - 1
		if end > e.tape.Len() {
			end = e.tape.Len()
		}
		for i := openIdx; i < end; i++ {
			if isRecordableLiteral(e.tape.At(i)) {
				survived++
			}
		}
		if survived > 0 {
			skipper.Skip(survived)
		}
	}

	e.pointer = openIdx
	return nil
}

// findCloseParenAfter finds the index of the matching close-paren marker
// after the given openIdx.
func (e *Engine) findCloseParenAfter(openIdx int) int {
	depth := 0
	for i := openIdx + 1; i < e.tape.Len(); i++ {
		if IsOpenParen(e.tape.At(i)) {
			depth++
		} else if IsCloseParen(e.tape.At(i)) {
			if depth == 0 {
				return i
			}
			depth--
		}
	}
	return -1
}

// effectiveResolved returns the resolved portion of the stack visible for
// stack matching. Function words and their collected forward args that are
// tracked by active forwards are excluded — they belong to the outer
// forward's context and should not be consumed by inner stack matching.
func (e *Engine) effectiveResolved() []Value {
	start := 0
	excludeIndices := make(map[int]bool)
	for i := e.pointer - 1; i >= 0; i-- {
		if IsOpenParen(e.tape.At(i)) {
			start = i + 1
			break
		}
		if IsForward(e.tape.At(i)) {
			fwd, _ := AsForward(e.tape.At(i))
			// Exclude the function word itself.
			excludeIndices[fwd.FuncIndex] = true
			// Exclude collected forward args (positioned before function word).
			for j := 0; j < fwd.CollectedArgs; j++ {
				idx := fwd.FuncIndex - 1 - j
				if idx >= 0 {
					excludeIndices[idx] = true
				}
			}
			// Exclude claimed stack args (positioned before collected forward args).
			stackStart := fwd.FuncIndex - fwd.CollectedArgs - fwd.StackArgs
			for j := stackStart; j < fwd.FuncIndex-fwd.CollectedArgs; j++ {
				if j >= 0 {
					excludeIndices[j] = true
				}
			}
		}
	}
	var resolved []Value
	for i := start; i < e.pointer; i++ {
		v := e.tape.At(i)
		if IsForward(v) || IsOpenParen(v) || IsMark(v) || IsMove(v) || excludeIndices[i] {
			continue
		}
		resolved = append(resolved, v)
	}
	return resolved
}

// isInsidePendingForward returns true if the current pointer is within the
// collection scope of a pending forward (i.e., another function is waiting
// to collect this function's result as a forward arg).
func (e *Engine) isInsidePendingForward() bool {
	for i := e.pointer - 1; i >= 0; i-- {
		if IsOpenParen(e.tape.At(i)) {
			return false
		}
		if IsForward(e.tape.At(i)) {
			return true
		}
	}
	return false
}

// curryOrStack handles a terminated forward. If the word at funcIdx can
// match a stack signature with the available resolved values, it forces
// stack mode for normal execution. Otherwise, it packages the word and
// its collectedCount forward args into a list value (partial application).
// When the list is later expanded (e.g., via def body substitution), the
// word and args are spliced back onto the stack for completion.
func (e *Engine) curryOrStack(funcIdx int, collectedCount int, stackArgCount ...int) {
	sac := 0
	if len(stackArgCount) > 0 {
		sac = stackArgCount[0]
	}

	if funcIdx >= e.tape.Len() || !IsWord(e.tape.At(funcIdx)) {
		e.pointer = funcIdx
		return
	}

	w, _ := AsWord(e.tape.At(funcIdx))
	fn := e.registry.Lookup(w.Name)

	// Check if stack match exists with current resolved values.
	if fn != nil {
		// Build resolved slice up to funcIdx, excluding function words
		// and their collected forward args that are tracked by active
		// forwards. This prevents stack matching from consuming values
		// that belong to an outer forward's context.
		start := 0
		excludeIndices := make(map[int]bool)
		for i := funcIdx - 1; i >= 0; i-- {
			if IsOpenParen(e.tape.At(i)) {
				start = i + 1
				break
			}
			if IsForward(e.tape.At(i)) {
				fwd, _ := AsForward(e.tape.At(i))
				// Exclude the function word itself.
				excludeIndices[fwd.FuncIndex] = true
				// Exclude collected forward args (before function word).
				for j := 0; j < fwd.CollectedArgs; j++ {
					idx := fwd.FuncIndex - 1 - j
					if idx >= 0 {
						excludeIndices[idx] = true
					}
				}
				// Exclude claimed stack args.
				stackStart := fwd.FuncIndex - fwd.CollectedArgs - fwd.StackArgs
				for j := stackStart; j < fwd.FuncIndex-fwd.CollectedArgs; j++ {
					if j >= 0 {
						excludeIndices[j] = true
					}
				}
			}
		}
		var resolved []Value
		for i := start; i < funcIdx; i++ {
			v := e.tape.At(i)
			if IsForward(v) || IsOpenParen(v) || excludeIndices[i] {
				continue
			}
			resolved = append(resolved, v)
		}

		testW := WordInfo{Name: w.Name, ArgCount: -1, ForceStack: true}

		// For words whose sigs collect forward args, rearrange values
		// so forward args are first and stack args are reversed before
		// matching.
		if fn.HasForwardSigs() && sac > 0 {
			e.pointer = funcIdx
			e.rearrangeForForward(sac, collectedCount)
		}

		match := MatchSignature(fn.Signatures, resolved, testW)
		if match != nil {
			e.forceStackWord(funcIdx, w)
			e.pointer = funcIdx
			return
		}
	}

	// Check if there's a pending outer forward that would collect the result.
	// Only create a curry list when an outer context is waiting for a value;
	// otherwise, fall through to normal stack retry (which may error).
	hasOuterForward := false
	checkStart := funcIdx - collectedCount
	if checkStart < 0 {
		checkStart = 0
	}
	for i := checkStart - 1; i >= 0; i-- {
		if IsOpenParen(e.tape.At(i)) {
			break
		}
		if IsForward(e.tape.At(i)) {
			hasOuterForward = true
			break
		}
	}

	if hasOuterForward {
		// Create a curry list: [word, arg1, arg2, ...].
		// When this list is expanded by def body substitution, it re-emits
		// the word and collected args for completion with additional args.
		startIdx := funcIdx - collectedCount
		if startIdx < 0 {
			startIdx = 0
		}

		elems := make([]Value, 0, 1+collectedCount)
		elems = append(elems, NewWord(w.Name))
		for i := startIdx; i < funcIdx; i++ {
			elems = append(elems, e.tape.At(i))
		}

		e.tape.Splice(startIdx, collectedCount+1, NewList(elems))
		e.pointer = startIdx
		return
	}

	// No outer forward - force stack (may result in error on next step).
	e.forceStackWord(funcIdx, w)
	e.pointer = funcIdx
}

// hasPendingForwardQuoteArg reports whether there is a pending forward
// whose next slot is marked /q (QuoteArgs) — meaning the upcoming Word
// should be captured as an Atom rather than executed. This is the
// general word-capture mechanism used by def, undef, type, untype,
// quote, inspect, etc.; see signature.go §1.5 on /q.
func (e *Engine) hasPendingForwardQuoteArg() bool {
	for i := e.pointer - 1; i >= 0; i-- {
		if IsOpenParen(e.tape.At(i)) {
			break
		}
		if IsForward(e.tape.At(i)) {
			fwd, _ := AsForward(e.tape.At(i))
			// Forward args fill from sigArgs[0]; the next forward slot
			// is at index CollectedArgs.
			nextIdx := fwd.CollectedArgs
			if nextIdx < fwd.Sig.TotalArgs() {
				return fwd.Sig.QuoteArgs != nil && fwd.Sig.QuoteArgs[nextIdx]
			}
			break
		}
	}
	return false
}

// hasPendingForwardFormArg reports whether the nearest enclosing pending
// Forward's next slot is FormArgs — meaning the upcoming Word should be
// collected as a raw Word (not executed, not coerced to an Atom). Mirrors
// hasPendingForwardQuoteArg. See design/MACROS-PHASE1.10.md §3.
func (e *Engine) hasPendingForwardFormArg() bool {
	for i := e.pointer - 1; i >= 0; i-- {
		if IsOpenParen(e.tape.At(i)) {
			break
		}
		if IsForward(e.tape.At(i)) {
			fwd, _ := AsForward(e.tape.At(i))
			nextIdx := fwd.CollectedArgs
			if nextIdx < fwd.Sig.TotalArgs() {
				return fwd.Sig.FormArgs != nil && fwd.Sig.FormArgs[nextIdx]
			}
			break
		}
	}
	return false
}

// hasPendingForwardExpectingFunction checks if there is a pending forward
// whose next expected argument is TFunction.
func (e *Engine) hasPendingForwardExpectingFunction() bool {
	for i := e.pointer - 1; i >= 0; i-- {
		if IsOpenParen(e.tape.At(i)) {
			break
		}
		if IsForward(e.tape.At(i)) {
			fwd, _ := AsForward(e.tape.At(i))
			// Forward args fill from sigArgs[0].
			nextIdx := fwd.CollectedArgs
			if nextIdx < fwd.Sig.TotalArgs() {
				return sigArgType(fwd.Sig, nextIdx).Equal(TFunction)
			}
			break
		}
	}
	return false
}

// matchSignature is the unified signature matching function.
//
// Algorithm:
//
//	0.1 Using the ordered signatures, attempt to match in order,
//	    stopping at the first match.
//	1.1 If stack-only (or /s) and not /f: skip forward, go to step 2.
//	    If stack-only but /f: override → do forward scan.
//	1.2 Match each parameter in order against future tokens.
//	1.3 Stop if all params matched, or if /N params reached.
//	1.4 Move to step 2 if you hit a boundary condition:
//	    a function word, a pipe barrier, or "end".
//	1.5 If you hit an open paren, treat as boundary (pre-evaluated).
//	2.1 Match the remaining parameters against the stack, working
//	    backwards (top of stack first).
//	2.2 Stop once all or /N params reached.
//
// This is implemented as one outer loop over signatures and one inner
// loop over parameters. No separate functions are called for matching.
//
// Returns: matched signature, arg positions (absolute stack indices
// in signature order), and the speculation marker. Positions > pointer
// are forward args that need deferred collection. Positions < pointer
// are stack args. Returns nil sig if no signature matches.
//
// The third return is the sig-order index of the FIRST forward slot
// the plan filled with a WORD bound to a dispatching definition (an
// FnDefInfo binding accepted as an operand — typically through an
// Any-typed slot), or -1 when no slot was filled that way. Such a
// token is planned as an operand but DISPATCHES at runtime — the
// plan-time stop condition insertForward records on ForwardInfo so
// the arrival side can observe it. See
// design/FORWARD-COLLECTION-PHASES.10.md.
//
//nolint:gocyclo,gocognit // dispatch is inherently a big switch; see STATIC_ANALYSIS_REPORT.10.md
func (e *Engine) matchSignature(fn *FnDefInfo, w WordInfo, resolved []Value) (*Signature, []int, int) {

	// Unified dispatch (post §1.4 fix): no more stackOnly/forward-prec
	// dichotomy at the word level. Each sig declares its own boundary
	// via BarrierPos — the count of leading args that may be collected
	// from forward tokens. Args at sig[BarrierPos..N-1] always come
	// from the stack, top-down. The /s and /f modifiers override
	// BarrierPos at the call site:
	//   - /s (ForceStack)   → boundary at 0, all stack
	//   - /f (ForceForward) → boundary at N, all forward
	insideForward := e.isInsidePendingForward()

	// Forward/stack split ambiguity (check mode only, compile-time advisory).
	// If a more-specific overload is rejected because the stack-top operand
	// is a genuinely MIXED gradual carrier (carrierMixedConform), and the
	// overload finally SELECTED forward-collects instead — leaving that
	// carrier on the stack — the static split diverges from the runtime one
	// (a concrete value would have matched the more-specific overload and
	// been grabbed). noteSplit flags it so the compiler refuses; dispatch
	// itself is unchanged. See CheckState.AmbiguousGradualSplit.
	checkActive := e.registry != nil && e.registry.Check.IsActive()
	mixedCarrierRejectIdx := -1
	noteSplit := func(positions []int, fwd int) {
		if !checkActive || mixedCarrierRejectIdx < 0 || fwd == 0 {
			return
		}
		for _, p := range positions {
			if p == mixedCarrierRejectIdx {
				return // the carrier was consumed after all — not skipped
			}
		}
		e.registry.Check.AmbiguousGradualSplit = true
	}

	// When the next forward token is a Word, prefer signatures with
	// /q at position 0 (inspect-style name capture). The user wrote a
	// Word, not a String — the /q sig captures the user's intent that
	// the name is data, not a call site. The non-/q TString sister
	// sig is for callers who pass a string literal. This also covers
	// untype Foo (Foo in r.Types), `m.Color` after import (Color is a
	// key in the imported map), and inspect-style name capture.
	preferWordSig := false
	if e.pointer+1 < e.tape.Len() {
		next := e.tape.At(e.pointer + 1)
		if IsWord(next) {
			preferWordSig = true
		}
	}

	// Build a map from resolved values to their absolute stack indices.
	// This lets us record exact positions for stack-matched args.
	resolvedIdx := e.resolvedIndicesBefore(len(resolved))

	// Track the best non-preferred match so that if no preferred sig
	// matches, we can fall back to it without a second pass.
	type matchResult struct {
		sig       *Signature
		positions []int
		specAt    int
	}
	var bestDeferred *matchResult

	// ── 0.1: one outer loop over sorted signatures ───────────────
	for si := range fn.Signatures {
		sig := &fn.Signatures[si]

		if sig.Fallback {
			continue
		}
		if w.ArgCount >= 0 && sig.TotalArgs() != w.ArgCount {
			continue
		}

		nArgs := sig.TotalArgs()

		// 0-arg sigs are deferred to the fallback section at the bottom.
		if nArgs == 0 {
			continue
		}

		// Check if this is a preferred (/q at arg[0]) signature.
		isPreferred := preferWordSig && nArgs > 0 &&
			sig.QuoteArgs != nil && sig.QuoteArgs[0]

		// Effective forward limit for this match attempt. /s and /f
		// override the sig's declared boundary.
		forwardLimit := sig.BarrierPos
		switch {
		case w.ForceStack:
			forwardLimit = 0
		case w.ForceForward:
			forwardLimit = nArgs
		}

		// ── Step 1: forward matching ─────────────────────────────

		positions := make([]int, nArgs)
		fwd := 0     // number of params matched by forward tokens
		specAt := -1 // first slot filled by a dispatching word, -1 = none

		// Always run the forward scan up to forwardLimit; if it's 0
		// the loop simply doesn't execute and all args come from
		// the stack below.
		{
			scanIdx := e.pointer + 1

			// One inner loop over parameters, matching forward tokens.
			for fwd < forwardLimit && scanIdx < e.tape.Len() {

				tok := e.tape.At(scanIdx)
				expectedType := sigArgType(sig, fwd)

				// 1.4: structural boundaries — stop forward scan.
				if IsForward(tok) || tok.Parent.ConformsTo(TMark) || tok.Parent.ConformsTo(TMove) ||
					tok.Parent.ConformsTo(TInternal) || tok.Parent.ConformsTo(TReturnCheck) {
					break
				}

				// 1.4: end, ) — boundary, stop.
				if IsEnd(tok) || IsCloseParen(tok) {
					break
				}

				// 1.5: open parens are pre-evaluated by preEvalParens
				// before matching begins. If one remains, treat as boundary.
				if IsOpenParen(tok) {
					break
				}

				// FormArgs (macro raw capture): accept ANY form at this
				// position — a word stays a Word, a paren/list/literal stays
				// as-is — with no resolution, no dispatch, no Word→Atom
				// coercion, and no function-word boundary. The operand is
				// captured unevaluated. See design/MACROS-PHASE1.10.md §3.
				if sig.FormArgs != nil && sig.FormArgs[fwd] {
					positions[fwd] = scanIdx
					fwd++
					scanIdx++
					continue
				}

				if IsWord(tok) {
					ww, _ := AsWord(tok)
					// /q modifier: capture the upcoming Word as an Atom
					// (the conversion happens at insertForward / stepLiteral
					// time; here we just count it as a match).
					if sig.QuoteArgs != nil && sig.QuoteArgs[fwd] {
						if TAtom.ConformsTo(expectedType) {
							positions[fwd] = scanIdx
							fwd++
							scanIdx++
							continue
						}
						break
					}

					// Defined word: resolves to its def type.
					if top, ok := e.registry.Defs.Top(ww.Name); ok {
						// Gradual typing: an Any-typed forward operand — a value
						// flowed from a dynamic `get`, or a param bound to Any at
						// a gradual call site — is optimistically accepted for a
						// concrete param in PURE CHECK mode. At runtime the value
						// is concrete and dispatches (or raises) exactly as the
						// interpreter does, so the static analysis stays advisory
						// rather than emitting a spurious no_signature. NOT in
						// compile mode: there the dispatch must remain UNMATCHED so
						// the emitter refuses (force-compile) instead of baking a
						// wrong direct call — preserving compile==interpret.
						gradualAny := checkActive && !e.registry.Check.Compiling &&
							top.Parent != nil && top.Parent.Equal(TAny)
						if sigArgMatches(sig, fwd, top) || expectedType.Equal(TAny) || gradualAny {
							// A dispatching binding (FnDefInfo) planned as an
							// operand is SPECULATIVE: at runtime this token
							// dispatches rather than arriving as a value
							// (the `def name fn […]` idiom relies on exactly
							// that — fn runs and its result completes def).
							// Record the first such slot so the parked
							// ForwardInfo carries the plan's stop condition.
							// A slot that specifically expects a Function
							// gets the word as a resolved REFERENCE at
							// collection time (stepWord's TFunction
							// intercept) — consistent, not speculative.
							if _, isFn := top.Data.(FnDefInfo); isFn &&
								specAt == -1 && !expectedType.Equal(TFunction) {
								specAt = fwd
							}
							positions[fwd] = scanIdx
							fwd++
							scanIdx++
							continue
						}
						if _, ok := top.Data.(FnDefInfo); !ok {
							break // simple def, type mismatch
						}
					}

					// Named type from r.Types: resolves to the type
					// value (mirror of stepWord's r.Types lookup so the
					// planner's expected type matches what stepWord
					// will actually push at runtime). Predicate types
					// arrive as TFnDef/TFunction values; plan against
					// that Parent for sig matching.
					if tv, ok := e.registry.TopTypeBody(ww.Name); ok {
						if sigArgMatches(sig, fwd, tv) || expectedType.Equal(TAny) {
							positions[fwd] = scanIdx
							fwd++
							scanIdx++
							continue
						}
						break // named-type value doesn't fit this slot
					}

					// 1.4: function word — boundary, stop.
					if e.registry.Lookup(ww.Name) != nil {
						break
					}

					// Known literals: true/false → Boolean, type names → type literal.
					if ww.Name == "true" || ww.Name == "false" {
						if sigArgMatches(sig, fwd, Value{Parent: TBoolean}) || expectedType.Equal(TAny) {
							positions[fwd] = scanIdx
							fwd++
							scanIdx++
							continue
						}
						break
					}
					if tn, isType := typeNames[ww.Name]; isType {
						if sigArgMatches(sig, fwd, NewTypeLiteral(tn)) {
							positions[fwd] = scanIdx
							fwd++
							scanIdx++
							continue
						}
						break
					}
					if tn, isType := ResolveTypePath(ww.Name); isType {
						if sigArgMatches(sig, fwd, NewTypeLiteral(tn)) {
							positions[fwd] = scanIdx
							fwd++
							scanIdx++
							continue
						}
						break
					}

					// Undefined word: always resolves to Atom.
					if sigArgMatches(sig, fwd, Value{Parent: TAtom}) || expectedType.Equal(TAny) {
						positions[fwd] = scanIdx
						fwd++
						scanIdx++
						continue
					}
					break // type mismatch
				}

				// Open paren marker: boundary, stop forward scan.
				if IsOpenParen(tok) {
					break
				}

				// A /q (QuoteArgs) position captures a literal word/ATOM (the
				// IsWord branch above handles a raw word). A non-concrete carrier
				// whose type is NOT atom-family — a computed check-mode value such
				// as the pre-evaluated result of `quote (s get k)` (an Any/Integer
				// carrier) — is not an atom, so it must not fill the /q slot via
				// the Any-conforms-to-everything rule: that would pick quote's
				// word-capture sig ([TAtom], QuoteArgs) over its value sig ([TAny],
				// ReturnsIdentity), refuse to compile, and (since the /q handler is
				// quoteWordHandler) never run the value path. A genuine Atom
				// carrier (e.g. `set (quote name) v`) DOES conform and still
				// matches. Inert at runtime (operands are concrete there). Mirrors
				// the stack-phase and positionalMatch /q guards. See
				// design/module-fn-checkstate-ownership.2.md.
				if sig.QuoteArgs != nil && sig.QuoteArgs[fwd] && tok.Carrier && !IsConcrete(tok) && !tok.Parent.ConformsTo(TAtom) {
					break
				}

				// Literal value: direct type check.
				if sigArgMatches(sig, fwd, tok) || expectedType.Equal(TAny) {
					isTypeArg := sig.TypeArgs != nil && sig.TypeArgs[fwd]
					if !isTypeArg && rejectsTypeLiteral(tok, expectedType) {
						break // reject type literal at concrete-payload sig
					}
					positions[fwd] = scanIdx
					fwd++
					scanIdx++
					continue
				}

				// *Type mismatch — stop forward scanning.
				break
			}
		}

		// 1.3: all params matched by forward?
		if fwd == nArgs {
			// Pattern check (post §1.1 fix): scalar literals route
			// through Signature.Patterns instead of value-tagged
			// type paths, so the pattern check has to run for
			// forward-matched positions too. The previous code
			// short-circuited here without consulting Patterns,
			// which made `def fact[0] (1)` fire for any integer.
			if !patternsOk(sig, positions, e.tape, fwd, e.registry) {
				continue
			}
			if preferWordSig && !isPreferred {
				if bestDeferred == nil {
					bestDeferred = &matchResult{sig, append([]int(nil), positions...), specAt}
				}
				continue
			}
			noteSplit(positions, fwd)
			return sig, positions, specAt
		}

		// Inside a pending forward scope: all args must come from
		// forward. Accept only if forward+stack would satisfy the sig.
		if insideForward && fwd > 0 {
			remaining := nArgs - fwd
			if len(resolved) >= remaining {
				canStack := true
				for j := 0; j < remaining; j++ {
					stackVal := resolved[len(resolved)-1-j]
					if !sigArgMatches(sig, fwd+j, stackVal) {
						canStack = false
						break
					}
				}
				if canStack {
					// Fill remaining positions from stack (nearest first).
					for j := 0; j < remaining; j++ {
						ri := len(resolvedIdx) - 1 - j
						positions[fwd+j] = resolvedIdx[ri]
					}
					if preferWordSig && !isPreferred {
						if bestDeferred == nil {
							bestDeferred = &matchResult{sig, append([]int(nil), positions...), specAt}
						}
						continue
					}
					noteSplit(positions, fwd)
					return sig, positions, specAt
				}
			}
			continue
		}

		// /f means all args must come from forward — if any args
		// remain unmatched after the forward scan, this sig fails.
		if w.ForceForward && fwd < nArgs {
			continue
		}

		// ── Step 2: stack matching ───────────────────────────────

		remaining := nArgs - fwd
		if len(resolved) < remaining {
			continue // not enough stack values
		}

		// 2.1: match remaining sig positions against the stack,
		// top-down. sig[fwd] = top of stack, sig[fwd+1] = next deeper,
		// etc. This is the "stack in reverse order" half of the
		// unified rule — same for stack-only sigs (BarrierPos=0)
		// as for partial-boundary sigs.

		allMatch := true
		for j := 0; j < remaining; j++ {
			ri := len(resolvedIdx) - 1 - j
			stackVal := resolved[ri]
			sigIdx := fwd + j

			// /q is a forward-only rule (see Signature.QuoteArgs doc).
			// stackVal cannot be a Word in normal execution: stepWord
			// has already resolved any Word at the pointer to a function
			// call, defined value, or Atom, and quote produces Atoms.
			// The branch below is defensive only — a stack Atom matches
			// an [Atom/q, ...] sig via the regular sigTypeMatches path
			// just below, no /q involvement required.
			if sig.QuoteArgs != nil && sig.QuoteArgs[sigIdx] && stackVal.Parent.Equal(TWord) {
				if !TAtom.ConformsTo(sigArgType(sig, sigIdx)) {
					allMatch = false
					break
				}
				positions[sigIdx] = resolvedIdx[ri]
				continue
			}
			// A /q position captures a literal word/atom; a NON-CONCRETE carrier
			// (a computed check-mode value) must not fill it via the
			// Any-conforms-to-everything rule — it belongs to a value overload.
			// Mirrors the forward-scan and positionalMatch /q guards. Inert at
			// runtime (operands are concrete there).
			if sig.QuoteArgs != nil && sig.QuoteArgs[sigIdx] && stackVal.Carrier && !IsConcrete(stackVal) && !stackVal.Parent.ConformsTo(TAtom) {
				allMatch = false
				break
			}
			if !sigArgMatches(sig, sigIdx, stackVal) {
				// A more-specific overload rejected because the stack-top
				// operand is a mixed gradual carrier: a concrete value drawn
				// from it might have matched here (and been grabbed). Remember
				// it; if a forward-collecting overload is then selected, the
				// split is ambiguous (noteSplit at the return points).
				if checkActive && j == 0 && carrierMixedConform(stackVal, sigArgType(sig, sigIdx)) {
					mixedCarrierRejectIdx = resolvedIdx[ri]
				}
				allMatch = false
				break
			}
			isTypeArg := sig.TypeArgs != nil && sig.TypeArgs[sigIdx]
			if !isTypeArg && rejectsTypeLiteral(stackVal, sigArgType(sig, sigIdx)) {
				allMatch = false
				break
			}
			positions[sigIdx] = resolvedIdx[ri]
		}
		if !allMatch {
			continue
		}

		// Check structural patterns on every matched position. Post
		// §1.1 fix: scalar literals route through Patterns regardless
		// of whether they came from forward or stack matching, so the
		// pattern check no longer skips forward positions.
		if !patternsOk(sig, positions, e.tape, fwd, e.registry) {
			continue
		}

		// Full match found.
		if preferWordSig && !isPreferred {
			if bestDeferred == nil {
				bestDeferred = &matchResult{sig, append([]int(nil), positions...), specAt}
			}
			continue
		}
		return sig, positions, specAt
	}

	// Return deferred non-preferred match if one was found.
	if bestDeferred != nil {
		return bestDeferred.sig, bestDeferred.positions, bestDeferred.specAt
	}

	// Try fallback (0-arg or Fallback handler).
	for si := range fn.Signatures {
		sig := &fn.Signatures[si]
		if w.ArgCount >= 0 && sig.TotalArgs() != w.ArgCount {
			continue
		}
		if sig.TotalArgs() == 0 || sig.Fallback {
			return sig, nil, -1
		}
	}

	return nil, nil, -1
}

// checkModeFallbackPositions returns up to n stack indices to use as
// argument positions when a check-mode fallback fires (no signature
// matched, assume first candidate). Values before the pointer are
// preferred (normal stack order); any shortfall is filled from
// values after the pointer, skipping control tokens. Types are not
// verified — this is the "assume" path.
func (e *Engine) checkModeFallbackPositions(n int) []int {
	positions := e.resolvedIndicesBefore(n)
	remaining := n - len(positions)
	// depth tracks open forward-groups entered during this walk so the close that
	// returns to depth 0 is the ENCLOSING group's `)` — a forward arg never crosses
	// it. Without this break the recovery could gather positions PAST the group's
	// close when its assumed arity exceeds the real arg count; splicing those out
	// then deletes tokens across the `)` boundary and leaves a phantom "unmatched
	// opening parenthesis" in a later (balanced) fn body — the emergent whole-module
	// paren bleed (template.aql's first-word / after-word / parts errors).
	depth := 0
	for i := e.pointer + 1; remaining > 0 && i < e.tape.Len(); i++ {
		v := e.tape.At(i)
		if IsCloseParen(v) {
			if depth == 0 {
				break
			}
			depth--
			continue
		}
		if IsOpenParen(v) {
			depth++
			continue
		}
		if IsForward(v) || IsMark(v) || IsMove(v) ||
			IsReturnCheck(v) || IsDefCleanup(v) {
			continue
		}
		positions = append(positions, i)
		remaining--
	}
	return positions
}

// sigOrderArgs reorders the recovery path's tape-ordered operands
// (the first nStack are stack args, ascending toward the pointer; the
// rest are forward args in source order) into SIGNATURE order, where
// sig[0] is the top of the stack. Per the one kernel arg convention,
// forward args fill the leading sig positions in source order and the
// stack args fill the remainder top-down — so the ascending stack run
// is reversed. The result feeds RecordPolyCall, which (like every other
// recorded dispatch) takes args in sig order so the VM re-match reads
// them faithfully.
func sigOrderArgs(args []Value, nStack int) []Value {
	if nStack < 0 || nStack > len(args) {
		nStack = len(args)
	}
	out := make([]Value, 0, len(args))
	// Forward args (sig[0..k-1]) — already in source order.
	out = append(out, args[nStack:]...)
	// Stack args (sig[k..N-1]) — top of stack is the last (closest to the
	// pointer) of the ascending run, so walk it in reverse.
	for i := nStack - 1; i >= 0; i-- {
		out = append(out, args[i])
	}
	return out
}

// checkModeAssumeSig is the recovery path for unmatched signatures in
// check mode: emit a diagnostic (with pos attached), gather up to N
// adjacent positions as synthetic args, synthesise carrier results
// from the assumed signature, and splice them over the word +
// consumed positions.
//
// This path deliberately bypasses forward collection and type
// matching — both would cascade failures. The trade-off is that the
// checker reports one diagnostic per site and keeps going with the
// assumed signature's declared return types (or Any if unannotated).
// checkModeSurfaceShape is the S2 typing path: when the concrete
// overloads of w reject the candidate args but one of them is a
// surface-typed carrier whose contract REQUIRES w, the call types as
// the surface's fnsig shape with Self := the surface node — the
// contract guarantees the operation exists for every member, so the
// checker uses the declared shape instead of degrading through the
// assume-sig path (which would emit a spurious no_signature and often
// land on Any). Returns handled=false when no nearby candidate arg is
// a surface carrier requiring w.
func (e *Engine) checkModeSurfaceShape(w WordInfo, pos SrcPos) (bool, error) {
	// Locate a surface-typed candidate arg requiring w among the
	// nearby positions (same neighbourhood the assume-sig path
	// gathers; MaxArgs would over-collect, 4 covers surface op
	// arities in practice).
	var sinfo *SurfaceInfo
	var shape Value
	for _, p := range e.checkModeFallbackPositions(4) {
		v := e.tape.At(p)
		// A position AFTER the pointer may still hold the raw Word
		// token (forward args resolve during collection, which the
		// fallback path bypasses). Resolve it the way the forward scan
		// would — via the def stack — so a def-bound surface carrier
		// (e.g. a generic fn's surface-bounded `x:T` param inside
		// AnalyseFnBody, design/GENERICS.10.md Phase 5) is visible to
		// the S2 scan.
		if IsWord(v) {
			if wv, werr := AsWord(v); werr == nil {
				if top, ok := e.registry.Defs.Top(wv.Name); ok {
					v = top
				}
			}
		}
		if v.Parent == nil {
			continue
		}
		info, ok := SurfaceInfoOf(v.Parent)
		if !ok {
			continue
		}
		sv, found := info.Required.Get(w.Name)
		if !found {
			continue
		}
		sinfo, shape = info, sv
		break
	}
	if sinfo == nil {
		return false, nil
	}
	undef, ok := shape.Data.(FnUndefInfo)
	if !ok || len(undef.Sigs) == 0 {
		return false, nil
	}
	spec := SubstituteSelf(undef.Sigs[0], sinfo.Type)
	e.registry.Check.Recorder().MarkUncompilable("surface-shape typed dispatch at " + w.Name)
	synth := &Signature{Params: spec.Params, Returns: spec.Returns}
	normalizeSig(synth)

	n := synth.TotalArgs()
	positions := e.checkModeFallbackPositions(n)
	args := make([]Value, len(positions))
	for i, p := range positions {
		args[i] = e.tape.At(p)
	}
	results := carrierResults(e.registry, w.Name, synth, args, pos, nil, false)
	e.spliceCheckResults(positions, results)
	return true, nil
}

// tryRecordRecoveredUserFn records a GUARDED CALL_USER for a user-fn dispatch that
// matchSignature could not statically commit (an Any- or disjunct-typed operand),
// when the fn has EXACTLY ONE real (arg-bearing) overload and no 0-arg real sig the
// synthetic fallback would courtesy-dispatch. The sole sig either matches the
// runtime arg (dispatch == interpreter) or misses it (the CALL_USER param contract
// raises == the interpreter's fallback raise). Returns true — and splices the
// recovered returns — when it records; false leaves the caller's refusal to stand
// (multi-overload → Cluster C). The L4 leaf: design/VOXGIG-COMPILE-LEAVES.1.md.
// singleOverloadRecoverable reports whether fn is a user fn with EXACTLY ONE
// real (arg-bearing, non-fallback) overload — the shape whose dispatch over an
// Any/disjunct-carrier arg is RECOVERABLE: runtime dispatch is unambiguous, and
// the VM's CALL_USER param contract raises exactly as the interpreter would if
// the concrete value misses the sole sig. Used both to RECORD a guarded call on
// an armed (compile) pass — tryRecordRecoveredUserFn below — and to SUPPRESS the
// no_signature diagnostic on a plain check pass (Emit inactive), so a plain
// check agrees with a compile pass instead of flagging what compile silently
// recovers (the `engine-known engine` FP: an `is String`-guarded Options-`get`
// value whose type the plain-check pass leaves a strict-Any carrier).
func singleOverloadRecoverable(sig *Signature, fn *FnDefInfo) bool {
	if fn == nil || sig == nil || sig.ReturnsFn == nil || sig.Fallback || sig.TotalArgs() == 0 {
		return false
	}
	realOverloads := 0
	has0ArgReal := false
	var sole *Signature
	for i := range fn.Signatures {
		s := &fn.Signatures[i]
		if s.Fallback {
			continue // synthetic 0-arg catch-all: raises for an arg-bearing fn
		}
		realOverloads++
		sole = s
		if s.TotalArgs() == 0 {
			has0ArgReal = true
		}
	}
	if realOverloads != 1 || has0ArgReal || sole == nil {
		return false
	}
	// AQL-BODIED user fn ONLY (Codex PR #211 review #2). A core native (one-sig
	// with a ReturnsFn, e.g. `enum`) and a trivial-delegation module wrapper
	// (body = [Word(inner)] short-circuiting to an inner native) do NOT get the
	// guarded CALL_USER param contract, so their unmatched dispatch is not
	// deferrable. The discriminator is a real AQL body (len(Body) > 0), NOT a nil
	// Handler: an AQL fn defined in a module preamble carries a synthesized CallAQL
	// Handler yet is genuinely AQL-bodied (the `engine-known` case) — gating on
	// Handler==nil wrongly excludes it. Core natives have an empty Body; delegation
	// wrappers are caught by trivialDelegationTarget.
	if _, isDelegation := trivialDelegationTarget(sole); isDelegation {
		return false
	}
	return len(sole.body()) > 0
}

func (e *Engine) tryRecordRecoveredUserFn(sig *Signature, fn *FnDefInfo, args []Value, nStack int, positions []int) bool {
	if !singleOverloadRecoverable(sig, fn) {
		return false
	}
	recovered := sig.ReturnsFn(sigOrderArgs(args, nStack), e.registry)
	e.spliceCheckResults(positions, recovered)
	return true
}

// concreteArgsMatch reports whether every NON-Any-carrier operand still MATCHES
// its sole-sig param position — so the failed dispatch is attributable SOLELY to
// the Any-typed (statically-unknown) operand(s), not a provably-wrong concrete
// arg. Guards the multi-arg case `g x 123` (g[a:Integer b:String]; x:Any->a ok,
// but 123->b:String wrong): the concrete mismatch stays a genuine error and must
// NOT be suppressed alongside the deferrable Any (Codex PR #211 review #1). Used
// only to gate the plain-check no_signature diagnostic; the armed recovery records
// a guarded CALL_USER that raises == interpreter regardless, so it needs no such check.
func concreteArgsMatch(sig *Signature, args []Value, nStack int) bool {
	window := sigOrderArgs(args, nStack)
	n := sig.TotalArgs()
	if len(window) < n {
		return false // arity shortfall — a genuine mismatch, do not suppress
	}
	for i := 0; i < n; i++ {
		v := window[i]
		if v.Carrier && v.Parent != nil && v.Parent.Equal(TAny) {
			continue // the deferrable unknown operand
		}
		if !sigArgMatches(sig, i, v) {
			return false
		}
	}
	return true
}

func (e *Engine) checkModeAssumeSig(w WordInfo, fn *FnDefInfo, fallback *Signature, pos SrcPos) error {
	// Gather candidate positions once and try to pick a signature
	// whose arity matches and whose declared types are compatible
	// with (or at least not contradicted by) the actual carrier
	// args. TAny carriers are treated as wildcards.
	best := fallback
	bestMatch := -1
	// Scan all signatures and pick the best fit. Scoring:
	//  - compatible concrete-type matches count.
	//  - ties break toward sigs with ReturnsFn (carry custom
	//    check-mode logic) over plain Returns (static list).
	// When nothing is concretely compatible, fall through to
	// scanning by arity alone so we still land on a ReturnsFn-
	// bearing sig when possible rather than a static catch-all.
	bestHasFn := fallback.ReturnsFn != nil
	for i := range fn.Signatures {
		s := &fn.Signatures[i]
		if s.Fallback {
			continue
		}
		n := s.TotalArgs()
		pos := e.checkModeFallbackPositions(n)
		if len(pos) != n {
			continue
		}
		score := 0
		compatible := true
		for j, p := range pos {
			av := e.tape.At(p)
			if av.Parent.Equal(TAny) {
				continue
			}
			if sigArgMatches(s, j, av) {
				score++
				continue
			}
			compatible = false
			break
		}
		if !compatible {
			continue
		}
		hasFn := s.ReturnsFn != nil
		if score > bestMatch || (score == bestMatch && hasFn && !bestHasFn) {
			bestMatch = score
			best = s
			bestHasFn = hasFn
		}
	}
	// Fallback pass: if no compatible sig was found at all, prefer a
	// sig with a ReturnsFn over one without (all else equal), at a
	// SATISFIABLE arity. The fallback (first-ranked candidate) may be
	// a wider overload than this call site can even supply positions
	// for — specificity ranking puts the 3-arg patrun `add` above the
	// 2-arg math adds — and a fallback that merely CARRIES a ReturnsFn
	// must not win on that alone: assuming the wider sig swallows an
	// unrelated operand into the recovery window (corrupting the
	// disjunct rescue for `add` over two disjuncts) or feeds its
	// ReturnsFn a short args slice (an index-out-of-range panic
	// class). So when the current best is Fn-less OR arity-
	// unsatisfiable, move to the first ReturnsFn-bearing sig whose
	// full window EXISTS; if none does, the fallback stands (its
	// ReturnsFn sees the short window — ReturnsFns are len-guarded).
	if bestMatch < 0 {
		fbn := best.TotalArgs()
		bestSat := len(e.checkModeFallbackPositions(fbn)) == fbn
		if !bestHasFn || !bestSat {
			for i := range fn.Signatures {
				s := &fn.Signatures[i]
				if s.Fallback || s.ReturnsFn == nil {
					continue
				}
				n := s.TotalArgs()
				if len(e.checkModeFallbackPositions(n)) != n {
					continue
				}
				best = s
				break
			}
		}
	}
	sig := best
	n := sig.TotalArgs()
	positions := e.checkModeFallbackPositions(n)
	// nStack is how many of the gathered positions are STACK args (before
	// the pointer, ascending); the remainder are FORWARD args (after the
	// pointer, source order). checkModeFallbackPositions lays them out in
	// that tape order — stack-before then forward-after — which is NOT
	// signature order. Recorded below for the poly-recovery operand rebuild.
	nStack := len(e.resolvedIndicesBefore(n))
	if nStack > len(positions) {
		nStack = len(positions)
	}
	args := make([]Value, len(positions))
	for i, p := range positions {
		av := e.tape.At(p)
		// Resolve simple word references to their def bindings — the
		// tape still holds raw Words for forward operands at this
		// recovery point, and both the partition probe below and the
		// assumed sig's ReturnsFn want values, not names.
		if IsWord(av) {
			if wi, werr := AsWord(av); werr == nil {
				if top, ok := e.registry.Defs.Top(wi.Name); ok {
					av = top
					e.registry.Check.recordUse(wi.Name)
				}
			}
		}
		// Auto-evaluate a raw eval-map operand exactly as the runtime match's
		// execMatch would (word members resolve against the live frame, the
		// recorder assembles a per-run OpMakeMap): the recovery otherwise
		// hands the RAW source map to the poly record, which either baked a
		// live word member as a frozen const (the repl-eval `{line: src}`
		// request map) or refuses. Errors leave the raw operand — the
		// assumed-sig model stays as before.
		if IsConcrete(av) && av.Parent != nil && av.Parent.ConformsTo(TMap) && bearsActiveTokens(av) {
			if ev, everr := e.autoEvalMap(av, false, true); everr == nil {
				e.tape.Set(p, ev)
				av = ev
			}
		}
		args[i] = av
	}
	// Strict disjunct rescue (design/checker-accuracy-review.10.md A1):
	// the whole disjunct matched no signature, but individual
	// alternatives may dispatch fine. If at least one does, splice the
	// per-alternative join — the failing alternatives have already
	// been flagged with partial_dispatch warnings, which name the
	// exact path that would fail; the blanket no_signature error
	// would be wrong for the paths that DO dispatch.
	if out, ok := disjunctPartitionReturns(e.registry, w.Name, args, pos); ok {
		// A strict-disjunct straddle is a runtime-dispatch case, not an
		// inherent refusal: when the word is a safe poly candidate (core
		// builtin, single result, no meta/fn-value/code-body sig), record
		// OpCallNativePoly so the VM re-matches the one concrete alternative
		// at run time — e.g. `(3 and "x") add 1` → `'x1'`, mirroring the
		// normal-path handling in carrierResults. The poly call needs its
		// operands in SIGNATURE order (sig[0] = top of stack): forward args
		// fill the leading positions in source order, then the stack args
		// fill the rest top-down (the deepest-last ascending run reversed).
		// Feeding the raw tape order here was the prior `[1x]`-vs-`[x1]`
		// operand-order divergence. Only refuse when poly isn't safe.
		if tryRecordPoly(e.registry, w.Name, sig, sigOrderArgs(args, nStack), out, pos, true, nil, false) {
			e.spliceCheckResults(positions, out)
			return nil
		}
		// A single-overload user fn over a disjunct-typed operand recovers here
		// (e.g. the aql:test framework's run-cases inside test-describe's body);
		// record a guarded CALL_USER instead of refusing (it splices its own returns).
		if e.tryRecordRecoveredUserFn(sig, fn, args, nStack, positions) {
			return nil
		}
		e.registry.Check.Recorder().MarkUncompilable("unmatched dispatch recovered at " + w.Name)
		e.spliceCheckResults(positions, out)
		return nil
	}
	// No disjunct partition. When an operand is an Any-typed carrier — a value
	// of statically-unknown type, e.g. `get` over a List/Map element
	// (`(cases _i get) get "in"`) — matchSignature could not commit to an
	// overload, but at run time the value is concrete and the SAME first-match
	// the interpreter takes dispatches it. For a SAFE pure core builtin, record
	// a runtime-re-matching OpCallNativePoly instead of refusing: tryRecordPoly's
	// gates keep meta / fn-value / mutating (set) / code-body / multi-result
	// words out, so only words whose runtime re-match is faithful poly. Results
	// are computed with recording suspended so the program records ONLY the poly
	// call, never a duplicate CALL_NATIVE for the same dispatch. Concrete (non-
	// Any) operands that reach here are a genuine type error and still refuse.
	es := e.registry.Check.Recorder()
	if es.active() && (anyAnyCarrier(args) || anyDisjunctCarrier(args)) {
		resume := es.Suspend()
		results := carrierResults(e.registry, w.Name, sig, args, pos, nil, false)
		resume()
		// Re-match over the dispatching registry: a module sub-registry word
		// (the test framework's `test-record`, run via CallAQL in the module's
		// sub-registry) must re-match over THAT registry at run time, not the
		// main one, or callPoly's Lookup misses it. e.registry is the same
		// sub-registry pointer the compiled run reaches (it lives on the module
		// export). For a core builtin e.registry is the main registry, so
		// PolyRef.Reg then equals the VM's own registry — the no-op the
		// get/add path already relied on.
		if tryRecordPoly(e.registry, w.Name, sig, sigOrderArgs(args, nStack), results, pos, false, e.registry, true) {
			e.spliceCheckResults(positions, results)
			return nil
		}
		// A SINGLE-overload user/module fn recovered over an Any-typed operand:
		// matchSignature could not statically commit (the operand's type is unknown),
		// but with exactly ONE overload the runtime dispatch is unambiguous — the
		// concrete value either matches the sole sig (dispatch == interpreter) or
		// fails the VM's CALL_USER param contract (raise == the interpreter's
		// no_signature). tryRecordPoly can't take it (user fns have an FnFrame; only
		// sub-registry builtins pass), so drive its ReturnsFn NON-suspended to compile
		// the body unit and record a GUARDED CALL_USER — buildFnBodyReturnsFn's
		// SetUnitParamTypes installs the param contract the VM enforces at entry, so a
		// runtime arg that misses the sole sig raises exactly as the interpreter does.
		// This is what unblocks the aql:test framework (run-cases) and the trie/
		// decision walkers (find-kid / mk-tnode / lex-mustache). A MULTI-overload fn
		// stays refused below (Cluster C): one baked overload would raise where the
		// interpreter runtime-dispatches a sibling.
		if e.tryRecordRecoveredUserFn(sig, fn, args, nStack, positions) {
			return nil
		}
		// On a REAL compile pass (Compiling) the MarkUncompilable already refuses
		// and Finalize surfaces THIS reason, so an error-severity no_signature
		// diagnostic here would only mask it as the generic "check diagnostics"
		// (aql.go:297). On a plain check pass this branch is still reachable —
		// IsolateEmit arms a fresh ACTIVE Emit while analysing each fn body — and
		// there the diagnostic IS the genuine static report, so gate it on
		// !Compiling, matching the fall-through path below.
		es.MarkUncompilable("unmatched dispatch recovered at " + w.Name)
		if !e.registry.Check.Compiling && bestMatch < 0 {
			e.registry.Check.AddDiagnostic(CheckDiagnostic{
				Code:   "no_signature",
				Detail: "no matching signature for " + w.Name + "; assuming best-fit candidate for analysis",
				Word:   w.Name,
				Row:    pos.Row,
				Col:    pos.Col,
			})
		}
		e.spliceCheckResults(positions, results)
		return nil
	}
	// A no-signature dispatch reached here UNDER A SUSPENDED outer recovery
	// (es.suspended > 0) is being ANALYSED to read an enclosing dispatch's result
	// type — carrierResults suspends recording and re-runs the body purely to
	// inspect its residual — NOT compiled. Its real compile decision happens on
	// the non-suspended recording pass (or it is subsumed by the enclosing poly's
	// runtime re-match). MarkUncompilable here PREMATURELY latches the whole
	// program refusal: the trie find-kid `(nd "kids" get) get (ch)` shape refuses
	// because the inner get's result-type probe analyses the outer get against a
	// transient String-carrier alternative. Skip the latch (and its diagnostic)
	// under suspend; still splice the analysis result so the enclosing probe
	// reads a residual.
	if !es.suspendedNow() {
		// A STATICALLY-DEFINITE unmatched dispatch — every value the failed
		// match examined is identical at run time — compiles to a terminal
		// OpTrap raising the interpreter's byte-identical error instead of
		// refusing the whole program (the error-row doctrine: a spec ERROR row
		// yields a Program that raises the same taxonomy at the same point).
		// Ineligible shapes (a carrier operand whose runtime tag could match, a
		// nested frame/unit, a plain check pass) keep the blanket refusal.
		if !e.tryRecordUnmatchedDispatchTrap(w, fn, pos) {
			// The trap DECLINED: the mismatch is not statically definite — a
			// carrier operand's runtime tag could still match. An IMPRECISE
			// carrier (a scalar tag a multi-branch narrowing settled on, the
			// mini-redis `join " " reply` where reply IS a list at run time) is a
			// checker stand-in, not a concrete value, so recover via the runtime-
			// re-matching poly instead of refusing — the DEFINITE mismatches (a
			// disjoint Box<String> vs Box<Integer> param) already trapped above,
			// so only genuinely could-match carriers reach here. tryRecordPoly /
			// the single-overload user-fn recovery decline (leaving es untouched)
			// for anything their own gates reject, and the refusal below stands.
			// ONLY the native-poly recovery (OpCallNativePoly), never the
			// single-overload USER-fn recovery: a user fn's guarded CALL_USER
			// enforces the param's NOMINAL type at entry, not a value-sensitive
			// PREDICATE / refinement (`def Big (Integer gt 10) def g […n:Big…]`),
			// so a nominally-typed but predicate-failing arg would run the body
			// compiled where the interpreter raises. tryRecordPoly re-runs the
			// native's own matchSignature over the concrete runtime value (the
			// redis `join` re-match), which is faithful; a user fn stays refused
			// and falls back.
			recovered := false
			if es.active() && anyImpreciseCarrier(args) {
				resume := es.Suspend()
				results := carrierResults(e.registry, w.Name, sig, args, pos, nil, false)
				resume()
				if tryRecordPoly(e.registry, w.Name, sig, sigOrderArgs(args, nStack), results, pos, false, e.registry, true) {
					e.spliceCheckResults(positions, results)
					recovered = true
				}
			}
			if recovered {
				return nil
			}
			e.registry.Check.Recorder().MarkUncompilable("unmatched dispatch recovered at " + w.Name)
		}
	}
	// Emit the error-severity no_signature diagnostic ONLY off a REAL compile pass
	// (!Compiling), where it is the genuine static report of an unmatched dispatch.
	// This gate is INDEPENDENT of the suspend skip above: a plain check reports a
	// genuine unmatched dispatch even when it is reached under a suspended
	// sub-probe (the over-suppression that dropping it inside the suspend branch
	// caused), while a compile pass never adds it (Finalize surfaces the
	// MarkUncompilable reason; a diagnostic would only mask it as the generic
	// "check diagnostics", aql.go:297). Do NOT additionally gate on
	// `bestMatch >= 0`: this fall-through is reached only when matchSignature
	// already FAILED to commit, so a positive best-fit score here is a
	// best-effort guess (a bare type-literal or wildcard operand that
	// sigArgMatches accepts but the runtime rejects — `class Map`, `get 'a'
	// Map`, `add/3 2 3`, `fold [add] {}` …). Suppressing the diagnostic when a
	// guess exists is UNSOUND: it dropped 16 genuine error rows the interpreter
	// raises on (TestCheckAccuracyRatchet coverage 208→192). A positive best-fit
	// here does NOT imply the dispatch matches at runtime — matchSignature
	// already failed; the recovery's `bestMatch` is a best-effort score that can
	// "fit" a bare type literal (`class Map`), a wrong-typed value, or a
	// dynamic operand whose runtime value still misses the sole sig (the
	// recursive `f` whose `next` holds a List where the sig wants an Integer —
	// which `signature_error`s at run time, NOT a false positive). The
	// `!Compiling` guard alone is the correct condition.
	// EXCEPTION to the "!Compiling alone" rule: a SINGLE-overload user fn
	// dispatched over an Any/disjunct-CARRIER arg (a value of statically-unknown
	// type, not a concrete mismatch) is NOT a genuine unmatched dispatch — it is
	// the exact shape the armed (compile) pass RECOVERS as a guarded CALL_USER
	// above (singleOverloadRecoverable), where the VM's param contract raises ==
	// the interpreter iff the runtime value misses the sole sig. A plain check
	// pass reaches HERE (Emit inactive, so the recovery block was skipped) and
	// would otherwise flag what compile silently recovers — the `engine-known
	// engine` false positive (`is String`-guarded Options-`get`). Suppressing is
	// SOUND and narrower than the rejected bestMatch>=0 gate: it fires only on an
	// unknown-TYPE carrier (never a concrete/type-literal operand) AND a one-
	// overload user fn, so it cannot drop the concrete-mismatch error rows.
	// anyAnyCarrier ONLY, NOT anyDisjunctCarrier: a DISJUNCT's members are
	// statically known, so a disjunct that misses the param in EVERY member
	// (Integer|String -> Map, String|List -> Integer) is a GENUINE error the
	// interpreter always raises on and must stay flagged (TestJoinCarriersDynamicArm
	// / TestSliceDynamicReceiverRefines). Only the fully-unknown Any carrier is
	// deferrable to the runtime CALL_USER contract.
	recoverableUnknownType := anyAnyCarrier(args) && singleOverloadRecoverable(sig, fn) && concreteArgsMatch(sig, args, nStack)
	if !e.registry.Check.Compiling && !recoverableUnknownType {
		// Expected-vs-actual: name the operand types the dispatch saw and
		// the nearest candidate's declared types, so the user can see the
		// mismatch without reconstructing the stack ("got (Map, Integer);
		// nearest [Number Number]").
		detail := "no matching signature for " + w.Name
		if got := argTypeSummary(args); got != "" {
			detail += " — got (" + got + ")"
			if near := sigTypeSummary(sig); near != "" {
				detail += "; nearest [" + near + "]"
			}
		}
		e.registry.Check.AddDiagnostic(CheckDiagnostic{
			Code:   "no_signature",
			Detail: detail + "; assuming best-fit candidate for analysis",
			Word:   w.Name,
			Row:    pos.Row,
			Col:    pos.Col,
		})
	}
	// The assumed dispatch runs its ReturnsFn against args the REAL
	// match already rejected — a user fn's body analysis under those args
	// produces CASCADE noise (an unbound param surfacing as a spurious
	// `undefined_word: x` from inside the body, a dependent no_signature on
	// a body word). The one honest diagnostic is the no_signature above;
	// suppress the error-level body diagnostics of the consequent analysis
	// (the SuppressBodyErrors discipline recursive re-entry already uses).
	e.registry.Check.SuppressBodyErrors++
	results := carrierResults(e.registry, w.Name, sig, args, pos, nil, false)
	e.registry.Check.SuppressBodyErrors--
	e.spliceCheckResults(positions, results)
	return nil
}

// tryRecordUnmatchedDispatchTrap compiles a STATICALLY-DEFINITE unmatched
// dispatch to a terminal OpTrap raising the interpreter's byte-identical
// error, so a spec ERROR row still yields a Program instead of falling back.
// Reached from checkModeAssumeSig's no-carrier fall-through, i.e. after
// matchSignature failed every candidate AND the disjunct-partition / Any-
// carrier poly recoveries declined.
//
// Definiteness is the soundness condition: the run-time dispatch must fail at
// this same point with the same error on EVERY run. That holds exactly when
// every value any candidate signature could examine is identical at run time —
// a concrete const, a bare type literal, or a raw word token (matchSignature
// is deterministic over identical inputs, and the check pass mirrors the
// interpreter's tape step-for-step over concrete values). It does NOT hold
// for:
//   - a CARRIER operand — UNLESS the carrier-disjointness extension proves
//     the failure anyway (Phase 6 M4, design/STAGE3-INLINING-DESIGN-ROUND.0.md
//     §6 Stage M4). The base hazard: a carrier's static tag is a declared
//     type, but the runtime value may carry a refined subtype tag (a fn
//     declared [Integer] can return a Pos-reparented value) or satisfy a
//     value-sensitive predicate param — the runtime match can succeed where
//     the static one failed. The extension re-admits exactly the shapes where
//     that cannot happen: for EVERY non-fallback overload, every carrier in
//     the overload's own candidate window is provably disjoint from EVERY
//     slot type of that overload (sigDefinitelyUnmatched) — the runtime
//     value's tag conforms to the carrier's static tag, so a Never meet with
//     no value-level membership admits no runtime refinement either (the
//     same proof checkBodyReturnConformance rides, residualProvablyDisjoint).
//     An overload whose window holds no carrier saw only runtime-identical
//     values, so its static failure replays verbatim; one whose window
//     cannot even fill (arity shortfall over the same tape) fails
//     deterministically too;
//   - a DYNAMIC operand (no static type at all);
//   - an UNDEFINED-word placeholder (the interpreter raises undefined_word,
//     a different taxonomy — and the pass already carries an error diagnostic
//     that refuses the program before Finalize);
//   - a 0-arg real (non-fallback) signature on the word: matchSignature's
//     fallback scan and the synthetic-fallback courtesy dispatch can run it
//     at run time instead of raising.
//
// The trap error mirrors the interpreter's raise at this point exactly: the
// void-argument-group override (def_error / no_value_error — voidArgErrorFor,
// whose void-group records the check pass populates identically) when a paren
// arg produced no value, else the plain "no matching signature" signature
// mismatch. Hints ride along verbatim where the interpreter's are static;
// sigError's stack-snapshot hint is advisory DX text, not part of the error
// taxonomy (code + detail + position) the differential gates.
//
// RecordTrap's own guard keeps this top-level-only (frames and units both at
// depth 1): a trap inside a branch arm or fn unit is conditional and stays a
// refusal. Returns true when the trap now owns the program's tail; false
// leaves the caller's MarkUncompilable refusal to stand.
func (e *Engine) tryRecordUnmatchedDispatchTrap(w WordInfo, fn *FnDefInfo, pos SrcPos) bool {
	es := e.registry.Check.Recorder()
	if !es.active() || !e.registry.Check.Compiling {
		return false
	}
	maxN := 0
	for i := range fn.Signatures {
		s := &fn.Signatures[i]
		if s.Fallback {
			continue
		}
		n := s.TotalArgs()
		if n == 0 {
			return false // courtesy-dispatchable at run time — not a definite failure
		}
		if n > maxN {
			maxN = n
		}
	}
	window := e.checkModeFallbackPositions(maxN)
	// The forward walk can collect positions INSIDE a not-yet-evaluated paren
	// group (checkModeFallbackPositions depth-tracks rather than stopping at
	// an open paren). The interpreter pre-evaluates the paren before its
	// match runs, so in-paren tokens are NOT the values the runtime match
	// examines — never definite. (`add 100 (for 4 [add i 1])`: the static
	// window holds the raw `for` word where the runtime sees the loop's
	// value; latent under the pre-M4 screens only because such programs
	// still refused at Finalize's residual seating, which the trap
	// truncation now legitimately skips.)
	for i := e.pointer + 1; i < e.tape.Len(); i++ {
		inWindow := false
		for _, p := range window {
			if p >= i {
				inWindow = true
				break
			}
		}
		if !inWindow {
			break
		}
		if IsOpenParen(e.tape.At(i)) {
			return false
		}
	}
	carriers := false
	for _, p := range window {
		v := e.tape.At(p)
		if IsWord(v) {
			if wi, werr := AsWord(v); werr == nil {
				if top, ok := e.registry.Defs.Top(wi.Name); ok {
					v = top // the binding is what matchSignature examined
				}
			}
		}
		if v.Dynamic || v.Undefined {
			return false
		}
		// A deferred-expression token (a reach path, an unexpanded paren
		// expr, a template string, a word splice) EXPANDS at dispatch/step
		// time, and its expansion can read state the check pass models only
		// abstractly — a reach over a mutated flex cell resolved at run time
		// where the static match saw the raw Reach token (flex.tsv L88/L95).
		// Its presence makes the failure non-definite; decline.
		if IsReach(v) || IsParenExpr(v) || IsInterpString(v) || IsSplice(v) {
			return false
		}
		if v.Carrier {
			carriers = true // judged per-overload below (Phase 6 M4)
		}
	}
	if carriers {
		// Carrier-disjointness extension (Phase 6 M4): a strict carrier in
		// the window is admissible only when EVERY non-fallback overload
		// still fails DEFINITELY — see sigDefinitelyUnmatched for the
		// per-overload proof obligations.
		for i := range fn.Signatures {
			s := &fn.Signatures[i]
			if s.Fallback {
				continue
			}
			if !e.sigDefinitelyUnmatched(s, window) {
				return false
			}
		}
	}
	if verr := e.voidArgErrorFor(w.Name, pos); verr != nil {
		return es.RecordTrap(verr.Code, verr.Detail, w.Name, verr.Hint, pos)
	}
	return es.RecordTrap("signature_error", "no matching signature for "+w.Name, w.Name, "", pos)
}

// sigDefinitelyUnmatched reports whether overload s of an already
// statically-failed dispatch PROVABLY fails at run time too (the Phase 6 M4
// carrier-disjointness extension; reached only from
// tryRecordUnmatchedDispatchTrap after the shared window screens passed —
// no dynamics, no deferred-expression tokens, no undefined placeholders
// anywhere a candidate could look, and window(k) ⊆ window(maxN) for every
// overload arity k, so the shared screens cover every value any overload can
// examine). The proof is per-overload,
// the assignment-FEASIBILITY argument: the runtime match, whatever
// forward/stack split it takes (the split may shift when a carrier's runtime
// value changes which type tests pass, so no single pairing may be assumed),
// fills s's n slots with n DISTINCT values drawn from the shared window —
// the window is the complete candidate pool (the stack side reaches at most
// arity-many nearest values and the forward side stops at boundaries the
// window walk also honours, both ⊆ window(maxN)). Model "slot j could accept
// candidate i" as a bipartite edge (the NEGATION of definiteSlotFail, which
// answers true only for PROVABLY-failing pairs); if no perfect matching over
// the slots exists, s can never complete at run time. This subsumes the
// arity shortfall (fewer candidates than slots), the zero-edge slot (every
// candidate definitely fails some slot), and the counting cases (two Point
// slots but only one Point-compatible candidate — open-words:100).
//
// Per-pair definiteness: a concrete value / resolved binding / raw token
// fails j deterministically when the matcher's own per-value test rejects it
// (definiteSlotFail mirrors the scan's branch chain over runtime-identical
// inputs); a strict carrier fails j when it is provably disjoint from j's
// slot type — the runtime value's tag conforms to the carrier's static tag,
// so a Never meet with no value-level membership admits no runtime
// refinement either (the residualProvablyDisjoint proof, extended for the
// matcher's structural carves in carrierSlotProvablyDisjoint).
func (e *Engine) sigDefinitelyUnmatched(s *Signature, window []int) bool {
	n := s.TotalArgs()
	// Edge matrix: canTake[j][i] — slot j could accept window candidate i.
	canTake := make([][]bool, n)
	for j := 0; j < n; j++ {
		canTake[j] = make([]bool, len(window))
		for i, p := range window {
			canTake[j][i] = !e.definiteSlotFail(s, j, e.tape.At(p))
		}
	}
	// Augmenting-path maximum bipartite matching, slots → candidates (n and
	// the window are dispatch-arity-sized, so the quadratic walk is trivial).
	matchedSlot := make([]int, len(window))
	for i := range matchedSlot {
		matchedSlot[i] = -1
	}
	var assign func(j int, seen []bool) bool
	assign = func(j int, seen []bool) bool {
		for i := range window {
			if !canTake[j][i] || seen[i] {
				continue
			}
			seen[i] = true
			if matchedSlot[i] == -1 || assign(matchedSlot[i], seen) {
				matchedSlot[i] = j
				return true
			}
		}
		return false
	}
	for j := 0; j < n; j++ {
		if !assign(j, make([]bool, len(window))) {
			return true // slot j cannot be filled by any candidate — definite failure
		}
	}
	return false
}

// definiteSlotFail reports whether candidate value v PROVABLY fails slot j of
// signature s on every run. Mirrors matchSignature's per-value admission
// branches over runtime-identical inputs and uses the carrier-disjointness
// proof for strict carriers; anything it cannot mirror exactly answers false
// (the sound direction — the trap then declines).
func (e *Engine) definiteSlotFail(s *Signature, j int, v Value) bool {
	t := sigArgType(s, j)
	if t == nil || t.Equal(TAny) {
		return false // an Any slot admits everything
	}
	// Slot kinds whose admission is not the plain per-value type test.
	if (s.QuoteArgs != nil && s.QuoteArgs[j]) || (s.FormArgs != nil && s.FormArgs[j]) {
		return false
	}
	typeSlot := s.TypeArgs != nil && s.TypeArgs[j]
	if IsWord(v) {
		wi, werr := AsWord(v)
		if werr != nil {
			return false
		}
		// The scan's word-resolution chain, in its order; every branch is
		// deterministic over the same registry state at this dispatch point.
		if top, ok := e.registry.Defs.Top(wi.Name); ok {
			return e.definiteSlotFail(s, j, top)
		}
		if tv, ok := e.registry.TopTypeBody(wi.Name); ok {
			return !sigArgMatches(s, j, tv)
		}
		if e.registry.Lookup(wi.Name) != nil {
			return true // a function word is a scan BOUNDARY — it never fills a slot
		}
		if wi.Name == "true" || wi.Name == "false" {
			return !sigArgMatches(s, j, Value{Parent: TBoolean})
		}
		if tn, isType := typeNames[wi.Name]; isType {
			return !sigArgMatches(s, j, NewTypeLiteral(tn))
		}
		if tn, isType := ResolveTypePath(wi.Name); isType {
			return !sigArgMatches(s, j, NewTypeLiteral(tn))
		}
		// Undefined word: the scan resolves it to an Atom.
		return !sigArgMatches(s, j, Value{Parent: TAtom})
	}
	if v.Carrier {
		if v.Dynamic || typeSlot {
			// Dynamics are screened upstream (belt here); a TypeArgs slot's
			// admission (sigTypeMatchesAsType over a possible runtime type
			// node) is not a tag test — decline.
			return false
		}
		// A carrier whose runtime value could itself be a raw Word re-enters
		// the word-resolution branches above; require Word-disjointness
		// alongside the slot proof.
		return carrierSlotProvablyDisjoint(v, t) && carrierSlotProvablyDisjoint(v, TWord)
	}
	// A runtime-identical value (concrete const, bare type literal): the
	// matcher's own per-position test, replayed.
	return !sigArgMatches(s, j, v)
}

// carrierSlotProvablyDisjoint reports whether a strict carrier can NEVER
// satisfy a signature slot of type t at run time: no runtime value whose tag
// conforms to the carrier's static tag passes sigTypeMatches against t. The
// core is residualProvablyDisjoint's proof (no conformance either direction,
// no value-level membership, Never tand meet), extended with declines for
// sigTypeMatches' structural carves — an Options slot admits any concrete
// map, and a Map/Node-family slot admits Options/Record-tagged values — so
// the disjointness claim matches the runtime matcher, not just the nominal
// lattice. A disjunct carrier is disjoint only if every alternative is.
func carrierSlotProvablyDisjoint(v Value, t *Type) bool {
	if t == nil || !v.Carrier || v.Dynamic {
		return false
	}
	if IsDisjunct(v) {
		di, err := AsDisjunct(v)
		if err != nil || len(di.Alternatives) == 0 {
			return false // opaque union — nothing provable
		}
		for _, alt := range di.Alternatives {
			probe := alt
			if IsBareTypeNode(alt) {
				probe = carrierOfLiteral(alt)
			}
			probe.Carrier = true
			if !carrierSlotProvablyDisjoint(probe, t) {
				return false
			}
		}
		return true
	}
	p := v.Parent
	if p == nil || p.ConformsTo(t) || t.ConformsTo(p) {
		return false
	}
	// A Function/FnDef-tagged carrier sits on the fn-value modelling
	// frontier (routinely "not modeled", per residualProvablyDisjoint) and
	// an opaque Disjunct tag denotes "one of several types" — decline both.
	if p.ConformsTo(TDisjunct) || p.ConformsTo(TFunction) || p.ConformsTo(TFnDef) {
		return false
	}
	// A container-family carrier can stand for a SPLICED runtime sequence,
	// not a single value: a check-mode `for` records its value-body residue
	// as ONE typed-list carrier where the runtime leaves the loose
	// per-iteration values (`add 100 (for 4 [add i 1])` — the
	// combination-parity counterexample: the Integers match Number slots the
	// list "tag" is disjoint from). The tag-conformance premise does not
	// hold for such stand-ins, so nothing is provable — decline.
	if p.ConformsTo(TList) || p.ConformsTo(TMap) {
		return false
	}
	// Value-level membership (predicate / disjunct / negation / member /
	// DepScalar slots) admits by value, not tag — nothing nominal is provable.
	if membershipBeyondNominal(t) || NewCarrier(p).Is(t) {
		return false
	}
	// sigTypeMatches' structural carves: an Options slot accepts any concrete
	// map value; a slot at-or-above Map accepts Options/Record-tagged values.
	if t.Equal(TOptions) && !isNeverShape(TandValues(NewCarrier(p), NewCarrier(TMap))) {
		return false
	}
	if TMap.ConformsTo(t) && (p.ConformsTo(TOptions) || p.ConformsTo(TRecord)) {
		return false
	}
	return isNeverShape(TandValues(NewCarrier(p), NewCarrier(t)))
}

// argTypeSummary renders the operand types of a failed dispatch for the
// no_signature expected-vs-actual message: comma-separated Parent names,
// dynamic carriers marked. Empty for a 0-arg dispatch.
func argTypeSummary(args []Value) string {
	if len(args) == 0 {
		return ""
	}
	parts := make([]string, len(args))
	for i, a := range args {
		switch {
		case a.Parent == nil:
			parts[i] = "?"
		case a.Dynamic:
			parts[i] = "dynamic(" + a.Parent.Leaf() + ")"
		default:
			parts[i] = a.Parent.Leaf()
		}
	}
	return strings.Join(parts, ", ")
}

// sigTypeSummary renders the best-fit candidate's declared arg types for the
// no_signature message. Empty for a 0-arg sig.
func sigTypeSummary(sig *Signature) string {
	if sig == nil || len(sig.Args) == 0 {
		return ""
	}
	parts := make([]string, len(sig.Args))
	for i, t := range sig.Args {
		if t == nil {
			parts[i] = "?"
			continue
		}
		parts[i] = t.Leaf()
	}
	return strings.Join(parts, " ")
}

// spliceCheckResults removes the word at the pointer plus the consumed
// candidate positions and splices the synthesised carrier results in
// at the word's slot — the shared tail of the check-mode recovery
// paths (assume-sig and the S2 surface-shape typing).
func (e *Engine) spliceCheckResults(positions []int, results []Value) {
	indices := append([]int{e.pointer}, positions...)
	// Insertion sort (small n).
	for i := 1; i < len(indices); i++ {
		for j := i; j > 0 && indices[j] < indices[j-1]; j-- {
			indices[j], indices[j-1] = indices[j-1], indices[j]
		}
	}
	// Deduplicate (defensive).
	uniq := indices[:0]
	prev := -1
	for _, idx := range indices {
		if idx != prev {
			uniq = append(uniq, idx)
			prev = idx
		}
	}
	// Remove from highest to lowest to avoid shifting.
	insertAt := e.pointer
	for i := len(uniq) - 1; i >= 0; i-- {
		if uniq[i] < insertAt {
			insertAt--
		}
		e.tape.Remove(uniq[i])
	}
	e.tape.Splice(insertAt, 0, results...)
	e.pointer = insertAt
}
