package eng

import (
	"errors"
	"strconv"
)

// exit_error.go — the reserved `aql/exit` control error.
//
// `IO.exit <code>` does not call os.Exit. It raises an ordinary AqlError
// carrying the reserved code below and `{code: N}` in Data, and the
// program's DRIVER decides what that means: the CLI and a built binary
// unwind, flush, and exit with the code printing nothing; an embedded host
// reads the code with ExitCode and decides for itself. Nothing inside the
// runtime terminates a process it does not own.
//
// It stays an ordinary error value on purpose. `do … error …` can observe
// it, and the documented contract — a handler that catches a foreign error
// it does not recognise must re-raise it — is the same rule every other
// error already relies on. Inventing an uncatchable error class would be a
// bigger mechanism than the problem deserves (design/CLI-PROGRAMS.0.md §4).

// ExitErrorCode is the reserved code an exit request carries. It is not a
// failure: a driver that recognises it exits with the requested status and
// prints nothing, including for a non-zero code.
const ExitErrorCode = "exit"

// ExitCodeKey is the Data key holding the requested status.
const ExitCodeKey = "code"

// NewExitError builds the reserved exit control error for the given status.
// src/pos blame the call site so an exit that escapes into a context which
// does NOT recognise it (a sub-engine boundary, an embedded host that just
// prints errors) still renders as a located diagnostic rather than a bare
// string.
func NewExitError(code int64, src string, pos SrcPos) *AqlError {
	e := makeAqlErrorAt(ExitErrorCode, "exit requested with code "+
		strconv.FormatInt(code, 10), "exit", src, "", pos)
	data := NewOrderedMap()
	data.Set(ExitCodeKey, NewInteger(code))
	e.Data = data
	return e
}

// ExitCode reports the status an exit request carries, and whether err is
// one at all. It unwraps, so a driver still recognises an exit that a
// caller wrapped with %w on the way up.
//
// An exit error whose Data lost its code key reports 0: the request to stop
// is what matters, and a missing status is not a reason to invent a failure.
func ExitCode(err error) (int, bool) {
	var ae *AqlError
	if !errors.As(err, &ae) || ae.Code != ExitErrorCode {
		return 0, false
	}
	if ae.Data != nil {
		if v, ok := ae.Data.Get(ExitCodeKey); ok {
			if n, err := AsInteger(v); err == nil {
				return int(n), true
			}
		}
	}
	return 0, true
}
