package engine_test
import (
	"github.com/metsitaba/voxgig-exp/lang/internal/engine"
	"github.com/metsitaba/voxgig-exp/lang/internal/native"
	"testing"
)
// --- make Path from list ---

func TestMakePathFromList(t *testing.T) {
	r, _ := engine.DefaultRegistry(native.Register)
	result := runAQL(t, r, []engine.Value{
		engine.NewWord("make"), engine.NewWord("Path"),
		engine.NewList([]engine.Value{engine.NewString("usr"), engine.NewString("local"), engine.NewString("bin")}),
	})
	if len(result) != 1 || !result[0].IsPath() {
		t.Fatalf("expected Path, got %v", result)
	}
	p, _ := result[0].AsPath()
	if p.Abs {
		t.Error("expected relative path")
	}
	if p.String() != "usr/local/bin" {
		t.Errorf("got %q, want %q", p.String(), "usr/local/bin")
	}
}

func TestMakePathFromListAtoms(t *testing.T) {
	r, _ := engine.DefaultRegistry(native.Register)
	result := runAQL(t, r, []engine.Value{
		engine.NewWord("make"), engine.NewWord("Path"),
		engine.NewList([]engine.Value{engine.NewAtom("a"), engine.NewAtom("b"), engine.NewAtom("c")}),
	})
	if len(result) != 1 || !result[0].IsPath() {
		t.Fatalf("expected Path, got %v", result)
	}
	_as0, _ := result[0].AsPath()
	if _as0.String() != "a/b/c" {
		_as1, _ := result[0].AsPath()
		t.Errorf("got %q, want %q", _as1.String(), "a/b/c")
	}
}

// --- make Path from string ---

func TestMakePathFromString(t *testing.T) {
	r, _ := engine.DefaultRegistry(native.Register)
	result := runAQL(t, r, []engine.Value{
		engine.NewWord("make"), engine.NewWord("Path"), engine.NewString("usr/local/bin"),
	})
	if len(result) != 1 || !result[0].IsPath() {
		t.Fatalf("expected Path, got %v", result)
	}
	p, _ := result[0].AsPath()
	if p.Abs {
		t.Error("expected relative path")
	}
	if p.String() != "usr/local/bin" {
		t.Errorf("got %q, want %q", p.String(), "usr/local/bin")
	}
}

func TestMakePathFromAbsString(t *testing.T) {
	r, _ := engine.DefaultRegistry(native.Register)
	result := runAQL(t, r, []engine.Value{
		engine.NewWord("make"), engine.NewWord("Path"), engine.NewString("/usr/local/bin"),
	})
	if len(result) != 1 || !result[0].IsPath() {
		t.Fatalf("expected Path, got %v", result)
	}
	p, _ := result[0].AsPath()
	if !p.Abs {
		t.Error("expected absolute path")
	}
	if p.String() != "/usr/local/bin" {
		t.Errorf("got %q, want %q", p.String(), "/usr/local/bin")
	}
}

// --- make Path with abs option ---

func TestMakePathAbsOption(t *testing.T) {
	r, _ := engine.DefaultRegistry(native.Register)
	opts := engine.NewOrderedMap()
	opts.Set("abs", engine.NewBoolean(true))
	result := runAQL(t, r, []engine.Value{
		engine.NewWord("make"), engine.NewWord("Path"), engine.NewMap(opts),
		engine.NewList([]engine.Value{engine.NewString("x"), engine.NewString("y")}),
	})
	if len(result) != 1 || !result[0].IsPath() {
		t.Fatalf("expected Path, got %v", result)
	}
	p, _ := result[0].AsPath()
	if !p.Abs {
		t.Error("expected absolute path from abs option")
	}
	if p.String() != "/x/y" {
		t.Errorf("got %q, want %q", p.String(), "/x/y")
	}
}

func TestMakePathAbsOptionString(t *testing.T) {
	r, _ := engine.DefaultRegistry(native.Register)
	opts := engine.NewOrderedMap()
	opts.Set("abs", engine.NewBoolean(true))
	result := runAQL(t, r, []engine.Value{
		engine.NewWord("make"), engine.NewWord("Path"), engine.NewMap(opts), engine.NewString("x/y"),
	})
	if len(result) != 1 || !result[0].IsPath() {
		t.Fatalf("expected Path, got %v", result)
	}
	_as2, _ := result[0].AsPath()
	if _as2.String() != "/x/y" {
		_as3, _ := result[0].AsPath()
		t.Errorf("got %q, want %q", _as3.String(), "/x/y")
	}
}

// --- Path string representation ---

func TestPathStringRelative(t *testing.T) {
	p := engine.NewPath([]string{"a", "b"}, false)
	if p.String() != "a/b" {
		t.Errorf("got %q, want %q", p.String(), "a/b")
	}
}

func TestPathStringAbsolute(t *testing.T) {
	p := engine.NewPath([]string{"a", "b"}, true)
	if p.String() != "/a/b" {
		t.Errorf("got %q, want %q", p.String(), "/a/b")
	}
}

func TestPathStringEmpty(t *testing.T) {
	p := engine.NewPath(nil, false)
	if p.String() != "" {
		t.Errorf("got %q, want %q", p.String(), "")
	}
}

func TestPathStringRoot(t *testing.T) {
	p := engine.NewPath(nil, true)
	if p.String() != "/" {
		t.Errorf("got %q, want %q", p.String(), "/")
	}
}

// --- Path type identity ---

func TestPathIsScalar(t *testing.T) {
	p := engine.NewPath([]string{"a"}, false)
	if !p.VType.Matches(engine.TScalar) {
		t.Error("Path should match Scalar")
	}
	if !p.VType.Matches(engine.TPath) {
		t.Error("Path should match Path")
	}
	if p.VType.Matches(engine.TString) {
		t.Error("Path should not match String")
	}
}
