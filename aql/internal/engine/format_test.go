package engine

import "testing"

func TestTextFormatDecode(t *testing.T) {
	f := &TextFormat{}
	result, err := f.Decode("hello world")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 || result[0].AsString() != "hello world" {
		t.Errorf("got %v, want ['hello world']", result)
	}
}

func TestTextFormatEncode(t *testing.T) {
	f := &TextFormat{}
	s, err := f.Encode(NewString("hello"))
	if err != nil {
		t.Fatal(err)
	}
	if s != "hello" {
		t.Errorf("got %q, want %q", s, "hello")
	}

	// Non-string uses String()
	s, err = f.Encode(NewInteger(42))
	if err != nil {
		t.Fatal(err)
	}
	if s != "42" {
		t.Errorf("got %q, want %q", s, "42")
	}
}

func TestJSONFormatDecode(t *testing.T) {
	f := &JSONFormat{}
	result, err := f.Decode(`{"x":1}`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 value, got %d", len(result))
	}
	if !result[0].VType.Equal(TMap) {
		t.Errorf("expected map, got %s", result[0].VType)
	}
}

func TestJSONFormatDecodeError(t *testing.T) {
	f := &JSONFormat{}
	_, err := f.Decode(`{invalid`)
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestJSONFormatEncode(t *testing.T) {
	f := &JSONFormat{}
	s, err := f.Encode(NewInteger(42))
	if err != nil {
		t.Fatal(err)
	}
	if s != "42" {
		t.Errorf("got %q, want %q", s, "42")
	}
}

func TestJsonicFormatDecode(t *testing.T) {
	f := &JsonicFormat{}
	result, err := f.Decode(`{x:1}`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 || !result[0].VType.Equal(TMap) {
		t.Errorf("expected map, got %v", result)
	}
}

func TestJsonicFormatDecodeNull(t *testing.T) {
	f := &JsonicFormat{}
	result, err := f.Decode(`null`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 || !result[0].VType.Equal(TNone) {
		t.Errorf("expected none, got %v", result)
	}
}

func TestJsonicFormatDecodeError(t *testing.T) {
	f := &JsonicFormat{}
	_, err := f.Decode(`{{{`)
	if err == nil {
		t.Error("expected error for invalid jsonic")
	}
}

func TestJsonicFormatEncode(t *testing.T) {
	f := &JsonicFormat{}
	s, err := f.Encode(NewString("hi"))
	if err != nil {
		t.Fatal(err)
	}
	if s != `"hi"` {
		t.Errorf("got %q, want %q", s, `"hi"`)
	}
}

func TestLinesFormatDecode(t *testing.T) {
	f := &LinesFormat{}
	result, err := f.Decode("a\nb\nc")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 value, got %d", len(result))
	}
	elems := result[0].AsList()
	if len(elems) != 3 {
		t.Fatalf("expected 3 lines, got %d", len(elems))
	}
	if elems[0].AsString() != "a" || elems[1].AsString() != "b" || elems[2].AsString() != "c" {
		t.Errorf("got %v", elems)
	}
}

func TestLinesFormatEncode(t *testing.T) {
	f := &LinesFormat{}
	list := NewList([]Value{NewString("x"), NewString("y")})
	s, err := f.Encode(list)
	if err != nil {
		t.Fatal(err)
	}
	if s != "x\ny" {
		t.Errorf("got %q, want %q", s, "x\ny")
	}
}

func TestLinesFormatEncodeNonString(t *testing.T) {
	f := &LinesFormat{}
	list := NewList([]Value{NewInteger(1), NewInteger(2)})
	s, err := f.Encode(list)
	if err != nil {
		t.Fatal(err)
	}
	if s != "1\n2" {
		t.Errorf("got %q, want %q", s, "1\n2")
	}
}

func TestLinesFormatEncodeNonList(t *testing.T) {
	f := &LinesFormat{}
	s, err := f.Encode(NewString("hello"))
	if err != nil {
		t.Fatal(err)
	}
	if s != "'hello'" {
		t.Errorf("got %q, want %q", s, "'hello'")
	}
}

func TestDefaultFormats(t *testing.T) {
	fmts := DefaultFormats()
	for _, name := range []string{"text", "json", "jsonic", "lines"} {
		if _, ok := fmts[name]; !ok {
			t.Errorf("missing format: %s", name)
		}
	}
}
