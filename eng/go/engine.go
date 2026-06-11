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
// Execution model: `stack` is a rewriting tape, not a LIFO stack. The
// input program is loaded onto it whole; `pointer` then walks forward
// and the value under the pointer dispatches (word → run, forward →
// advance, open-paren → group, ...). A dispatched word may splice
// values into the tape in place — consuming neighbours, inserting
// results. Execution ends when the pointer walks off the end; the
// residual tape is the result.
type Engine struct {
	stack     []Value
	pointer   int
	registry  *Registry
	trace     TraceCallback
	traceNote string          // annotation set during execution for the next trace call
	recorder  Recorder        // optional StackForm recorder; see stackform package
	stepLimit int             // hard cap on the Run loop; always positive, set by the New/NewTop constructors below
	marks     map[string]bool // active mark IDs (for mark/move control flow)
	source    string          // original source text for error reporting
	isTop     bool            // true for engines created via NewTop; an unhandled FlowCtrl at end-of-Run is an error here, propagates upward otherwise
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

// Default step limits for the Run loop. Exposed as named constants so
// every Engine constructor names them explicitly — there is no
// "zero means default" sentinel on `stepLimit`; the field is always
// set to a positive value by the constructors below.
const (
	DefaultStepLimit    = 22222 // top-level engine cap
	DefaultSubStepLimit = 2222  // sub-engine cap (autoEvalMap, CallAQL, etc.)
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

	// Forward-precedence hint: when the word has forward-collecting
	// signatures, the most common cause of this error is that forward
	// collection ran into a following word (another call, a builtin)
	// before it could gather enough arguments — e.g. `inc inc 5` or
	// `f a g b`. Point the user at the fixes (group with parens, or
	// terminate collection with `end` / `;`) so they aren't left to
	// guess from a bare "no matching signature".
	if fn != nil && fn.HasForwardSigs() {
		if hint.Len() > 0 {
			hint.WriteString("\n  = ")
		}
		hint.WriteString("forward args for " + name +
			" may have run into the next word; group the call with parens " +
			"— (" + name + " …) — or end it with `end` or `;`")
	}

	if len(e.stack) > 0 {
		if hint.Len() > 0 {
			hint.WriteString("\n  = ")
		}
		hint.WriteString("stack: " + describeStackTypes(e.stack, e.pointer))
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
		if IsOpenParen(e.stack[i]) {
			return false
		}
		if !IsForward(e.stack[i]) {
			continue
		}
		fwd, _ := AsForward(e.stack[i])
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
		if mapIdx < 0 || mapIdx >= len(e.stack) {
			return false
		}
		m, _ := AsMap(e.stack[mapIdx])
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
		if IsOpenParen(e.stack[i]) {
			return ""
		}
		if IsForward(e.stack[i]) {
			fwd, _ := AsForward(e.stack[i])
			return fwd.FuncName
		}
	}
	return ""
}

// insufficientArgsError builds a detailed AqlError for forward argument
// collection failure (not enough arguments after the word).
func (e *Engine) insufficientArgsError(name string, expected int, pos SrcPos) *AqlError {
	detail := fmt.Sprintf("insufficient arguments for %s (expected %d forward args)", name, expected)
	hint := "stack: " + describeStackTypes(e.stack, e.pointer)
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
			if IsOpenParen(e.stack[i]) {
				break
			}
			if IsAtom(e.stack[i]) {
				n, _ = AsAtom(e.stack[i])
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
	if !ok || ae.Row != 0 {
		return err
	}
	pos := e.currentPos()
	if pos.Row != 0 {
		ae.Row, ae.Col = pos.Row, pos.Col
		if pos.Src != "" {
			ae.Src = pos.Src
		}
	}
	return err
}

// returnCountError builds a detailed AqlError for wrong number of return values.
func (e *Engine) returnCountError(funcName string, expected, got int, pos SrcPos) *AqlError {
	detail := fmt.Sprintf("%s: expected %d return value(s), got %d", funcName, expected, got)
	src := e.effectiveSource()
	return makeAqlErrorAt("type_error", detail, funcName, src, "", pos)
}

// returnTypeError builds a detailed AqlError for a return type mismatch.
func (e *Engine) returnTypeError(funcName string, index int, expected *Type, got Value, pos SrcPos) *AqlError {
	detail := fmt.Sprintf("%s: return value %d: expected %s, got %s",
		funcName, index, expected, got.Parent)
	hint := "value: " + diagValue(got)
	src := e.effectiveSource()
	return makeAqlErrorAt("type_error", detail, funcName, src, hint, pos)
}

// currentPos returns the source position of the value at the pointer — the
// token currently being processed — or the unknown SrcPos when the pointer
// is out of range. Engine-side error builders use it so an error is located
// at the token that triggered it.
func (e *Engine) currentPos() SrcPos {
	if e.pointer >= 0 && e.pointer < len(e.stack) {
		return e.stack[e.pointer].Pos
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

	// Push a scoped context Store whose prototype is the parent context.
	parent := e.registry.Contexts.Top()
	e.registry.Contexts.Push(parent)
	defer e.registry.Contexts.Pop()

	// In static type-check mode, convert concrete literal values to
	// carriers before execution. The same dispatch/matching machinery
	// then runs over carrier values; execMatch short-circuits handler
	// calls to push carrier return values declared on the signature.
	if e.registry.Check.IsActive() {
		input = StripToCarriers(input)
	}

	// Load the program onto the tape. Reuse the existing backing array
	// when it already fits; otherwise allocate len(input)+stackHeadroom
	// so the first few in-place splices don't have to grow the slice.
	// `copy` (not alias) — later mutations don't touch the caller's input.
	if cap(e.stack) >= len(input) {
		e.stack = e.stack[:len(input)]
	} else {
		e.stack = make([]Value, len(input), len(input)+stackHeadroom)
	}
	copy(e.stack, input)
	e.pointer = 0

	// Post-parse referent resolution: stamp each /q-style atom in the loaded
	// top-level program with a snapshot of what its name refers to, for any
	// name already bound when the program loads (pre-installed module/global
	// defs). Names bound only later, during execution, stay unresolved here —
	// the `quote` word captures those at quote time instead. Top engine only
	// (sub-engines run fragments already walked) and not in check mode (the
	// program has been stripped to carriers).
	if e.isTop && !e.registry.Check.IsActive() {
		resolveAtomReferents(e.registry, e.stack)
	}

	// stepLimit is always set by the constructors (New / NewTop); the
	// defensive check that used to substitute a default if the field
	// was zero was load-bearing for callers that built Engine{}
	// directly, but no longer — the constructors are the only entry.
	limit := e.stepLimit
	for step := 0; step < limit; step++ {
		if e.pointer >= len(e.stack) {
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
				break
			}
		}

		val := e.stack[e.pointer]

		if e.trace != nil {
			snapshot := make([]Value, len(e.stack))
			copy(snapshot, e.stack)
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
			e.pointer++

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
				stackSplice(&e.stack, e.pointer, 1, expandParenExpr(items)...)
			}

		case IsReach(val):
			// A parsed Reach (dot-access node, m.a.b — Eval=true) evaluates
			// by lowering to its get/getr chain in place, exactly like the
			// ParenExpr it replaced. An inert reach (Eval=false, from `reach`)
			// or a codequote'd one (Quoted) is data — left via stepLiteral.
			if isEvalReach(val) && !e.pendingForwardWantsRawParen() {
				info, _ := AsReach(val)
				stackSplice(&e.stack, e.pointer, 1, expandReach(info)...)
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
			e.stack[e.pointer] = result

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

	// Implicit end-of-input: resolve any pending forwards from the stack.
	if err := e.resolveOrphanedForwards(); err != nil {
		return nil, err
	}

	for _, v := range e.stack {
		if IsOpenParen(v) {
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
		}
	}

	// Runtime uncalled-function residue (ERRORS.8.md §5, VOXGIG T1): a
	// named Function value placed by a FAILED dispatch that nothing
	// ever consumed. Higher-order uses consume the value, so they never
	// reach here; only at the top level — where no consumer can exist
	// anymore — does the residue become an error, with the original
	// call-site span. The same bug check mode names uncalled_function.
	if e.isTop && !e.registry.Check.IsActive() {
		for _, v := range e.stack {
			if v.FailedDispatch && (v.Parent.Equal(TFnDef) || v.Parent.Equal(TFunction)) {
				name := ""
				if info, ok := v.Data.(FnDefInfo); ok {
					name = info.Name
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
	for i, v := range e.stack {
		if !v.Undefined {
			continue
		}
		if e.registry.Check.IsActive() {
			e.stack[i] = NewCarrier(TAny)
		}
	}

	return e.stack, nil
}

// resolveOrphanedForwards handles end-of-input by resolving pending forwards.
func (e *Engine) resolveOrphanedForwards() error {
	for attempt := 0; attempt < 222; attempt++ {
		fwdIdx := -1
		for i, v := range e.stack {
			if IsForward(v) {
				fwdIdx = i
				break
			}
		}
		if fwdIdx < 0 {
			return nil
		}

		fwd, _ := AsForward(e.stack[fwdIdx])
		funcIdx := fwd.FuncIndex
		collectedCount := fwd.CollectedArgs
		stackArgCount := fwd.StackArgs

		// Remove the forward marker.
		stackRemove(&e.stack, fwdIdx)
		if fwdIdx < funcIdx {
			funcIdx--
		}

		// Try stack match or create curry list.
		e.curryOrStack(funcIdx, collectedCount, stackArgCount)

		// Retry from the current pointer position.
		for step := 0; step < 100; step++ {
			if e.pointer >= len(e.stack) {
				break
			}
			val := e.stack[e.pointer]
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
		if IsOpenParen(e.stack[i]) {
			return false
		}
		if IsForward(e.stack[i]) {
			fwd, _ := AsForward(e.stack[i])
			// RawParens or FormArgs (macro raw capture) both want a forward
			// ParenExpr / Reach left raw at the pointer rather than evaluated
			// during collection.
			return fwd.Sig != nil && (len(fwd.Sig.RawParens) > 0 || len(fwd.Sig.FormArgs) > 0)
		}
	}
	return false
}

// resolveForwardArgs implements structure-first, lazy forward-argument
// resolution (design/LAZY-ARG-RESOLUTION.0.md). It replaces the former
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
	for pos < maxBarrier && scanIdx < len(e.stack) {
		tok := e.stack[scanIdx]

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
			e.stack[scanIdx] = result
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
			stackSplice(&e.stack, scanIdx, 1, expandParenExpr(peItems)...)
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
			stackSplice(&e.stack, scanIdx, 1, expandReach(info)...)
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
						e.stack[scanIdx] = pe
						continue
					}
				}
			}
		}

		// Non-group token. A concrete literal carries a final type that
		// matchSignature tests identically, so it is sound to prune the
		// viable set on it. Words and other non-concrete tokens are left
		// un-pruned (their matchSignature treatment is contextual) but are
		// still counted as one resolved position — so, exactly like the
		// former scan, groups beyond a word remain reachable.
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
	for limit := 0; limit < 2222 && depth > 0; limit++ {
		if e.pointer >= len(e.stack) {
			break
		}
		v := e.stack[e.pointer]

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
			e.stack[e.pointer] = result
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
			e.stack[e.pointer] = placeholder
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
			placeholder := NewAtom(w.Name)
			placeholder.Pos = val.Pos
			placeholder.Undefined = true
			e.stack[e.pointer] = placeholder
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
		e.stack[e.pointer] = v
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
	e.stack[e.pointer] = v
	// Dispatch the unquoted wrapper now: stepLiteral routes it through
	// execFnDefLiteral, which forward-collects any trailing args.
	return e.stepLiteral()
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
				e.stack[e.pointer] = placeholder
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
				placeholder := NewAtom(w.Name)
				placeholder.Pos = val.Pos
				placeholder.Undefined = true
				e.stack[e.pointer] = placeholder
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
		e.stack[e.pointer] = v
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
			e.stack[e.pointer] = NewFunction(*fnDef)
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
			e.stack[e.pointer] = push
			return e.stepLiteral()
		}
	}

	// Simple value def: substitute the word with its value directly,
	// bypassing function dispatch entirely. FnDefInfo and ObjectTypeInfo
	// entries are not simple values — they go through normal Lookup.
	if top, ok := e.registry.Defs.Top(w.Name); ok {
		switch top.Data.(type) {
		case FnDefInfo, *ObjectTypeInfo:
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
					if fwd, ferr := AsForward(e.stack[fwdIdx]); ferr == nil && !bindsReferent(fwd.FuncName) {
						pe := NewParenExpr([]Value{val})
						pe.Pos = val.Pos
						e.stack[e.pointer] = pe
						return e.stepLiteral()
					}
				}
			}
			// Record the substitution as a "use" for unused-def
			// tracking in check mode.
			e.registry.Check.recordUse(w.Name)
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
			e.stack[e.pointer] = top
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
			e.stack[e.pointer] = NewBoolean(true)
			return nil
		}
		if w.Name == "false" {
			e.stack[e.pointer] = NewBoolean(false)
			return nil
		}
		if w.Name == "none" {
			e.stack[e.pointer] = NewNone()
			return nil
		}
		if w.Name == "null" {
			e.stack[e.pointer] = NewAtom("null")
			return nil
		}
		if t, ok := typeNames[w.Name]; ok {
			e.stack[e.pointer] = NewTypeLiteral(t)
			return nil
		}
		if t, ok := ResolveTypePath(w.Name); ok {
			e.stack[e.pointer] = NewTypeLiteral(t)
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
		e.stack[e.pointer] = v
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

	// Check-mode advisory: the forward-greediness gotcha. When a word
	// forward-collects an argument AND also takes a stack argument (a
	// swap-form dispatch) while a SIBLING operand — a value of the same
	// type the word just consumed — remains unconsumed on the stack below
	// it, the author likely meant the stacked operands to be consumed
	// together (the `1 2 add 3 mul → 5` surprise: `add` grabs the forward
	// `3` and strands the `1`). Advisory only (info severity), emitted in
	// check mode, never gating. See design/FORWARD-STRAND-ADVISORY.10.md.
	if e.registry.Check.IsActive() && fwdCount > 0 && stkCount > 0 {
		e.checkForwardStrandsOperand(w, sig, positions, val.Pos)
		// Mixed-form advisory (ERRORS.8.md §6.2, VOXGIG T9.4): a call
		// of three or more args that takes operand(s) from a PRECEDING
		// expression while also forward-collecting binds differently
		// from the all-forward reading — `(cond) if [a] [b]` is the
		// reported shape — and the divergence is silent. Two-arg mixed
		// calls are the documented swap form (`10 sub 3`) and stay
		// clean. Advisory only (info severity), never gating.
		if sig.TotalArgs() >= 3 {
			e.registry.Check.AddDiagnostic(CheckDiagnostic{
				Code: "mixed_form_call",
				Detail: w.Name + " takes " + strconv.Itoa(stkCount) + " argument(s) from the stack while forward-collecting " +
					strconv.Itoa(fwdCount) + " — the mixed form binds differently from the all-forward form; " +
					"prefer " + w.Name + " arg1 arg2 … or group explicitly",
				Word: w.Name,
				Row:  val.Pos.Row,
				Col:  val.Pos.Col,
			})
		}
	}

	// Forward collection needed: defer execution.
	if fwdCount > 0 {
		e.traceNote = "forward→ " + traceSigStr(w.Name, sig)
		return e.insertForward(w, sig, fwdCount, stkCount, specAt)
	}

	// Immediate execution: read args from recorded positions.
	match := &MatchResult{Sig: sig, Positions: positions, Name: w.Name}
	if stkCount > 0 {
		match.Args = make([]Value, stkCount)
		for i, pos := range positions {
			match.Args[i] = e.stack[pos]
		}
	}
	e.traceNote = "stack " + traceSigStr(w.Name, sig)
	return e.execMatch(match)
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
		v := e.stack[i]
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
					evaluated, err := e.autoEvalMap(match.Args[i])
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
				if !noEval {
					// Bare words never degrade to data: a list element
					// that fails to evaluate (an undefined name, or a
					// valid name dispatched with the wrong arity) is an
					// error, not a silent fallback to its literal word.
					// Use `foo/q` for an atom, `quote […]` for a literal
					// word list / quotation.
					evaluated, err := e.autoEvalList(match.Args[i])
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
	if e.registry.Check.IsActive() && !match.Sig.RunInCheckMode {
		name := ""
		var pos SrcPos
		if e.pointer < len(e.stack) && IsWord(e.stack[e.pointer]) {
			pos = e.stack[e.pointer].Pos
			if w, err := AsWord(e.stack[e.pointer]); err == nil {
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
		if match.Sig.FullStack && match.Sig.CheckFullStackFn != nil {
			base := 0
			for i := e.pointer - 1; i >= 0; i-- {
				if IsOpenParen(e.stack[i]) {
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
			results := match.Sig.CheckFullStackFn(match.Args, preserved, e.registry)
			stackSplice(&e.stack, base, end+1-base, results...)
			e.pointer = base
			return nil
		}

		results := carrierResults(e.registry, name, match.Sig, match.Args, pos)
		return e.spliceMatchResults(match, sortedIndices, n, results)
	}

	// Compute context (cheap O(1) call).
	ctx := e.registry.Contexts.TopData()

	var fullStack []Value
	if match.Sig.FullStack {
		// Find the nearest open-paren barrier so that FullStack handlers
		// only replace within the current paren scope, not below it.
		base := 0
		for i := e.pointer - 1; i >= 0; i-- {
			if IsOpenParen(e.stack[i]) {
				base = i + 1
				break
			}
		}
		// Collect the full resolved stack before the pointer (from base),
		// excluding the matched args and forwards.
		fullStack = e.resolvedStackBeforeFrom(base, sortedIndices)
		results, err := match.Sig.Handler(match.Args, ctx, fullStack, e.registry)
		if err != nil {
			return e.stampErrPos(err)
		}
		if e.recorder != nil {
			e.recorder.OnCall(match.Name, n, len(results))
		}
		// FullStack handler returns the complete replacement for
		// everything from base through the pointer (inclusive).
		stackSplice(&e.stack, base, e.pointer+1-base, results...)
		e.pointer = base
		return nil
	}

	results, err := match.Sig.Handler(match.Args, ctx, nil, e.registry)
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
	if e.pointer >= 0 && e.pointer < len(e.stack) {
		stampResultPos(results, e.stack[e.pointer].Pos)
	}

	if err := e.spliceMatchResults(match, sortedIndices, n, results); err != nil {
		return err
	}
	// ParkResult words (notably `ref`) leave their result as inert data at
	// the call site rather than re-stepping it: advance the pointer past the
	// spliced result so an unquoted Function value does NOT auto-dispatch
	// here (matching the `/r` word-suffix). The value still dispatches when
	// re-stepped elsewhere — retrieved from a map, unwrapped from a paren.
	if match.Sig.ParkResult {
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
				e.stack[dst] = e.stack[i]
				dst++
			}
		}
		// Splice out the compacted garbage, insert results.
		stackSplice(&e.stack, dst, e.pointer+1-dst, results...)
		e.pointer = firstArgIdx
	} else if n == 0 {
		// No args, just replace the word with results.
		stackSplice(&e.stack, e.pointer, 1, results...)
		// Pointer stays at same position to re-examine results.
	} else {
		// Fallback: simple contiguous splice.
		argStart := e.pointer - n
		if argStart < 0 {
			argStart = 0
		}
		stackSplice(&e.stack, argStart, e.pointer+1-argStart, results...)
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
		values[i] = e.stack[idx]
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
		e.stack[idx] = reordered[i]
	}
}

// resolvedIndicesBefore returns the indices of the last n resolved values
// before the current pointer, stopping at open-paren barriers.
func (e *Engine) resolvedIndicesBefore(n int) []int {
	var indices []int
	for i := e.pointer - 1; i >= 0 && len(indices) < n; i-- {
		if IsOpenParen(e.stack[i]) {
			break
		}
		if IsForward(e.stack[i]) || IsMark(e.stack[i]) || IsMove(e.stack[i]) {
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
		if exclude[i] || IsForward(e.stack[i]) || IsOpenParen(e.stack[i]) || IsMark(e.stack[i]) || IsMove(e.stack[i]) {
			continue
		}
		stack = append(stack, e.stack[i])
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
	pos := e.stack[idx].Pos
	e.stack[idx] = NewWordModified(w.Name, w.ArgCount, true, false)
	e.stack[idx].Pos = pos // preserve source position across force-stack rewrite
}

func (e *Engine) insertForward(w WordInfo, sig *Signature, forwardNeeded, stackArgs, specAt int) error {
	var pos SrcPos
	if e.pointer >= 0 && e.pointer < len(e.stack) {
		pos = e.stack[e.pointer].Pos
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

	stackInsert(&e.stack, e.pointer+1, fwd)

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
		if IsOpenParen(e.stack[i]) {
			return -1
		}
		if IsForward(e.stack[i]) {
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
	if IsParenExpr(e.stack[valIdx]) && !e.stack[valIdx].Quoted && !e.pendingForwardWantsRawParen() {
		items, _ := AsParenExpr(e.stack[valIdx])
		stackSplice(&e.stack, valIdx, 1, expandParenExpr(items)...)
		return nil
	}
	// A Reach reaching stepLiteral (nested in a collapsing span, or a
	// collected list/map element) lowers to its get-chain in place, like a
	// ParenExpr (Reach Phase B). Quoted/raw-pending reaches fall through.
	if isEvalReach(e.stack[valIdx]) && !e.pendingForwardWantsRawParen() {
		info, _ := AsReach(e.stack[valIdx])
		stackSplice(&e.stack, valIdx, 1, expandReach(info)...)
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
		if IsSplice(e.stack[valIdx]) {
			info, _ := AsSplice(e.stack[valIdx])
			stackSplice(&e.stack, valIdx, 1, spliceExpand(info.Data)...)
			return nil
		}
		// A dispatch-modifier marker reaching the pointer standalone means
		// the preceding value was NOT a pending function (so execFnDefLiteral
		// never consumed it) — e.g. `(1 add 2)/s`. The modifier is a no-op on
		// a non-function result: drop the marker.
		if IsDispatchMod(e.stack[valIdx]) {
			e.stack = append(e.stack[:valIdx], e.stack[valIdx+1:]...)
			return nil
		}
		// If the value is a FnDef/TFunction, execute it. Quoted function
		// values are treated as data (not executed).
		val := e.stack[valIdx]
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

	fwd, _ := AsForward(e.stack[fwdIdx])
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
		val := e.stack[valIdx]
		nextIdx := fwd.CollectedArgs
		matches := sigArgMatches(fwd.Sig, nextIdx, val)
		if !matches && fwd.Sig.QuoteArgs != nil && fwd.Sig.QuoteArgs[nextIdx] &&
			val.Parent.Equal(TWord) && TAtom.ConformsTo(sigArgType(fwd.Sig, nextIdx)) {
			w, _ := AsWord(val)
			atom := NewAtom(w.Name)
			atom.Pos = val.Pos // preserve source position across /q Word→Atom conversion
			e.stack[valIdx] = atom
			matches = true
		}
		if !matches {
			// Type mismatch — implicit end: resolve forward from stack.
			return e.implicitEnd(fwdIdx)
		}
	}

	// Remove the value from its current position.
	val := e.stack[valIdx]
	stackRemove(&e.stack, valIdx)

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

	stackInsert(&e.stack, insertIdx, val)

	funcIdx++
	fwdIdx++

	fwd.CollectedArgs++
	fwd.FuncIndex = funcIdx

	e.traceNote = fmt.Sprintf("collect %s %d/%d",
		fwd.FuncName, fwd.CollectedArgs, fwd.ExpectedArgs)

	if fwd.CollectedArgs >= fwd.ExpectedArgs {
		// All forward args collected. Remove forward, force stack, retry.
		stackRemove(&e.stack, fwdIdx)
		// Adjust funcIdx if forward was before it (shouldn't normally happen).
		if fwdIdx < funcIdx {
			funcIdx--
		}

		if funcIdx < len(e.stack) && IsWord(e.stack[funcIdx]) {
			w, _ := AsWord(e.stack[funcIdx])
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
				if i >= 0 && i < len(e.stack) {
					v := e.stack[i]
					if isRecordableLiteral(v) {
						e.recorder.OnPushLit(v)
					}
				}
			}
		}
	} else {
		e.stack[fwdIdx] = NewForward(fwd)
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
	for i := 0; i < len(e.stack); i++ {
		val := e.stack[i]
		if !val.Eval || val.Quoted {
			continue
		}
		if val.Parent.Equal(TList) && val.Data != nil && !IsTypedList(val) && !IsTableType(val) {
			result, err := e.autoEvalList(val)
			if err != nil {
				return err
			}
			e.stack[i] = result
		} else if val.Parent.Equal(TMap) && val.Data != nil && !IsTypedMap(val) && !IsRecordType(val) && !IsOptionsType(val) {
			result, err := e.autoEvalMap(val)
			if err != nil {
				return err
			}
			e.stack[i] = result
		}
	}
	return nil
}

// autoEvalList evaluates the contents of a plain list in a sub-engine,
// returning a new list containing the results. For example, [1 add 2] → [3].
func (e *Engine) autoEvalList(val Value) (Value, error) {
	elems, _ := AsList(val)
	if elems.Len() == 0 {
		return val, nil
	}
	sub := New(e.registry)
	input := make([]Value, elems.Len())
	copy(input, elems.Slice())
	result, err := sub.Run(input)
	if err != nil {
		return Value{}, err
	}
	return NewList(result), nil
}

// evalInterpString evaluates an interpolated string by running each
// expression part in a sub-engine, converting results to strings, and
// concatenating everything into a single string value.
func (e *Engine) evalInterpString(val Value) (Value, error) {
	parts, err := AsInterpString(val)
	if err != nil || parts == nil {
		return NewString(""), nil
	}
	var buf strings.Builder
	for _, part := range parts {
		if part.Expr == nil {
			buf.WriteString(part.Lit)
			continue
		}
		sub := New(e.registry)
		result, err := sub.Run(part.Expr)
		if err != nil {
			return Value{}, err
		}
		for _, r := range result {
			buf.WriteString(ValToString(r))
		}
		// If the expression raised a flow-control signal, stop
		// evaluating further parts. The outer Run loop will catch
		// the flag and unwind. Continuing would call sub.Run with
		// a stale flag still set and could produce observable
		// side effects from later parts.
		if e.registry.FlowCtrl != FlowNone {
			break
		}
	}
	return NewString(buf.String()), nil
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
			out = append(out, NewWord("getr"))
		} else {
			out = append(out, NewWord("get"))
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
	sub := New(r)
	res, err := sub.Run(expandParenExpr(toks))
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
	sub := New(e.registry)
	return sub.Run(expandParenExpr(items))
}

// autoEvalMap evaluates each value in a plain map using a sub-engine.
// Word values resolve directly; lists auto-evaluate via autoEvalStack:
//
//	{r:rv}        → {r:10}      (word evaluated to its def'd value)
//	{x:[1 add 2]} → {x:[3]}     (list evaluated, stays as list)
//	{a:[1,2]}     → {a:[1,2]}   (literal list unchanged)
//	{x:"hello"}   → {x:"hello"} (strings pass through unchanged)
func (e *Engine) autoEvalMap(val Value) (Value, error) {
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
			sub := New(e.registry)
			keyResult, err := sub.Run([]Value{NewWord(key)})
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

		// Paren expression: evaluate items as an isolated sub-expression
		// (shared via evalParenExprResults with the main-stack path).
		if IsParenExpr(v) {
			items, _ := AsParenExpr(v)
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

		// Evaluate each value in a sub-engine.
		sub := New(e.registry)
		result, err := sub.Run([]Value{v})
		if err != nil {
			return Value{}, err
		}
		if len(result) == 1 {
			out.Set(resolvedKey, result[0])
		} else if len(result) > 1 {
			out.Set(resolvedKey, NewList(result))
		}
	}
	return NewMap(out), nil
}

// the function. If the FnDef carries a captured Registry (closure from a
// module), execution happens in a sub-engine using that registry so that
// module-internal words are available. Otherwise, body tokens are spliced
// into the current engine's stack.
func (e *Engine) execFnDefLiteral(valIdx int) error {
	val := e.stack[valIdx]
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
	if fn == nil {
		e.pointer++
		return nil
	}

	w := WordInfo{Name: fnDef.Name, ArgCount: -1}

	// A `/r` or `/q` modifier on a paren / dotted-path result is emitted by
	// the parser as a Word/__DM marker right after the group (/u /s /f /N
	// are the usurp / stack-args / forward-args / force-arity words). Peek
	// and consume it: it leaves the function inert (data).
	if valIdx+1 < len(e.stack) {
		if _, ok := AsDispatchMod(e.stack[valIdx+1]); ok {
			e.stack = append(e.stack[:valIdx+1], e.stack[valIdx+2:]...)
			v := e.stack[valIdx]
			v.Quoted = true
			e.stack[valIdx] = v
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

	// Fall through to FnSig-based pure-stack matching when
	// matchSignature finds nothing — this preserves the legacy
	// anonymous-fn-on-stack dispatch for AQL fns whose Sigs carry
	// named params. The same path runs when matched but the sig
	// has no Go Handler AND this isn't an `afn`-produced lambda:
	// predicate-type FnDefs landing bare are intentionally inert.
	if sig == nil || (sig.Handler == nil && !fnDef.Anonymous) {
		return e.execFnDefSigStackMatch(valIdx, fnDef, resolved)
	}

	// Count forward vs stack positions.
	fwdCount := 0
	for _, pos := range positions {
		if pos > e.pointer {
			fwdCount++
		}
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
	if sig.Handler == nil {
		return e.execFnDefSigStackMatch(valIdx, fnDef, resolved)
	}

	// Module closures: the FnDef carries a captured sub-registry
	// (Registry != e.registry). Two cases:
	//
	//  1. **Trivial-delegation wrapper** — the wrapper FnSig body is
	//     a single Word referencing the inner native of the same name
	//     (e.g. rand.string's wrapper body `[Word(rand-string)]`).
	//     matchSignature already found and matched that inner native;
	//     its handler is `sig.Handler`. Direct-call it via execMatch
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
			if len(ownSigs[i].Body) == 0 {
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
			trivialDelegation := isTrivialDelegationBody(wrapperSig, fnDef.Name)
			if trivialDelegation && sig.Handler != nil {
				match := &MatchResult{Sig: sig, Positions: positions, Name: fnDef.Name}
				if len(positions) > 0 {
					match.Args = make([]Value, len(positions))
					for i, pos := range positions {
						match.Args[i] = e.stack[pos]
					}
				}
				return e.execMatch(match)
			}
			// Non-trivial body — run via CallAQL in the captured sub-
			// registry. The wrapper's body has module-private references
			// that need fnDef.Registry's scope for resolution.
			args := make([]Value, len(positions))
			for i, pos := range positions {
				args[i] = e.stack[pos]
			}
			return e.execFnDefSig(valIdx, wrapperSig, args, fnDef.Registry)
		}
	}

	// Pure-stack match: dispatch via execMatch the same way a bare
	// word with no forward args would.
	match := &MatchResult{Sig: sig, Positions: positions, Name: fnDef.Name}
	if len(positions) > 0 {
		match.Args = make([]Value, len(positions))
		for i, pos := range positions {
			match.Args[i] = e.stack[pos]
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

// isTrivialDelegationBody reports whether a wrapper FnSig is a pure
// pass-through to an inner native of the same name — body of the form
// `[Word(name)]` with all-unnamed Params. Module wrappers built by
// the `makeXxxFnDef` / `wrapXxxFnDef` helpers all have this shape;
// AQL fns defined inside a module preamble (with real bodies + named
// params) do not. Used by execFnDefLiteral to decide whether to
// direct-call the inner handler (trivial) or run the body in the
// captured sub-registry (non-trivial).
func isTrivialDelegationBody(sig *FnSig, name string) bool {
	inner, ok := trivialDelegationTarget(sig)
	return ok && inner == name
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
	if len(sig.Body) != 1 {
		return "", false
	}
	for _, p := range sig.Params {
		if p.Name != "" {
			return "", false
		}
	}
	if !IsWord(sig.Body[0]) {
		return "", false
	}
	w, err := AsWord(sig.Body[0])
	if err != nil {
		return "", false
	}
	return w.Name, true
}

// argTypeList renders a comma-separated list of the values' lattice
// types, for the uncalled_function diagnostic's "arguments: …" detail.
func argTypeList(vals []Value) string {
	parts := make([]string, len(vals))
	for i, v := range vals {
		parts[i] = v.Parent.String()
	}
	return strings.Join(parts, ", ")
}

// upcomingArgs returns the value/literal tokens that follow valIdx up to
// the next statement or group boundary (End / CloseParen / Mark / Move)
// — the forward args a function-value call had available to collect.
// Forward-collection markers are skipped; everything before the first
// boundary is a candidate arg. Lets a failed forward-form call
// (`Pkg.fn a b`) be recognised as a call, not a bare value reference.
func (e *Engine) upcomingArgs(valIdx int) []Value {
	var out []Value
	for i := valIdx + 1; i < len(e.stack); i++ {
		v := e.stack[i]
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
	ownSigs := fnDef.OwnSigs()
	for i := range ownSigs {
		sig := &ownSigs[i]
		nArgs := len(sig.Params)
		if nArgs == 0 {
			if checkMode {
				return e.spliceAnonCheckResult(valIdx, 0, sig, nil, fnDef.Captured)
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
					args[j] = e.stack[resolvedIdx[ri]]
				}
				if checkMode {
					return e.spliceAnonCheckResult(valIdx, nArgs, sig, args, fnDef.Captured)
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
					args[j] = e.stack[resolvedIdx[startIdx+j]]
				}
				if checkMode {
					return e.spliceAnonCheckResult(valIdx, nArgs, sig, args, fnDef.Captured)
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
		valIdx < len(e.stack) && !e.stack[valIdx].Quoted {
		candidates := append(append([]Value{}, resolved...), e.upcomingArgs(valIdx)...)
		if len(candidates) > 0 {
			if e.registry.Check.IsActive() {
				pos := e.stack[valIdx].Pos
				e.registry.Check.AddDiagnostic(CheckDiagnostic{
					Code:   "uncalled_function",
					Detail: "call to '" + fnDef.Name + "' matched no signature and was left on the stack as data (arguments: " + argTypeList(candidates) + ")",
					Word:   fnDef.Name,
					Row:    pos.Row,
					Col:    pos.Col,
				})
			} else {
				e.stack[valIdx].FailedDispatch = true
				// Borrow a span from the nearest argument when the
				// FnDef value itself carries none, so the end-of-run
				// report can point somewhere real.
				if e.stack[valIdx].Pos.Row == 0 {
					for _, c := range candidates {
						if c.Pos.Row > 0 {
							e.stack[valIdx].Pos = c.Pos
							break
						}
					}
				}
			}
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
	result := AnalyseFnBody(e.registry, "", paramNames, sig.Body, args, captures)
	if len(result) == 0 {
		result = []Value{NewCarrier(TAny)}
	}

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
				e.stack[dst] = e.stack[i]
				dst++
			}
		}
		stackSplice(&e.stack, dst, valIdx+1-dst, result...)
		e.pointer = firstArgIdx
	} else if nArgs == 0 {
		stackSplice(&e.stack, valIdx, 1, result...)
	} else {
		argStart := valIdx - nArgs
		if argStart < 0 {
			argStart = 0
		}
		stackSplice(&e.stack, argStart, valIdx+1-argStart, result...)
		e.pointer = argStart
	}
	return nil
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
			compiled.Handler = buildFnBodyHandler(r, fnDef.Name, sig, fnDef)
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
	}
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
	if valIdx >= 0 && valIdx < len(e.stack) {
		callPos = e.stack[valIdx].Pos
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
					evaluated, err := e.autoEvalMap(args[i])
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
					evaluated, err := e.autoEvalList(args[i])
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

	if capturedReg != nil {
		// Execute in the captured module's registry via CallAQL.
		// Pass the FnDef's lexical captures so the body sees them as
		// defs (alongside the module-registry's own bindings).
		var captures []CapturedBinding
		if valIdx < len(e.stack) {
			if fd, ok := e.stack[valIdx].Data.(FnDefInfo); ok {
				captures = fd.Captured
			}
		}
		result, err := capturedReg.CallAQL(sig, args, captures)
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
					e.stack[dst] = e.stack[i]
					dst++
				}
			}
			stackSplice(&e.stack, dst, valIdx+1-dst, result...)
			e.pointer = firstArgIdx
		} else if nArgs == 0 {
			stackSplice(&e.stack, valIdx, 1, result...)
		} else {
			argStart := valIdx - nArgs
			if argStart < 0 {
				argStart = 0
			}
			stackSplice(&e.stack, argStart, valIdx+1-argStart, result...)
			e.pointer = argStart
		}
		return nil
	}

	// No captured registry — splice body tokens into the current stack.
	var tokens []Value
	tokens = append(tokens, NewOpenParen())

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
	if valIdx < len(e.stack) {
		if fd, ok := e.stack[valIdx].Data.(FnDefInfo); ok {
			captures = fd.Captured
		}
	}
	var names []string
	for _, cb := range captures {
		InstallDef(e.registry, cb.Name, cb.Value)
		names = append(names, cb.Name)
	}

	// args in top-first sig order (matchSignature convention).
	// Named params bind by name; unnamed params push to body tokens in
	// i-order. No reordering — same convention as InstallFnDef and
	// CallAQL. See design/SIG-ORDER-REFACTOR.10.md.
	unnamedCount := 0
	for i, p := range sig.Params {
		if p.Name != "" {
			InstallDef(e.registry, p.Name, args[i])
			names = append(names, p.Name)
		} else {
			tokens = append(tokens, args[i])
			unnamedCount++
		}
	}
	body := make([]Value, len(sig.Body))
	copy(body, sig.Body)
	tokens = append(tokens, body...)

	tokens = append(tokens, NewWord("__pa"))
	for i := len(names) - 1; i >= 0; i-- {
		tokens = append(tokens,
			NewWordModified("undef", -1, false, true),
			NewWord(names[i]),
		)
	}
	if len(sig.Returns) > 0 {
		tokens = append(tokens, NewReturnCheck(ReturnCheckInfo{
			FuncName:     "<fn>",
			Returns:      sig.Returns,
			UnnamedCount: unnamedCount,
			Pos:          callPos,
		}))
	}
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
				e.stack[dst] = e.stack[i]
				dst++
			}
		}
		stackSplice(&e.stack, dst, valIdx+1-dst, tokens...)
		e.pointer = firstArgIdx
	} else if nArgs == 0 {
		stackSplice(&e.stack, valIdx, 1, tokens...)
	} else {
		argStart := valIdx - nArgs
		if argStart < 0 {
			argStart = 0
		}
		stackSplice(&e.stack, argStart, valIdx+1-argStart, tokens...)
		e.pointer = argStart
	}

	return nil
}

// implicitEnd resolves a forward early when a type mismatch occurs.
func (e *Engine) implicitEnd(fwdIdx int) error {
	fwd, _ := AsForward(e.stack[fwdIdx])
	funcIdx := fwd.FuncIndex
	collectedCount := fwd.CollectedArgs
	stackArgCount := fwd.StackArgs

	stackRemove(&e.stack, fwdIdx)
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
		if IsOpenParen(e.stack[i]) {
			break
		}
		if IsForward(e.stack[i]) {
			fwdIdx = i
			break
		}
	}
	if fwdIdx < 0 {
		return false
	}

	fwd, _ := AsForward(e.stack[fwdIdx])
	funcIdx := fwd.FuncIndex
	claimed := fwd.CollectedArgs + fwd.StackArgs
	if claimed == 0 {
		// Nothing collected yet — no smaller-arity dispatch to commit.
		return false
	}
	if funcIdx < 0 || funcIdx >= len(e.stack) || !IsWord(e.stack[funcIdx]) {
		return false
	}
	w, _ := AsWord(e.stack[funcIdx])
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
		v := e.stack[i]
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
	stackRemove(&e.stack, fwdIdx)
	if fwdIdx < funcIdx {
		funcIdx--
	}
	if funcIdx < len(e.stack) && IsWord(e.stack[funcIdx]) {
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
	if e.pointer >= 0 && e.pointer < len(e.stack) {
		if bw, err := AsWord(e.stack[e.pointer]); err == nil {
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
		if IsOpenParen(e.stack[i]) {
			break
		}
		if IsForward(e.stack[i]) {
			fwdIdx = i
			break
		}
	}

	if fwdIdx < 0 {
		stackRemove(&e.stack, endIdx)
		return nil
	}

	fwd, _ := AsForward(e.stack[fwdIdx])
	funcIdx := fwd.FuncIndex

	// Remove forward and end from the stack.
	// Remove higher index first to preserve lower indices.
	if endIdx > fwdIdx {
		stackRemove(&e.stack, endIdx)
		stackRemove(&e.stack, fwdIdx)
		if fwdIdx < funcIdx {
			funcIdx-- // forward removal
		}
		// end was already removed (endIdx > fwdIdx), endIdx > funcIdx always
	} else {
		stackRemove(&e.stack, fwdIdx)
		newEndIdx := endIdx
		if fwdIdx < endIdx {
			newEndIdx--
		}
		stackRemove(&e.stack, newEndIdx)
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
	for i := 0; i < len(e.stack); i++ {
		_as2, _ := AsMark(e.stack[i])
		if IsMark(e.stack[i]) && _as2.ID == info.To {
			markIdx = i
			break
		}
	}
	if markIdx < 0 {
		// Mark was removed from the stack (e.g. by a for-loop controller
		// signalling loop completion). Remove this orphaned move quietly.
		delete(e.marks, info.To)
		stackRemove(&e.stack, e.pointer)
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
	markInfo, _ := AsMark(e.stack[markIdx])

	// Remove from hash table.
	delete(e.marks, info.To)

	// Replace everything from mark through move (inclusive) with the body copy.
	body := make([]Value, len(markInfo.Body))
	copy(body, markInfo.Body)
	stackSplice(&e.stack, markIdx, moveIdx-markIdx+1, body...)

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
		cont.Results = append(cont.Results, e.stack[j])
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
		stackSplice(&e.stack, markIdx, moveIdx-markIdx+1, tokens...)
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
	stackSplice(&e.stack, markIdx, moveIdx-markIdx+1, cont.Results...)
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
		condResult = e.stack[j]
	}

	// Remove mark from hash table.
	delete(e.marks, info.To)

	// Check if condition produced a value.
	if condResult.Parent == nil {
		stackSplice(&e.stack, markIdx, moveIdx-markIdx+1)
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
	stackSplice(&e.stack, markIdx, moveIdx-markIdx+1, branch...)
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
		return nil, e.runtimeError("halt", fmt.Sprintf("%s outside loop", ctrl), ctrl.String(), "")
	}
	return e.stack, nil
}

// handleLoopBreak resolves a FlowBreak signal by finding the nearest
// enclosing for-loop (move with continuation) on this tape and
// terminating it. Returns true if a loop was found and rewritten,
// false if no enclosing loop was on the tape.
func (e *Engine) handleLoopBreak() bool {
	// Scan forward from current pointer for a move with continuation.
	for i := e.pointer; i < len(e.stack); i++ {
		if IsMove(e.stack[i]) {
			info, _ := AsMove(e.stack[i])
			if info.Cont != nil {
				// Found the for-loop's move. Find its mark.
				markIdx := -1
				for j := 0; j < i; j++ {
					_as3, _ := AsMark(e.stack[j])
					if IsMark(e.stack[j]) && _as3.ID == info.To {
						markIdx = j
						break
					}
				}
				if markIdx < 0 {
					delete(e.marks, info.To)
					continue
				}

				// Uninstall iterator, splice in accumulated results.
				UninstallDef(info.Cont.Registry, info.Cont.IterName)
				delete(e.marks, info.To)
				stackSplice(&e.stack, markIdx, i-markIdx+1, info.Cont.Results...)
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
	for i := e.pointer; i < len(e.stack); i++ {
		if IsMove(e.stack[i]) {
			info, _ := AsMove(e.stack[i])
			if info.Cont != nil {
				// Found the for-loop's move. Find its mark.
				markIdx := -1
				for j := 0; j < i; j++ {
					_as4, _ := AsMark(e.stack[j])
					if IsMark(e.stack[j]) && _as4.ID == info.To {
						markIdx = j
						break
					}
				}
				if markIdx < 0 {
					delete(e.marks, info.To)
					continue
				}

				// Remove values between mark and move (discard partial results).
				if i-markIdx > 1 {
					stackSplice(&e.stack, markIdx+1, i-markIdx-1)
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
	for i < len(e.stack) {
		if IsMark(e.stack[i]) || IsMove(e.stack[i]) {
			stackRemove(&e.stack, i)
		} else {
			i++
		}
	}
	e.marks = nil
}

// stepOpenParen replaces the "(" word with an open-paren marker.
func (e *Engine) stepOpenParen() error {
	e.stack[e.pointer] = NewOpenParen()
	e.pointer++
	return nil
}

// stepCloseParen handles the ")" word. It resolves any pending forwards
// inside the paren scope via implicit end, then collapses the sub-expression.
func (e *Engine) stepCloseParen() error {
	closeIdx := e.pointer

	openIdx := -1
	for i := closeIdx - 1; i >= 0; i-- {
		if IsOpenParen(e.stack[i]) {
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
			if IsForward(e.stack[i]) {
				hasFwd = true
				fwd, _ := AsForward(e.stack[i])
				funcIdx := fwd.FuncIndex
				collectedCount := fwd.CollectedArgs
				stackArgCount := fwd.StackArgs

				// Remove the forward.
				stackRemove(&e.stack, i)
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
					val := e.stack[e.pointer]
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
		if IsForward(e.stack[i]) {
			fwd, _ := AsForward(e.stack[i])
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
			v := e.stack[i]
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
		if IsDefCleanup(e.stack[i]) {
			e.stepDefCleanup(e.stack[i])
			stackRemove(&e.stack, i)
			closeIdx--
			i--
		}
	}

	// Check for return type validation.
	for i := openIdx + 1; i < closeIdx; i++ {
		if IsReturnCheck(e.stack[i]) {
			rc, _ := AsReturnCheck(e.stack[i])
			stackRemove(&e.stack, i)
			closeIdx--

			// Collect resolved values in scope.
			var results []Value
			for j := openIdx + 1; j < closeIdx; j++ {
				results = append(results, e.stack[j])
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
			// Use the membership predicate v.Is(exp) — the SAME question
			// the parameter boundary asks (sigTypeMatches → v.Is) — so a
			// type's Behavior governs both ends symmetrically: a predicate
			// refine runs its predicate on the way out (subset semantics),
			// a bare refine stays nominal (newtype), and builtins/objects
			// are unchanged (v.Is ≡ v.Parent.ConformsTo on concrete values).
			// See design/REFINE-NEWTYPE-VS-SUBSET.10.md.
			for k, exp := range rc.Returns {
				if !results[extra+k].Is(exp) {
					return e.returnTypeError(rc.FuncName, k+1, exp, results[extra+k], rc.Pos)
				}
			}

			// Discard unconsumed unnamed args from the bottom of the scope.
			for j := 0; j < extra; j++ {
				stackRemove(&e.stack, openIdx+1)
				closeIdx--
			}
			break
		}
	}

	// Remove the close paren (higher index first) and open paren.
	// The values between them are already in place.
	stackRemove(&e.stack, closeIdx)
	stackRemove(&e.stack, openIdx)

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
		if end > len(e.stack) {
			end = len(e.stack)
		}
		for i := openIdx; i < end; i++ {
			if isRecordableLiteral(e.stack[i]) {
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
	for i := openIdx + 1; i < len(e.stack); i++ {
		if IsOpenParen(e.stack[i]) {
			depth++
		} else if IsCloseParen(e.stack[i]) {
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
		if IsOpenParen(e.stack[i]) {
			start = i + 1
			break
		}
		if IsForward(e.stack[i]) {
			fwd, _ := AsForward(e.stack[i])
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
		v := e.stack[i]
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
		if IsOpenParen(e.stack[i]) {
			return false
		}
		if IsForward(e.stack[i]) {
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

	if funcIdx >= len(e.stack) || !IsWord(e.stack[funcIdx]) {
		e.pointer = funcIdx
		return
	}

	w, _ := AsWord(e.stack[funcIdx])
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
			if IsOpenParen(e.stack[i]) {
				start = i + 1
				break
			}
			if IsForward(e.stack[i]) {
				fwd, _ := AsForward(e.stack[i])
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
			v := e.stack[i]
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
		if IsOpenParen(e.stack[i]) {
			break
		}
		if IsForward(e.stack[i]) {
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
			elems = append(elems, e.stack[i])
		}

		stackSplice(&e.stack, startIdx, collectedCount+1, NewList(elems))
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
		if IsOpenParen(e.stack[i]) {
			break
		}
		if IsForward(e.stack[i]) {
			fwd, _ := AsForward(e.stack[i])
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
		if IsOpenParen(e.stack[i]) {
			break
		}
		if IsForward(e.stack[i]) {
			fwd, _ := AsForward(e.stack[i])
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
		if IsOpenParen(e.stack[i]) {
			break
		}
		if IsForward(e.stack[i]) {
			fwd, _ := AsForward(e.stack[i])
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

	// When the next forward token is a Word, prefer signatures with
	// /q at position 0 (inspect-style name capture). The user wrote a
	// Word, not a String — the /q sig captures the user's intent that
	// the name is data, not a call site. The non-/q TString sister
	// sig is for callers who pass a string literal. This also covers
	// untype Foo (Foo in r.Types), `m.Color` after import (Color is a
	// key in the imported map), and inspect-style name capture.
	preferWordSig := false
	if e.pointer+1 < len(e.stack) {
		next := e.stack[e.pointer+1]
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
			for fwd < forwardLimit && scanIdx < len(e.stack) {

				tok := e.stack[scanIdx]
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
						if sigArgMatches(sig, fwd, top) || expectedType.Equal(TAny) {
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
			if !patternsOk(sig, positions, e.stack, fwd, e.registry) {
				continue
			}
			if preferWordSig && !isPreferred {
				if bestDeferred == nil {
					bestDeferred = &matchResult{sig, append([]int(nil), positions...), specAt}
				}
				continue
			}
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
			if !sigArgMatches(sig, sigIdx, stackVal) {
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
		if !patternsOk(sig, positions, e.stack, fwd, e.registry) {
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
	for i := e.pointer + 1; remaining > 0 && i < len(e.stack); i++ {
		v := e.stack[i]
		if IsForward(v) || IsMark(v) || IsMove(v) ||
			IsOpenParen(v) || IsReturnCheck(v) || IsDefCleanup(v) {
			continue
		}
		positions = append(positions, i)
		remaining--
	}
	return positions
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
		v := e.stack[p]
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
	synth := &Signature{Params: spec.Params, Returns: spec.Returns}
	normalizeSig(synth)

	n := synth.TotalArgs()
	positions := e.checkModeFallbackPositions(n)
	args := make([]Value, len(positions))
	for i, p := range positions {
		args[i] = e.stack[p]
	}
	results := carrierResults(e.registry, w.Name, synth, args, pos)
	e.spliceCheckResults(positions, results)
	return true, nil
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
			av := e.stack[p]
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
	// Fallback pass: if no compatible sig was found at all, prefer
	// a sig with a ReturnsFn over one without (all else equal).
	if bestMatch < 0 {
		for i := range fn.Signatures {
			s := &fn.Signatures[i]
			if s.Fallback {
				continue
			}
			if s.ReturnsFn != nil && !bestHasFn {
				best = s
				break
			}
		}
	}
	sig := best
	e.registry.Check.AddDiagnostic(CheckDiagnostic{
		Code:   "no_signature",
		Detail: "no matching signature for " + w.Name + "; assuming best-fit candidate for analysis",
		Word:   w.Name,
		Row:    pos.Row,
		Col:    pos.Col,
	})
	n := sig.TotalArgs()
	positions := e.checkModeFallbackPositions(n)
	args := make([]Value, len(positions))
	for i, p := range positions {
		args[i] = e.stack[p]
	}
	results := carrierResults(e.registry, w.Name, sig, args, pos)
	e.spliceCheckResults(positions, results)
	return nil
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
		stackRemove(&e.stack, uniq[i])
	}
	stackSplice(&e.stack, insertAt, 0, results...)
	e.pointer = insertAt
}
