package eng

import (
	"errors"
	"fmt"
	"testing"
)

// A well-formed exit error round-trips its code.
func TestExitCodeRoundTrip(t *testing.T) {
	for _, want := range []int64{0, 1, 42, 125} {
		e := NewExitError(want, "IO.exit", SrcPos{Row: 1, Col: 1})
		got, isExit := ExitCode(e)
		if !isExit || int64(got) != want {
			t.Errorf("ExitCode(NewExitError(%d)) = (%d, %v)", want, got, isExit)
		}
		if e.Code != ExitErrorCode {
			t.Errorf("code = %q, want %q", e.Code, ExitErrorCode)
		}
	}
}

// It unwraps, so a driver still recognises an exit a caller wrapped on the
// way up — the reason ExitCode uses errors.As rather than a type assertion.
func TestExitCodeUnwraps(t *testing.T) {
	wrapped := fmt.Errorf("running the program: %w", NewExitError(7, "IO.exit", SrcPos{}))
	if got, isExit := ExitCode(wrapped); !isExit || got != 7 {
		t.Errorf("ExitCode(wrapped) = (%d, %v), want (7, true)", got, isExit)
	}
	// But NOT through a %s flattening, which erases the type. This is why
	// buildrt's runAndPrint returns an exit unflattened.
	flattened := fmt.Errorf("running the program: %s", NewExitError(7, "IO.exit", SrcPos{}))
	if _, isExit := ExitCode(flattened); isExit {
		t.Error("a flattened error should not be recognisable as an exit")
	}
}

// Anything that is not an exit request reports false — an ordinary failure
// is not an exit, and neither is nil.
func TestExitCodeRejectsNonExits(t *testing.T) {
	for _, err := range []error{
		nil,
		errors.New("plain"),
		&BoruError{Code: "signature_error", Detail: "nope"},
	} {
		if _, isExit := ExitCode(err); isExit {
			t.Errorf("ExitCode(%v) reported an exit", err)
		}
	}
}

// A malformed exit error still means "stop", and reports 0. The request to
// stop is what matters; a missing or unreadable status is not a reason to
// invent a failure the program never asked for. These shapes cannot come
// from NewExitError — they are what a HOST could hand back after building
// or mutating the error itself.
func TestExitCodeMalformedReportsZero(t *testing.T) {
	noData := &BoruError{Code: ExitErrorCode, Detail: "exit"}

	noKey := &BoruError{Code: ExitErrorCode, Detail: "exit", Data: NewOrderedMap()}

	wrongType := &BoruError{Code: ExitErrorCode, Detail: "exit", Data: NewOrderedMap()}
	wrongType.Data.Set(ExitCodeKey, NewString("three"))

	for name, err := range map[string]*BoruError{
		"no Data":     noData,
		"no code key": noKey,
		"non-Integer": wrongType,
	} {
		got, isExit := ExitCode(err)
		if !isExit {
			t.Errorf("%s: not recognised as an exit at all", name)
		}
		if got != 0 {
			t.Errorf("%s: ExitCode = %d, want 0", name, got)
		}
	}
}
