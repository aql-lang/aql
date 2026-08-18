// Package flagutil holds the flag-parsing helper shared by the boru
// subcommands that take both flags and file operands.
package flagutil

import "flag"

// ParseInterleaved parses args with fs and returns the positional
// operands, allowing flags and operands to appear in any order.
//
// Go's flag package stops parsing at the FIRST non-flag token, so a
// single fs.Parse over `prog.boru --json` leaves `--json` sitting in
// fs.Args() as if it were a second operand — the command then treats a
// flag as a filename and reports a bogus read error. Re-parsing what is
// left after each operand is consumed hands the remaining flags back to
// the FlagSet, so `check a.boru b.boru --json` sees two files and one
// flag rather than three files.
//
// A `--` terminator ends flag parsing for the segment it appears in, so
// `check -- -weird.boru` names a file whose first character is a dash.
//
// Parse errors — an unknown flag, a missing flag value, or -h, which the
// flag package reports as flag.ErrHelp — are returned unchanged after fs
// has written its own message to fs.Output(); callers turn them into an
// exit code and print nothing further.
func ParseInterleaved(fs *flag.FlagSet, args []string) ([]string, error) {
	var positionals []string
	rest := args
	for len(rest) > 0 {
		if err := fs.Parse(rest); err != nil {
			return nil, err
		}
		if fs.NArg() == 0 {
			break
		}
		positionals = append(positionals, fs.Arg(0))
		rest = fs.Args()[1:]
	}
	return positionals, nil
}
