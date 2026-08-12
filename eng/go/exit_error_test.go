package eng

import (
	"errors"
	"fmt"
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// A well-formed exit error round-trips its code.
func TestExitCodeRoundTrip(t *testing.T) {
	for _, want := range []int64{0, 1, 42, 125} {
		e := core.NewExitError(want, "IO.exit", core.SrcPos{Row: 1, Col: 1})
		got, isExit := core.ExitCode(e)
		if !isExit || int64(got) != want {
			t.Errorf("ExitCode(NewExitError(%d)) = (%d, %v)", want, got, isExit)
		}
		if e.Code != core.ExitErrorCode {
			t.Errorf("code = %q, want %q", e.Code, core.ExitErrorCode)
		}
	}
}

// It unwraps, so a driver still recognises an exit a caller wrapped on the
// way up — the reason ExitCode uses errors.As rather than a type assertion.
func TestExitCodeUnwraps(t *testing.T) {
	wrapped := fmt.Errorf("running the program: %w", core.NewExitError(7, "IO.exit", core.SrcPos{}))
	if got, isExit := core.ExitCode(wrapped); !isExit || got != 7 {
		t.Errorf("ExitCode(wrapped) = (%d, %v), want (7, true)", got, isExit)
	}
	// But NOT through a %s flattening, which erases the type. This is why
	// buildrt's runAndPrint returns an exit unflattened.
	flattened := fmt.Errorf("running the program: %s", core.NewExitError(7, "IO.exit", core.SrcPos{}))
	if _, isExit := core.ExitCode(flattened); isExit {
		t.Error("a flattened error should not be recognisable as an exit")
	}
}

// Anything that is not an exit request reports false — an ordinary failure
// is not an exit, and neither is nil.
func TestExitCodeRejectsNonExits(t *testing.T) {
	for _, err := range []error{
		nil,
		errors.New("plain"),
		&core.BoruError{Code: "signature_error", Detail: "nope"},
	} {
		if _, isExit := core.ExitCode(err); isExit {
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
	noData := &core.BoruError{Code: core.ExitErrorCode, Detail: "exit"}

	noKey := &core.BoruError{Code: core.ExitErrorCode, Detail: "exit", Data: core.NewOrderedMap()}

	wrongType := &core.BoruError{Code: core.ExitErrorCode, Detail: "exit", Data: core.NewOrderedMap()}
	wrongType.Data.Set(core.ExitCodeKey, core.NewString("three"))

	for name, err := range map[string]*core.BoruError{
		"no Data":     noData,
		"no code key": noKey,
		"non-Integer": wrongType,
	} {
		got, isExit := core.ExitCode(err)
		if !isExit {
			t.Errorf("%s: not recognised as an exit at all", name)
		}
		if got != 0 {
			t.Errorf("%s: ExitCode = %d, want 0", name, got)
		}
	}
}
