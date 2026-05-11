package lang

import (
	"testing"

	"github.com/aql-lang/aql/lang/engine"
)

func TestRegisterFormat(t *testing.T) {
	a, err := New()
	if err != nil {
		t.Fatal(err)
	}

	// Create a simple format that wraps text in brackets.
	a.RegisterFormat("bracket", &bracketFormat{})

	// Verify the format was registered by checking the registry directly.
	if engine.HostFormats(a.registry)["bracket"] == nil {
		t.Fatal("expected bracket format to be registered")
	}
}

// bracketFormat is a test format.
type bracketFormat struct{}

func (f *bracketFormat) Decode(content string) ([]engine.Value, error) {
	return []engine.Value{engine.NewString(content)}, nil
}

func (f *bracketFormat) Encode(v engine.Value) (string, error) {
	return "[" + v.String() + "]", nil
}
