package help

func init() {
	register(&Entry{
		Word:    "print",
		Summary: "Print a value to stdout followed by a newline.",
		Description: "Consumes the top value and writes its formatted representation to the output, " +
			"followed by a newline. Strings are printed as-is; maps and lists as JSON-like text; " +
			"tables are formatted with column headers. ORDERING PITFALL: print is " +
			"forward-precedent, so in a sequence like `\"a\" print \"b\" print` each print " +
			"grabs the NEXT statement's value and output comes out scrambled. For " +
			"sequential prints use the stack-only modifier print/s, or terminate each " +
			"statement with `end` / `;` — `\"a\" print; \"b\" print;`.",
		Examples: []string{
			`"a" print/s  "b" print/s   ;# a then b — stack-only, in order`,
			`"a" print; "b" print;      ;# same, via statement terminators`,
		},
	})

	register(&Entry{
		Word:        "printstr",
		Summary:     "Print a value to stdout without a trailing newline.",
		Description: "Same as print but does not append a newline at the end.",
	})

	register(&Entry{
		Word:    "read",
		Summary: "Read a file and return its contents.",
		Description: "Reads a file by path and returns its contents. The format is inferred from the " +
			"file extension (.csv, .tsv, .json, .jsonic, .txt) or can be set via {fmt: \"...\"}. " +
			"CSV/TSV files are loaded into SQLite-backed tables automatically.",
		Notes: []string{
			"Options: enc, fmt, nl.",
			"Pass the stdin stream handle to read from standard input.",
		},
	})

	register(&Entry{
		Word:    "write",
		Summary: "Write content to a file.",
		Description: "Writes content to the file at path. Returns the path. Non-string values are " +
			"serialized automatically (no options map required). " +
			"Use {mode: \"append\"} to append instead of overwriting.",
		Notes: []string{
			"Options: enc, fmt, mode (write/append), nl.",
			"Pass the stdout or stderr stream handle to write to a standard stream.",
		},
	})

	register(&Entry{
		Word:    "trace",
		Summary: "Evaluate a list with step-by-step tracing output.",
		Description: "Evaluates a list as code (like do) and prints a color-coded trace showing " +
			"the stack state at each step. Shows resolved vs pending values, pointer position, " +
			"and annotations for dispatch decisions (forward/prefix, precedence, collection).",
	})

	register(&Entry{
		Word:        "stdin",
		Summary:     "Push the stdin stream handle.",
		Description: "Pushes the `stdin` atom — a value of the StreamKind type (IO.StreamKind) — for use with read. Stack-only.",
	})

	register(&Entry{
		Word:        "stdout",
		Summary:     "Push the stdout stream handle.",
		Description: "Pushes the `stdout` atom — a value of the StreamKind type (IO.StreamKind) — for use with write. Stack-only.",
	})

	register(&Entry{
		Word:        "stderr",
		Summary:     "Push the stderr stream handle.",
		Description: "Pushes the `stderr` atom — a value of the StreamKind type (IO.StreamKind) — for use with write. Stack-only.",
	})
}
