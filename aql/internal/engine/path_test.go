package engine

import "testing"

// --- make Path from list ---

func TestMakePathFromList(t *testing.T) {
	r, _ := DefaultRegistry()
	result := runAQL(t, r, []Value{
		NewWord("make"), NewWord("Path"),
		NewList([]Value{NewString("usr"), NewString("local"), NewString("bin")}),
	})
	if len(result) != 1 || !result[0].IsPath() {
		t.Fatalf("expected Path, got %v", result)
	}
	p := result[0].AsPath()
	if p.Abs {
		t.Error("expected relative path")
	}
	if p.String() != "usr/local/bin" {
		t.Errorf("got %q, want %q", p.String(), "usr/local/bin")
	}
}

func TestMakePathFromListAtoms(t *testing.T) {
	r, _ := DefaultRegistry()
	result := runAQL(t, r, []Value{
		NewWord("make"), NewWord("Path"),
		NewList([]Value{NewAtom("a"), NewAtom("b"), NewAtom("c")}),
	})
	if len(result) != 1 || !result[0].IsPath() {
		t.Fatalf("expected Path, got %v", result)
	}
	if result[0].AsPath().String() != "a/b/c" {
		t.Errorf("got %q, want %q", result[0].AsPath().String(), "a/b/c")
	}
}

// --- make Path from string ---

func TestMakePathFromString(t *testing.T) {
	r, _ := DefaultRegistry()
	result := runAQL(t, r, []Value{
		NewWord("make"), NewWord("Path"), NewString("usr/local/bin"),
	})
	if len(result) != 1 || !result[0].IsPath() {
		t.Fatalf("expected Path, got %v", result)
	}
	p := result[0].AsPath()
	if p.Abs {
		t.Error("expected relative path")
	}
	if p.String() != "usr/local/bin" {
		t.Errorf("got %q, want %q", p.String(), "usr/local/bin")
	}
}

func TestMakePathFromAbsString(t *testing.T) {
	r, _ := DefaultRegistry()
	result := runAQL(t, r, []Value{
		NewWord("make"), NewWord("Path"), NewString("/usr/local/bin"),
	})
	if len(result) != 1 || !result[0].IsPath() {
		t.Fatalf("expected Path, got %v", result)
	}
	p := result[0].AsPath()
	if !p.Abs {
		t.Error("expected absolute path")
	}
	if p.String() != "/usr/local/bin" {
		t.Errorf("got %q, want %q", p.String(), "/usr/local/bin")
	}
}

// --- make Path with abs option ---

func TestMakePathAbsOption(t *testing.T) {
	r, _ := DefaultRegistry()
	opts := NewOrderedMap()
	opts.Set("abs", NewBoolean(true))
	result := runAQL(t, r, []Value{
		NewWord("make"), NewWord("Path"), NewMap(opts),
		NewList([]Value{NewString("x"), NewString("y")}),
	})
	if len(result) != 1 || !result[0].IsPath() {
		t.Fatalf("expected Path, got %v", result)
	}
	p := result[0].AsPath()
	if !p.Abs {
		t.Error("expected absolute path from abs option")
	}
	if p.String() != "/x/y" {
		t.Errorf("got %q, want %q", p.String(), "/x/y")
	}
}

func TestMakePathAbsOptionString(t *testing.T) {
	r, _ := DefaultRegistry()
	opts := NewOrderedMap()
	opts.Set("abs", NewBoolean(true))
	result := runAQL(t, r, []Value{
		NewWord("make"), NewWord("Path"), NewMap(opts), NewString("x/y"),
	})
	if len(result) != 1 || !result[0].IsPath() {
		t.Fatalf("expected Path, got %v", result)
	}
	if result[0].AsPath().String() != "/x/y" {
		t.Errorf("got %q, want %q", result[0].AsPath().String(), "/x/y")
	}
}

// --- Path string representation ---

func TestPathStringRelative(t *testing.T) {
	p := NewPath([]string{"a", "b"}, false)
	if p.String() != "a/b" {
		t.Errorf("got %q, want %q", p.String(), "a/b")
	}
}

func TestPathStringAbsolute(t *testing.T) {
	p := NewPath([]string{"a", "b"}, true)
	if p.String() != "/a/b" {
		t.Errorf("got %q, want %q", p.String(), "/a/b")
	}
}

func TestPathStringEmpty(t *testing.T) {
	p := NewPath(nil, false)
	if p.String() != "" {
		t.Errorf("got %q, want %q", p.String(), "")
	}
}

func TestPathStringRoot(t *testing.T) {
	p := NewPath(nil, true)
	if p.String() != "/" {
		t.Errorf("got %q, want %q", p.String(), "/")
	}
}

// --- Path type identity ---

func TestPathIsScalar(t *testing.T) {
	p := NewPath([]string{"a"}, false)
	if !p.VType.Matches(TScalar) {
		t.Error("Path should match Scalar")
	}
	if !p.VType.Matches(TPath) {
		t.Error("Path should match Path")
	}
	if p.VType.Matches(TString) {
		t.Error("Path should not match String")
	}
}
