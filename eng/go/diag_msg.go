package eng

import (
	"strconv"
	"strings"
)

// This file is the single text source for the DISPATCH error family
// (design/DIAGNOSTICS.0.md, phase 3) — the message builders every
// surface that reports an unmatched dispatch shares: the interpreter
// (sigError / the swap-probe interception), the VM's compiled param
// contracts (checkParamContract / checkNativeParamContract), the
// check-pass trap (tryRecordUnmatchedDispatchTrap), the registry's
// 0-arg fallback (fnFallbackSig), and the checker's no_signature
// diagnostics (checkModeAssumeSig). One text source is what keeps the
// compiled-vs-interpreted differential gate (Detail equality) holding
// by construction — the return_check_msg.go pattern, generalized.

// noMatchDetail is the Detail line for an unmatched dispatch.
//
// CONSTRAINT (load-bearing): the text is a function of the word name
// ALONE. The VM's param-contract guards and the check pass's OpTrap
// bake this Detail at COMPILE time, while the interpreter builds it at
// the runtime failure — any operand- or stack-dependent content here
// would desync the two and fail the compiled_fullcorpus
// Detail-equality gate. Everything richer (the received arguments,
// per-candidate verdicts, the stack snapshot, fix suggestions) rides
// in interpreter-side Notes/Suggestions, which the gate does not
// compare (the same latitude Hint has always had).
func noMatchDetail(name string) string {
	return "cannot call `" + name + "` — no signature matches the arguments"
}

// insufficientArgsDetail is the Detail line for a forward collection
// that could not gather the arguments its matched signature planned
// for (the word's strict forward window ran out of usable tokens).
func insufficientArgsDetail(name string, expected int) string {
	return "cannot call `" + name + "` — it expects " +
		strconv.Itoa(expected) + " forward " + pluralArg(expected) +
		" but fewer were supplied"
}

// forwardParensSuggestion is the shared "group the call in parens" fix
// for a forward-collecting word starved by the next word — raised both
// by the interpreter's sigError and the registry's 0-arg fallback.
func forwardParensSuggestion(name string) string {
	return "forward args for " + name +
		" may have run into the next word; group the call in parens so its " +
		"RESULT becomes the argument — (" + name + " …). `end` / `;` only ends " +
		"the statement — it does NOT turn a following word into a nested call."
}

// describeSuggestion points a dispatch failure at the word's full
// reference entry. Only offered when the candidate list was truncated
// or the word carries many overloads — for a one-signature word the
// candidate verdict already says everything describe would.
func describeSuggestion(name string) string {
	return "see `aql describe " + name + "` for its signatures and examples"
}

// callArgsNote describes the tuple the failed dispatch saw:
// "the argument was 99 (an Integer)" /
// "the arguments were 'x' (a String) and 2 (an Integer)".
// Empty when no operands were in view.
func callArgsNote(written []Value) string {
	if len(written) == 0 {
		return ""
	}
	parts := make([]string, len(written))
	for i, v := range written {
		parts[i] = describeArgValue(v)
	}
	if len(parts) == 1 {
		return "the argument was " + parts[0]
	}
	last := parts[len(parts)-1]
	return "the arguments were " + strings.Join(parts[:len(parts)-1], ", ") +
		" and " + last
}

// describeArgValue renders one operand as “value (a Type)” — the
// expected-vs-found voice used across the dispatch notes.
func describeArgValue(v Value) string {
	leaf := "?"
	if t := ValueType(v); t != nil {
		leaf = t.Leaf()
	}
	return diagValue(v) + " (" + typeArticle(leaf) + " " + leaf + ")"
}

// typeArticle picks the indefinite article for a type name.
func typeArticle(name string) string {
	if name == "" {
		return "a"
	}
	switch name[0] {
	case 'A', 'E', 'I', 'O', 'U', 'a', 'e', 'i', 'o', 'u':
		return "an"
	}
	return "a"
}

// diagMaxCandidates caps the per-candidate verdict notes so a heavily
// overloaded word (add: numeric + concat + patrun forms) does not bury
// the error; the remainder collapses into an “…and N more” line and
// the describe suggestion.
const diagMaxCandidates = 3

// candidateNotes renders explainCandidates' verdicts as notes, ranked
// nearest-first (highest Score), capped at diagMaxCandidates.
// totalSigs is the word's non-fallback overload count; nWritten the
// probed tuple length (for the arity verdict's "but N were supplied").
func candidateNotes(name string, fails []CandidateFailure, totalSigs, nWritten int) []string {
	if len(fails) == 0 {
		return nil
	}
	shown := len(fails)
	if shown > diagMaxCandidates {
		shown = diagMaxCandidates
	}
	notes := make([]string, 0, shown+1)
	for _, f := range fails[:shown] {
		label := "candidate `" + name + " " + describeSigArgs(f.Sig) + "`"
		switch f.Kind {
		case candArity:
			n := f.Sig.TotalArgs()
			supplied := "none were"
			if nWritten == 1 {
				supplied = "1 was"
			} else if nWritten > 1 {
				supplied = strconv.Itoa(nWritten) + " were"
			}
			notes = append(notes, label+" takes "+strconv.Itoa(n)+" "+
				pluralArg(n)+", but "+supplied+" supplied")
		case candSlotType:
			notes = append(notes, label+" — argument "+strconv.Itoa(f.SlotIdx+1)+
				": expected "+f.Expected.String()+", got "+describeArgValue(f.Got))
		default: // candPattern
			notes = append(notes, label+" — argument "+strconv.Itoa(f.SlotIdx+1)+
				": "+diagValue(f.Got)+" does not satisfy its declared pattern "+
				diagValue(f.Pattern))
		}
	}
	if hidden := totalSigs - shown; hidden > 0 {
		notes = append(notes, "…and "+strconv.Itoa(hidden)+" more "+
			pluralSig(hidden))
	}
	return notes
}

// pluralArg / pluralSig — grammatical number for the two nouns the
// dispatch notes count.
func pluralArg(n int) string {
	if n == 1 {
		return "argument"
	}
	return "arguments"
}

func pluralSig(n int) string {
	if n == 1 {
		return "signature"
	}
	return "signatures"
}
