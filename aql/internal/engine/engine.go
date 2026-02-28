package engine

import "fmt"

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
			// Forward entries are skipped during normal traversal.
			// They are consumed by stepLiteral when a value is resolved
			// after a forward.
			e.pointer++

		case val.IsOpenParen():
			// Open-paren markers are skipped during normal traversal.
			// They act as barriers for forward search and prefix matching.
			e.pointer++

		default:
			// Literal / resolved value.
			if err := e.stepLiteral(); err != nil {
				return nil, err
			}
		}
	}

	// Check for orphaned forward entries — suffix args were never collected.
	for _, v := range e.stack {
		if v.IsForward() {
			fwd := v.AsForward()
			return nil, fmt.Errorf("signature error: insufficient arguments for %s (expected %d suffix args)",
				fwd.FuncName, fwd.ExpectedArgs)
		}
		if v.IsOpenParen() {
			return nil, fmt.Errorf("syntax error: unmatched opening parenthesis")
		}
	}

	return e.stack, nil
}

// stepWord handles a word (function reference) at the current pointer.
func (e *Engine) stepWord(val Value) error {
	w := val.AsWord()

	// Language keywords handled directly by the engine.
	if w.Name == "end" {
		return e.stepEnd()
	}
	if w.Name == "(" {
		return e.stepOpenParen()
	}
	if w.Name == ")" {
		return e.stepCloseParen()
	}

	fn := e.registry.Lookup(w.Name)

	if fn == nil {
		// Unknown word — treat as a bare string value.
		// Don't advance pointer so stepLiteral runs on the next iteration
		// and can collect this value for any pending forward.
		e.stack[e.pointer] = NewString(w.Name)
		return nil
	}

	resolved := e.effectiveResolved()
	match := MatchSignature(fn.Signatures, resolved, w)

	if match == nil {
		// No prefix match. Check if any suffix signature exists.
		suffixMatch := matchSuffixOnly(fn.Signatures, w)
		if suffixMatch != nil {
			return e.insertForward(w, suffixMatch)
		}
		return fmt.Errorf("signature error: no matching signature for %s", w.Name)
	}

	if match.Sig.IsPrefixOnly() {
		return e.execPrefix(match)
	}

	// Signature has suffix args — use forward mechanism to collect them.
	return e.insertForward(w, match)
}

// execPrefix executes a prefix-matched signature.
func (e *Engine) execPrefix(match *MatchResult) error {
	n := match.PrefixLen
	argStart := e.pointer - n
	args := make([]Value, n)
	copy(args, e.stack[argStart:e.pointer])

	results, err := match.Sig.Handler(args)
	if err != nil {
		return err
	}

	// Splice: remove args + word, insert results.
	newStack := make([]Value, 0, len(e.stack)-n-1+len(results))
	newStack = append(newStack, e.stack[:argStart]...)
	newStack = append(newStack, results...)
	newStack = append(newStack, e.stack[e.pointer+1:]...)
	e.stack = newStack

	// Pointer moves to start of results so they re-enter the main loop.
	// This allows results to be collected by any pending forward.
	e.pointer = argStart
	return nil
}

// insertForward handles a suffix signature by placing a forward primitive
// after the word on the stack.
func (e *Engine) insertForward(w WordInfo, match *MatchResult) error {
	fwd := NewForward(ForwardInfo{
		FuncName:     w.Name,
		ExpectedArgs: len(match.Sig.Suffix),
		FuncIndex:    e.pointer,
		Precedence:   match.Sig.Precedence,
		Sig:          match.Sig,
	})

	// Insert fwd after the word at e.pointer.
	newStack := make([]Value, 0, len(e.stack)+1)
	newStack = append(newStack, e.stack[:e.pointer+1]...)
	newStack = append(newStack, fwd)
	newStack = append(newStack, e.stack[e.pointer+1:]...)
	e.stack = newStack

	// Advance past word and forward.
	e.pointer += 2
	return nil
}

