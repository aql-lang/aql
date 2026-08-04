package eng

import (
	"errors"
	"strings"
	"testing"
)

// TestInstallTypeNamePartConflictTaxonomy pins the taxonomy wrap on the
// name-part conflict refusal: `def Integer 42` (a builtin type name as a
// def target) must surface as a *BoruError with code `type_error`, not
// the raw fmt.Errorf ValidateTypeNameParts returns — the raw error
// leaked to hosts as a non-taxonomy failure (the TS crossdiff reported
// it as UNEXPECTED:… instead of a code).
func TestInstallTypeNamePartConflictTaxonomy(t *testing.T) {
	r, err := NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	instErr := InstallType(r, "Integer", NewInteger(42))
	if instErr == nil {
		t.Fatalf("rebinding a builtin type name must refuse")
	}
	var be *BoruError
	if !errors.As(instErr, &be) {
		t.Fatalf("refusal must be a BoruError, got %T: %v", instErr, instErr)
	}
	if be.Code != "type_error" {
		t.Errorf("code = %q, want type_error", be.Code)
	}
	if !strings.Contains(be.Detail, "conflicts with an existing type name") {
		t.Errorf("detail %q lacks the conflict message", be.Detail)
	}

	// Negative pairing: a fresh non-conflicting name installs cleanly.
	if err := InstallType(r, "Zebra", NewTypeLiteral(TInteger)); err != nil {
		t.Errorf("fresh type name must install, got %v", err)
	}
}
