package lang

import (
	"testing"

	"github.com/aql-lang/aql/lang/go/native"
)

func TestRegisterFormat(t *testing.T) {
	a, err := New()
	if err != nil {
		t.Fatal(err)
	}

	// Create a simple format that wraps text in brackets.
	a.RegisterFormat("bracket", &bracketFormat{})

	// Verify the format was registered by checking the registry directly.
	if native.HostFormats(a.registry)["bracket"] == nil {
		t.Fatal("expected bracket format to be registered")
	}
}

// TestNewFormatParserFnLangSurface drives the lang-level NewFormatParserFn
// wrapper: the built value binds with DefineValue and parses import-free.
func TestNewFormatParserFnLangSurface(t *testing.T) {
	a, err := New()
	if err != nil {
		t.Fatal(err)
	}
	v, err := NewFormatParserFn("bracket", &bracketFormat{})
	if err != nil {
		t.Fatalf("NewFormatParserFn: %v", err)
	}
	if err := a.DefineValue("bracket", v); err != nil {
		t.Fatalf("DefineValue: %v", err)
	}
	got, err := a.Run(`parse bracket 'hi'`)
	if err != nil {
		t.Fatalf("parse bracket: %v", err)
	}
	if len(got) != 1 || got[0] != "hi" {
		t.Fatalf("parse bracket = %v, want [hi]", got)
	}
}

// bracketFormat is a test format.
type bracketFormat struct{}

func (f *bracketFormat) Decode(content string) ([]native.Value, error) {
	return []native.Value{native.NewString(content)}, nil
}

func (f *bracketFormat) Encode(v native.Value) (string, error) {
	return "[" + v.String() + "]", nil
}
