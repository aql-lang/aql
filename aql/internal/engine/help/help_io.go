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
			`"hello" print                  (prints: hello)`,
			`42 print                       (prints: 42)`,
			`[1 2 3] print                  (prints: [1, 2, 3])`,
			`{name: "Alice"} print          (prints: {"name": "Alice"})`,
		},
	})

	register(&Entry{
		Word:    "printstr",
		Summary: "Print a value to stdout without a trailing newline.",
		Signatures: []string{"[any] -> []"},
		Description: "Same as print but does not append a newline at the end.",
		Examples: []string{
			`"hello" printstr               (prints: hello, no newline)`,
			`42 printstr                    (prints: 42, no newline)`,
			`"a" printstr "b" print         (prints: ab)`,
			`1 printstr 2 print             (prints: 12)`,
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
			`"data.csv" read                => (table value)`,
			`"config.json" read             => (map value)`,
			`"notes.txt" read               => (string value)`,
			`stdin read                     => (reads from standard input)`,
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
			`"out.txt" "hello" write        => 'out.txt'`,
			`stdout "hello" write           (prints: hello)`,
			`stderr "error!" write          (prints to stderr)`,
			`"log.txt" "line" {mode: "append"} write => 'log.txt'`,
		},
		Notes: []string{
			"Options: enc, fmt, mode (write/append), nl.",
			"Use stdout or stderr for standard streams.",
		},
	})

	register(&Entry{
		Word:    "trace",
		Summary: "Evaluate a list with step-by-step tracing output.",
		Signatures: []string{"[list] -> [any...]"},
		Description: "Evaluates a list as code (like do) and prints a color-coded trace showing " +
			"the stack state at each step. Shows resolved vs pending values, pointer position, " +
			"and annotations for dispatch decisions (forward/prefix, precedence, collection).",
		Examples: []string{
			`trace [1 add 2]                => 3 (prints step-by-step stack trace)`,
			`trace [3 4 mul]                => 12 (traces multiplication)`,
			`trace ["hello" upper]          => 'HELLO' (traces string op)`,
			`trace [1 2 3 rot add mul]      => 8 (traces stack operations)`,
		},
	})

	register(&Entry{
		Word:    "stdin",
		Summary: "Push the stdin path string.",
		Signatures: []string{"[] -> [string]"},
		Description: "Pushes the special path \"<stdin>\" for use with read. Stack-only.",
		Examples: []string{
			`stdin read                     => (reads all of standard input)`,
			`stdin                          => '<stdin>'`,
			`stdin read trim                => (reads stdin and trims whitespace)`,
			`stdin read "," split           => (reads stdin and splits by comma)`,
		},
	})

	register(&Entry{
		Word:    "stdout",
		Summary: "Push the stdout path string.",
		Signatures: []string{"[] -> [string]"},
		Description: "Pushes the special path \"<stdout>\" for use with write. Stack-only.",
		Examples: []string{
			`stdout "hello" write           (writes hello to stdout)`,
			`stdout                         => '<stdout>'`,
			`stdout "line1\nline2" write     (writes two lines to stdout)`,
			`stdout 42 String convert write (writes 42 to stdout)`,
		},
	})

	register(&Entry{
		Word:    "stderr",
		Summary: "Push the stderr path string.",
		Signatures: []string{"[] -> [string]"},
		Description: "Pushes the special path \"<stderr>\" for use with write. Stack-only.",
		Examples: []string{
			`stderr "error!" write          (writes to stderr)`,
			`stderr                         => '<stderr>'`,
			`stderr "warning: x" write      (writes warning to stderr)`,
			`stderr "debug\n" write         (writes debug line to stderr)`,
		},
	})
}
