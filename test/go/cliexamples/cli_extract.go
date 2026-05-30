// Package cliexamples extracts and runs the shell-style `aql …` examples
// embedded in the CLI docs (CLI.md, HOWTO.md) against the real built
// binary, so the documented command-line behavior can't drift. Like the
// docexamples package, the extractor here is pure text→[]CLIExample with
// no process/exec dependency, so it is unit-testable on its own; the
// runner (cliexamples_test.go) builds the `aql` binary once and executes
// each example inside a per-example temporary directory.
//
// Only invocations that carry an inline output assertion are checked:
//
//	aql do '1 add 2'            # prints 3
//	aql do '"hi" upper'         # => HI
//
// A `<!-- aql-test: skip -->` marker on the line above a fence opts the
// whole block out. Invocations whose subcommand needs network, external
// services, a missing file, or otherwise can't run deterministically in a
// sandbox are skipped by the runner (see needsSandboxSkip).
package cliexamples

import "strings"

// CLIExample is one extracted shell example with an output assertion.
type CLIExample struct {
	File string // source basename, e.g. "CLI.md"
	Line int    // 1-based line number

	// Args is the argv (excluding the leading "aql") to pass to the
	// built binary, already shell-split with quotes honored.
	Args []string

	// Raw is the original command text (for diagnostics / skip checks).
	Raw string

	// Expected is the asserted stdout (trimmed), with the doc's
	// surrounding quotes — `"3"` / `'HI'` — removed.
	Expected string
}

const skipMarker = "<!-- aql-test: skip -->"

// Extract pulls every `aql … # prints/=> output` example from a markdown
// file. file is the basename recorded on each example; src is the body.
func Extract(file, src string) []CLIExample {
	var out []CLIExample
	lines := strings.Split(src, "\n")

	inFence := false
	skipBlock := false
	prev := ""

	for i, raw := range lines {
		trimmed := strings.TrimSpace(raw)

		if strings.HasPrefix(trimmed, "```") {
			if !inFence {
				info := strings.TrimSpace(strings.TrimPrefix(trimmed, "```"))
				// CLI examples live in shell fences (```bash / ```sh /
				// ```console); a plain ``` is fine too. Anything else
				// (e.g. ```aql) is not a shell block.
				switch info {
				case "", "bash", "sh", "shell", "console":
					skipBlock = strings.Contains(prev, skipMarker)
				default:
					skipBlock = true
				}
				inFence = true
			} else {
				inFence = false
				skipBlock = false
			}
			prev = trimmed
			continue
		}

		if !inFence {
			if trimmed != "" {
				prev = trimmed
			}
			continue
		}
		if skipBlock {
			continue
		}

		cmd, expected, ok := splitOutputComment(raw)
		if !ok {
			continue
		}
		cmd = strings.TrimSpace(cmd)
		// Strip an optional leading shell prompt.
		cmd = strings.TrimPrefix(cmd, "$ ")
		args, ok := parseAQLInvocation(cmd)
		if !ok {
			continue
		}

		out = append(out, CLIExample{
			File:     file,
			Line:     i + 1,
			Args:     args,
			Raw:      cmd,
			Expected: unquote(strings.TrimSpace(expected)),
		})
	}

	return out
}

// splitOutputComment splits a shell line on its inline output-assertion
// comment (`# prints X`, `# => X`, `# → X`, `# outputs X`). Returns
// (command, expected, true) when one is present.
func splitOutputComment(line string) (string, string, bool) {
	hash := indexUnquotedHash(line)
	if hash < 0 {
		return "", "", false
	}
	cmd := line[:hash]
	comment := strings.TrimSpace(line[hash+1:]) // after '#'

	for _, kw := range []string{"prints", "outputs", "=>", "→"} {
		if rest, ok := strings.CutPrefix(comment, kw); ok {
			return cmd, strings.TrimSpace(rest), true
		}
	}
	return "", "", false
}

// indexUnquotedHash returns the index of the first `#` not inside a
// single- or double-quoted span, or -1.
func indexUnquotedHash(s string) int {
	var quote byte
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case quote != 0:
			if c == quote {
				quote = 0
			}
		case c == '\'' || c == '"':
			quote = c
		case c == '#':
			return i
		}
	}
	return -1
}

// parseAQLInvocation shell-splits a command line and, if it invokes
// `aql`, returns the argv after the program name. Returns ok=false for
// non-aql commands or ones that don't parse cleanly.
func parseAQLInvocation(cmd string) ([]string, bool) {
	toks, ok := shellSplit(cmd)
	if !ok || len(toks) == 0 || toks[0] != "aql" {
		return nil, false
	}
	return toks[1:], true
}

// shellSplit splits s into tokens, honoring single and double quotes
// (no escape or variable handling — adequate for the doc examples).
// Returns ok=false on an unterminated quote.
func shellSplit(s string) ([]string, bool) {
	var toks []string
	var cur strings.Builder
	var quote byte
	inTok := false

	flush := func() {
		if inTok {
			toks = append(toks, cur.String())
			cur.Reset()
			inTok = false
		}
	}

	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case quote != 0:
			if c == quote {
				quote = 0
			} else {
				cur.WriteByte(c)
			}
		case c == '\'' || c == '"':
			quote = c
			inTok = true
		case c == ' ' || c == '\t':
			flush()
		default:
			cur.WriteByte(c)
			inTok = true
		}
	}
	if quote != 0 {
		return nil, false
	}
	flush()
	return toks, true
}

// unquote removes a single pair of surrounding `"` or `'` quotes that the
// docs use to denote output (`# prints "3"`), leaving bare output as-is.
func unquote(s string) string {
	if len(s) >= 2 {
		if (s[0] == '"' && s[len(s)-1] == '"') || (s[0] == '\'' && s[len(s)-1] == '\'') {
			return s[1 : len(s)-1]
		}
	}
	return s
}
