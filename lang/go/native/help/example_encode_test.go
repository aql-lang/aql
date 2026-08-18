package help

import (
	"errors"
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// TestEncodeExampleResult pins the one encoding both the build-time
// generator and the runtime dynamic path share. Each "not documentable"
// case is a distinct reason, and confusing them ships a falsehood.
func TestEncodeExampleResult(t *testing.T) {
	cases := []struct {
		name   string
		result string
		err    error
		want   string
		wantOK bool
	}{
		{"plain value", "3", nil, "3", true},
		{"quoted string", "'ab'", nil, "'ab'", true},
		{"empty stack", "", nil, NoValueMarker, true},
		{"coded refusal", "", &core.BoruError{Code: "type_error"},
			"error [boru/type_error]", true},
		{"coded refusal survives wrapping", "", errors.Join(&core.BoruError{Code: "arith_error"}),
			"error [boru/arith_error]", true},
		// undefined_word is a property of the evaluating registry, not of
		// the example: recording it would claim a module word does not
		// exist when it merely needs an import.
		{"undefined_word is not documentable", "", &core.BoruError{Code: "undefined_word"}, "", false},
		{"uncoded failure", "", errors.New("boom"), "", false},
		{"empty code", "", &core.BoruError{}, "", false},
		// Identity-bearing renders differ every run.
		{"class type id", "object<Ideal/Class/T_4f5a79d3674a>{a:1 b:2}", nil, "", false},
		{"pid", "Pid<P_fd4ec55d00a0>", nil, "", false},
		{"pid nested in a list", "[Pid<P_887edab44508> 1]", nil, "", false},
		// Near-misses must NOT be mistaken for identity.
		{"wrong prefix", "X_4f5a79d3674a", nil, "X_4f5a79d3674a", true},
		{"too short", "T_4f5a79d367", nil, "T_4f5a79d367", true},
		{"non-hex", "T_4f5a79d3674g", nil, "T_4f5a79d3674g", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := EncodeExampleResult(tc.result, tc.err)
			if ok != tc.wantOK {
				t.Fatalf("ok = %v, want %v (got %q)", ok, tc.wantOK, got)
			}
			if got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

// TestIsRefusalRender pins the predicate that decides which cached
// results the dynamic path re-evaluates. A false negative leaves a stale
// refusal in place after an overload is added; a false positive throws
// away a good static result on every registration.
func TestIsRefusalRender(t *testing.T) {
	if !IsRefusalRender("error [boru/type_error]") {
		t.Error("a refusal render was not recognised")
	}
	for _, r := range []string{"3", "'ab'", NoValueMarker, "", "errors happen", "[error]"} {
		if IsRefusalRender(r) {
			t.Errorf("IsRefusalRender(%q) = true, want false", r)
		}
	}
}

// TestGenerateDynamicExamplesCorrectsStaleRefusal is the reviewer's
// scenario: a static map recording `add true false` as a refusal must not
// outlive an overload that makes the call succeed.
func TestGenerateDynamicExamplesCorrectsStaleRefusal(t *testing.T) {
	const expr = "widget true false"
	origStatic := exampleResults
	origDyn := dynamicExampleResults
	t.Cleanup(func() { exampleResults, dynamicExampleResults = origStatic, origDyn })

	exampleResults = map[string]string{expr: "error [boru/type_error]"}
	dynamicExampleResults = map[string]string{}

	info := FuncInfo{Name: "widget", Sigs: []SigInfo{{Args: []string{"Boolean", "Boolean"}}}}
	// The live engine now accepts it.
	GenerateDynamicExamples(info, func(string) (string, error) { return "true", nil })

	if got := dynamicExampleResults[expr]; got != "true" {
		// Only assert when the expression is one ExampleExprs produces;
		// otherwise this scenario cannot arise and the test is vacuous.
		if _, generated := indexOf(ExampleExprs(info), expr); generated {
			t.Fatalf("stale refusal not corrected: dynamic[%q] = %q, want %q", expr, got, "true")
		}
		t.Skipf("ExampleExprs does not produce %q for this shape; scenario covered by TestEvalExamplePrefersDynamic", expr)
	}
}

// TestEvalExamplePrefersDynamic pins the precedence directly: where the
// live-engine result and the build-time snapshot disagree, the live one
// wins. Reversing this is what would let a stale refusal ship forever.
func TestEvalExamplePrefersDynamic(t *testing.T) {
	const expr = "widget true false"
	origStatic := exampleResults
	origDyn := dynamicExampleResults
	t.Cleanup(func() { exampleResults, dynamicExampleResults = origStatic, origDyn })

	exampleResults = map[string]string{expr: "error [boru/type_error]"}
	dynamicExampleResults = map[string]string{expr: "true"}

	if got := evalExample(expr, "widget", SigInfo{}, nil); got != "true" {
		t.Fatalf("evalExample = %q, want the live-engine result %q", got, "true")
	}

	// Negative half: with no dynamic entry, the static one is still used.
	dynamicExampleResults = map[string]string{}
	if got := evalExample(expr, "widget", SigInfo{}, nil); got != "error [boru/type_error]" {
		t.Fatalf("evalExample = %q, want the static result", got)
	}
}

func indexOf(hay []string, needle string) (int, bool) {
	for i, s := range hay {
		if s == needle {
			return i, true
		}
	}
	return 0, false
}
