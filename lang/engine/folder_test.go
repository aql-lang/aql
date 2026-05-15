package engine_test

import (
	"github.com/aql-lang/aql/lang/engine"
	"github.com/aql-lang/aql/lang/native"
	"testing"

	"github.com/aql-lang/aql/lang/internal/fileops"
)

// helper: enable in-memory FS and return the MemFileOps
func setupMemFS(t *testing.T, r *engine.Registry) *fileops.MemFileOps {
	t.Helper()
	mem := fileops.NewMem()
	r.Capabilities.Set(engine.CapMemFileOps, fileops.FileOps(mem))
	e := engine.New(r)
	_, err := e.Run([]engine.Value{
		engine.NewWord("context"), engine.NewWord("get"), engine.NewWord("__sys"),
		engine.NewWord("get"), engine.NewWord("fs"),
		engine.NewWord("set"), engine.NewWord("mem"), engine.NewBoolean(true),
	})
	if err != nil {
		t.Fatalf("enable mem fs: %v", err)
	}
	return mem
}

// --- folder with Path ---

func TestFolderCreatesDir(t *testing.T) {
	r, _ := engine.DefaultRegistry(native.Register)
	mem := setupMemFS(t, r)

	result := runAQL(t, r, []engine.Value{
		engine.NewWord("folder"), engine.NewPath([]string{"a", "b", "c"}, false),
	})
	if len(result) != 1 || !result[0].IsPath() {
		t.Fatalf("expected Path result, got %v", result)
	}
	_as0, _ := engine.AsPath(result[0])
	if _as0.String() != "a/b/c" {
		_as1, _ := engine.AsPath(result[0])
		t.Errorf("got %q, want %q", _as1.String(), "a/b/c")
	}
	// Check that the directory was created in mem FS
	resolved, _ := mem.ResolvePath("a/b/c")
	if !mem.Dirs[resolved] {
		t.Errorf("directory %q not created; dirs=%v", resolved, mem.Dirs)
	}
}

func TestFolderAbsolutePath(t *testing.T) {
	r, _ := engine.DefaultRegistry(native.Register)
	mem := setupMemFS(t, r)

	result := runAQL(t, r, []engine.Value{
		engine.NewWord("folder"), engine.NewPath([]string{"tmp", "data"}, true),
	})
	if len(result) != 1 || !result[0].IsPath() {
		t.Fatalf("expected Path, got %v", result)
	}
	_as2, _ := engine.AsPath(result[0])
	if _as2.String() != "/tmp/data" {
		_as3, _ := engine.AsPath(result[0])
		t.Errorf("got %q, want %q", _as3.String(), "/tmp/data")
	}
	if !mem.Dirs["/tmp/data"] {
		t.Errorf("absolute dir not created; dirs=%v", mem.Dirs)
	}
}

func TestFolderIdempotent(t *testing.T) {
	r, _ := engine.DefaultRegistry(native.Register)
	setupMemFS(t, r)

	path := engine.NewPath([]string{"x", "y"}, false)
	// Create twice — should not error
	runAQL(t, r, []engine.Value{engine.NewWord("folder"), path})
	result := runAQL(t, r, []engine.Value{engine.NewWord("folder"), path})
	if len(result) != 1 || !result[0].IsPath() {
		t.Fatalf("idempotent call failed: got %v", result)
	}
}

// --- folder with Options ---

func TestFolderWithParentsTrue(t *testing.T) {
	r, _ := engine.DefaultRegistry(native.Register)
	mem := setupMemFS(t, r)

	opts := engine.NewOrderedMap()
	opts.Set("parents", engine.NewBoolean(true))
	result := runAQL(t, r, []engine.Value{
		engine.NewWord("folder"), engine.NewOptionsType(opts), engine.NewPath([]string{"deep", "nested", "dir"}, false),
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
	r, _ := engine.DefaultRegistry(native.Register)
	setupMemFS(t, r)

	opts := engine.NewOrderedMap()
	opts.Set("parents", engine.NewBoolean(false))
	// Even with parents=false, MkdirAll is used (idempotent single dir)
	result := runAQL(t, r, []engine.Value{
		engine.NewWord("folder"), engine.NewOptionsType(opts), engine.NewPath([]string{"single"}, false),
	})
	if len(result) != 1 || !result[0].IsPath() {
		t.Fatalf("expected Path, got %v", result)
	}
}

// --- folder with make Path ---

func TestFolderWithMakePath(t *testing.T) {
	r, _ := engine.DefaultRegistry(native.Register)
	mem := setupMemFS(t, r)

	result := runAQL(t, r, []engine.Value{
		engine.NewWord("folder"),
		engine.NewOpenParen(),
		engine.NewWord("make"), engine.NewWord("Path"),
		engine.NewList([]engine.Value{engine.NewString("foo"), engine.NewString("bar")}),
		engine.NewCloseParen(),
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
	r, _ := engine.DefaultRegistry(native.Register)
	mem := setupMemFS(t, r)

	runAQL(t, r, []engine.Value{
		engine.NewWord("folder"), engine.NewPath([]string{"a", "b", "c"}, false),
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
