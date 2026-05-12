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
type Engine struct {
	stack     []Value
	pointer   int
	registry  *Registry
	trace     TraceCallback
	traceNote string          // annotation set during execution for the next trace call
	stepLimit int             // 0 means use default (22222 for top-level, 2222 for sub-engines)
	marks     map[string]bool // active mark IDs (for mark/move control flow)
	source    string          // original source text for error reporting
}

// New creates an Engine with the given function registry.
// The returned engine uses the sub-engine step limit (2222).
// Use NewTop for the top-level engine with a higher limit (22222).
func New(registry *Registry) *Engine {
	return &Engine{registry: registry, stepLimit: 2222}
}

// NewTop creates a top-level Engine with the maximum step limit (22222).
func NewTop(registry *Registry) *Engine {
	return &Engine{registry: registry, stepLimit: 22222}
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
		if e.stack[i].IsOpenParen() {
			return false
		}
		if !e.stack[i].IsForward() {
			continue
		}
		fwd, _ := e.stack[i].AsForward()
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
		m := e.stack[mapIdx].AsMap()
		if m == nil || m.Len() != 1 {
			return false
		}
		constraint, _ := m.Get(m.Keys()[0])
		if constraint.IsWord() {
			cw, _ := constraint.AsWord()
			if tv, ok := e.registry.ResolveTypedName(cw.Name); ok {
				constraint = tv
			}
		}
		return constraint.VType.Equal(TFnUndef)
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
		funcName, index, expected, got.VType)
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

// stackInsert inserts val at index i, shifting elements right.
// Only allocates when capacity is exhausted.
func (e *Engine) stackInsert(i int, val Value) {
	e.stack = append(e.stack, Value{})
	copy(e.stack[i+1:], e.stack[i:len(e.stack)-1])
	e.stack[i] = val
}

// stackRemove removes the element at index i, shifting elements left.
// Zeroes the freed slot to release interface references.
func (e *Engine) stackRemove(i int) {
	copy(e.stack[i:], e.stack[i+1:])
	e.stack[len(e.stack)-1] = Value{}
	e.stack = e.stack[:len(e.stack)-1]
}

// stackSplice removes count elements starting at index i and inserts
// replacements in their place. Only allocates when net growth exceeds capacity.
func (e *Engine) stackSplice(i, count int, replacements ...Value) {
	delta := len(replacements) - count
	oldLen := len(e.stack)
	newLen := oldLen + delta

	if delta > 0 {
		// Grow: ensure capacity, then shift tail right.
		for cap(e.stack) < newLen {
			e.stack = append(e.stack, Value{})
		}
		e.stack = e.stack[:newLen]
		copy(e.stack[i+len(replacements):], e.stack[i+count:oldLen])
	} else if delta < 0 {
		// Shrink: shift tail left, zero freed slots.
		copy(e.stack[i+len(replacements):], e.stack[i+count:])
		for j := newLen; j < oldLen; j++ {
			e.stack[j] = Value{}
		}
		e.stack = e.stack[:newLen]
	}
	copy(e.stack[i:], replacements)
}

