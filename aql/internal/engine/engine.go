package engine

import (
	"fmt"
	"strings"
)

// typeNames maps well-known type names to their Type, so bare words like
// "number" or "string" resolve to type-literal values instead of strings.
var typeNames = map[string]Type{
	"Any":      TAny,
	"None":     TNone,
	"Scalar":   TScalar,
	"Number":   TNumber,
	"Integer":  TInteger,
	"Decimal":  TDecimal,
	"String":   TString,
	"Boolean":  TBoolean,
	"Path":     TPath,
	"Atom":     TAtom,
	"Node":     TNode,
	"List":     TList,
	"Map":      TMap,
	"Table":    TTable,
	"Record":   TRecord,
	"Options":  TOptions,
	"Object":   TObject,
	"Resource":   TResource,
	"Entity":     TResourceEntity,
	"Array":      TArray,
	"Type":       TType,
	"ScalarType": TScalarType,
	"NodeType":   TNodeType,
	"ObjectType": TObjectType,
}

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
				if isBreak(err) {
					if e.handleLoopBreak() {
						continue
					}
				}
				if isContinue(err) {
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
			if val.VType.Equal(Type{}) {
				return nil, fmt.Errorf("halt: undefined stack entry at position %d", e.pointer)
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
			return nil, fmt.Errorf("syntax error: unmatched opening parenthesis")
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

		fwd := e.stack[fwdIdx].AsForward()
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
			ww := tok.AsWord()
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
				for limit := 0; limit < 2222; limit++ {
					if e.pointer >= len(e.stack) {
						break
					}
					v := e.stack[e.pointer]

					// Check if this is the ")" that closes our paren.
					if v.IsWord() && v.AsWord().Name == ")" {
						if err := e.stepCloseParen(); err != nil {
							e.pointer = savedPointer
							return err
						}
						break
					}

					// Normal evaluation inside paren.
					switch {
					case v.IsWord():
						if err := e.stepWord(v); err != nil {
							e.pointer = savedPointer
							return err
						}
					case v.IsForward():
						e.pointer++
					case v.IsOpenParen():
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

			// Function word: boundary, stop.
			if e.registry.Lookup(ww.Name) != nil {
				break
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
	w := val.AsWord()

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
		if stack, ok := e.registry.DefStacks[w.Name]; ok {
			for i := len(stack) - 1; i >= 0; i-- {
				if fnDef, ok := stack[i].Data.(FnDefInfo); ok {
					e.stack[e.pointer] = NewFunction(fnDef)
					return e.stepLiteral()
				}
			}
		}
		// Not a def fn — fall through to normal execution.
	}

	// Simple value def: substitute the word with its value directly,
	// bypassing function dispatch entirely. FnDefInfo and ObjectTypeInfo
	// entries are not simple values — they go through normal Lookup.
	if ds := e.registry.DefStacks[w.Name]; len(ds) > 0 {
		top := ds[len(ds)-1]
		switch top.Data.(type) {
		case FnDefInfo, *ObjectTypeInfo:
			// Not a simple value — fall through to Lookup.
		default:
			// For list bodies, expand onto the stack like the fallback handler does.
			// Quoted lists are treated as data values (not expanded).
			if top.VType.Equal(TList) && !top.IsTypedList() && !top.IsTableType() && !top.Quoted {
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

	if fn == nil {
		if w.Name == "true" {
			e.stack[e.pointer] = NewBoolean(true)
			return nil
		}
		if w.Name == "false" {
			e.stack[e.pointer] = NewBoolean(false)
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
		e.stack[e.pointer] = NewAtom(w.Name)
		return nil
	}

	// Pre-evaluate paren expressions in the forward scan range so that
	// matchSignature sees fully resolved values (rule 1.5).
	if fn.ForwardPrecedence || w.ForceForward {
		if err := e.preEvalParens(fn.MaxForwardArgs); err != nil {
			return err
		}
	}

	// Unified signature matching: one path for all words.
	resolved := e.effectiveResolved()
	sig, positions := e.matchSignature(fn, w, resolved)

	if sig == nil {
		return fmt.Errorf("signature error: no matching signature for %s", w.Name)
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
				evaluated, err := e.autoEvalMap(match.Args[i])
				if err == nil {
					match.Args[i] = evaluated
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
		return err
	}

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

// rearrangeForForward reorders the N = stackArgs + forwardArgs resolved values
// before the current pointer so that forward-collected args come first (mapped
// to the beginning of the signature) and stack args follow in reverse order
// (top of stack → first remaining sig arg).
//
// Before: [..., stack_0, stack_1, ..., stack_{S-1}, fwd_0, fwd_1, ..., fwd_{F-1}, WORD]
// After:  [..., fwd_0, fwd_1, ..., fwd_{F-1}, stack_{S-1}, ..., stack_1, stack_0, WORD]
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

	// Reorder: forward args first, then stack args reversed.
	reordered := make([]Value, total)
	for i := 0; i < forwardArgs; i++ {
		reordered[i] = values[stackArgs+i]
	}
	for i := 0; i < stackArgs; i++ {
		reordered[forwardArgs+i] = values[stackArgs-1-i]
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

// resolvedStackBefore returns all resolved values before the pointer,
// excluding forwards, open-parens, and the matched arg indices.
func (e *Engine) resolvedStackBefore(excludeIndices []int) []Value {
	return e.resolvedStackBeforeFrom(0, excludeIndices)
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

// insertForward handles a forward-precedence word by placing a forward
// primitive after the word on the stack.
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

	fwd := e.stack[fwdIdx].AsForward()
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
			w := e.stack[funcIdx].AsWord()
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
					resolvedKey = keyResult[0].AsString()
				} else if keyResult[0].IsAtom() {
					resolvedKey = keyResult[0].AsAtom()
				} else {
					resolvedKey = valToString(keyResult[0])
				}
			}
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
			Name:              fnDef.Name,
			Signatures:        fnSigsToSignatures(fnDef.Sigs),
			ForwardPrecedence: true,
		}
	}
	if fn == nil {
		e.pointer++
		return nil
	}

	resolved := e.effectiveResolved()
	w := WordInfo{Name: fnDef.Name, ArgCount: -1}

	// Use unified matchSignature for all matching (forward and stack).
	matchedSig, positions := e.matchSignature(fn, w, resolved)

	if matchedSig == nil {
		// No matching signature — just advance (treat as data).
		e.pointer++
		return nil
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

	if fwdCount > 0 {
		return e.insertForward(w, matchedSig, fwdCount, stkCount)
	}

	// Pure stack match. Find the corresponding FnSig to pass to
	// execFnDefSig (which needs FnSig for named params and CallAQL).
	nArgs := len(matchedSig.Args)
	for i := range fnDef.Sigs {
		fs := &fnDef.Sigs[i]
		if len(fs.Params) != nArgs {
			continue
		}
		paramsMatch := true
		for j := range fs.Params {
			if !fs.Params[j].Type.Equal(matchedSig.Args[j]) {
				paramsMatch = false
				break
			}
		}
		if !paramsMatch {
			continue
		}
		// Build args from recorded positions.
		var args []Value
		if nArgs > 0 {
			args = make([]Value, nArgs)
			for j, pos := range positions {
				args[j] = e.stack[pos]
			}
		}
		return e.execFnDefSig(valIdx, fs, args, fnDef.Registry)
	}

	// Matched a 0-arg signature.
	if nArgs == 0 {
		for i := range fnDef.Sigs {
			if len(fnDef.Sigs[i].Params) == 0 {
				return e.execFnDefSig(valIdx, &fnDef.Sigs[i], nil, fnDef.Registry)
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
		argTypes := make([]Type, len(sig.Params))
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
	}

	if capturedReg != nil {
		// Execute in the captured module's registry via CallAQL.
		fnVal := e.stack[valIdx]
		result, err := capturedReg.CallAQL(fnVal, args)
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
	e.registry.argsStack = append(e.registry.argsStack, NewList(argsCopy))

	var names []string
	unnamedCount := 0
	for i, p := range sig.Params {
		if p.Name != "" {
			installDef(e.registry, p.Name, args[i])
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
	fwd := e.stack[fwdIdx].AsForward()
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

	fwd := e.stack[fwdIdx].AsForward()
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
// the body ran. Any defs added since are popped via uninstallDef.
func (e *Engine) stepDefCleanup(val Value) {
	info := val.AsDefCleanup()
	reg := info.Registry
	for name, stack := range reg.DefStacks {
		prevLen := info.Snapshot[name] // 0 for names not in snapshot
		for len(stack) > prevLen {
			uninstallDef(reg, name)
			stack = reg.DefStacks[name]
		}
	}
}

func (e *Engine) stepMark(val Value) {
	info := val.AsMark()
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
	info := val.AsMove()
	moveIdx := e.pointer

	if e.marks == nil || !e.marks[info.To] {
		return fmt.Errorf("move error: mark %q not found (%s)", info.To, info.Reason)
	}

	// Scan the stack to find the mark's current position.
	markIdx := -1
	for i := 0; i < len(e.stack); i++ {
		if e.stack[i].IsMark() && e.stack[i].AsMark().ID == info.To {
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
	markInfo := e.stack[markIdx].AsMark()

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
		uninstallDef(cont.Registry, cont.IterName)
		installDef(cont.Registry, cont.IterName, NewInteger(cont.Current))

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
	uninstallDef(cont.Registry, cont.IterName)
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
	if condResult.VType.Parts == nil {
		e.stackSplice(markIdx, moveIdx-markIdx+1)
		e.pointer = markIdx
		return fmt.Errorf("if: condition produced no value")
	}

	// Evaluate truthiness and choose branch.
	cond := isTruthy(condResult)

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
			info := e.stack[i].AsMove()
			if info.Cont != nil {
				// Found the for-loop's move. Find its mark.
				markIdx := -1
				for j := 0; j < i; j++ {
					if e.stack[j].IsMark() && e.stack[j].AsMark().ID == info.To {
						markIdx = j
						break
					}
				}
				if markIdx < 0 {
					delete(e.marks, info.To)
					continue
				}

				// Uninstall iterator, splice in accumulated results.
				uninstallDef(info.Cont.Registry, info.Cont.IterName)
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
			info := e.stack[i].AsMove()
			if info.Cont != nil {
				// Found the for-loop's move. Find its mark.
				markIdx := -1
				for j := 0; j < i; j++ {
					if e.stack[j].IsMark() && e.stack[j].AsMark().ID == info.To {
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
		return fmt.Errorf("syntax error: unmatched closing parenthesis")
	}

	// Resolve any forwards inside the paren scope via implicit end.
	// We loop because resolving a forward may cause re-evaluation.
	for attempt := 0; attempt < 222; attempt++ {
		hasFwd := false
		for i := openIdx + 1; i < closeIdx; i++ {
			if e.stack[i].IsForward() {
				hasFwd = true
				fwd := e.stack[i].AsForward()
				funcIdx := fwd.FuncIndex
				collectedCount := fwd.CollectedArgs
				stackArgCount := fwd.StackArgs

				// Remove the forward.
				e.stackRemove(i)
				closeIdx--
				if i < funcIdx {
					funcIdx--
				}

				// Try stack match or create curry list.
				e.curryOrStack(funcIdx, collectedCount, stackArgCount)

				// Recalculate closeIdx after potential stack changes.
				closeIdx = e.findCloseParenAfter(openIdx)
				if closeIdx < 0 {
					return fmt.Errorf("syntax error: unmatched closing parenthesis")
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
							return fmt.Errorf("syntax error: unmatched closing parenthesis")
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
							return fmt.Errorf("syntax error: unmatched closing parenthesis")
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
			fwd := e.stack[i].AsForward()
			return fmt.Errorf("signature error: insufficient arguments for %s (expected %d forward args)",
				fwd.FuncName, fwd.ExpectedArgs)
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
			rc := e.stack[i].AsReturnCheck()
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
				return fmt.Errorf("%s: expected %d return value(s), got %d",
					rc.FuncName, nret, len(results))
			}
			extra := len(results) - nret
			if extra > rc.UnnamedCount {
				return fmt.Errorf("%s: expected %d return value(s), got %d",
					rc.FuncName, nret, len(results)-rc.UnnamedCount)
			}

			// Validate the top nret values match declared return types.
			for k, exp := range rc.Returns {
				if !results[extra+k].VType.Matches(exp) {
					return fmt.Errorf("%s: return value %d: expected %s, got %s",
						rc.FuncName, k+1, exp, results[extra+k].VType)
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
		} else if e.stack[i].IsWord() && e.stack[i].AsWord().Name == ")" {
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
		if e.stack[i].IsOpenParen() {
			start = i + 1
			break
		}
		if e.stack[i].IsForward() {
			fwd := e.stack[i].AsForward()
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


// peekForwardValue returns a value representing what the next stack element
// would resolve to, for use in speculative signature matching. Unknown words
// become atoms; true/false become booleans; literals are returned as-is.
func (e *Engine) peekForwardValue() Value {
	if e.pointer+1 >= len(e.stack) {
		return Value{}
	}
	next := e.stack[e.pointer+1]
	if next.IsWord() {
		nw := next.AsWord()
		switch nw.Name {
		case "true":
			return NewBoolean(true)
		case "false":
			return NewBoolean(false)
		default:
			// If the word has a FnDef in DefStacks, peek as TFunction.
			if stack, ok := e.registry.DefStacks[nw.Name]; ok {
				for i := len(stack) - 1; i >= 0; i-- {
					if fnDef, ok := stack[i].Data.(FnDefInfo); ok {
						return NewFunction(fnDef)
					}
				}
			}
			return NewAtom(nw.Name)
		}
	}
	return next
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

	w := e.stack[funcIdx].AsWord()
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
				fwd := e.stack[i].AsForward()
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

		// For forward-precedence functions, rearrange values so forward
		// args are first and stack args are reversed before matching.
		if fn.ForwardPrecedence && sac > 0 {
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
			fwd := e.stack[i].AsForward()
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
			fwd := e.stack[i].AsForward()
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
