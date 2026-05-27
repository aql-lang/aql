package eng

import (
	"fmt"
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
	stepLimit int             // hard cap on the Run loop; always positive, set by the New/NewTop constructors below
	marks     map[string]bool // active mark IDs (for mark/move control flow)
	source    string          // original source text for error reporting
	isTop     bool            // true for engines created via NewTop; an unhandled FlowCtrl at end-of-Run is an error here, propagates upward otherwise
}

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
func (e *Engine) sigError(name string, fn *FnDefInfo) *AqlError {
	detail := "no matching signature for " + name

	// Build hint with available signatures and actual stack types.
	var hint strings.Builder
	if fn != nil && len(fn.Signatures) > 0 {
		hint.WriteString("expected: " + name + " " + describeAllSigs(fn))
	}
	if len(e.stack) > 0 {
		if hint.Len() > 0 {
			hint.WriteString("\n  = ")
		}
		hint.WriteString("stack: " + describeStackTypes(e.stack, e.pointer))
	}

	src := e.effectiveSource()
	return e.maybeAddFnShapeHint(makeAqlError("signature_error", detail, name, src, hint.String())).(*AqlError)
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
		if len(fwd.Sig.Args) < 2 || !fwd.Sig.Args[0].Equal(TMap) {
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

// insufficientArgsError builds a detailed AqlError for forward argument
// collection failure (not enough arguments after the word).
func (e *Engine) insufficientArgsError(name string, expected int) *AqlError {
	detail := fmt.Sprintf("insufficient arguments for %s (expected %d forward args)", name, expected)
	hint := "stack: " + describeStackTypes(e.stack, e.pointer)
	src := e.effectiveSource()
	return makeAqlError("signature_error", detail, name, src, hint)
}

// returnCountError builds a detailed AqlError for wrong number of return values.
func (e *Engine) returnCountError(funcName string, expected, got int) *AqlError {
	detail := fmt.Sprintf("%s: expected %d return value(s), got %d", funcName, expected, got)
	src := e.effectiveSource()
	return makeAqlError("type_error", detail, funcName, src, "")
}

// returnTypeError builds a detailed AqlError for a return type mismatch.
func (e *Engine) returnTypeError(funcName string, index int, expected *Type, got Value) *AqlError {
	detail := fmt.Sprintf("%s: return value %d: expected %s, got %s",
		funcName, index, expected, got.Parent)
	hint := "value: " + ValToString(got)
	src := e.effectiveSource()
	return makeAqlError("type_error", detail, funcName, src, hint)
}

// syntaxError builds a detailed AqlError for a syntax error.
func (e *Engine) syntaxError(msg, token string) *AqlError {
	src := e.effectiveSource()
	return makeAqlError("syntax_error", msg, token, src, "")
}

// runtimeError builds a detailed AqlError for a runtime error.
func (e *Engine) runtimeError(code, detail, word, hint string) *AqlError {
	src := e.effectiveSource()
	return makeAqlError(code, detail, word, src, hint)
}

// traceSigStr formats a signature as "name(type, type) prec=N" for trace annotations.
func traceSigStr(name string, sig *Signature) string {
	args := make([]string, len(sig.Args))
	for i, t := range sig.Args {
		args[i] = t.String()
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
func (e *Engine) Run(input []Value) ([]Value, error) {
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
			// ParenExpr values are only used inside maps (created by
			// the parser for paren groups in data context). They should
			// not appear on the main stack; skip if encountered.
			e.pointer++

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

// preEvalParens scans forward from the current pointer and evaluates any
// paren expressions in-place before signature matching. This implements
// rule 1.5: paren expressions are resolved to their results so that
// matchSignature sees fully evaluated values.
//
// maxFwd is the maximum number of forward values needed (FnDefInfo.MaxForwardArgs).
// The scan stops after finding maxFwd resolved values or hitting a boundary
// (function word, pipe, "end", ")").
func (e *Engine) preEvalParens(maxFwd int) error {
	if maxFwd <= 0 {
		return nil
	}
	resolved := 0
	scanIdx := e.pointer + 1

	for resolved < maxFwd && scanIdx < len(e.stack) {
		tok := e.stack[scanIdx]

		// Boundary conditions: stop scanning.
		if IsForward(tok) || tok.Parent.Matches(TMark) || tok.Parent.Matches(TMove) ||
			tok.Parent.Matches(TInternal) || tok.Parent.Matches(TReturnCheck) {
			break
		}

		// Boundary tokens: end / ) stop the pre-eval scan.
		if IsEnd(tok) || IsCloseParen(tok) {
			break
		}

		// Open paren: evaluate the sub-expression in-place.
		if IsOpenParen(tok) {
			savedPointer := e.pointer
			e.pointer = scanIdx

			// Advance past the OpenParen marker.
			if err := e.stepOpenParen(); err != nil {
				e.pointer = savedPointer
				return err
			}

			// Step through contents until we reach the matching ")".
			// Track paren depth so that inner parens (e.g. from fn
			// body expansion) are processed without prematurely
			// breaking on their ")" tokens.
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
				default:
					if err := e.stepLiteral(); err != nil {
						e.pointer = savedPointer
						return err
					}
				}
				// Propagate any flow-control signal raised by
				// the step; the outer Run frame will resolve it.
				if e.registry.FlowCtrl != FlowNone {
					e.pointer = savedPointer
					return nil
				}
			}

			e.pointer = savedPointer
			// The paren has been collapsed; the result value(s) are now
			// at scanIdx. Each result counts as a resolved value.
			// Count how many values replaced the paren expression.
			// We don't know exactly, but at least one was produced.
			// Just count the value at scanIdx as one resolved value.
			resolved++
			scanIdx++
			continue
		}

		if IsWord(tok) {
			ww, _ := AsWord(tok)
			// Function word: count as resolved (may be captured by
			// QuoteArgs/TWord matching). Don't stop — continue scanning
			// so that parens beyond function words are pre-evaluated
			// (e.g. undef foo (fn [...]) needs the paren evaluated).
			if e.registry.Lookup(ww.Name) != nil {
				resolved++
				scanIdx++
				continue
			}
		}

		// Any other token: count as one resolved value.
		resolved++
		scanIdx++
	}
	return nil
}

// stepWord handles a word (function reference) at the current pointer.
//
// The reserved tape-syntactic tokens — `(`, `)`, `end` — never reach
// here: the parser emits them as typed OpenParen / CloseParen / End
// values, and the Run-loop switch dispatches them directly. stepWord
// therefore deals only with regular named words.
func (e *Engine) stepWord(val Value) error {
	w, _ := AsWord(val)

	// /r modifier: resolve the name to its bound value as data, with no
	// argument collection or dispatch. An FnDef binding comes back as a
	// Quoted Function value so it sits on the stack like any other piece
	// of data — exactly the case `ref` exists to enable.
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
		v.Pos = val.Pos
		e.stack[e.pointer] = v
		return e.stepLiteral()
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

	// If a pending forward expects TFunction, resolve this word to a
	// function reference value rather than executing it. The word must
	// have a FnDef entry in DefStacks.
	if e.hasPendingForwardExpectingFunction() {
		stack := e.registry.Defs.Stack(w.Name)
		for i := len(stack) - 1; i >= 0; i-- {
			if fnDef, ok := stack[i].Data.(FnDefInfo); ok {
				e.stack[e.pointer] = NewFunction(fnDef)
				return e.stepLiteral()
			}
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
			// Record the substitution as a "use" for unused-def
			// tracking in check mode.
			e.registry.Check.recordUse(w.Name)
			// For list bodies, expand onto the stack like the fallback handler does.
			// Quoted lists are treated as data values (not expanded).
			// Type literals (Data == nil) are values, not bodies — they
			// fall through to stepLiteral so the type itself is pushed
			// onto the stack rather than splicing nothing.
			if top.Parent.Equal(TList) && top.Data != nil && !IsTypedList(top) && !IsTableType(top) && !top.Quoted {
				elems, _ := AsList(top)
				expanded := make([]Value, elems.Len())
				copy(expanded, elems.Slice())
				stackSplice(&e.stack, e.pointer, 1, expanded...)
				return nil
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
			return &AqlError{
				Code:       "undefined_word",
				Detail:     "undefined word: " + w.Name,
				Src:        w.Name,
				Row:        val.Pos.Row,
				Col:        val.Pos.Col,
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
		if err := e.preEvalParens(fn.MaxForwardArgs); err != nil {
			return err
		}
	}

	// Unified signature matching: one path for all words.
	resolved := e.effectiveResolved()
	sig, positions := e.matchSignature(fn, w, resolved)

	// Retry fallback for words with forward-collecting sigs: when
	// nearest-first matching fails, retry with deepest-first
	// (ForceStack). Handles CallAQL sub-engines where FnDef args are
	// placed in deepest-first order on the input stack.
	if sig == nil && fn.HasForwardSigs() && !w.ForceStack {
		wDeep := w
		wDeep.ForceStack = true
		sig, positions = e.matchSignature(fn, wDeep, resolved)
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
			return e.checkModeAssumeSig(w, fn, &fn.Signatures[0], val.Pos)
		}
		return e.sigError(w.Name, fn)
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
	// Forward collection needed: defer execution.
	if fwdCount > 0 {
		e.traceNote = "forward→ " + traceSigStr(w.Name, sig)
		return e.insertForward(w, sig, fwdCount, stkCount)
	}

	// Immediate execution: read args from recorded positions.
	match := &MatchResult{Sig: sig, Positions: positions}
	if stkCount > 0 {
		match.Args = make([]Value, stkCount)
		for i, pos := range positions {
			match.Args[i] = e.stack[pos]
		}
	}
	e.traceNote = "stack " + traceSigStr(w.Name, sig)
	return e.execMatch(match)
}

// execMatch executes a matched signature, splicing args and results.
func (e *Engine) execMatch(match *MatchResult) error {
	n := len(match.Sig.Args)

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
					if err == nil {
						match.Args[i] = evaluated
					}
				}
			} else if match.Args[i].Parent.Equal(TList) &&
				match.Args[i].Data != nil && !IsTypedList(match.Args[i]) && !IsTableType(match.Args[i]) {
				// NoEvalArgs suppresses list auto-evaluation for code-body
				// positions (def body, if branches, for body, etc.).
				noEval := match.Sig.NoEvalArgs != nil && match.Sig.NoEvalArgs[i]
				if !noEval {
					evaluated, err := e.autoEvalList(match.Args[i])
					if err == nil {
						match.Args[i] = evaluated
					}
				}
			}
		}
		match.Args[i].Eval = false
		match.Args[i].Undefined = false
	}

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
			return err
		}
		// FullStack handler returns the complete replacement for
		// everything from base through the pointer (inclusive).
		stackSplice(&e.stack, base, e.pointer+1-base, results...)
		e.pointer = base
		return nil
	}

	results, err := match.Sig.Handler(match.Args, ctx, nil, e.registry)
	if err != nil {
		return e.maybeAddFnShapeHint(err)
	}

	return e.spliceMatchResults(match, sortedIndices, n, results)
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
func (e *Engine) insertForward(w WordInfo, sig *Signature, forwardNeeded int, stackArgs ...int) error {
	pArgs := 0
	if len(stackArgs) > 0 {
		pArgs = stackArgs[0]
	}
	fwd := NewForward(ForwardInfo{
		FuncName:     w.Name,
		ExpectedArgs: forwardNeeded,
		StackArgs:    pArgs,
		FuncIndex:    e.pointer,
		Sig:          sig,
	})

	stackInsert(&e.stack, e.pointer+1, fwd)

	e.pointer += 2
	return nil
}

// stepLiteral handles a resolved (non-word, non-forward) value at the pointer.
func (e *Engine) stepLiteral() error {
	valIdx := e.pointer

	// Look backwards for the nearest forward entry, stopping at open-paren barriers.
	fwdIdx := -1
	for i := valIdx - 1; i >= 0; i-- {
		if IsOpenParen(e.stack[i]) {
			break
		}
		if IsForward(e.stack[i]) {
			fwdIdx = i
			break
		}
	}

	if fwdIdx < 0 {
		// If the value is a FnDef/TFunction, execute it. Quoted function
		// values are treated as data (not executed).
		val := e.stack[valIdx]
		if (val.Parent.Equal(TFnDef) || val.Parent.Equal(TFunction)) &&
			val.Data != nil && !val.Quoted {
			if _, ok := val.Data.(FnDefInfo); ok {
				return e.execFnDefLiteral(valIdx)
			}
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
			val.Parent.Equal(TWord) && TAtom.Matches(fwd.Sig.Args[nextIdx]) {
			w, _ := AsWord(val)
			e.stack[valIdx] = NewAtom(w.Name)
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
			e.stack[funcIdx] = NewWordModified(w.Name, w.ArgCount, true, false)
		}

		// Rearrange values for forward-first matching: forward args at
		// the deep end (sigArgs[0..F-1]), stack args reversed after them.
		e.pointer = funcIdx
		e.rearrangeForForward(fwd.StackArgs, fwd.CollectedArgs)
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
				if keyResult[0].Parent.Matches(TString) {
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

		// Paren expression: evaluate items with paren markers so the
		// engine's stepCloseParen collapses to a single result.
		if IsParenExpr(v) {
			items, _ := AsParenExpr(v)
			sub := New(e.registry)
			input := make([]Value, 0, len(items)+2)
			input = append(input, NewOpenParen())
			input = append(input, items...)
			input = append(input, NewCloseParen())
			result, err := sub.Run(input)
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

	// Resolve compiled signatures. A captured Function value is a
	// STABLE handle to the fn it was minted from: we prefer the
	// captured FnDef's own Signatures over a fresh registry lookup so
	// that `undef foo; def foo …` doesn't change the meaning of a
	// previously-captured value. Word-driven dispatch (in stepWord)
	// still re-resolves through the registry every call, which is
	// what recursive self-references rely on.
	var fn *FnDefInfo
	if len(fnDef.Signatures) > 0 {
		captured := fnDef
		fn = &captured
	}
	if fn == nil && fnDef.Name != "" {
		reg := fnDef.Registry
		if reg == nil {
			reg = e.registry
		}
		fn = reg.Lookup(fnDef.Name)
	}
	if fn == nil && len(fnDef.Sigs) > 0 {
		synth := fnSigsToSignatures(fnDef.Sigs)
		fn = &FnDefInfo{
			Name:           fnDef.Name,
			Signatures:     synth,
			MaxForwardArgs: calcMaxForwardArgs(synth),
			Registry:       fnDef.Registry,
		}
	}
	if fn == nil {
		e.pointer++
		return nil
	}

	w := WordInfo{Name: fnDef.Name, ArgCount: -1}

	// Pre-evaluate paren expressions in the forward scan range so that
	// matchSignature sees fully resolved values. Mirrors stepWord's
	// pre-eval pass — the unified rule says Function values at the
	// pointer dispatch with the same forward+stack matching as words.
	if (fn.HasForwardSigs() && !w.ForceStack) || w.ForceForward {
		if err := e.preEvalParens(fn.MaxForwardArgs); err != nil {
			return err
		}
	}

	resolved := e.effectiveResolved()
	sig, positions := e.matchSignature(fn, w, resolved)

	// Retry fallback for words with forward-collecting sigs: when
	// nearest-first matching fails, retry with deepest-first
	// (ForceStack). Mirrors stepWord's CallAQL-input recovery.
	if sig == nil && fn.HasForwardSigs() && !w.ForceStack {
		wDeep := w
		wDeep.ForceStack = true
		sig, positions = e.matchSignature(fn, wDeep, resolved)
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
	if fnDef.Anonymous && fwdCount == 0 && len(positions) == 0 {
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
		return e.insertForward(w, sig, fwdCount, stkCount)
	}

	// All args resolved on the stack. Anonymous FnDefs (no Go
	// Handler) take the legacy stack-match path, which splices the
	// body via execFnDefSig and binds named params via def-stack.
	if sig.Handler == nil {
		return e.execFnDefSigStackMatch(valIdx, fnDef, resolved)
	}

	// Module closures: with all args now on the stack (either because
	// no forward collection was needed, or after the forward-completion
	// re-dispatch), route through execFnDefSig with the captured
	// registry. CallAQL runs the body in a sub-engine on modReg so
	// the fn's def-body word lookups resolve in its own scope.
	if fnDef.Registry != nil && fnDef.Registry != e.registry && len(fnDef.Sigs) > 0 {
		return e.execFnDefSigStackMatch(valIdx, fnDef, resolved)
	}

	// Pure-stack match: dispatch via execMatch the same way a bare
	// word with no forward args would.
	match := &MatchResult{Sig: sig, Positions: positions}
	if len(positions) > 0 {
		match.Args = make([]Value, len(positions))
		for i, pos := range positions {
			match.Args[i] = e.stack[pos]
		}
	}
	return e.execMatch(match)
}

// execFnDefSigStackMatch is the legacy FnSig-based pure-stack
// dispatch path for AQL-defined functions whose Sigs carry named
// params. Used as a fallback when matchSignature's Signatures-based
// match returns nothing.
func (e *Engine) execFnDefSigStackMatch(valIdx int, fnDef FnDefInfo, resolved []Value) error {
	resolvedIdx := e.resolvedIndicesBefore(len(resolved))
	checkMode := e.registry != nil && e.registry.Check.Mode && fnDef.Anonymous
	for i := range fnDef.Sigs {
		sig := &fnDef.Sigs[i]
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

// fnSigsToSignatures converts FnSig params into Signature objects for the
// forward planner. Used for anonymous functions that have no registered name.
func fnSigsToSignatures(sigs []FnSig) []Signature {
	out := make([]Signature, len(sigs))
	for i, sig := range sigs {
		argTypes := make([]*Type, len(sig.Params))
		var patterns map[int]Value
		for j, p := range sig.Params {
			argTypes[j] = p.Type
			if p.Pattern != nil {
				if patterns == nil {
					patterns = make(map[int]Value)
				}
				patterns[j] = *p.Pattern
			}
		}
		// Resolve the FnSig BarrierPos sentinel: -1 means "use the
		// all-forward default" (the same defaulting RegisterNative
		// applies to registered fns). 0 means explicit all-stack
		// from a leading `|`; >0 is an explicit boundary. All Go-
		// side FnSig{} construction sites set BarrierPos: -1 so
		// they reach the dispatcher with the correct default.
		barrier := sig.BarrierPos
		if barrier == -1 {
			barrier = len(argTypes)
		}
		out[i] = Signature{Args: argTypes, Patterns: patterns, BarrierPos: barrier}
	}
	SortSignatures(out)
	return out
}

// execFnDefSig executes a matched FnDef signature. If capturedReg is non-nil
// (module closure), execution uses CallAQL on that registry. Otherwise, body
// tokens are spliced into the current engine's stack.
func (e *Engine) execFnDefSig(valIdx int, sig *FnSig, args []Value, capturedReg *Registry) error {
	nArgs := len(sig.Params)
	indices := e.resolvedIndicesBefore(nArgs)

	// Auto-evaluate consumed arguments with Eval=true so FnDef handlers
	// receive resolved data. Maps: {base:hex} → {base:atom(hex)}.
	// Lists: [c1 c2] → [map1, map2].
	for i := range args {
		if args[i].Eval && !args[i].Quoted {
			if args[i].Parent.Equal(TMap) &&
				args[i].Data != nil && !IsTypedMap(args[i]) && !IsRecordType(args[i]) && !IsOptionsType(args[i]) {
				evaluated, err := e.autoEvalMap(args[i])
				if err == nil {
					args[i] = evaluated
				}
			} else if args[i].Parent.Equal(TList) &&
				args[i].Data != nil && !IsTypedList(args[i]) && !IsTableType(args[i]) {
				evaluated, err := e.autoEvalList(args[i])
				if err == nil {
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

// stepEnd handles the "end" keyword.
func (e *Engine) stepEnd() error {
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
			return e.insufficientArgsError(fwd.FuncName, fwd.ExpectedArgs)
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
				return e.returnCountError(rc.FuncName, nret, len(results))
			}
			extra := len(results) - nret
			if extra > rc.UnnamedCount {
				return e.returnCountError(rc.FuncName, nret, len(results)-rc.UnnamedCount)
			}

			// Validate the top nret values match declared return types.
			for k, exp := range rc.Returns {
				if !results[extra+k].Parent.Matches(exp) {
					return e.returnTypeError(rc.FuncName, k+1, exp, results[extra+k])
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
			e.stack[funcIdx] = NewWordModified(w.Name, w.ArgCount, true, false)
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
	e.stack[funcIdx] = NewWordModified(w.Name, w.ArgCount, true, false)
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
			if nextIdx < len(fwd.Sig.Args) {
				return fwd.Sig.QuoteArgs != nil && fwd.Sig.QuoteArgs[nextIdx]
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
			if nextIdx < len(fwd.Sig.Args) {
				return fwd.Sig.Args[nextIdx].Equal(TFunction)
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
// Returns: matched signature and arg positions (absolute stack indices
// in signature order). Positions > pointer are forward args that need
// deferred collection. Positions < pointer are stack args. Returns nil
// sig if no signature matches.
//
//nolint:gocyclo,gocognit // dispatch is inherently a big switch; see STATIC_ANALYSIS_REPORT.md
func (e *Engine) matchSignature(fn *FnDefInfo, w WordInfo, resolved []Value) (*Signature, []int) {
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

		nArgs := len(sig.Args)

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
		fwd := 0 // number of params matched by forward tokens

		// Always run the forward scan up to forwardLimit; if it's 0
		// the loop simply doesn't execute and all args come from
		// the stack below.
		{
			scanIdx := e.pointer + 1

			// One inner loop over parameters, matching forward tokens.
			for fwd < forwardLimit && scanIdx < len(e.stack) {

				tok := e.stack[scanIdx]
				expectedType := sig.Args[fwd]

				// 1.4: structural boundaries — stop forward scan.
				if IsForward(tok) || tok.Parent.Matches(TMark) || tok.Parent.Matches(TMove) ||
					tok.Parent.Matches(TInternal) || tok.Parent.Matches(TReturnCheck) {
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

				if IsWord(tok) {
					ww, _ := AsWord(tok)
					// /q modifier: capture the upcoming Word as an Atom
					// (the conversion happens at insertForward / stepLiteral
					// time; here we just count it as a match).
					if sig.QuoteArgs != nil && sig.QuoteArgs[fwd] {
						if TAtom.Matches(expectedType) {
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
			if !patternsOk(sig, positions, e.stack, fwd) {
				continue
			}
			if preferWordSig && !isPreferred {
				if bestDeferred == nil {
					bestDeferred = &matchResult{sig, append([]int(nil), positions...)}
				}
				continue
			}
			return sig, positions
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
							bestDeferred = &matchResult{sig, append([]int(nil), positions...)}
						}
						continue
					}
					return sig, positions
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
				if !TAtom.Matches(sig.Args[sigIdx]) {
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
			if !isTypeArg && rejectsTypeLiteral(stackVal, sig.Args[sigIdx]) {
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
		if !patternsOk(sig, positions, e.stack, fwd) {
			continue
		}

		// Full match found.
		if preferWordSig && !isPreferred {
			if bestDeferred == nil {
				bestDeferred = &matchResult{sig, append([]int(nil), positions...)}
			}
			continue
		}
		return sig, positions
	}

	// Return deferred non-preferred match if one was found.
	if bestDeferred != nil {
		return bestDeferred.sig, bestDeferred.positions
	}

	// Try fallback (0-arg or Fallback handler).
	for si := range fn.Signatures {
		sig := &fn.Signatures[si]
		if w.ArgCount >= 0 && sig.TotalArgs() != w.ArgCount {
			continue
		}
		if len(sig.Args) == 0 || sig.Fallback {
			return sig, nil
		}
	}

	return nil, nil
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
		n := len(s.Args)
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
	n := len(sig.Args)
	positions := e.checkModeFallbackPositions(n)
	args := make([]Value, len(positions))
	for i, p := range positions {
		args[i] = e.stack[p]
	}
	results := carrierResults(e.registry, w.Name, sig, args, pos)

	// Remove the word and any consumed positions, then splice results
	// in at the word's slot. We rely on ascending order for removal.
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
	return nil
}
