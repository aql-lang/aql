package eng

import (
	"strings"
	"testing"
)

// TestValidateWordNameAccepts pins the names the language-fundamental
// rule accepts.
func TestValidateWordNameAccepts(t *testing.T) {
	good := []string{
		// Plain lowercase.
		"add", "sub", "mul", "dup", "swap", "drop",
		"over", "rot", "nip", "tuck",
		// With trailing digits.
		"dup2", "swap2", "drop2", "over2",
		"add2", "sub2", "x99",
		// Hyphenated kebab-case.
		"anti-rot", "add-two", "sub-two", "double-then-inc",
		"dup2-alt", "incr-twice", "incr-four-times",
		// Snake-case mid-name.
		"fact_acc", "fact_acc_loop", "args_frame",
		// Mixed.
		"dup2-alt_inner",
		// Predicate names (is- prefix replaces the dropped ?-suffix).
		"is-leap-year", "is-before", "is-equal", "is-empty",
		"is-odd", "is-prime-factor",
		// Single letter.
		"a", "x", "n",
		// Underscore-prefix (discard placeholder, engine internals).
		"_", "__pa", "__mark", "_internal", "_-leading",
		"_unused-arg",
		// CLI-style flag names (leading hyphen).
		"-h", "-v", "-x5", "--help", "--limit", "--no-push",
		// Dollar-sign anywhere (shell-style names).
		"$", "$path", "$home", "foo$", "foo$bar", "$$",
		"$1", "$a-b", "f$o", "_$inner",
	}
	for _, name := range good {
		if err := ValidateWordName(name); err != nil {
			t.Errorf("ValidateWordName(%q) should accept, got %v", name, err)
		}
	}
}

// TestValidateWordNameRejects pins the names the rule rejects.
func TestValidateWordNameRejects(t *testing.T) {
	cases := []struct {
		name      string
		wantInMsg string // substring expected in the error detail
	}{
		{"", "empty"},
		{"Integer", "[a-z_-$]"},   // uppercase first
		{"String", "[a-z_-$]"},    // uppercase first
		{"X", "[a-z_-$]"},         // uppercase first
		{"123", "[a-z_-$]"},       // digit first
		{"2dup", "[a-z_-$]"},      // digit first
		{"?question", "[a-z_-$]"}, // ? first
		{"!bang", "[a-z_-$]"},     // ! first
		// All-hyphen names rejected (carry no identifier).
		{"-", "only hyphens"},
		{"--", "only hyphens"},
		{"---", "only hyphens"},
		{"foo bar", "illegal"}, // space mid-name
		{"foo!bar", "illegal"}, // ! mid-name
		{"foo*bar", "illegal"}, // * mid-name
		{"foo+bar", "illegal"}, // + mid-name
		{"foo.bar", "illegal"}, // . mid-name
		{"fooBar", "illegal"},  // uppercase mid-name
		{"foo/", "illegal"},    // / mid-name
		{"foo?", "illegal"},    // ? suffix (predicate convention dropped)
		{"leap-year?", "illegal"},
		{"is-empty?", "illegal"},
	}
	for _, c := range cases {
		err := ValidateWordName(c.name)
		if err == nil {
			t.Errorf("ValidateWordName(%q) should reject, got nil error", c.name)
			continue
		}
		aql, ok := err.(*AqlError)
		if !ok {
			t.Errorf("ValidateWordName(%q): expected *AqlError, got %T", c.name, err)
			continue
		}
		if aql.Code != "invalid_word_name" {
			t.Errorf("ValidateWordName(%q): wrong code %q", c.name, aql.Code)
		}
		if !strings.Contains(aql.Detail, c.wantInMsg) {
			t.Errorf("ValidateWordName(%q): detail %q should contain %q",
				c.name, aql.Detail, c.wantInMsg)
		}
	}
}

// TestRegisterNativeFuncRejectsBadName — the validation chains all
// the way out to RegisterNativeFunc; a bad name lands in r.errs.
func TestRegisterNativeFuncRejectsBadName(t *testing.T) {
	r, err := NewRegistry()
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	r.RegisterNativeFunc(NativeFunc{
		Name: "Integer", // uppercase — invalid
		Signatures: []NativeSig{{
			Args: []*Type{TInteger},
			Handler: func(_ []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
				return nil, nil
			}, BarrierPos: 0,
		}},
	})
	if r.Err() == nil {
		t.Fatal("expected r.Err() to surface the invalid_word_name, got nil")
	}
	if !strings.Contains(r.Err().Error(), "invalid_word_name") {
		t.Errorf("unexpected error: %v", r.Err())
	}
}

// TestDefRejectsBadName — the engine's word-name validation rule
// (ValidateWordName) rejects uppercase names; here we exercise the
// rule through a minimal in-test `def` fixture so the assertion
// reaches the same code path users hit with the production lang word.
func TestDefRejectsBadName(t *testing.T) {
	r, _ := NewRegistry()
	r.RegisterNativeFunc(NativeFunc{
		Name: "def",

		Signatures: []NativeSig{{
			Args:       []*Type{TAtom, TAny},
			QuoteArgs:  map[int]bool{0: true},
			NoEvalArgs: map[int]bool{1: true},
			Handler: func(args []Value, _ map[string]Value, _ []Value, reg *Registry) ([]Value, error) {
				name, _ := args[0].AsConcreteAtom()
				if err := ValidateWordName(name); err != nil {
					return nil, err
				}
				reg.Defs.Push(name, args[1])
				return nil, nil
			},
			Returns: []*Type{}, BarrierPos: -1,
		}},
	})
	r.InitRootContext()

	_, err := NewTop(r).Run([]Value{
		NewWord("def"), NewWord("Integer"), NewInteger(42),
	})
	if err == nil {
		t.Fatal("expected def to reject uppercase name, got nil")
	}
	if !strings.Contains(err.Error(), "invalid_word_name") {
		t.Errorf("unexpected error: %v", err)
	}
}

// TestUnderscoreLeading covers the underscore-as-first-char rule:
// `_` (single discard placeholder) and `__pa` (engine-internal
// marker) are both valid under the unified [a-z_] first-char rule.
func TestUnderscoreLeading(t *testing.T) {
	for _, name := range []string{
		"_",                      // discard placeholder
		"__pa", "__mark", "__fw", // engine-internal markers
		"_unused", "_tmp-result", // user-facing leading underscore
	} {
		if err := ValidateWordName(name); err != nil {
			t.Errorf("underscore-leading %q should be valid, got %v", name, err)
		}
	}
}
