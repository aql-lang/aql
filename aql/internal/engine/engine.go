package engine

import "fmt"

// typeNames maps well-known type names to their Type, so bare words like
// "number" or "string" resolve to type-literal values instead of strings.
var typeNames = map[string]Type{
	"any":     TAny,
	"none":    TNone,
	"number":  TNumber,
	"string":  TString,
	"boolean": TBoolean,
	"list":    TList,
	"map":     TMap,
}

// Engine is the AQL stack machine.
type Engine struct {
	stack    []Value
	pointer  int
	registry *Registry
}

// New creates an Engine with the given function registry.
func New(registry *Registry) *Engine {
	return &Engine{registry: registry}
}

// Run executes the input values through the stack machine and returns the
// resulting stack.
func (e *Engine) Run(input []Value) ([]Value, error) {
	e.stack = make([]Value, len(input))
	copy(e.stack, input)
	e.pointer = 0

	limit := 1000 // safety bound
	for step := 0; step < limit; step++ {
		if e.pointer >= len(e.stack) {
			break
		}

		val := e.stack[e.pointer]

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

		// Remove the forward marker.
		e.stack = append(e.stack[:fwdIdx], e.stack[fwdIdx+1:]...)
		if fwdIdx < funcIdx {
			funcIdx--
		}

		// Force prefix on the function word.
		if funcIdx < len(e.stack) && e.stack[funcIdx].IsWord() {
			w := e.stack[funcIdx].AsWord()
			e.stack[funcIdx] = NewWordModified(w.Name, w.ArgCount, true, false)
		}

		// Retry from the function word.
		e.pointer = funcIdx
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
		e.stack[e.pointer] = NewString(w.Name)
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
		// the END of sig.Args (prefix portion). In the suffix-first model,
		// suffix fills Args[0..S-1] and prefix fills Args[S..N-1].
		// So prefix args match against the last slots.
		prefixCount := 0
		for tryN := len(sig.Args); tryN >= 1; tryN-- {
			if tryN > len(resolved) {
				continue
			}
			match := true
			for j := 0; j < tryN; j++ {
				rIdx := len(resolved) - tryN + j
				sigIdx := len(sig.Args) - tryN + j
				if !resolved[rIdx].VType.Matches(sig.Args[sigIdx]) {
					match = false
					break
				}
			}
			if match {
				prefixCount = tryN
				break
			}
		}

		score := signatureScore(sig)

		// Bonus: if the peeked suffix value matches the first expected
		// suffix arg type, boost this sig's score to prefer it.
		// In the suffix-first model, suffix always fills from Args[0].
		if peekVal != nil && prefixCount < len(sig.Args) {
			firstSuffixType := sig.Args[0]
			if peekVal.VType.Matches(firstSuffixType) {
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

	results, err := match.Sig.Handler(match.Args)
	if err != nil {
		return err
	}

	if len(indices) == n && n > 0 {
		firstArgIdx := indices[0]

		// Build new stack: keep everything before first arg, skip arg indices
		// and the word, keep internal forwards, insert results.
		skipSet := make(map[int]bool, n+1)
		for _, idx := range indices {
			skipSet[idx] = true
		}
		skipSet[e.pointer] = true // skip the word itself

		newStack := make([]Value, 0, len(e.stack)-n-1+len(results))
		newStack = append(newStack, e.stack[:firstArgIdx]...)

		// Copy non-arg, non-word items between first arg and after word.
		for i := firstArgIdx; i <= e.pointer; i++ {
			if !skipSet[i] {
				newStack = append(newStack, e.stack[i])
			}
		}

		newStack = append(newStack, results...)
		newStack = append(newStack, e.stack[e.pointer+1:]...)
		e.stack = newStack
		e.pointer = firstArgIdx
	} else if n == 0 {
		// No args, just replace the word with results.
		newStack := make([]Value, 0, len(e.stack)-1+len(results))
		newStack = append(newStack, e.stack[:e.pointer]...)
		newStack = append(newStack, results...)
		newStack = append(newStack, e.stack[e.pointer+1:]...)
		e.stack = newStack
		// Pointer stays at same position to re-examine results.
	} else {
		// Fallback: simple contiguous splice.
		argStart := e.pointer - n
		if argStart < 0 {
			argStart = 0
		}
		newStack := make([]Value, 0, len(e.stack)-n-1+len(results))
		newStack = append(newStack, e.stack[:argStart]...)
		newStack = append(newStack, results...)
		newStack = append(newStack, e.stack[e.pointer+1:]...)
		e.stack = newStack
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

	newStack := make([]Value, 0, len(e.stack)+1)
	newStack = append(newStack, e.stack[:e.pointer+1]...)
	newStack = append(newStack, fwd)
	newStack = append(newStack, e.stack[e.pointer+1:]...)
	e.stack = newStack

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

	// Check if the value matches the next expected suffix type.
	// In the suffix-first model, suffix fills Args[0..S-1] in order.
	nextArgIdx := fwd.CollectedArgs
	if nextArgIdx < len(fwd.Sig.Args) {
		expectedType := fwd.Sig.Args[nextArgIdx]
		val := e.stack[valIdx]
		if !val.VType.Matches(expectedType) {
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
	e.stack = append(e.stack[:valIdx], e.stack[valIdx+1:]...)

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

	newStack := make([]Value, 0, len(e.stack)+1)
	newStack = append(newStack, e.stack[:insertIdx]...)
	newStack = append(newStack, val)
	newStack = append(newStack, e.stack[insertIdx:]...)
	e.stack = newStack

	funcIdx++
	fwdIdx++

	fwd.CollectedArgs++
	fwd.FuncIndex = funcIdx

	if fwd.CollectedArgs >= fwd.ExpectedArgs {
		// All suffix args collected. Remove forward, force prefix, retry.
		e.stack = append(e.stack[:fwdIdx], e.stack[fwdIdx+1:]...)
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

	e.stack = append(e.stack[:fwdIdx], e.stack[fwdIdx+1:]...)
	if fwdIdx < funcIdx {
		funcIdx--
	}

	if funcIdx < len(e.stack) && e.stack[funcIdx].IsWord() {
		w := e.stack[funcIdx].AsWord()
		e.stack[funcIdx] = NewWordModified(w.Name, w.ArgCount, true, false)
	}

	e.pointer = funcIdx
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
		e.stack = append(e.stack[:endIdx], e.stack[endIdx+1:]...)
		return nil
	}

	fwd := e.stack[fwdIdx].AsForward()
	funcIdx := fwd.FuncIndex

	// Remove forward and end from the stack.
	// Remove higher index first to preserve lower indices.
	if endIdx > fwdIdx {
		e.stack = append(e.stack[:endIdx], e.stack[endIdx+1:]...)
		e.stack = append(e.stack[:fwdIdx], e.stack[fwdIdx+1:]...)
		if fwdIdx < funcIdx {
			funcIdx-- // forward removal
		}
		// end was already removed (endIdx > fwdIdx), endIdx > funcIdx always
	} else {
		e.stack = append(e.stack[:fwdIdx], e.stack[fwdIdx+1:]...)
		newEndIdx := endIdx
		if fwdIdx < endIdx {
			newEndIdx--
		}
		e.stack = append(e.stack[:newEndIdx], e.stack[newEndIdx+1:]...)
		if fwdIdx < funcIdx {
			funcIdx--
		}
		if newEndIdx < funcIdx {
			funcIdx--
		}
	}

	_ = fwd // consumed above

	if funcIdx < len(e.stack) && e.stack[funcIdx].IsWord() {
		w := e.stack[funcIdx].AsWord()
		e.stack[funcIdx] = NewWordModified(w.Name, w.ArgCount, true, false)
	}

	e.pointer = funcIdx
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

				// Remove the forward.
				e.stack = append(e.stack[:i], e.stack[i+1:]...)
				closeIdx--
				if i < funcIdx {
					funcIdx--
				}

				// Force prefix on the function word.
				if funcIdx < len(e.stack) && e.stack[funcIdx].IsWord() {
					w := e.stack[funcIdx].AsWord()
					e.stack[funcIdx] = NewWordModified(w.Name, w.ArgCount, true, false)
				}

				// Re-evaluate from funcIdx up to closeIdx.
				e.pointer = funcIdx
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

	// Collect resolved values.
	results := make([]Value, 0, closeIdx-openIdx-1)
	for i := openIdx + 1; i < closeIdx; i++ {
		results = append(results, e.stack[i])
	}

	// Splice.
	newStack := make([]Value, 0, len(e.stack)-(closeIdx-openIdx+1)+len(results))
	newStack = append(newStack, e.stack[:openIdx]...)
	newStack = append(newStack, results...)
	newStack = append(newStack, e.stack[closeIdx+1:]...)
	e.stack = newStack

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
