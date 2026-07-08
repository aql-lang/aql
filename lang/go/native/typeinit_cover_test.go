package native

import (
	"errors"
	"testing"
)

// RecordTypeInitError is the exported funnel sibling packages (aql:net's
// Socket/Listener registration) use to surface init-time type failures at
// registry construction. Pin both directions: a real error is recorded
// (and restored so later constructions stay clean), a nil is ignored.
func TestRecordTypeInitError(t *testing.T) {
	saved := typeInitErrs
	defer func() { typeInitErrs = saved }()
	typeInitErrs = nil

	RecordTypeInitError(nil)
	if TypeInitError() != nil {
		t.Error("nil must not be recorded")
	}
	injected := errors.New("injected init failure")
	RecordTypeInitError(injected)
	if got := TypeInitError(); !errors.Is(got, injected) {
		t.Errorf("TypeInitError = %v, want the injected error", got)
	}
}
