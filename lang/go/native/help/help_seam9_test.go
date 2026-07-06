package help

import (
	"testing"
)

// Seam-9 coverage (W9_nativeC) for help.go: examplePrefixes' empty-arg
// short-circuit and computeBinaryResult's min/max branches.

func TestW9ExamplePrefixesNoArgs(t *testing.T) {
	// A signature with no args yields no example prefixes (430.16,432.3).
	if got := examplePrefixes(FuncInfo{}, SigInfo{}); got != nil {
		t.Fatalf("examplePrefixes(no args) = %v, want nil", got)
	}
}

func TestW9ComputeBinaryResultMinMax(t *testing.T) {
	sig := SigInfo{Args: []string{"Integer", "Integer"}, Returns: []string{"Scalar/Number/Integer"}}

	// min: the else branch (aF >= bF) picks the further arg (648.10,650.5).
	if got := computeBinaryResult("min", sig, "5", "3"); got != "3" {
		t.Fatalf("computeBinaryResult(min, 5, 3) = %q, want 3", got)
	}
	// max: the if branch (aF > bF) picks the nearer arg (652.15,654.5).
	if got := computeBinaryResult("max", sig, "5", "3"); got != "5" {
		t.Fatalf("computeBinaryResult(max, 5, 3) = %q, want 5", got)
	}
}