// stepLiteral handles a resolved (non-word, non-forward) value at the pointer.
// After advancing, it checks whether a pending forward should consume this value.
func (e *Engine) stepLiteral() error {
	valIdx := e.pointer

	// Look backwards for the nearest forward entry, stopping at open-paren barriers.
	fwdIdx := -1
	for i := valIdx - 1; i >= 0; i-- {
		if e.stack[i].IsOpenParen() {
			break // open-paren marker acts as a barrier
		}
		if e.stack[i].IsForward() {
			fwdIdx = i
			break
		}
	}

	if fwdIdx < 0 {
		// No pending forward — just advance.
		e.pointer++
		return nil
	}

	fwd := e.stack[fwdIdx].AsForward()
	funcIdx := fwd.FuncIndex

	// Peek ahead: if the next item is a higher-precedence infix operator,
	// defer collection and let that operator execute first.
	// Only applies when the forward itself participates in precedence (> 0).
	// Non-arithmetic forwards (get, set, etc.) have precedence 0 and should
	// not be affected by upcoming operators.
	if fwd.Precedence > 0 {
		if nextPrec := e.peekPrecedence(valIdx + 1); nextPrec > fwd.Precedence {
			e.pointer++
			return nil
		}
	}

	// Remove the value from its current position.
	val := e.stack[valIdx]
	e.stack = append(e.stack[:valIdx], e.stack[valIdx+1:]...)

	// Insert the value before the function word.
	newStack := make([]Value, 0, len(e.stack)+1)
	newStack = append(newStack, e.stack[:funcIdx]...)
	newStack = append(newStack, val)
	newStack = append(newStack, e.stack[funcIdx:]...)
	e.stack = newStack

	// The forward and function word shifted right by 1.
	funcIdx++
	fwdIdx++

	fwd.CollectedArgs++
	fwd.FuncIndex = funcIdx

	if fwd.CollectedArgs >= fwd.ExpectedArgs {
		// All suffix args collected. Remove the forward and execute directly.
		e.stack = append(e.stack[:fwdIdx], e.stack[fwdIdx+1:]...)
		// fwdIdx was after funcIdx, so funcIdx is unaffected by the removal.

		// The stack now has [... | prefix_args | suffix_args | func_word | ...].
		// Execute the handler directly with all args.
		totalArgs := len(fwd.Sig.Prefix) + len(fwd.Sig.Suffix)
		argStart := funcIdx - totalArgs
		args := make([]Value, totalArgs)
		copy(args, e.stack[argStart:funcIdx])

		// Validate arg types — the suffix args were collected without type checks.
		for i, arg := range args {
			var expectedType Type
			if i < len(fwd.Sig.Prefix) {
				expectedType = fwd.Sig.Prefix[i]
			} else {
				expectedType = fwd.Sig.Suffix[i-len(fwd.Sig.Prefix)]
			}
			if !arg.VType.Matches(expectedType) {
				return fmt.Errorf("signature error: no matching signature for %s", fwd.FuncName)
			}
		}

		results, err := fwd.Sig.Handler(args)
		if err != nil {
			return err
		}

		// Splice: remove args + word, insert results.
		newStack := make([]Value, 0, len(e.stack)-totalArgs-1+len(results))
		newStack = append(newStack, e.stack[:argStart]...)
		newStack = append(newStack, results...)
		newStack = append(newStack, e.stack[funcIdx+1:]...)
		e.stack = newStack

		// Pointer moves to start of results for re-examination.
		e.pointer = argStart
	} else {
		// Update the forward in the stack.
		e.stack[fwdIdx] = NewForward(fwd)
		e.pointer = fwdIdx + 1
	}

	return nil
}

// stepEnd handles the "end" keyword. If a forward is pending, it terminates
// the forward early by pulling remaining args from the prefix stack and
// rearranging so the function retries with correct arg order. If no forward
// is pending, it simply removes itself (no-op).
func (e *Engine) stepEnd() error {
	endIdx := e.pointer

	// Find nearest pending forward, stopping at open-paren barriers.
	fwdIdx := -1
	for i := endIdx - 1; i >= 0; i-- {
		if e.stack[i].IsOpenParen() {
			break // open-paren marker acts as a barrier
		}
		if e.stack[i].IsForward() {
			fwdIdx = i
			break
		}
	}

	if fwdIdx < 0 {
		// No pending forward — just remove end (no-op).
		e.stack = append(e.stack[:endIdx], e.stack[endIdx+1:]...)
		return nil
	}

	fwd := e.stack[fwdIdx].AsForward()
	funcIdx := fwd.FuncIndex
	collected := fwd.CollectedArgs
	remaining := fwd.ExpectedArgs - collected
	suffixStart := funcIdx - collected

	// Check if enough prefix args available for the remaining suffix args.
	if suffixStart < remaining {
		return fmt.Errorf("end: insufficient arguments for %s (need %d more, have %d)",
			fwd.FuncName, remaining, suffixStart)
	}

	takenStart := suffixStart - remaining

	// Rebuild the stack with rearranged args:
	//   [untouched_prefix | collected_suffix | taken_prefix | func | rest...]
	// The taken prefix args are moved after the collected suffix args so the
	// prefix handler sees them in the correct order (suffix args deepest).
	newStack := make([]Value, 0, len(e.stack)-2) // -2 for forward and end
	newStack = append(newStack, e.stack[:takenStart]...)          // untouched prefix
	newStack = append(newStack, e.stack[suffixStart:funcIdx]...)  // collected suffix
	newStack = append(newStack, e.stack[takenStart:suffixStart]...) // taken prefix
	newStack = append(newStack, e.stack[funcIdx])                 // function word
	// Everything after forward, excluding end.
	for i := fwdIdx + 1; i < len(e.stack); i++ {
		if i == endIdx {
			continue
		}
		newStack = append(newStack, e.stack[i])
	}
	e.stack = newStack

	// funcIdx in the new stack: same total items before the function word.
	newFuncIdx := takenStart + collected + remaining

	// Clear modifiers on the function word for retry.
	if e.stack[newFuncIdx].IsWord() {
		w := e.stack[newFuncIdx].AsWord()
		e.stack[newFuncIdx] = NewWord(w.Name)
	}

	e.pointer = newFuncIdx
	return nil
}

