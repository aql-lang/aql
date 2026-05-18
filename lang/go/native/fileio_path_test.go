package native

import (
	"testing"

	"github.com/aql-lang/aql/lang/go/internal/fileops"
)

func setupMemFSForIO(t *testing.T, r *Registry) *fileops.MemFileOps {
	t.Helper()
	mem := fileops.NewMem()
	if err := r.Capabilities.Set(CapMemFileOps, fileops.FileOps(mem)); err != nil {
		t.Fatalf("set capability: %v", err)
	}
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
	if len(result) != 1 || !IsPath(result[0]) {
		t.Fatalf("expected Path result, got %v", result)
	}
	_as0, _ := AsPath(result[0])
	if _as0.String() != "data/test.txt" {
		_as1, _ := AsPath(result[0])
		t.Errorf("got %q, want %q", _as1.String(), "data/test.txt")
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
	if len(result) != 1 || !IsPath(result[0]) {
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
	_as2, _ := AsString(result[0])
	if len(result) != 1 || _as2 != "hello" {
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
	_as3, _ := AsString(result[0])
	if len(result) != 1 || _as3 != "key=val" {
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
	_as4, _ := AsString(result[0])
	if len(result) != 1 || _as4 != "round and round" {
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
	if len(result) != 1 || !IsPath(result[0]) {
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
	_as5, _ := AsString(result[0])
	if len(result) != 1 || _as5 != "content here" {
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
	_as6, _ := AsString(result[0])
	if len(result) != 1 || _as6 != "old.txt" {
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
	_as7, _ := AsString(result[0])
	if len(result) != 1 || _as7 != "compat" {
		t.Fatalf("got %v, want 'compat'", result)
	}
}
