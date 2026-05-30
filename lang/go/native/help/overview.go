package help

import "strings"

// RepoURL is the canonical AQL source repository.
const RepoURL = "https://github.com/aql-lang/aql"

// ReferenceURL points at the word/syntax reference in the repo.
const ReferenceURL = RepoURL + "/blob/main/REFERENCE.md"

// TutorialURL points at the step-by-step tutorial in the repo.
const TutorialURL = RepoURL + "/blob/main/TUTORIAL.md"

// Overview returns a short orientation to the AQL language: the
// basics of how words and values compose, and how to reach the
// per-word/per-module documentation via `describe`. It is the single
// source of truth shared by the `help` word and the REPL `/help`
// meta-command.
func Overview() string {
	var b strings.Builder
	b.WriteString("AQL — a concatenative query language.\n")
	b.WriteString("\n")
	b.WriteString("Basics:\n")
	b.WriteString("  - A program is a sequence of words and values, read left to right:\n")
	b.WriteString("        add 2 3            ;# 5\n")
	b.WriteString("  - A word takes its arguments from the tokens that follow it, or\n")
	b.WriteString("    from the stack. These three forms are equivalent:\n")
	b.WriteString("        add 2 3   <=>   2 add 3   <=>   2 3 add\n")
	b.WriteString("  - Lists evaluate by default; quote to keep code as data:\n")
	b.WriteString("        [1 add 2]          ;# [3]\n")
	b.WriteString("        quote [1 add 2]    ;# [1 add 2]\n")
	b.WriteString("  - Define your own words with def and fn:\n")
	b.WriteString("        def double fn [[n:Integer] [Integer] [n mul 2]]\n")
	b.WriteString("        double 21          ;# 42\n")
	b.WriteString("  - Build maps and lists, and reach into them with dotted access:\n")
	b.WriteString("        def m {a: 1 b: 2}\n")
	b.WriteString("        m.a                ;# 1\n")
	b.WriteString("\n")
	b.WriteString("Discovering words — use describe:\n")
	b.WriteString("  describe add           Full docs for a word (signatures, examples).\n")
	b.WriteString("  \"concat\" describe      Same, by string name.\n")
	b.WriteString("  describe               How to use describe itself.\n")
	b.WriteString("\n")
	b.WriteString("Modules add more words; import one, then describe what it exports:\n")
	b.WriteString("  \"aql:math\" import\n")
	b.WriteString("  0.5 math.sin           ;# call an imported word\n")
	b.WriteString("\n")
	b.WriteString("Learn more:\n")
	b.WriteString("  Tutorial:  " + TutorialURL + "\n")
	b.WriteString("  Reference: " + ReferenceURL + "\n")
	return b.String()
}
