package engine

import (
	"testing"

	"github.com/metsitaba/voxgig-exp/aql/internal/fileops"
)

func setupMemFSForIO(t *testing.T, r *Registry) *fileops.MemFileOps {
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

// --- write with Path ---

func TestWriteWithPath(t *testing.T) {
	r, _ := DefaultRegistry()
	mem := setupMemFSForIO(t, r)

	path := NewPath([]string{"data", "test.txt"}, false)
	result := runAQL(t, r, []Value{
		NewWord("write"), path, NewString("hello world"),
	})
	if len(result) != 1 || !result[0].IsPath() {
		t.Fatalf("expected Path result, got %v", result)
	}
	if result[0].AsPath().String() != "data/test.txt" {
		t.Errorf("got %q, want %q", result[0].AsPath().String(), "data/test.txt")
	}
	// Verify content in mem FS
	resolved, _ := mem.ResolvePath("data/test.txt")
	if string(mem.Files[resolved]) != "hello world" {
		t.Errorf("file content = %q, want %q", string(mem.Files[resolved]), "hello world")
	}
}

func TestWriteWithAbsPath(t *testing.T) {
	r, _ := DefaultRegistry()
	mem := setupMemFSForIO(t, r)

	path := NewPath([]string{"tmp", "out.txt"}, true)
	result := runAQL(t, r, []Value{
		NewWord("write"), path, NewString("abs content"),
	})
	if len(result) != 1 || !result[0].IsPath() {
		t.Fatalf("expected Path result, got %v", result)
	}
	if string(mem.Files["/tmp/out.txt"]) != "abs content" {
		t.Errorf("file content = %q, want %q", string(mem.Files["/tmp/out.txt"]), "abs content")
	}
}

// --- read with Path ---

func TestReadWithPath(t *testing.T) {
	r, _ := DefaultRegistry()
	mem := setupMemFSForIO(t, r)
	mem.Files["greeting.txt"] = []byte("hello")

	path := NewPath([]string{"greeting.txt"}, false)
	result := runAQL(t, r, []Value{
		NewWord("read"), path,
	})
	if len(result) != 1 || result[0].AsString() != "hello" {
		t.Fatalf("got %v, want 'hello'", result)
	}
}

func TestReadWithAbsPath(t *testing.T) {
	r, _ := DefaultRegistry()
	mem := setupMemFSForIO(t, r)
	mem.Files["/etc/config"] = []byte("key=val")

	path := NewPath([]string{"etc", "config"}, true)
	result := runAQL(t, r, []Value{
		NewWord("read"), path,
	})
	if len(result) != 1 || result[0].AsString() != "key=val" {
		t.Fatalf("got %v, want 'key=val'", result)
	}
}

// --- write then read roundtrip with Path ---

func TestWriteReadRoundtripPath(t *testing.T) {
	r, _ := DefaultRegistry()
	setupMemFSForIO(t, r)

	path := NewPath([]string{"roundtrip.txt"}, false)
	runAQL(t, r, []Value{
		NewWord("write"), path, NewString("round and round"),
	})
	result := runAQL(t, r, []Value{
		NewWord("read"), path,
	})
	if len(result) != 1 || result[0].AsString() != "round and round" {
		t.Fatalf("got %v, want 'round and round'", result)
	}
}

// --- write with Path and options ---

func TestWriteWithPathAndOptions(t *testing.T) {
	r, _ := DefaultRegistry()
	mem := setupMemFSForIO(t, r)

	path := NewPath([]string{"log.txt"}, false)
	opts := NewOrderedMap()
	opts.Set("mode", NewString("write"))
	result := runAQL(t, r, []Value{
		NewWord("write"), path, NewString("line1"), NewMap(opts),
	})
	if len(result) != 1 || !result[0].IsPath() {
		t.Fatalf("expected Path, got %v", result)
	}
	resolved, _ := mem.ResolvePath("log.txt")
	if string(mem.Files[resolved]) != "line1" {
		t.Errorf("content = %q, want %q", string(mem.Files[resolved]), "line1")
	}
}

// --- read with Path and options ---

func TestReadWithPathAndOptions(t *testing.T) {
	r, _ := DefaultRegistry()
	mem := setupMemFSForIO(t, r)
	mem.Files["data.txt"] = []byte("content here")

	path := NewPath([]string{"data.txt"}, false)
	opts := NewOrderedMap()
	opts.Set("fmt", NewString("text"))
	result := runAQL(t, r, []Value{
		NewWord("read"), path, NewMap(opts),
	})
	if len(result) != 1 || result[0].AsString() != "content here" {
		t.Fatalf("got %v, want 'content here'", result)
	}
}

// --- String paths still work (backward compat) ---

func TestWriteStringPathStillWorks(t *testing.T) {
	r, _ := DefaultRegistry()
	mem := setupMemFSForIO(t, r)

	result := runAQL(t, r, []Value{
		NewWord("write"), NewString("old.txt"), NewString("old style"),
	})
	if len(result) != 1 || result[0].AsString() != "old.txt" {
		t.Fatalf("got %v, want 'old.txt'", result)
	}
	resolved, _ := mem.ResolvePath("old.txt")
	if string(mem.Files[resolved]) != "old style" {
		t.Errorf("content = %q", string(mem.Files[resolved]))
	}
}

func TestReadStringPathStillWorks(t *testing.T) {
	r, _ := DefaultRegistry()
	mem := setupMemFSForIO(t, r)
	mem.Files["compat.txt"] = []byte("compat")

	result := runAQL(t, r, []Value{
		NewWord("read"), NewString("compat.txt"),
	})
	if len(result) != 1 || result[0].AsString() != "compat" {
		t.Fatalf("got %v, want 'compat'", result)
	}
}
