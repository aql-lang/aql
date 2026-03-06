package engine

import "fmt"

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
// It receives the step number, pointer position, full stack, and stack length.
type TraceCallback func(step int, pointer int, stack []Value)

// Engine is the AQL stack machine.
type Engine struct {
	stack    []Value
	pointer  int
	registry *Registry
	trace    TraceCallback
}

// New creates an Engine with the given function registry.
func New(registry *Registry) *Engine {
	return &Engine{registry: registry}
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
	if cap(e.stack) >= len(input) {
		e.stack = e.stack[:len(input)]
	} else {
		e.stack = make([]Value, len(input), len(input)+stackHeadroom)
	}
	copy(e.stack, input)
	e.pointer = 0

	limit := 1000 // safety bound
	for step := 0; step < limit; step++ {
		if e.pointer >= len(e.stack) {
			break
		}

		val := e.stack[e.pointer]

		if e.trace != nil {
			snapshot := make([]Value, len(e.stack))
			copy(snapshot, e.stack)
			e.trace(step, e.pointer, snapshot)
		}

		switch {
		case val.IsWord():
			if err := e.stepWord(val); err != nil {
				return nil, err
			}

		case val.IsForward():
			e.pointer++

		case val.IsOpenParen():
			e.pointer++

		default:
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

	return e.stack, nil
}

// resolveOrphanedForwards handles end-of-input by resolving pending forwards.
func (e *Engine) resolveOrphanedForwards() error {
	for attempt := 0; attempt < 100; attempt++ {
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
		return e.execMatch(match)
	}

	if w.ForceSuffix {
		// Force suffix: skip prefix attempt, collect all args from suffix.
		resolved := e.effectiveResolved()
		bestSig, _ := e.bestSigForForward(fn, w, resolved)
		if bestSig == nil {
			return fmt.Errorf("signature error: no matching signature for %s", w.Name)
		}
		return e.insertForward(w, bestSig, len(bestSig.Args))
	}

	if fn.SuffixPrecedence {
		resolved := e.effectiveResolved()
		match := MatchSignature(fn.Signatures, resolved, w)

		// Use prefix match only if it has args (typed signature).
		// Defer 0-arg matches (generic def handler) so suffix-mode
		// typed signatures get a chance to collect arguments first.
		if match != nil && len(match.Sig.Args) > 0 {
			return e.execMatch(match)
		}

		// Try suffix: create forward to collect remaining args.
		bestSig, prefixCount := e.bestSigForForward(fn, w, resolved)
		if bestSig != nil {
			suffixNeeded := len(bestSig.Args) - prefixCount
			if suffixNeeded <= 0 {
				suffixNeeded = len(bestSig.Args)
			}
			return e.insertForward(w, bestSig, suffixNeeded)
		}

		// Fall back to 0-arg match (generic def handler).
		if match != nil {
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
		if !e.stack[i].IsForward() {
			indices = append(indices, i)
		}
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
		if exclude[i] || e.stack[i].IsForward() || e.stack[i].IsOpenParen() {
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

	if fwd.CollectedArgs >= fwd.ExpectedArgs {
		// All suffix args collected. Remove forward, force prefix, retry.
		e.stackRemove(fwdIdx)
		// fwdIdx is after funcIdx, so funcIdx is unaffected.

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
	for attempt := 0; attempt < 50; attempt++ {
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
		if !v.IsForward() && !v.IsOpenParen() {
			resolved = append(resolved, v)
		}
	}
	return resolved
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
