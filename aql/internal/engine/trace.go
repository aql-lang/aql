package engine

import (
	"fmt"
	"io"
	"strings"
)

// ANSI color codes for trace output.
const (
	cReset   = "\033[0m"
	cBold    = "\033[1m"
	cDim     = "\033[2m"
	cRed     = "\033[31m"
	cGreen   = "\033[32m"
	cYellow  = "\033[33m"
	cBlue    = "\033[34m"
	cMagenta = "\033[35m"
	cCyan    = "\033[36m"
	cWhite   = "\033[37m"
	cGray    = "\033[90m"
)

// traceColorize returns a colored string representation of a Value for trace output.
func traceColorize(v Value) string {
	switch {
	case v.IsWord():
		w, _ := v.AsWord()
		if w.ForceStack {
			return cYellow + w.Name + "/s" + cReset
		}
		if w.ForceForward {
			return cYellow + w.Name + "/f" + cReset
		}
		return cYellow + w.Name + cReset
	case v.IsForward():
		f, _ := v.AsForward()
		return cMagenta + fmt.Sprintf("→%s(%d/%d)", f.FuncName, f.CollectedArgs, f.ExpectedArgs) + cReset
	case v.IsOpenParen():
		return cDim + "(" + cReset
	case v.Data == nil:
		// Type literal
		return cCyan + v.VType.String() + cReset
	case v.VType.Matches(TString):
		return cGreen + fmt.Sprintf("%q", v.Data) + cReset
	case v.VType.Matches(TInteger):
		return cBlue + fmt.Sprintf("%d", v.Data) + cReset
	case v.VType.Matches(TBoolean):
		_as0, _ := v.AsBoolean()
		if _as0 {
			return cCyan + "true" + cReset
		}
		return cCyan + "false" + cReset
	case v.VType.Equal(TAtom):
		s, ok := v.Data.(string)
		if !ok {
			return cRed + fmt.Sprintf("%v", v.Data) + cReset
		}
		return cRed + s + cReset
	case v.VType.Equal(TList):
		elems := v.AsList().Slice()
		parts := make([]string, len(elems))
		for i, e := range elems {
			parts[i] = traceColorize(e)
		}
		return cDim + "[" + cReset + strings.Join(parts, " ") + cDim + "]" + cReset
	case v.VType.Equal(TMap):
		m := v.AsMap()
		parts := make([]string, 0, m.Len())
		for _, k := range m.Keys() {
			val, _ := m.Get(k)
			parts = append(parts, cWhite+k+cReset+cDim+":"+cReset+traceColorize(val))
		}
		return cDim + "{" + cReset + strings.Join(parts, " ") + cDim + "}" + cReset
	default:
		return cWhite + v.String() + cReset
	}
}

// traceStripANSI returns the visible length of a string, excluding ANSI escape codes.
func traceVisibleLen(s string) int {
	n := 0
	inEsc := false
	for _, r := range s {
		if r == '\033' {
			inEsc = true
			continue
		}
		if inEsc {
			if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
				inEsc = false
			}
			continue
		}
		n++
	}
	return n
}

// registerTrace registers the "trace" word for debugging.
//
// trace operates like do: it takes a list, evaluates it in a sub-engine,
// and prints a step-by-step trace of the stack evolution.
//
//	trace [1 add 2 mul 3]
func registerTrace(r *Registry) {
	r.RegisterNativeFunc(NativeFunc{
		Name:              "trace",
		ForwardPrecedence: true,
		Signatures: []NativeSig{{
			Args: []Type{TList},
			Handler: func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
				if args[0].Data == nil {
					return nil, fmt.Errorf("trace: argument must be a concrete list, got type literal")
				}
				elems := args[0].AsList().Slice()
				return runTrace(r, elems, r.Output)
			},
			Returns: []Type{TAny},
		}},
	})
}

