// Package docexamples extracts and runs the `expr => result` examples
// embedded in AQL's prose documentation (README, REFERENCE, TUTORIAL,
// HOWTO, EXPLANATION) so the docs can't silently drift from real engine
// behavior. The extractor (this file) is pure text→[]Example with no
// engine dependency, so it is unit-testable in isolation; the runner
// (docexamples_test.go) evaluates each Example through the production
// language layer and compares the rendered stack to the documented
// result.
//
// Two surface forms are recognised inside fenced code blocks:
//
//   - inline arrow:   `expr => result`        (REFERENCE/HOWTO/README/…)
//   - REPL prompt:     `aql> expr => result`   (TUTORIAL), and the bare
//     `aql> expr` continuation lines that set up state (e.g. `def`)
//     consumed by a later `=>` line in the same block.
//
// A documented result of `error` / `build error` / `error: …` means the
// example is expected to fail (loose substring match), not to equal a
// rendered value.
package docexamples

import "strings"

// Example is one extracted documentation example.
type Example struct {
	File string // source file basename, e.g. "REFERENCE.md"
	Line int    // 1-based line number of the `=>` line in File

	// Program is the full AQL source to evaluate: the block's preceding
	// setup lines (everything before this line that was not itself an
	// `=>` line) joined by newlines, then the expression on this line.
	// Setup-only `=>` lines are deliberately excluded so a block with
	// several result lines doesn't pile multiple values on the stack.
	Program string

	// Expr is just this line's left-hand expression (for subtest names
	// and diagnostics).
	Expr string

	// Expected is the documented right-hand side with any trailing
	// ` # comment` stripped. Meaningful only when WantErr is false.
	Expected string

	// WantErr is true when the documented result denotes a failure
	// (`error`, `build error`, `error: …`). ErrSubstr, when non-empty,
	// is the text after `error:` that the actual error must contain.
	WantErr   bool
	ErrSubstr string
}

// skipMarker on the line directly above a fence opts that whole block
// out of extraction (used for the handful of non-runnable examples:
// fetch/network, time/random, file/import, ellipsis output, multi-value
// stacks, and syntax-template fragments).
const skipMarker = "<!-- aql-test: skip -->"

// Extract pulls every checkable `=>` example out of one markdown file.
// file is the basename recorded on each Example; src is the file body.
func Extract(file, src string) []Example {
	var out []Example
	lines := strings.Split(src, "\n")

	inFence := false
	skipBlock := false
	prevNonBlank := "" // last non-blank line seen before the current fence

	// setup accumulates the binding (`def`/`undef`) lines of the current
	// block, in order, so a later `=>` line can prepend them as shared
	// state. setupOpen tracks unbalanced `[`/`(` from a multi-line `def`
	// so its continuation lines are captured too.
	var setup []string
	setupOpen := 0

	// prevCode/prevLine hold the most recent non-arrow expression line so
	// a following `=> result` (result on its own line) can attach to it.
	prevCode := ""
	prevLine := 0

	for i, raw := range lines {
		trimmed := strings.TrimSpace(raw)

		// Fence boundary: a line whose first non-space content is ```.
		if strings.HasPrefix(trimmed, "```") {
			if !inFence {
				// Opening fence. Only plain (untagged) or ```aql blocks
				// carry AQL examples; ```bash / ```text etc. are skipped.
				info := strings.TrimSpace(strings.TrimPrefix(trimmed, "```"))
				skipBlock = (info != "" && info != "aql") ||
					strings.Contains(prevNonBlank, skipMarker)
				inFence = true
				setup = setup[:0]
				setupOpen = 0
				prevCode, prevLine = "", 0
			} else {
				// Closing fence.
				inFence = false
				skipBlock = false
			}
			prevNonBlank = trimmed
			continue
		}

		if !inFence {
			if trimmed != "" {
				prevNonBlank = trimmed
			}
			continue
		}
		if skipBlock {
			continue
		}

		// Inside a runnable fence. Strip an `aql> ` / `aql>` prompt.
		line := stripPrompt(raw)
		code := strings.TrimSpace(line)
		if code == "" {
			continue
		}

		exprPart, rhs, isArrow := splitArrow(code)
		if !isArrow {
			// A non-arrow line is shared state for later `=>` lines ONLY
			// when it's a binding statement (`def`/`undef`/import) or a
			// continuation of one still open across physical lines
			// (multi-line `def … fn [ … ]`). Other bare expressions are
			// illustrative — their result isn't asserted and they must
			// NOT pile onto a following example's stack.
			if isSetupLine(code, setupOpen) {
				setup = append(setup, code)
				setupOpen += bracketDelta(code)
				prevCode, prevLine = "", 0
			} else {
				// Remember it: a following `=> result` line (result on its
				// own line) attaches to this expression.
				prevCode, prevLine = code, i+1
			}
			continue
		}

		expr := strings.TrimSpace(exprPart)
		exprLine := i + 1
		if expr == "" {
			// Result-on-own-line form: `=> result` whose expression was
			// the preceding code line in this block.
			if prevCode == "" {
				continue // stray `=>` with nothing to evaluate
			}
			expr, exprLine = prevCode, prevLine
		}
		prevCode, prevLine = "", 0

		expected, wantErr, errSub := classifyRHS(rhs)

		program := expr
		if len(setup) > 0 {
			program = strings.Join(setup, "\n") + "\n" + expr
		}

		out = append(out, Example{
			File:      file,
			Line:      exprLine,
			Program:   program,
			Expr:      expr,
			Expected:  expected,
			WantErr:   wantErr,
			ErrSubstr: errSub,
		})
	}

	return out
}

