package engine

// plannerSequentialForward selects the best signature for forward collection
// by attempting each pre-sorted signature in order and scanning forward tokens
// sequentially. Unlike the scoring-based plannerBestSigForForward, this
// approach walks the actual forward token stream and uses structural cues
// (end, function words, open parens) to decide when to stop.
//
// For each signature (already sorted by signatureScore, highest first):
//  1. Try to fill args from front to back using forward tokens.
//  2. Any remaining unfilled suffix args are matched from the stack (reversed).
//  3. The first signature that can be fully satisfied wins.
//
// Token handling during forward scan:
//   - Literal/value: match against the current sig arg type
//   - end: stop scanning, try stack match for remaining args
//   - Known function word: stop before it, try stack match for remaining
//   - Open paren "(" : will resolve to a value — assume it matches (like TAny)
//   - Word with /q: treat as quoted (Atom) for matching
//   - Defined word: resolve to its def type for matching
//   - Unknown word: becomes Atom
func (e *Engine) plannerSequentialForward(fn *Function, w WordInfo, resolved []Value) (*Signature, int) {
	for i := range fn.Signatures {
		sig := &fn.Signatures[i]
		if len(sig.Args) == 0 {
			continue
		}
		if w.ArgCount >= 0 && sig.TotalArgs() != w.ArgCount {
			continue
		}
		if sig.Fallback {
			continue
		}

		forwardMatched, stackCount := e.trySequentialMatch(sig, resolved)
		if forwardMatched+stackCount == len(sig.Args) && forwardMatched > 0 {
			return sig, stackCount
		}
	}
	return nil, 0
}

// seqDebug is a temporary debug flag for development tracing.
var seqDebug = false

// trySequentialMatch walks the forward token stream and tries to match
// sig args from the front, then fills remaining args from the stack.
// Returns (forwardMatched, stackCount) where forwardMatched is how many
// args were matched from forward tokens and stackCount is how many were
// matched from the stack suffix.
func (e *Engine) trySequentialMatch(sig *Signature, resolved []Value) (int, int) {
	nArgs := len(sig.Args)
	forwardMatched := 0

	// Scan forward tokens starting after the current pointer.
	scanIdx := e.pointer + 1

	for forwardMatched < nArgs && scanIdx < len(e.stack) {
		tok := e.stack[scanIdx]
		expectedType := sig.Args[forwardMatched]

		// Structural token: stop forward scanning.
		if tok.IsForward() {
			break
		}

		if tok.IsWord() {
			ww := tok.AsWord()

			// "end" terminates forward collection.
			if ww.Name == "end" {
				break
			}

			// ")" terminates forward collection.
			if ww.Name == ")" {
				break
			}

			// "(" starts a sub-expression — assume it can produce any type.
			if ww.Name == "(" {
				// A paren expression will evaluate to some value.
				// Accept it if the expected type could be satisfied by any value.
				forwardMatched++
				// Skip past the matching ")" to find the next forward token.
				// For now, just count this as one token consumed.
				scanIdx++
				continue
			}

			// /q modifier: word at this position is treated as Atom.
			if sig.QuoteArgs != nil && sig.QuoteArgs[forwardMatched] {
				// Word will be captured as-is; check if Atom matches expected type.
				if TAtom.Matches(expectedType) {
					forwardMatched++
					scanIdx++
					continue
				}
				break // /q position but type doesn't match
			}

			// Defined word (simple value): resolves to its def type.
			// Check DefStacks BEFORE Lookup — simple defs don't register
			// functions, so Lookup won't find them. For fn-based defs,
			// both DefStacks and Lookup exist; we check DefStacks first
			// to get the actual value type for accurate matching.
			if ds := e.registry.DefStacks[ww.Name]; len(ds) > 0 {
				top := ds[len(ds)-1]
				if top.VType.Matches(expectedType) || expectedType.Equal(TAny) {
					forwardMatched++
					scanIdx++
					continue
				}
				// Def'd value type doesn't match — but it might be a
				// fn-based def that Lookup knows as a function. Fall
				// through to Lookup check.
				if _, ok := top.Data.(FnDefInfo); !ok {
					break // simple def, type mismatch
				}
			}

			// Known function with forward precedence: it will execute as a
			// sub-expression and produce a result.
			if knownFn := e.registry.Lookup(ww.Name); knownFn != nil {
				if knownFn.ForwardPrecedence {
					// This function will produce a value. We can't know the
					// exact type, so accept it optimistically.
					forwardMatched++
					scanIdx++
					continue
				}
				// Stack-only function: can't produce a value in forward context.
				// Stop scanning.
				break
			}

			// Unknown word: becomes Atom.
			resolvedType := TAtom
			if ww.Name == "true" || ww.Name == "false" {
				resolvedType = TBoolean
			} else if _, isType := typeNames[ww.Name]; isType {
				// Type names stay as type literals — not forward-collectible values.
				break
			}
			if resolvedType.Matches(expectedType) || expectedType.Equal(TAny) {
				forwardMatched++
				scanIdx++
				continue
			}
			break // unknown word type doesn't match
		}

		// Open paren value (already an OpenParen marker).
		if tok.IsOpenParen() {
			// Will resolve to some value — accept optimistically.
			forwardMatched++
			scanIdx++
			continue
		}

		// Literal value: direct type check.
		if tok.VType.Matches(expectedType) || expectedType.Equal(TAny) {
			// Reject type literals (nil data) for concrete Map/List.
			if tok.Data == nil && (expectedType.Equal(TMap) || expectedType.Equal(TList)) {
				break
			}
			forwardMatched++
			scanIdx++
			continue
		}

		// Type mismatch — stop forward scanning.
		break
	}

	// If all args matched from forward, no stack args needed.
	if forwardMatched == nArgs {
		return forwardMatched, 0
	}

	// Try to fill remaining args from the stack (suffix matching).
	remaining := nArgs - forwardMatched
	stackCount := e.sequentialStackMatch(sig.Args, forwardMatched, resolved)
	if stackCount == remaining {
		return forwardMatched, stackCount
	}

	// Partial match — not enough args.
	return forwardMatched, 0
}

// sequentialStackMatch checks if the top of the resolved stack can fill
// sig.Args[startIdx:] in reverse order (top of stack → first remaining arg).
func (e *Engine) sequentialStackMatch(sigArgs []Type, startIdx int, resolved []Value) int {
	remaining := len(sigArgs) - startIdx
	if remaining <= 0 || len(resolved) < remaining {
		return 0
	}

	// Stack top fills sigArgs[startIdx], next fills sigArgs[startIdx+1], etc.
	for j := 0; j < remaining; j++ {
		stackVal := resolved[len(resolved)-1-j]
		if !stackVal.VType.Matches(sigArgs[startIdx+j]) {
			return 0
		}
	}
	return remaining
}