// runTrace executes tokens in a sub-engine with tracing enabled,
// printing the stack evolution to w.
func runTrace(r *Registry, tokens []Value, w io.Writer) ([]Value, error) {
	const termWidth = 120
	const stepWidth = 4 // "NNN "

	type traceStep struct {
		step    int
		pointer int
		stack   []Value
		note    string
	}

	var steps []traceStep

	sub := New(r)
	sub.trace = func(step int, pointer int, stack []Value, note string) {
		steps = append(steps, traceStep{step, pointer, stack, note})
	}

	result, err := sub.Run(tokens)

	// Print header.
	fmt.Fprintf(w, "\n%s%s─── trace ──────────────────────────────────────────────────%s\n", cBold, cCyan, cReset)

	for _, s := range steps {
		// Build left side: resolved stack with pointer marker.
		// Format: [ val val ^val val | pending pending ]
		var leftParts []string
		for i, v := range s.stack {
			tok := traceColorize(v)
			if i == s.pointer {
				tok = cBold + cWhite + "^" + cReset + tok
			}
			leftParts = append(leftParts, tok)
		}

		// Split into resolved (before pointer) and pending (pointer onward).
		var resolved, pending []string
		for i, part := range leftParts {
			if i < s.pointer {
				resolved = append(resolved, part)
			} else {
				pending = append(pending, part)
			}
		}

		leftResolved := strings.Join(resolved, " ")
		leftPending := strings.Join(pending, " ")

		var leftStr string
		if len(resolved) > 0 && len(pending) > 0 {
			leftStr = leftResolved + " " + cDim + "│" + cReset + " " + leftPending
		} else if len(resolved) > 0 {
			leftStr = leftResolved
		} else {
			leftStr = leftPending
		}

		leftDisplay := cDim + "[" + cReset + " " + leftStr + " " + cDim + "]" + cReset
		leftVisLen := traceVisibleLen(leftDisplay)

		// Step number: right-aligned, 3 digits.
		stepStr := fmt.Sprintf("%s%3d%s", cGray, s.step, cReset)

		// Format the annotation (right-aligned).
		noteStr := ""
		noteVisLen := 0
		if s.note != "" {
			noteStr = cGray + s.note + cReset
			noteVisLen = traceVisibleLen(noteStr)
		}

		// Determine if we need wrapping.
		usedLen := stepWidth + leftVisLen
		if usedLen <= termWidth {
			if noteVisLen > 0 && usedLen+2+noteVisLen <= termWidth {
				// Everything fits — right-align the note.
				gap := termWidth - usedLen - noteVisLen
				if gap < 2 {
					gap = 2
				}
				fmt.Fprintf(w, "%s %s%s%s\n", stepStr, leftDisplay,
					strings.Repeat(" ", gap), noteStr)
			} else {
				// Stack fits, note on next line (right-aligned).
				fmt.Fprintf(w, "%s %s\n", stepStr, leftDisplay)
				if noteVisLen > 0 {
					gap := termWidth - noteVisLen
					if gap < stepWidth+1 {
						gap = stepWidth + 1
					}
					fmt.Fprintf(w, "%s%s\n", strings.Repeat(" ", gap), noteStr)
				}
			}
		} else {
			// Multi-line: wrap the stack display.
			lines := traceWrap(leftParts, s.pointer, termWidth-stepWidth-4)
			for i, line := range lines {
				if i == 0 {
					fmt.Fprintf(w, "%s %s\n", stepStr, line)
				} else {
					fmt.Fprintf(w, "%s %s\n", cGray+"   "+cReset, line)
				}
			}
			if noteVisLen > 0 {
				gap := termWidth - noteVisLen
				if gap < stepWidth+1 {
					gap = stepWidth + 1
				}
				fmt.Fprintf(w, "%s%s\n", strings.Repeat(" ", gap), noteStr)
			}
		}
	}

	// Print final state.
	if result != nil {
		var resParts []string
		for _, v := range result {
			resParts = append(resParts, traceColorize(v))
		}
		fmt.Fprintf(w, "%s%s─── result: %s%s[ %s %s]%s\n\n",
			cBold, cCyan, cReset,
			cDim, strings.Join(resParts, " "), cDim, cReset)
	}

	if err != nil {
		fmt.Fprintf(w, "%s%s─── error: %s%s\n\n", cBold, cRed, err.Error(), cReset)
		return nil, err
	}

	return result, nil
}

// traceWrap wraps a list of colored tokens into multiple display lines,
// each fitting within maxWidth visible characters. Maintains the [ ] and | framing.
func traceWrap(parts []string, pointer int, maxWidth int) []string {
	if maxWidth < 20 {
		maxWidth = 20
	}

	var lines []string
	var curLine []string
	curLen := 4 // account for "[ " prefix and " ]" suffix
	isFirst := true

	flush := func() {
		if len(curLine) == 0 {
			return
		}
		content := strings.Join(curLine, " ")
		if isFirst {
			lines = append(lines, cDim+"["+cReset+" "+content)
			isFirst = false
		} else {
			lines = append(lines, "  "+content)
		}
		curLine = nil
		curLen = 2 // indent for continuation
	}

	addedSep := false
	for i, part := range parts {
		// Insert separator between resolved and pending.
		if i == pointer && !addedSep && i > 0 {
			sep := cDim + "│" + cReset
			curLine = append(curLine, sep)
			curLen += 2
			addedSep = true
		}

		partLen := traceVisibleLen(part)
		if curLen+partLen+1 > maxWidth && len(curLine) > 0 {
			flush()
		}
		curLine = append(curLine, part)
		curLen += partLen + 1
	}

	// Close the last line with ]
	if len(curLine) > 0 {
		content := strings.Join(curLine, " ")
		if isFirst {
			lines = append(lines, cDim+"["+cReset+" "+content+" "+cDim+"]"+cReset)
		} else {
			lines = append(lines, "  "+content+" "+cDim+"]"+cReset)
		}
	} else if len(lines) > 0 {
		lines[len(lines)-1] += " " + cDim + "]" + cReset
	} else {
		lines = append(lines, cDim+"[ ]"+cReset)
	}

	return lines
}