// stepOpenParen replaces the "(" word at the current pointer with an open-paren
// marker and advances past it.
func (e *Engine) stepOpenParen() error {
	e.stack[e.pointer] = NewOpenParen()
	e.pointer++
	return nil
}

// stepCloseParen handles the ")" word. It finds the nearest open-paren marker,
// checks for orphaned forwards inside the paren scope, collapses the
// sub-expression results, removes the marker and ")", and sets the pointer
// so results re-enter the main loop.
func (e *Engine) stepCloseParen() error {
	closeIdx := e.pointer

	// Find the nearest open-paren marker scanning backwards.
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

	// Check for orphaned forwards inside the paren scope.
	for i := openIdx + 1; i < closeIdx; i++ {
		if e.stack[i].IsForward() {
			fwd := e.stack[i].AsForward()
			return fmt.Errorf("signature error: insufficient arguments for %s (expected %d suffix args)",
				fwd.FuncName, fwd.ExpectedArgs)
		}
	}

	// Collect resolved values between open marker and close paren.
	results := make([]Value, 0, closeIdx-openIdx-1)
	for i := openIdx + 1; i < closeIdx; i++ {
		results = append(results, e.stack[i])
	}

	// Splice: remove open marker + contents + close paren, insert results.
	newStack := make([]Value, 0, len(e.stack)-(closeIdx-openIdx+1)+len(results))
	newStack = append(newStack, e.stack[:openIdx]...)
	newStack = append(newStack, results...)
	newStack = append(newStack, e.stack[closeIdx+1:]...)
	e.stack = newStack

	// Set pointer to start of results so they re-enter the main loop.
	// This lets them be collected by any pending forward before the paren.
	e.pointer = openIdx
	return nil
}

// effectiveResolved returns the resolved portion of the stack visible for
// prefix matching. It starts from after the last open-paren marker (if any)
// up to the current pointer, excluding internal values (forwards, markers).
func (e *Engine) effectiveResolved() []Value {
	start := 0
	for i := e.pointer - 1; i >= 0; i-- {
		if e.stack[i].IsOpenParen() {
			start = i + 1
			break
		}
	}
	// Filter out forward entries from the resolved range.
	var resolved []Value
	for i := start; i < e.pointer; i++ {
		v := e.stack[i]
		if !v.IsForward() && !v.IsOpenParen() {
			resolved = append(resolved, v)
		}
	}
	return resolved
}

// peekPrecedence returns the highest infix precedence of the word at stack[idx],
// or 0 if idx is out of range or the entry is not an infix word.
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
	if fn == nil {
		return 0
	}
	var maxPrec int
	for i := range fn.Signatures {
		sig := &fn.Signatures[i]
		if len(sig.Suffix) > 0 && sig.Precedence > maxPrec {
			maxPrec = sig.Precedence
		}
	}
	return maxPrec
}

// matchSuffixOnly finds a suffix-only signature, ignoring prefix requirements.
func matchSuffixOnly(sigs []Signature, modifiers WordInfo) *MatchResult {
	var best *MatchResult
	var bestScore int

	for i := range sigs {
		sig := &sigs[i]
		if len(sig.Suffix) == 0 {
			continue
		}
		if modifiers.ForcePrefix {
			continue
		}
		if modifiers.ArgCount >= 0 && sig.TotalArgs() != modifiers.ArgCount {
			continue
		}

		score := signatureScore(sig)
		if best == nil || score > bestScore {
			best = &MatchResult{Sig: sig, PrefixLen: 0}
			bestScore = score
		}
	}

	return best
}
