package help

func init() {
	register(&Entry{
		Word:    "print",
		Summary: "Print a value to stdout followed by a newline.",
		Signatures: []string{"[any] -> []"},
		Description: "Consumes the top value and writes its formatted representation to the output, " +
			"followed by a newline. Strings are printed as-is; maps and lists as JSON-like text; " +
			"tables are formatted with column headers.",
		Examples: []string{
			`"hello" print        => (prints: hello)`,
			`42 print             => (prints: 42)`,
		},
	})

	register(&Entry{
		Word:    "printstr",
		Summary: "Print a value to stdout without a trailing newline.",
		Signatures: []string{"[any] -> []"},
		Description: "Same as print but does not append a newline at the end.",
		Examples: []string{
			`"hello" printstr "world" print   => (prints: helloworld)`,
		},
	})

	register(&Entry{
		Word:    "read",
		Summary: "Read a file and return its contents.",
		Signatures: []string{
			"[string] -> [string|list|map]",
			"[string map] -> [string|list|map]",
		},
		Description: "Reads a file by path and returns its contents. The format is inferred from the " +
			"file extension (.csv, .tsv, .json, .jsonic, .txt) or can be set via {fmt: \"...\"}. " +
			"CSV/TSV files are loaded into SQLite-backed tables automatically.",
		Examples: []string{
			`"data.csv" read              => (table value)`,
			`"config.json" read           => (map value)`,
			`"notes.txt" read             => (string value)`,
		},
		Notes: []string{
			"Options: enc, fmt, nl.",
			"Use stdin to read from standard input.",
		},
	})

	register(&Entry{
		Word:    "write",
		Summary: "Write content to a file.",
		Signatures: []string{
			"[string string] -> [string]",
			"[string string map] -> [string]",
			"[string any map] -> [string]",
		},
		Description: "Writes content to the file at path. Returns the path. " +
			"Use {mode: \"append\"} to append instead of overwriting.",
		Examples: []string{
			`"output.txt" "hello" write`,
		},
		Notes: []string{
			"Options: enc, fmt, mode (write/append), nl.",
			"Use stdout or stderr for standard streams.",
		},
	})

	register(&Entry{
		Word:    "trace",
		Summary: "Print a debug trace of a value without consuming it.",
		Signatures: []string{"[any] -> [any]"},
		Description: "Prints a debug representation of the top value to stderr, " +
			"then leaves the value on the stack.",
		Examples: []string{`42 trace => 42 (and prints debug info)`},
	})

	register(&Entry{
		Word:    "stdin",
		Summary: "Push the stdin path string.",
		Signatures: []string{"[] -> [string]"},
		Description: "Pushes the special path \"<stdin>\" for use with read.",
		Examples: []string{`stdin read`},
	})

	register(&Entry{
		Word:    "stdout",
		Summary: "Push the stdout path string.",
		Signatures: []string{"[] -> [string]"},
		Description: "Pushes the special path \"<stdout>\" for use with write.",
		Examples: []string{`stdout "hello" write`},
	})

	register(&Entry{
		Word:    "stderr",
		Summary: "Push the stderr path string.",
		Signatures: []string{"[] -> [string]"},
		Description: "Pushes the special path \"<stderr>\" for use with write.",
		Examples: []string{`stderr "error!" write`},
	})
}
