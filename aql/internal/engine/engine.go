package engine

import (
	"fmt"
	"strings"
)

// typeNames maps well-known type names to their Type, so bare words like
// "number" or "string" resolve to type-literal values instead of strings.
var typeNames = map[string]Type{
	"any":     TAny,
	"none":    TNone,
	"scalar":  TScalar,
	"number":  TNumber,
	"integer": TInteger,
	"string":  TString,
	"boolean": TBoolean,
	"atom":    TAtom,
	"list":    TList,
	"map":     TMap,
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
	traceNote string // annotation set during execution for the next trace call
	stepLimit int    // 0 means use default (22222 for top-level, 2222 for sub-engines)
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
	s := name + "(" + strings.Join(args, ", ") + ")"
	if sig.Precedence > 0 {
		s += fmt.Sprintf(" prec=%d", sig.Precedence)
	}
	return s
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
	// Push a scoped context layer (shallow copy of parent context).
	parent := e.registry.Context()
	if parent == nil {
		parent = make(map[string]Value)
	}
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

		case val.IsMark():
			e.stepMark(val)

		case val.IsMove():
			if err := e.stepMove(val); err != nil {
				return nil, err
			}

		case val.IsReturnCheck():
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

		// Remove the forward marker.
		e.stackRemove(fwdIdx)
		if fwdIdx < funcIdx {
			funcIdx--
		}

		// Try prefix match or create curry list.
		e.curryOrPrefix(funcIdx, collectedCount)

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
		e.stack[e.pointer] = NewAtom(w.Name)
		return nil
	}

	if w.ForcePrefix {
		resolved := e.effectiveResolved()
		match := MatchSignature(fn.Signatures, resolved, w)
		if match == nil {
			return fmt.Errorf("signature error: no matching signature for %s", w.Name)
		}
		e.traceNote = "prefix " + traceSigStr(w.Name, match.Sig)
		return e.execMatch(match)
	}

	if w.ForceSuffix {
		// Force suffix: skip prefix attempt, collect all args from suffix.
		resolved := e.effectiveResolved()
		bestSig, _ := e.bestSigForForward(fn, w, resolved)
		if bestSig == nil {
			return fmt.Errorf("signature error: no matching signature for %s", w.Name)
		}
		e.traceNote = "suffix→ " + traceSigStr(w.Name, bestSig)
		return e.insertForward(w, bestSig, len(bestSig.Args))
	}

	if fn.SuffixPrecedence {
		resolved := e.effectiveResolved()
		match := MatchSignature(fn.Signatures, resolved, w)

		// When prefix has a full match (typed signature), check if
		// suffix tokens should take priority. Suffix precedence means
		// we prefer to consume tokens after the word when available.
		if match != nil && len(match.Sig.Args) > 0 {
			if e.hasSuffixValues(fn) {
				// Suffix tokens exist. Verify that collecting from
				// suffix would still produce a valid signature match
				// before switching away from the working prefix match.
				suffixVal := e.peekSuffixValue()
				extended := append(resolved, suffixVal)
				if MatchSignature(fn.Signatures, extended, w) != nil {
					bestSig, prefixCount := e.bestSigForForward(fn, w, resolved)
					if bestSig != nil {
						suffixNeeded := len(bestSig.Args) - prefixCount
						if suffixNeeded <= 0 {
							suffixNeeded = 1
						}
						e.traceNote = "suffix→ " + traceSigStr(w.Name, bestSig)
						return e.insertForward(w, bestSig, suffixNeeded)
					}
				}
			}
			// No viable suffix — use prefix match.
			e.traceNote = "prefix " + traceSigStr(w.Name, match.Sig)
			return e.execMatch(match)
		}

		// No full prefix match — try suffix (create forward to collect
		// remaining args), preserving original behavior.
		bestSig, prefixCount := e.bestSigForForward(fn, w, resolved)
		if bestSig != nil {
			suffixNeeded := len(bestSig.Args) - prefixCount
			if suffixNeeded <= 0 {
				suffixNeeded = len(bestSig.Args)
			}
			e.traceNote = "suffix→ " + traceSigStr(w.Name, bestSig)
			return e.insertForward(w, bestSig, suffixNeeded)
		}

		// Fall back to 0-arg match (generic def handler).
		if match != nil {
			e.traceNote = "prefix " + traceSigStr(w.Name, match.Sig)
			return e.execMatch(match)
		}

		return fmt.Errorf("signature error: no matching signature for %s", w.Name)
	}

	// Prefix-only function (dup, swap, drop).
	resolved := e.effectiveResolved()
	match := MatchSignature(fn.Signatures, resolved, w)
	if match == nil {
		return fmt.Errorf("signature error: no matching signature for %s", w.Name)
	}
	e.traceNote = "prefix " + traceSigStr(w.Name, match.Sig)
	return e.execMatch(match)
}

