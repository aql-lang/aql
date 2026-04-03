package engine

import (
	"testing"

	"github.com/metsitaba/voxgig-exp/aql/internal/fileops"
)

// helper: enable in-memory FS and return the MemFileOps
func setupMemFS(t *testing.T, r *Registry) *fileops.MemFileOps {
	t.Helper()
	mem := fileops.NewMem()
	r.MemOps = mem
	e := New(r)
	_, err := e.Run([]Value{
		NewWord("context"), NewWord("get"), NewWord("__sys"),
		NewWord("get"), NewWord("fs"),
		NewWord("set"), NewWord("mem"), NewBoolean(true),
	})
	if err != nil {
		t.Fatalf("enable mem fs: %v", err)
	}
	return mem
}

// --- folder with Path ---

func TestFolderCreatesDir(t *testing.T) {
	r, _ := DefaultRegistry()
	mem := setupMemFS(t, r)

	result := runAQL(t, r, []Value{
		NewWord("folder"), NewPath([]string{"a", "b", "c"}, false),
	})
	if len(result) != 1 || !result[0].IsPath() {
		t.Fatalf("expected Path result, got %v", result)
	}
	_as0, _ := result[0].AsPath()
	if _as0.String() != "a/b/c" {
		_as1, _ := result[0].AsPath()
		t.Errorf("got %q, want %q", _as1.String(), "a/b/c")
	}
	// Check that the directory was created in mem FS
	resolved, _ := mem.ResolvePath("a/b/c")
	if !mem.Dirs[resolved] {
		t.Errorf("directory %q not created; dirs=%v", resolved, mem.Dirs)
	}
}

func TestFolderAbsolutePath(t *testing.T) {
	r, _ := DefaultRegistry()
	mem := setupMemFS(t, r)

	result := runAQL(t, r, []Value{
		NewWord("folder"), NewPath([]string{"tmp", "data"}, true),
	})
	if len(result) != 1 || !result[0].IsPath() {
		t.Fatalf("expected Path, got %v", result)
	}
	_as2, _ := result[0].AsPath()
	if _as2.String() != "/tmp/data" {
		_as3, _ := result[0].AsPath()
		t.Errorf("got %q, want %q", _as3.String(), "/tmp/data")
	}
	if !mem.Dirs["/tmp/data"] {
		t.Errorf("absolute dir not created; dirs=%v", mem.Dirs)
	}
}

func TestFolderIdempotent(t *testing.T) {
	r, _ := DefaultRegistry()
	setupMemFS(t, r)

	path := NewPath([]string{"x", "y"}, false)
	// Create twice — should not error
	runAQL(t, r, []Value{NewWord("folder"), path})
	result := runAQL(t, r, []Value{NewWord("folder"), path})
	if len(result) != 1 || !result[0].IsPath() {
		t.Fatalf("idempotent call failed: got %v", result)
	}
}

// --- folder with Options ---

func TestFolderWithParentsTrue(t *testing.T) {
	r, _ := DefaultRegistry()
	mem := setupMemFS(t, r)

	opts := NewOrderedMap()
	opts.Set("parents", NewBoolean(true))
	result := runAQL(t, r, []Value{
		NewWord("folder"), NewOptionsType(opts), NewPath([]string{"deep", "nested", "dir"}, false),
	})
	if len(result) != 1 || !result[0].IsPath() {
		t.Fatalf("expected Path, got %v", result)
	}
	resolved, _ := mem.ResolvePath("deep/nested/dir")
	if !mem.Dirs[resolved] {
		t.Errorf("parents dir not created; dirs=%v", mem.Dirs)
	}
}

func TestFolderWithParentsFalse(t *testing.T) {
	r, _ := DefaultRegistry()
	setupMemFS(t, r)

	opts := NewOrderedMap()
	opts.Set("parents", NewBoolean(false))
	// Even with parents=false, MkdirAll is used (idempotent single dir)
	result := runAQL(t, r, []Value{
		NewWord("folder"), NewOptionsType(opts), NewPath([]string{"single"}, false),
	})
	if len(result) != 1 || !result[0].IsPath() {
		t.Fatalf("expected Path, got %v", result)
	}
}

// --- folder with make Path ---

func TestFolderWithMakePath(t *testing.T) {
	r, _ := DefaultRegistry()
	mem := setupMemFS(t, r)

	result := runAQL(t, r, []Value{
		NewWord("folder"),
		NewWord("("),
		NewWord("make"), NewWord("Path"),
		NewList([]Value{NewString("foo"), NewString("bar")}),
		NewWord(")"),
	})
	if len(result) != 1 || !result[0].IsPath() {
		t.Fatalf("expected Path, got %v", result)
	}
	resolved, _ := mem.ResolvePath("foo/bar")
	if !mem.Dirs[resolved] {
		t.Errorf("dir not created via make Path; dirs=%v", mem.Dirs)
	}
}

// --- folder creates parent dirs ---

func TestFolderCreatesParentDirs(t *testing.T) {
	r, _ := DefaultRegistry()
	mem := setupMemFS(t, r)

	runAQL(t, r, []Value{
		NewWord("folder"), NewPath([]string{"a", "b", "c"}, false),
	})
	// Parent dirs should also be recorded
	resolvedA, _ := mem.ResolvePath("a")
	resolvedAB, _ := mem.ResolvePath("a/b")
	if !mem.Dirs[resolvedA] {
		t.Errorf("parent dir 'a' not created; dirs=%v", mem.Dirs)
	}
	if !mem.Dirs[resolvedAB] {
		t.Errorf("parent dir 'a/b' not created; dirs=%v", mem.Dirs)
	}
}