// Run executes the input values through the stack machine and returns the
// resulting stack.
func (e *Engine) Run(input []Value) ([]Value, error) {
	// Push a scoped context Store whose prototype is the parent context.
	parent := e.registry.ContextStore()
	e.registry.PushContext(parent)
	defer e.registry.PopContext()

	// In static type-check mode, convert concrete literal values to
	// carriers before execution. The same dispatch/matching machinery
	// then runs over carrier values; execMatch short-circuits handler
	// calls to push carrier return values declared on the signature.
	if e.registry.IsCheckMode() {
		input = StripToCarriers(input)
	}

	if cap(e.stack) >= len(input) {
		e.stack = e.stack[:len(input)]
	} else {
		e.stack = make([]Value, len(input), len(input)+stackHeadroom)
	}
	copy(e.stack, input)
	e.pointer = 0

	limit := e.stepLimit
	if limit <= 0 {
		limit = 22222
	}
	for step := 0; step < limit; step++ {
		if e.pointer >= len(e.stack) {
			break
		}

		// Check-mode global step budget: abort the whole run
		// gracefully once exceeded. Emits one diagnostic and
		// then short-circuits every subsequent sub-engine too.
		if e.registry.IsCheckMode() {
			budget := e.registry.Check.StepBudget
			if budget == 0 {
				budget = DefaultCheckStepBudget
			}
			e.registry.Check.StepCount++
			if e.registry.Check.StepCount > budget {
				if !e.registry.Check.BudgetTripped {
					e.registry.Check.BudgetTripped = true
					e.registry.AddCheckDiagnostic(CheckDiagnostic{
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
		case val.IsWord():
			if err := e.stepWord(val); err != nil {
				if IsBreak(err) {
					if e.handleLoopBreak() {
						continue
					}
				}
				if IsContinue(err) {
					if e.handleLoopContinue() {
						continue
					}
				}
				return nil, err
			}

		case val.IsForward():
			e.pointer++

		case val.IsOpenParen():
			e.pointer++

		case val.IsParenExpr():
			// ParenExpr values are only used inside maps (created by
			// the parser for paren groups in data context). They should
			// not appear on the main stack; skip if encountered.
			e.pointer++

		case val.IsInterpString():
			result, err := e.evalInterpString(val)
			if err != nil {
				return nil, err
			}
			// Replace with the evaluated string but do NOT advance the
			// pointer. The resulting string value needs to go through
			// stepLiteral so forward collection works correctly.
			e.stack[e.pointer] = result

		case val.IsMark():
			e.stepMark(val)

		case val.IsMove():
			if err := e.stepMove(val); err != nil {
				return nil, err
			}

		case val.IsReturnCheck():
			e.pointer++

		case val.IsDefCleanup():
			e.stepDefCleanup(val)
			e.pointer++

		default:
			if val.VType.Equal(nil) {
				return nil, e.runtimeError("halt", fmt.Sprintf("undefined stack entry at position %d", e.pointer), "", "")
			}
			if err := e.stepLiteral(); err != nil {
				return nil, err
			}
		}
	}

	// Implicit end-of-input: resolve any pending forwards from the stack.
	if err := e.resolveOrphanedForwards(); err != nil {
		return nil, err
	}

	for _, v := range e.stack {
		if v.IsOpenParen() {
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
		if e.registry.IsCheckMode() {
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
			if v.IsForward() {
				fwdIdx = i
				break
			}
		}
		if fwdIdx < 0 {
			return nil
		}

		fwd, _ := e.stack[fwdIdx].AsForward()
		funcIdx := fwd.FuncIndex
		collectedCount := fwd.CollectedArgs
		stackArgCount := fwd.StackArgs

		// Remove the forward marker.
		e.stackRemove(fwdIdx)
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
			case val.IsWord():
				if err := e.stepWord(val); err != nil {
					return err
				}
			case val.IsForward():
				e.pointer++
			case val.IsOpenParen():
				e.pointer++
			default:
				if err := e.stepLiteral(); err != nil {
					return err
				}
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
		if tok.IsForward() || tok.VType.Matches(TMark) || tok.VType.Matches(TMove) ||
			tok.VType.Matches(TInternal) || tok.VType.Matches(TReturnCheck) {
			break
		}

		if tok.IsWord() {
			ww, _ := tok.AsWord()
			if ww.Name == "end" || ww.Name == ")" {
				break
			}

			// Open paren: evaluate the sub-expression in-place.
			if ww.Name == "(" {
				savedPointer := e.pointer
				e.pointer = scanIdx

				// stepOpenParen converts "(" to OpenParen marker.
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

					// Track depth changes from open/close parens.
					if v.IsOpenParen() {
						depth++
						e.pointer++
						continue
					}
					// Also catch word("(") not yet converted to OpenParen.
					_as0, _ := v.AsWord()
					if v.IsWord() && _as0.Name == "(" {
						depth++
						_ = e.stepOpenParen() // converts to OpenParen and advances pointer; never errors
						continue
					}
					_as1, _ := v.AsWord()
					if v.IsWord() && _as1.Name == ")" {
						depth--
						if depth == 0 {
							// This is the matching ")" for our paren.
							if err := e.stepCloseParen(); err != nil {
								e.pointer = savedPointer
								return err
							}
							break
						}
						// Inner ")" — process normally.
						if err := e.stepCloseParen(); err != nil {
							e.pointer = savedPointer
							return err
						}
						continue
					}

					// Normal evaluation inside paren.
					switch {
					case v.IsWord():
						if err := e.stepWord(v); err != nil {
							e.pointer = savedPointer
							return err
						}
					case v.IsMark():
						e.stepMark(v)
					case v.IsMove():
						if err := e.stepMove(v); err != nil {
							e.pointer = savedPointer
							return err
						}
					case v.IsForward():
						e.pointer++
					case v.IsReturnCheck():
						e.pointer++
					case v.IsDefCleanup():
						e.stepDefCleanup(v)
						e.pointer++
					default:
						if err := e.stepLiteral(); err != nil {
							e.pointer = savedPointer
							return err
						}
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
func (e *Engine) stepWord(val Value) error {
	w, _ := val.AsWord()

	if w.Name == "end" {
		return e.stepEnd()
	}
	if w.Name == "(" {
		return e.stepOpenParen()
	}
	if w.Name == ")" {
		return e.stepCloseParen()
	}

	// If there is a pending forward whose next expected argument is TWord,
	// collect this word as-is rather than executing it. This lets words
	// like def, undef, and var receive word names even for already-defined
	// words (e.g. "undef foo" when foo is defined).
	if e.hasPendingForwardExpectingWord() {
		return e.stepLiteral()
	}

	// If a pending forward expects TFunction, resolve this word to a
	// function reference value rather than executing it. The word must
	// have a FnDef entry in DefStacks.
	if e.hasPendingForwardExpectingFunction() {
		stack := e.registry.DefStack(w.Name)
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
		if tv, ok := e.registry.TopOfTypeStack(w.Name); ok {
			push := tv
			if push.VType.Equal(TFnDef) || push.VType.Equal(TFunction) {
				push.Quoted = true
			}
			push.Pos = val.Pos
			e.stack[e.pointer] = push
			return e.stepLiteral()
		}
	}

	// Simple value def: substitute the word with its value directly,
	// bypassing function dispatch entirely. FnDefInfo and ObjectTypeInfo
	// entries are not simple values — they go through normal Lookup.
	if top, ok := e.registry.TopOfDefStack(w.Name); ok {
		switch top.Data.(type) {
		case FnDefInfo, *ObjectTypeInfo:
			// Not a simple value — fall through to Lookup.
		default:
			// Record the substitution as a "use" for unused-def
			// tracking in check mode.
			e.registry.recordCheckUse(w.Name)
			// For list bodies, expand onto the stack like the fallback handler does.
			// Quoted lists are treated as data values (not expanded).
			// Type literals (Data == nil) are values, not bodies — they
			// fall through to stepLiteral so the type itself is pushed
			// onto the stack rather than splicing nothing.
			if top.VType.Equal(TList) && top.Data != nil && !top.IsTypedList() && !top.IsTableType() && !top.Quoted {
				elems := top.AsList()
				expanded := make([]Value, elems.Len())
				copy(expanded, elems.Slice())
				e.stackSplice(e.pointer, 1, expanded...)
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
		e.registry.recordCheckUse(w.Name)
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
		if !e.registry.IsCheckMode() {
			return &AqlError{
				Code:       "undefined_word",
				Detail:     "undefined word: " + w.Name,
				Src:        w.Name,
				Row:        val.Pos.Row,
				Col:        val.Pos.Col,
				fullSource: e.effectiveSource(),
			}
		}
		e.registry.AddCheckDiagnostic(CheckDiagnostic{
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
	if sig != nil && sig.Fallback && e.registry.IsCheckMode() {
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
		if e.registry.IsCheckMode() && len(fn.Signatures) > 0 {
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
			if match.Args[i].VType.Equal(TMap) &&
				match.Args[i].Data != nil && !match.Args[i].IsTypedMap() && !match.Args[i].IsRecordType() && !match.Args[i].IsOptionsType() {
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
			} else if match.Args[i].VType.Equal(TList) &&
				match.Args[i].Data != nil && !match.Args[i].IsTypedList() && !match.Args[i].IsTableType() {
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
	if e.registry.IsCheckMode() && !match.Sig.RunInCheckMode {
		name := ""
		var pos SrcPos
		if e.pointer < len(e.stack) && e.stack[e.pointer].IsWord() {
			pos = e.stack[e.pointer].Pos
			if w, err := e.stack[e.pointer].AsWord(); err == nil {
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
				if e.stack[i].IsOpenParen() {
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
			e.stackSplice(base, end+1-base, results...)
			e.pointer = base
			return nil
		}

		results := carrierResults(e.registry, name, match.Sig, match.Args, pos)
		return e.spliceMatchResults(match, sortedIndices, n, results)
	}

	// Compute context (cheap O(1) call).
	ctx := e.registry.Context()

	var fullStack []Value
	if match.Sig.FullStack {
		// Find the nearest open-paren barrier so that FullStack handlers
		// only replace within the current paren scope, not below it.
		base := 0
		for i := e.pointer - 1; i >= 0; i-- {
			if e.stack[i].IsOpenParen() {
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
		e.stackSplice(base, e.pointer+1-base, results...)
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
	hint := "this is a typed-binding context expecting a function value — did you mean `(quote " + aqlErr.Src + ")`?"
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
		e.stackSplice(dst, e.pointer+1-dst, results...)
		e.pointer = firstArgIdx
	} else if n == 0 {
		// No args, just replace the word with results.
		e.stackSplice(e.pointer, 1, results...)
		// Pointer stays at same position to re-examine results.
	} else {
		// Fallback: simple contiguous splice.
		argStart := e.pointer - n
		if argStart < 0 {
			argStart = 0
		}
		e.stackSplice(argStart, e.pointer+1-argStart, results...)
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
		if e.stack[i].IsOpenParen() {
			break
		}
		if e.stack[i].IsForward() || e.stack[i].IsMark() || e.stack[i].IsMove() {
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
		if exclude[i] || e.stack[i].IsForward() || e.stack[i].IsOpenParen() || e.stack[i].IsMark() || e.stack[i].IsMove() {
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

	e.stackInsert(e.pointer+1, fwd)

	e.pointer += 2
	return nil
}

// stepLiteral handles a resolved (non-word, non-forward) value at the pointer.
func (e *Engine) stepLiteral() error {
	valIdx := e.pointer

	// Look backwards for the nearest forward entry, stopping at open-paren barriers.
	fwdIdx := -1
	for i := valIdx - 1; i >= 0; i-- {
		if e.stack[i].IsOpenParen() {
			break
		}
		if e.stack[i].IsForward() {
			fwdIdx = i
			break
		}
	}

	if fwdIdx < 0 {
		// If the value is a FnDef/TFunction, execute it. Quoted function
		// values are treated as data (not executed).
		val := e.stack[valIdx]
		if (val.VType.Equal(TFnDef) || val.VType.Equal(TFunction)) &&
			val.Data != nil && !val.Quoted {
			if _, ok := val.Data.(FnDefInfo); ok {
				return e.execFnDefLiteral(valIdx)
			}
		}
		e.pointer++
		return nil
	}

	fwd, _ := e.stack[fwdIdx].AsForward()
	funcIdx := fwd.FuncIndex

	// Check if the value matches the next expected arg positionally.
	// Once matchSignature has chosen a signature, args are collected in
	// order — no permutation or sig switching is permitted.
	if fwd.CollectedArgs < fwd.ExpectedArgs {
		val := e.stack[valIdx]
		nextIdx := fwd.CollectedArgs
		matches := sigTypeMatches(val, fwd.Sig.Args[nextIdx])
		if !matches && fwd.Sig.QuoteArgs != nil && fwd.Sig.QuoteArgs[nextIdx] &&
			val.VType.Equal(TWord) && TAtom.Matches(fwd.Sig.Args[nextIdx]) {
			matches = true
		}
		if !matches {
			// Type mismatch — implicit end: resolve forward from stack.
			return e.implicitEnd(fwdIdx)
		}
	}

	// Remove the value from its current position.
	val := e.stack[valIdx]
	e.stackRemove(valIdx)

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

	e.stackInsert(insertIdx, val)

	funcIdx++
	fwdIdx++

	fwd.CollectedArgs++
	fwd.FuncIndex = funcIdx

	e.traceNote = fmt.Sprintf("collect %s %d/%d",
		fwd.FuncName, fwd.CollectedArgs, fwd.ExpectedArgs)

	if fwd.CollectedArgs >= fwd.ExpectedArgs {
		// All forward args collected. Remove forward, force stack, retry.
		e.stackRemove(fwdIdx)
		// Adjust funcIdx if forward was before it (shouldn't normally happen).
		if fwdIdx < funcIdx {
			funcIdx--
		}

		if funcIdx < len(e.stack) && e.stack[funcIdx].IsWord() {
			w, _ := e.stack[funcIdx].AsWord()
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
		if val.VType.Equal(TList) && val.Data != nil && !val.IsTypedList() && !val.IsTableType() {
			result, err := e.autoEvalList(val)
			if err != nil {
				return err
			}
			e.stack[i] = result
		} else if val.VType.Equal(TMap) && val.Data != nil && !val.IsTypedMap() && !val.IsRecordType() && !val.IsOptionsType() {
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
	elems := val.AsList()
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
	parts := val.AsInterpString()
	if parts == nil {
		return NewString(""), nil
	}
	var buf strings.Builder
	for _, part := range parts {
		if part.Expr == nil {
			buf.WriteString(part.Lit)
		} else {
			sub := New(e.registry)
			result, err := sub.Run(part.Expr)
			if err != nil {
				return Value{}, err
			}
			for _, r := range result {
				buf.WriteString(ValToString(r))
			}
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
	m := val.AsMutableMap()
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
				if keyResult[0].VType.Matches(TString) {
					resolvedKey, _ = keyResult[0].AsString()
				} else if keyResult[0].IsAtom() {
					resolvedKey, _ = keyResult[0].AsAtom()
				} else {
					resolvedKey = ValToString(keyResult[0])
				}
			}
		}

		// Interpolated string: evaluate inline.
		if v.IsInterpString() {
			result, err := e.evalInterpString(v)
			if err != nil {
				return Value{}, err
			}
			out.Set(resolvedKey, result)
			continue
		}

		// Paren expression: evaluate items with paren markers so the
		// engine's stepCloseParen collapses to a single result.
		if v.IsParenExpr() {
			items := v.AsParenExpr()
			sub := New(e.registry)
			input := make([]Value, 0, len(items)+2)
			input = append(input, NewOpenParen())
			input = append(input, items...)
			input = append(input, NewWord(")"))
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

	// Look up compiled signatures. Named functions use the registry;
	// anonymous/unregistered functions build signatures from FnSig params.
	var fn *FnDefInfo
	if fnDef.Name != "" {
		reg := fnDef.Registry
		if reg == nil {
			reg = e.registry
		}
		fn = reg.Lookup(fnDef.Name)
	}
	if fn == nil && len(fnDef.Sigs) > 0 {
		fn = &FnDefInfo{
			Name:       fnDef.Name,
			Signatures: fnSigsToSignatures(fnDef.Sigs),
		}
	}
	if fn == nil {
		e.pointer++
		return nil
	}

	resolved := e.effectiveResolved()
	w := WordInfo{Name: fnDef.Name, ArgCount: -1}

	// Use matchSignature for forward collection only.
	matchedSig, positions := e.matchSignature(fn, w, resolved)

	if matchedSig != nil {
		// Count forward vs stack args from positions.
		fwdCount := 0
		for _, pos := range positions {
			if pos > e.pointer {
				fwdCount++
			}
		}

		if fwdCount > 0 {
			stkCount := len(positions) - fwdCount
			return e.insertForward(w, matchedSig, fwdCount, stkCount)
		}
	}

	// Pure stack match against FnSig params. Two strategies:
	// - Unnamed params (matrix-style): deepest-first so CallAQL's token
	//   push order matches the registered handler's nearest-first matching.
	// - Named params (decision-style): nearest-first because CallAQL
	//   installs them as defs and the body's reversal expects this order.
	resolvedIdx := e.resolvedIndicesBefore(len(resolved))
	for i := range fnDef.Sigs {
		sig := &fnDef.Sigs[i]
		nArgs := len(sig.Params)
		if nArgs == 0 {
			return e.execFnDefSig(valIdx, sig, nil, fnDef.Registry)
		}
		if len(resolved) < nArgs {
			continue
		}

		// Determine ordering: named params → nearest-first, unnamed → deepest-first.
		hasNamed := false
		for _, p := range sig.Params {
			if p.Name != "" {
				hasNamed = true
				break
			}
		}

		match := true
		if hasNamed {
			// Nearest-first: top-of-stack → sig[0].
			for j, p := range sig.Params {
				ri := len(resolved) - 1 - j
				if !sigTypeMatches(resolved[ri], p.Type) {
					match = false
					break
				}
				if p.Pattern != nil {
					pat := *p.Pattern
					if pat.VType.Equal(TMap) && resolved[ri].VType.Equal(TMap) &&
						pat.Data != nil && resolved[ri].Data != nil &&
						!pat.IsOptionsType() {
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
				return e.execFnDefSig(valIdx, sig, args, fnDef.Registry)
			}
		} else {
			// Deepest-first: bottom-of-resolved → sig[0].
			candidate := resolved[len(resolved)-nArgs:]
			for j, p := range sig.Params {
				if !sigTypeMatches(candidate[j], p.Type) {
					match = false
					break
				}
				if p.Pattern != nil {
					pat := *p.Pattern
					if pat.VType.Equal(TMap) && candidate[j].VType.Equal(TMap) &&
						pat.Data != nil && candidate[j].Data != nil &&
						!pat.IsOptionsType() {
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
				return e.execFnDefSig(valIdx, sig, args, fnDef.Registry)
			}
		}
	}

	e.pointer++
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
		out[i] = Signature{Args: argTypes, Patterns: patterns, BarrierPos: sig.BarrierPos}
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
			if args[i].VType.Equal(TMap) &&
				args[i].Data != nil && !args[i].IsTypedMap() && !args[i].IsRecordType() && !args[i].IsOptionsType() {
				evaluated, err := e.autoEvalMap(args[i])
				if err == nil {
					args[i] = evaluated
				}
			} else if args[i].VType.Equal(TList) &&
				args[i].Data != nil && !args[i].IsTypedList() && !args[i].IsTableType() {
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
		result, err := capturedReg.CallAQL(sig, args)
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
			e.stackSplice(dst, valIdx+1-dst, result...)
			e.pointer = firstArgIdx
		} else if nArgs == 0 {
			e.stackSplice(valIdx, 1, result...)
		} else {
			argStart := valIdx - nArgs
			if argStart < 0 {
				argStart = 0
			}
			e.stackSplice(argStart, valIdx+1-argStart, result...)
			e.pointer = argStart
		}
		return nil
	}

	// No captured registry — splice body tokens into the current stack.
	var tokens []Value
	tokens = append(tokens, NewOpenParen())

	argsCopy := make([]Value, len(args))
	copy(argsCopy, args)
	e.registry.PushArgs(NewList(argsCopy))

	var names []string
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
	tokens = append(tokens, NewWord(")"))

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
		e.stackSplice(dst, valIdx+1-dst, tokens...)
		e.pointer = firstArgIdx
	} else if nArgs == 0 {
		e.stackSplice(valIdx, 1, tokens...)
	} else {
		argStart := valIdx - nArgs
		if argStart < 0 {
			argStart = 0
		}
		e.stackSplice(argStart, valIdx+1-argStart, tokens...)
		e.pointer = argStart
	}

	return nil
}

// implicitEnd resolves a forward early when a type mismatch occurs.
func (e *Engine) implicitEnd(fwdIdx int) error {
	fwd, _ := e.stack[fwdIdx].AsForward()
	funcIdx := fwd.FuncIndex
	collectedCount := fwd.CollectedArgs
	stackArgCount := fwd.StackArgs

	e.stackRemove(fwdIdx)
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
		if e.stack[i].IsOpenParen() {
			break
		}
		if e.stack[i].IsForward() {
			fwdIdx = i
			break
		}
	}

	if fwdIdx < 0 {
		e.stackRemove(endIdx)
		return nil
	}

	fwd, _ := e.stack[fwdIdx].AsForward()
	funcIdx := fwd.FuncIndex

	// Remove forward and end from the stack.
	// Remove higher index first to preserve lower indices.
	if endIdx > fwdIdx {
		e.stackRemove(endIdx)
		e.stackRemove(fwdIdx)
		if fwdIdx < funcIdx {
			funcIdx-- // forward removal
		}
		// end was already removed (endIdx > fwdIdx), endIdx > funcIdx always
	} else {
		e.stackRemove(fwdIdx)
		newEndIdx := endIdx
		if fwdIdx < endIdx {
			newEndIdx--
		}
		e.stackRemove(newEndIdx)
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
	info, _ := val.AsDefCleanup()
	reg := info.Registry
	for _, name := range reg.DefNames() {
		prevLen := info.Snapshot[name] // 0 for names not in snapshot
		for reg.DefStackDepth(name) > prevLen {
			UninstallDef(reg, name)
		}
	}
}

func (e *Engine) stepMark(val Value) {
	info, _ := val.AsMark()
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
	info, _ := val.AsMove()
	moveIdx := e.pointer

	if e.marks == nil || !e.marks[info.To] {
		return e.runtimeError("move_error", fmt.Sprintf("mark %q not found (%s)", info.To, info.Reason), info.To, "")
	}

	// Scan the stack to find the mark's current position.
	markIdx := -1
	for i := 0; i < len(e.stack); i++ {
		_as2, _ := e.stack[i].AsMark()
		if e.stack[i].IsMark() && _as2.ID == info.To {
			markIdx = i
			break
		}
	}
	if markIdx < 0 {
		// Mark was removed from the stack (e.g. by a for-loop controller
		// signalling loop completion). Remove this orphaned move quietly.
		delete(e.marks, info.To)
		e.stackRemove(e.pointer)
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
	markInfo, _ := e.stack[markIdx].AsMark()

	// Remove from hash table.
	delete(e.marks, info.To)

	// Replace everything from mark through move (inclusive) with the body copy.
	body := make([]Value, len(markInfo.Body))
	copy(body, markInfo.Body)
	e.stackSplice(markIdx, moveIdx-markIdx+1, body...)

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
		e.stackSplice(markIdx, moveIdx-markIdx+1, tokens...)
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
	e.stackSplice(markIdx, moveIdx-markIdx+1, cont.Results...)
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
	if condResult.VType == nil {
		e.stackSplice(markIdx, moveIdx-markIdx+1)
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
	e.stackSplice(markIdx, moveIdx-markIdx+1, branch...)
	e.pointer = markIdx
	e.traceNote = fmt.Sprintf("if %v", cond)
	return nil
}

// handleLoopBreak handles a break sentinel error by finding the nearest
// enclosing for-loop (move with continuation) and terminating it.
// Returns true if break was handled, false if no enclosing loop was found.
func (e *Engine) handleLoopBreak() bool {
	// Scan forward from current pointer for a move with continuation.
	for i := e.pointer; i < len(e.stack); i++ {
		if e.stack[i].IsMove() {
			info, _ := e.stack[i].AsMove()
			if info.Cont != nil {
				// Found the for-loop's move. Find its mark.
				markIdx := -1
				for j := 0; j < i; j++ {
					_as3, _ := e.stack[j].AsMark()
					if e.stack[j].IsMark() && _as3.ID == info.To {
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
				e.stackSplice(markIdx, i-markIdx+1, info.Cont.Results...)
				e.pointer = markIdx
				return true
			}
		}
	}
	return false
}

// handleLoopContinue handles a continue sentinel error by finding the nearest
// enclosing for-loop and advancing to the next iteration (discarding the
// current iteration's partial results).
// Returns true if continue was handled, false if no enclosing loop was found.
func (e *Engine) handleLoopContinue() bool {
	// Scan forward from current pointer for a move with continuation.
	for i := e.pointer; i < len(e.stack); i++ {
		if e.stack[i].IsMove() {
			info, _ := e.stack[i].AsMove()
			if info.Cont != nil {
				// Found the for-loop's move. Find its mark.
				markIdx := -1
				for j := 0; j < i; j++ {
					_as4, _ := e.stack[j].AsMark()
					if e.stack[j].IsMark() && _as4.ID == info.To {
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
					e.stackSplice(markIdx+1, i-markIdx-1)
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
		if e.stack[i].IsMark() || e.stack[i].IsMove() {
			e.stackRemove(i)
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
		if e.stack[i].IsOpenParen() {
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
			if e.stack[i].IsForward() {
				hasFwd = true
				fwd, _ := e.stack[i].AsForward()
				funcIdx := fwd.FuncIndex
				collectedCount := fwd.CollectedArgs
				stackArgCount := fwd.StackArgs

				// Remove the forward.
				e.stackRemove(i)
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
					case val.IsWord():
						if err := e.stepWord(val); err != nil {
							return err
						}
						// Recalculate closeIdx: stack may have changed.
						closeIdx = e.findCloseParenAfter(openIdx)
						if closeIdx < 0 {
							return e.syntaxError("unmatched closing parenthesis", ")")
						}
					case val.IsForward():
						e.pointer++
					case val.IsOpenParen():
						e.pointer++
					case val.IsReturnCheck():
						e.pointer++
					case val.IsDefCleanup():
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
		if e.stack[i].IsForward() {
			fwd, _ := e.stack[i].AsForward()
			return e.insufficientArgsError(fwd.FuncName, fwd.ExpectedArgs)
		}
	}

	// Remove any surviving def-cleanup markers.
	for i := openIdx + 1; i < closeIdx; i++ {
		if e.stack[i].IsDefCleanup() {
			e.stepDefCleanup(e.stack[i])
			e.stackRemove(i)
			closeIdx--
			i--
		}
	}

	// Check for return type validation.
	for i := openIdx + 1; i < closeIdx; i++ {
		if e.stack[i].IsReturnCheck() {
			rc, _ := e.stack[i].AsReturnCheck()
			e.stackRemove(i)
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
				if !results[extra+k].VType.Matches(exp) {
					return e.returnTypeError(rc.FuncName, k+1, exp, results[extra+k])
				}
			}

			// Discard unconsumed unnamed args from the bottom of the scope.
			for j := 0; j < extra; j++ {
				e.stackRemove(openIdx + 1)
				closeIdx--
			}
			break
		}
	}

	// Remove the close paren (higher index first) and open paren.
	// The values between them are already in place.
	e.stackRemove(closeIdx)
	e.stackRemove(openIdx)

	e.pointer = openIdx
	return nil
}

// findCloseParenAfter finds the index of the ")" word after the given openIdx.
func (e *Engine) findCloseParenAfter(openIdx int) int {
	depth := 0
	for i := openIdx + 1; i < len(e.stack); i++ {
		if e.stack[i].IsOpenParen() {
			depth++
		} else if e.stack[i].IsWord() {
			sw, _ := e.stack[i].AsWord()
			if sw.Name == ")" {
				if depth == 0 {
					return i
				}
				depth--
			}
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
		if e.stack[i].IsOpenParen() {
			start = i + 1
			break
		}
		if e.stack[i].IsForward() {
			fwd, _ := e.stack[i].AsForward()
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
		if v.IsForward() || v.IsOpenParen() || v.IsMark() || v.IsMove() || excludeIndices[i] {
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
		if e.stack[i].IsOpenParen() {
			return false
		}
		if e.stack[i].IsForward() {
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

	if funcIdx >= len(e.stack) || !e.stack[funcIdx].IsWord() {
		e.pointer = funcIdx
		return
	}

	w, _ := e.stack[funcIdx].AsWord()
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
			if e.stack[i].IsOpenParen() {
				start = i + 1
				break
			}
			if e.stack[i].IsForward() {
				fwd, _ := e.stack[i].AsForward()
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
			if v.IsForward() || v.IsOpenParen() || excludeIndices[i] {
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
		if e.stack[i].IsOpenParen() {
			break
		}
		if e.stack[i].IsForward() {
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

		e.stackSplice(startIdx, collectedCount+1, NewList(elems))
		e.pointer = startIdx
		return
	}

	// No outer forward - force stack (may result in error on next step).
	e.stack[funcIdx] = NewWordModified(w.Name, w.ArgCount, true, false)
	e.pointer = funcIdx
}

// hasPendingForwardExpectingWord checks if there is a pending forward
// whose next expected argument is TWord.
func (e *Engine) hasPendingForwardExpectingWord() bool {
	for i := e.pointer - 1; i >= 0; i-- {
		if e.stack[i].IsOpenParen() {
			break
		}
		if e.stack[i].IsForward() {
			fwd, _ := e.stack[i].AsForward()
			// Forward args fill from sigArgs[0]; the next forward slot
			// is at index CollectedArgs.
			nextIdx := fwd.CollectedArgs
			if nextIdx < len(fwd.Sig.Args) {
				if fwd.Sig.Args[nextIdx].Equal(TWord) {
					return true
				}
				// /q modifier: capture word without evaluation.
				if fwd.Sig.QuoteArgs != nil && fwd.Sig.QuoteArgs[nextIdx] {
					return true
				}
				return false
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
		if e.stack[i].IsOpenParen() {
			break
		}
		if e.stack[i].IsForward() {
			fwd, _ := e.stack[i].AsForward()
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