// bestSigForForward finds the best signature for creating a forward and how
// many prefix args from the resolved stack can be consumed.
func (e *Engine) bestSigForForward(fn *Function, w WordInfo, resolved []Value) (*Signature, int) {
	var best *Signature
	var bestScore int
	var bestPrefixCount int

	// Peek at the first potential suffix value to help disambiguate sigs.
	// Skip special words ("(", ")", "end") since they're not real suffix args.
	var peekVal *Value
	peekIdx := e.pointer + 1
	if peekIdx < len(e.stack) {
		v := e.stack[peekIdx]
		if !v.IsForward() && !v.IsOpenParen() {
			skip := false
			if v.IsWord() {
				w := v.AsWord()
				if w.Name == "(" || w.Name == ")" || w.Name == "end" {
					skip = true
				}
			}
			if !skip {
				peekVal = &v
			}
		}
	}

	for i := range fn.Signatures {
		sig := &fn.Signatures[i]
		if len(sig.Args) == 0 {
			continue
		}
		if w.ArgCount >= 0 && sig.TotalArgs() != w.ArgCount {
			continue
		}

		// Count how many args from the top of the resolved stack match
		// sig.Args in any position (flexible prefix matching).
		// Only consider the top N contiguous resolved values.
		prefixCount := 0
		usedArgs := make([]bool, len(sig.Args))
		maxTry := len(sig.Args)
		if maxTry > len(resolved) {
			maxTry = len(resolved)
		}
		for tryN := maxTry; tryN >= 1; tryN-- {
			top := resolved[len(resolved)-tryN:]
			tempUsed := make([]bool, len(sig.Args))
			matched := 0
			for _, v := range top {
				for si := 0; si < len(sig.Args); si++ {
					if !tempUsed[si] && v.VType.Matches(sig.Args[si]) {
						tempUsed[si] = true
						matched++
						break
					}
				}
			}
			if matched == tryN {
				prefixCount = tryN
				copy(usedArgs, tempUsed)
				break
			}
		}

		score := signatureScore(sig)

		// Bonus for prefix args already on the stack: each matching
		// prefix arg adds 25 to the score. This helps signatures that
		// partially match the existing stack win over those that don't.
		score += prefixCount * 25

		// Bonus: if the peeked suffix value matches the first unmatched
		// sig arg type, boost this sig's score. Only check the first
		// unmatched arg to avoid false positives on heterogeneous sigs.
		if peekVal != nil && prefixCount < len(sig.Args) {
			firstUnmatched := -1
			for si := 0; si < len(sig.Args); si++ {
				if !usedArgs[si] {
					firstUnmatched = si
					break
				}
			}
			matched := false
			if firstUnmatched >= 0 {
				firstSuffixType := sig.Args[firstUnmatched]
				matched = peekVal.VType.Matches(firstSuffixType)
				// Predict resolved types for words that haven't executed yet.
				if !matched && peekVal.IsWord() {
					pw := peekVal.AsWord()
					switch {
					case pw.Name == "true" || pw.Name == "false":
						matched = TBoolean.Matches(firstSuffixType)
					case pw.Name == "(" || pw.Name == ")" || pw.Name == "end":
						// Skip structural words.
					default:
						if _, isType := typeNames[pw.Name]; isType {
							// Type names stay as type literals.
						} else if e.registry.Lookup(pw.Name) == nil {
							// Unknown word → will resolve to atom.
							matched = TAtom.Matches(firstSuffixType)
						}
						// Also check TWord for sigs expecting word literals.
						if !matched {
							matched = peekVal.VType.Matches(firstSuffixType)
						}
					}
				}
			}
			if matched {
				score += 50
			}
		}

		if best == nil || score > bestScore {
			best = sig
			bestScore = score
			bestPrefixCount = prefixCount
		}
	}
	return best, bestPrefixCount
}