// isSetupLine reports whether a non-arrow code line is shared state for
// later examples in the same block. True for binding / side-effecting
// statements (`def`, `undef`, and the `… import end` / `use` module
// forms) and for continuation lines of a multi-line statement still open
// (setupOpen > 0). Other bare expressions are unasserted illustrations
// and must not pile onto a following example's stack.
func isSetupLine(code string, setupOpen int) bool {
	if setupOpen > 0 {
		return true
	}
	first := code
	if sp := strings.IndexAny(first, " \t"); sp >= 0 {
		first = first[:sp]
	}
	switch first {
	case "def", "undef", "use", "context", "set", "ctx-set":
		return true
	}
	// `"module-name" import end` — a module import statement whose
	// effect (bringing `pkg.word` into scope) later lines rely on.
	return strings.Contains(code, "import end")
}

// bracketDelta returns the net change in open `[`/`(` brackets on a line,
// ignoring brackets inside single-quoted strings. Used to keep capturing
// the continuation lines of a multi-line `def … fn [ … ]`.
func bracketDelta(code string) int {
	delta, inStr := 0, false
	for i := 0; i < len(code); i++ {
		switch code[i] {
		case '\'':
			inStr = !inStr
		case '[', '(':
			if !inStr {
				delta++
			}
		case ']', ')':
			if !inStr {
				delta--
			}
		}
	}
	return delta
}

// stripPrompt removes a leading `aql> ` (or bare `aql>`) REPL prompt,
// preserving the rest of the line verbatim.
func stripPrompt(line string) string {
	t := strings.TrimLeft(line, " \t")
	if rest, ok := strings.CutPrefix(t, "aql>"); ok {
		return strings.TrimPrefix(rest, " ")
	}
	return line
}

// splitArrow splits a code line on its first ` => ` / `=>` separator.
// Returns (lhs, rhs, true) when an arrow is present, else ("", "", false).
func splitArrow(code string) (string, string, bool) {
	idx := strings.Index(code, "=>")
	if idx < 0 {
		return "", "", false
	}
	return code[:idx], code[idx+len("=>"):], true
}

// classifyRHS interprets the documented right-hand side. It strips a
// trailing ` # comment` (quote-aware so a `#` inside a '…' string
// survives) and decides whether the example expects an error.
func classifyRHS(rhs string) (expected string, wantErr bool, errSubstr string) {
	rhs = stripTrailingComment(rhs)
	rhs = strings.TrimSpace(rhs)

	low := strings.ToLower(rhs)
	switch {
	case strings.HasPrefix(low, "error:"):
		return "", true, strings.TrimSpace(rhs[len("error:"):])
	case low == "error" || low == "build error":
		return "", true, ""
	case strings.HasPrefix(rhs, "[aql/"):
		// Documented error-code notation, e.g.
		// `[aql/type_error] return value 1: expected Integer got …`.
		// Match on the bracketed code (`aql/type_error`) — the prose
		// after it is illustrative and not the engine's exact wording.
		if end := strings.IndexByte(rhs, ']'); end > 0 {
			return "", true, rhs[1:end]
		}
		return "", true, ""
	default:
		return rhs, false, ""
	}
}

// stripTrailingComment removes a ` #…` trailing comment from s, ignoring
// any `#` that appears inside a single-quoted AQL string so result
// values like `'a # b'` are preserved.
func stripTrailingComment(s string) string {
	inStr := false
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '\'':
			inStr = !inStr
		case '#':
			if !inStr {
				return s[:i]
			}
		}
	}
	return s
}
