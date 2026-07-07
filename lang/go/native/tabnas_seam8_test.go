package native

import (
	"strings"
	"testing"
)

// --- tabnasAnyToValue scalar + fallback branches ---

func TestW8TabnasAnyToValueIntAndDefault(t *testing.T) {
	// `int` case -> Integer.
	v := tabnasAnyToValue(int(5))
	if n, _ := AsInteger(v); n != 5 {
		t.Errorf("int(5) -> %v, want Integer 5", v)
	}
	// unrecognised shape -> string form (a float32 is not one of the
	// recognised cases).
	v = tabnasAnyToValue(float32(1.5))
	if s, _ := AsString(v); !strings.Contains(s, "1.5") {
		t.Errorf("float32 fallback -> %q, want a string containing 1.5", s)
	}
}

// --- tabnasXmlToValue non-string/non-map child (default arm) ---

func TestW8TabnasXmlValueOtherChild(t *testing.T) {
	m := map[string]any{
		"name":     "tag",
		"children": []any{42}, // neither string nor map[string]any
	}
	v := tabnasXmlToValue(m)
	// A well-formed element value is produced without panicking; the
	// numeric child was stringified.
	if !v.Parent.ConformsTo(TXml) {
		t.Errorf("expected an Xml element value, got %s", v.Parent)
	}
}

// --- jsonRoundTrip marshal-error path ---

func TestW8JsonRoundTripMarshalError(t *testing.T) {
	// A channel cannot be marshalled by encoding/json.
	if _, err := jsonRoundTrip(make(chan int)); err == nil {
		t.Fatal("jsonRoundTrip of an unmarshalable value must error")
	}
}

// --- feed parse error (the j.Parse error arm inside the feed kind) ---

func TestW8TabnasFeedParseError(t *testing.T) {
	var feed TabnasKind
	for _, k := range TabnasKinds() {
		if k.Name == "feed" {
			feed = k
		}
	}
	if feed.Parse == nil {
		t.Fatal("feed kind not found")
	}
	if _, err := feed.Parse("<<<>>>", nil); err == nil {
		t.Fatal("feed.Parse of malformed input must error")
	}
}