// execMatch executes a matched signature, splicing args and results.
func (e *Engine) execMatch(match *MatchResult) error {
	n := len(match.Sig.Args)

	// Find the indices of the n resolved values before the pointer.
	indices := e.resolvedIndicesBefore(n)

	var results []Value
	var err error
	if match.Sig.FullStackHandler != nil {
		// Collect the full resolved stack before the pointer,
		// excluding the matched args and forwards.
		fullStack := e.resolvedStackBefore(indices)
		results, err = match.Sig.FullStackHandler(match.Args, fullStack)
		if err != nil {
			return err
		}
		// FullStackHandler returns the complete replacement for
		// everything from start through the pointer (inclusive).
		e.stackSplice(0, e.pointer+1, results...)
		e.pointer = 0
		return nil
	}

	results, err = match.Sig.Handler(match.Args)
	if err != nil {
		return err
	}

	if len(indices) == n && n > 0 {
		firstArgIdx := indices[0]

		// Compact: slide non-skip elements over skip elements in
		// [firstArgIdx..pointer] to preserve internal forwards.
		skipSet := make(map[int]bool, n+1)
		for _, idx := range indices {
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
	exclude := make(map[int]bool, len(excludeIndices))
	for _, idx := range excludeIndices {
		exclude[idx] = true
	}
	var stack []Value
	for i := 0; i < e.pointer; i++ {
		if exclude[i] || e.stack[i].IsForward() || e.stack[i].IsOpenParen() || e.stack[i].IsMark() || e.stack[i].IsMove() {
			continue
		}
		stack = append(stack, e.stack[i])
	}
	return stack
}

// insertForward handles a suffix-precedence word by placing a forward
// primitive after the word on the stack.
func (e *Engine) insertForward(w WordInfo, sig *Signature, suffixNeeded int) error {
	fwd := NewForward(ForwardInfo{
		FuncName:     w.Name,
		ExpectedArgs: suffixNeeded,
		FuncIndex:    e.pointer,
		Precedence:   sig.Precedence,
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
		e.pointer++
		return nil
	}

	fwd := e.stack[fwdIdx].AsForward()
	funcIdx := fwd.FuncIndex

	// Check if the value matches ANY remaining (uncollected) arg type.
	// Suffix collection is flexible: the value can satisfy any arg slot,
	// with final ordering handled by flexibleMatch during prefix retry.
	if fwd.CollectedArgs < len(fwd.Sig.Args) {
		val := e.stack[valIdx]
		matchesAny := false
		for i := 0; i < len(fwd.Sig.Args); i++ {
			if val.VType.Matches(fwd.Sig.Args[i]) {
				matchesAny = true
				break
			}
		}
		if !matchesAny {
			// The forward's chosen sig doesn't accept this value, but
			// another overload of the same function might. Check all
			// signatures and switch if we find a compatible one.
			if fn := e.registry.Lookup(fwd.FuncName); fn != nil {
				for si := range fn.Signatures {
					altSig := &fn.Signatures[si]
					if len(altSig.Args) != len(fwd.Sig.Args) {
						continue
					}
					for ai := range altSig.Args {
						if val.VType.Matches(altSig.Args[ai]) {
							fwd.Sig = altSig
							e.stack[fwdIdx] = NewForward(fwd)
							matchesAny = true
							break
						}
					}
					if matchesAny {
						break
					}
				}
			}
		}
		if !matchesAny {
			// Type mismatch — implicit end: resolve forward from stack.
			return e.implicitEnd(fwdIdx)
		}
	}

	// Peek ahead: if the next item is a higher-precedence infix operator,
	// defer collection and let that operator execute first.
	if fwd.Precedence > 0 {
		if nextPrec := e.peekPrecedence(valIdx + 1); nextPrec > fwd.Precedence {
			nextName := ""
			if valIdx+1 < len(e.stack) && e.stack[valIdx+1].IsWord() {
				nextName = e.stack[valIdx+1].AsWord().Name
			}
			e.traceNote = fmt.Sprintf("defer %s prec=%d < %s prec=%d",
				fwd.FuncName, fwd.Precedence, nextName, nextPrec)
			e.pointer++
			return nil
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

	// Insert the suffix value right before the function word (on top of the
	// stack relative to prefix args). This means after collection, the stack is:
	// [..., prefix_args..., suffix0, suffix1, ..., func_word, fwd, ...]
	// The word retries as prefix with args in their natural order.
	insertIdx := funcIdx

	e.stackInsert(insertIdx, val)

	funcIdx++
	fwdIdx++

	fwd.CollectedArgs++
	fwd.FuncIndex = funcIdx

	e.traceNote = fmt.Sprintf("collect %s %d/%d",
		fwd.FuncName, fwd.CollectedArgs, fwd.ExpectedArgs)

	if fwd.CollectedArgs >= fwd.ExpectedArgs {
		// All suffix args collected. Remove forward, force prefix, retry.
		e.stackRemove(fwdIdx)
		// fwdIdx is after funcIdx, so funcIdx is unaffected.

		if e.stack[funcIdx].IsWord() {
			w := e.stack[funcIdx].AsWord()
			e.stack[funcIdx] = NewWordModified(w.Name, w.ArgCount, true, false)
		}

		e.pointer = funcIdx
	} else if e.shouldResolveForwardEarly(fwd, fwdIdx) {
		// A shorter sig is fully satisfied and the next token can't
		// produce the longer sig's next expected type. Resolve now
		// so the function fires before subsequent tokens evaluate.
		e.traceNote = fmt.Sprintf("early-resolve %s %d/%d",
			fwd.FuncName, fwd.CollectedArgs, fwd.ExpectedArgs)
		e.stackRemove(fwdIdx)

		if e.stack[funcIdx].IsWord() {
			w := e.stack[funcIdx].AsWord()
			e.stack[funcIdx] = NewWordModified(w.Name, w.ArgCount, true, false)
		}

		e.pointer = funcIdx
	} else {
		e.stack[fwdIdx] = NewForward(fwd)
		e.pointer = fwdIdx + 1
	}

	return nil
}

// shouldResolveForwardEarly checks whether a forward that hasn't collected
// all its expected args should resolve now because a shorter signature of the
// same function is fully satisfied and the next token on the stack cannot
// plausibly produce the longer sig's next expected type. This prevents the
// forward from delaying the function's execution when it's clear the shorter
// sig is the right match (e.g., "undef foo foo" should use the 1-arg [TWord]
// sig immediately rather than waiting for a TFnUndef that won't come).
func (e *Engine) shouldResolveForwardEarly(fwd ForwardInfo, fwdIdx int) bool {
	fn := e.registry.Lookup(fwd.FuncName)
	if fn == nil {
		return false
	}

	// Check if any shorter sig with exactly CollectedArgs args can
	// accept the types of the already-collected suffix values.
	funcIdx := fwd.FuncIndex
	collectedTypes := make([]Type, fwd.CollectedArgs)
	for i := 0; i < fwd.CollectedArgs; i++ {
		collectedTypes[i] = e.stack[funcIdx-fwd.CollectedArgs+i].VType
	}

	hasShorterMatch := false
	for si := range fn.Signatures {
		sig := &fn.Signatures[si]
		if len(sig.Args) != fwd.CollectedArgs {
			continue
		}
		// Flexible match: each collected type must match some sig arg.
		used := make([]bool, len(sig.Args))
		allMatch := true
		for _, ct := range collectedTypes {
			found := false
			for j := range sig.Args {
				if !used[j] && ct.Matches(sig.Args[j]) {
					used[j] = true
					found = true
					break
				}
			}
			if !found {
				allMatch = false
				break
			}
		}
		if allMatch {
			hasShorterMatch = true
			break
		}
	}
	if !hasShorterMatch {
		return false
	}

	// A shorter sig is satisfied. Check whether the next token on the
	// stack could produce the type needed for the longer sig's next slot.
	nextArgType := fwd.Sig.Args[fwd.CollectedArgs]
	peekIdx := fwdIdx + 1
	if peekIdx >= len(e.stack) {
		return true // no more tokens → resolve with shorter sig
	}

	return !e.couldProduceType(e.stack[peekIdx], nextArgType)
}

// couldProduceType predicts whether a stack value, when evaluated, could
// produce a value matching the expected type. For literal values, this is
// a direct type check. For words, it predicts based on definitions or
// assumes built-in functions could produce any type.
func (e *Engine) couldProduceType(v Value, expected Type) bool {
	// Direct type match (works for all literals).
	if v.VType.Matches(expected) {
		return true
	}

	if v.IsForward() {
		return false // structural, can't produce values
	}

	// An open paren starts a sub-expression that will evaluate to a
	// value of unknown type, so assume it could produce anything.
	if v.IsOpenParen() {
		return true
	}

	if v.IsWord() {
		w := v.AsWord()
		// "(" starts a sub-expression that will produce a value.
		if w.Name == "(" {
			return true
		}
		// ")" and "end" are terminators, not value-producers.
		if w.Name == ")" || w.Name == "end" {
			return false
		}
		// Boolean literals.
		if w.Name == "true" || w.Name == "false" {
			return TBoolean.Matches(expected)
		}
		// Type names stay as type literals.
		if _, isType := typeNames[w.Name]; isType {
			return false
		}
		// Defined word (via DefStacks): resolves to a known type.
		if ds := e.registry.DefStacks[w.Name]; len(ds) > 0 {
			return ds[len(ds)-1].VType.Matches(expected)
		}
		// Registered built-in function: could produce most types.
		// Specialized internal types (TFnUndef, TFnDef) can only be
		// produced by specific functions (fn).
		if e.registry.Lookup(w.Name) != nil {
			if expected.Equal(TFnUndef) || expected.Equal(TFnDef) {
				return w.Name == "fn"
			}
			return true
		}
		// Unknown word → becomes atom.
		return TAtom.Matches(expected)
	}

	// Non-word literal: already checked VType.Matches above.
	return false
}

// implicitEnd resolves a forward early when a type mismatch occurs.
func (e *Engine) implicitEnd(fwdIdx int) error {
	fwd := e.stack[fwdIdx].AsForward()
	funcIdx := fwd.FuncIndex
	collectedCount := fwd.CollectedArgs

	e.stackRemove(fwdIdx)
	if fwdIdx < funcIdx {
		funcIdx--
	}

	e.curryOrPrefix(funcIdx, collectedCount)
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

	e.curryOrPrefix(funcIdx, fwd.CollectedArgs)
	return nil
}

// stepMark records the mark's ID in the marks hash table and advances.
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

	// Delegate to apply callback continuation handler.
	if info.ApplyCont != nil {
		return e.stepMoveApply(markIdx, moveIdx, info)
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

// stepMoveApply handles an apply callback continuation move. It collects
// the resolved values between mark and move and splices them in place,
// completing the callback invocation.
func (e *Engine) stepMoveApply(markIdx, moveIdx int, info MoveInfo) error {
	// Collect resolved values between mark and move.
	var results []Value
	for j := markIdx + 1; j < moveIdx; j++ {
		results = append(results, e.stack[j])
	}

	// Remove mark from hash table.
	delete(e.marks, info.To)

	// Splice results in place of mark+body+move.
	e.stackSplice(markIdx, moveIdx-markIdx+1, results...)
	e.pointer = markIdx
	e.traceNote = "apply done"
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

				// Remove the forward.
				e.stackRemove(i)
				closeIdx--
				if i < funcIdx {
					funcIdx--
				}

				// Try prefix match or create curry list.
				e.curryOrPrefix(funcIdx, collectedCount)

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
			return fmt.Errorf("signature error: insufficient arguments for %s (expected %d suffix args)",
				fwd.FuncName, fwd.ExpectedArgs)
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

			if len(results) != len(rc.Returns) {
				return fmt.Errorf("%s: expected %d return value(s), got %d",
					rc.FuncName, len(rc.Returns), len(results))
			}
			for k, exp := range rc.Returns {
				if !results[k].VType.Matches(exp) {
					return fmt.Errorf("%s: return value %d: expected %s, got %s",
						rc.FuncName, k+1, exp, results[k].VType)
				}
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
// prefix matching.
func (e *Engine) effectiveResolved() []Value {
	start := 0
	for i := e.pointer - 1; i >= 0; i-- {
		if e.stack[i].IsOpenParen() {
			start = i + 1
			break
		}
	}
	var resolved []Value
	for i := start; i < e.pointer; i++ {
		v := e.stack[i]
		if !v.IsForward() && !v.IsOpenParen() && !v.IsMark() && !v.IsMove() {
			resolved = append(resolved, v)
		}
	}
	return resolved
}

// hasSuffixValues checks whether there are collectible value tokens after the
// current pointer. Literals and unknown words are collectible. Known function
// words are not directly collectible (they execute via stepWord), so they
// don't count — unless the function has signatures expecting TWord arguments
// (e.g., def needs to collect word names).
func (e *Engine) hasSuffixValues(fn *Function) bool {
	if e.pointer+1 >= len(e.stack) {
		return false
	}
	next := e.stack[e.pointer+1]
	if next.IsForward() || next.IsOpenParen() {
		return false
	}
	if next.IsWord() {
		nw := next.AsWord()
		if nw.Name == ")" || nw.Name == "end" {
			return false
		}
		if e.registry.Lookup(nw.Name) != nil {
			// Known function — only collectible if fn expects TWord args.
			for si := range fn.Signatures {
				for _, argType := range fn.Signatures[si].Args {
					if argType.Equal(TWord) {
						return true
					}
				}
			}
			return false
		}
	}
	return true
}

// peekSuffixValue returns a value representing what the next stack element
// would resolve to, for use in speculative signature matching. Unknown words
// become atoms; true/false become booleans; literals are returned as-is.
func (e *Engine) peekSuffixValue() Value {
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
			return NewAtom(nw.Name)
		}
	}
	return next
}

// peekPrecedence returns the highest precedence of the word at stack[idx].
func (e *Engine) peekPrecedence(idx int) int {
	if idx >= len(e.stack) {
		return 0
	}
	v := e.stack[idx]
	if !v.IsWord() {
		return 0
	}
	w := v.AsWord()
	fn := e.registry.Lookup(w.Name)
	if fn == nil || !fn.SuffixPrecedence {
		return 0
	}
	var maxPrec int
	for i := range fn.Signatures {
		sig := &fn.Signatures[i]
		if sig.Precedence > maxPrec {
			maxPrec = sig.Precedence
		}
	}
	return maxPrec
}

// curryOrPrefix handles a terminated forward. If the word at funcIdx can
// match a prefix signature with the available resolved values, it forces
// prefix mode for normal execution. Otherwise, it packages the word and
// its collectedCount suffix args into a list value (partial application).
// When the list is later expanded (e.g., via def body substitution), the
// word and args are spliced back onto the stack for completion.
func (e *Engine) curryOrPrefix(funcIdx int, collectedCount int) {
	if funcIdx >= len(e.stack) || !e.stack[funcIdx].IsWord() {
		e.pointer = funcIdx
		return
	}

	w := e.stack[funcIdx].AsWord()
	fn := e.registry.Lookup(w.Name)

	// Check if prefix match exists with current resolved values.
	if fn != nil {
		// Build resolved slice up to funcIdx.
		start := 0
		for i := funcIdx - 1; i >= 0; i-- {
			if e.stack[i].IsOpenParen() {
				start = i + 1
				break
			}
		}
		var resolved []Value
		for i := start; i < funcIdx; i++ {
			v := e.stack[i]
			if !v.IsForward() && !v.IsOpenParen() {
				resolved = append(resolved, v)
			}
		}

		testW := WordInfo{Name: w.Name, ArgCount: -1, ForcePrefix: true}
		match := MatchSignature(fn.Signatures, resolved, testW)
		if match != nil {
			// Prefix match works - proceed normally.
			e.stack[funcIdx] = NewWordModified(w.Name, w.ArgCount, true, false)
			e.pointer = funcIdx
			return
		}
	}

	// Check if there's a pending outer forward that would collect the result.
	// Only create a curry list when an outer context is waiting for a value;
	// otherwise, fall through to normal prefix retry (which may error).
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

	// No outer forward - force prefix (may result in error on next step).
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
			nextIdx := fwd.CollectedArgs
			if nextIdx < len(fwd.Sig.Args) {
				return fwd.Sig.Args[nextIdx].Equal(TWord)
			}
			break
		}
	}
	return false
}
