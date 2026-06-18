package eng

import "fmt"

// Shared return-check diagnostic text — the single source of truth for the
// strings both engines raise when a fn body violates its declared return
// contract. The interpreter (Engine.returnTypeError / returnCountError,
// engine.go) and the bytecode VM (vmReturnTypeErr / vmReturnCountErr, vm.go)
// MUST produce byte-identical detail/hint for the same mismatch, so
// error-scraping tooling can never tell which engine ran. They used to hold
// two hand-kept copies of the format strings; the copies are gone — both build
// the text here and wrap it in their own error-construction plumbing (the
// interpreter stamps a SrcPos + effectiveSource; the VM routes through
// r.AqlError* / stampAt).

// returnTypeErrorText is the detail + hint for a fn return-type mismatch:
// funcName's index-th (1-based) return value was expected to be `expected` but
// the body produced `got`.
func returnTypeErrorText(funcName string, index int, expected *Type, got Value) (detail, hint string) {
	detail = fmt.Sprintf("%s: return value %d: expected %s, got %s", funcName, index, expected, got.Parent)
	hint = "value: " + diagValue(got)
	return detail, hint
}

// returnCountErrorText is the detail for funcName leaving the wrong number of
// return values (expected vs got).
func returnCountErrorText(funcName string, expected, got int) string {
	return fmt.Sprintf("%s: expected %d return value(s), got %d", funcName, expected, got)
}
