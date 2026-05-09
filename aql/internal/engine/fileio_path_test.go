package engine_test
import (
	"github.com/metsitaba/voxgig-exp/aql/internal/engine"
	"github.com/metsitaba/voxgig-exp/aql/internal/native"
	"testing"

	"github.com/metsitaba/voxgig-exp/aql/internal/fileops"
)

func setupMemFSForIO(t *testing.T, r *engine.Registry) *fileops.MemFileOps {
	t.Helper()
	mem := fileops.NewMem()
	r.SetCapability(engine.CapMemFileOps, fileops.FileOps(mem))
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

// --- write with Path ---

func TestWriteWithPath(t *testing.T) {
	r, _ := engine.DefaultRegistry(native.Register)
	mem := setupMemFSForIO(t, r)

	path := engine.NewPath([]string{"data", "test.txt"}, false)
	result := runAQL(t, r, []engine.Value{
		engine.NewWord("write"), path, engine.NewString("hello world"),
	})
	if len(result) != 1 || !result[0].IsPath() {
		t.Fatalf("expected Path result, got %v", result)
	}
	_as0, _ := result[0].AsPath()
	if _as0.String() != "data/test.txt" {
		_as1, _ := result[0].AsPath()
		t.Errorf("got %q, want %q", _as1.String(), "data/test.txt")
	}
	// Verify content in mem FS
	resolved, _ := mem.ResolvePath("data/test.txt")
	if string(mem.Files[resolved]) != "hello world" {
		t.Errorf("file content = %q, want %q", string(mem.Files[resolved]), "hello world")
	}
}

func TestWriteWithAbsPath(t *testing.T) {
	r, _ := engine.DefaultRegistry(native.Register)
	mem := setupMemFSForIO(t, r)

	path := engine.NewPath([]string{"tmp", "out.txt"}, true)
	result := runAQL(t, r, []engine.Value{
		engine.NewWord("write"), path, engine.NewString("abs content"),
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
	r, _ := engine.DefaultRegistry(native.Register)
	mem := setupMemFSForIO(t, r)
	mem.Files["greeting.txt"] = []byte("hello")

	path := engine.NewPath([]string{"greeting.txt"}, false)
	result := runAQL(t, r, []engine.Value{
		engine.NewWord("read"), path,
	})
	_as2, _ := result[0].AsString()
	if len(result) != 1 || _as2 != "hello" {
		t.Fatalf("got %v, want 'hello'", result)
	}
}

func TestReadWithAbsPath(t *testing.T) {
	r, _ := engine.DefaultRegistry(native.Register)
	mem := setupMemFSForIO(t, r)
	mem.Files["/etc/config"] = []byte("key=val")

	path := engine.NewPath([]string{"etc", "config"}, true)
	result := runAQL(t, r, []engine.Value{
		engine.NewWord("read"), path,
	})
	_as3, _ := result[0].AsString()
	if len(result) != 1 || _as3 != "key=val" {
		t.Fatalf("got %v, want 'key=val'", result)
	}
}

// --- write then read roundtrip with Path ---

func TestWriteReadRoundtripPath(t *testing.T) {
	r, _ := engine.DefaultRegistry(native.Register)
	setupMemFSForIO(t, r)

	path := engine.NewPath([]string{"roundtrip.txt"}, false)
	runAQL(t, r, []engine.Value{
		engine.NewWord("write"), path, engine.NewString("round and round"),
	})
	result := runAQL(t, r, []engine.Value{
		engine.NewWord("read"), path,
	})
	_as4, _ := result[0].AsString()
	if len(result) != 1 || _as4 != "round and round" {
		t.Fatalf("got %v, want 'round and round'", result)
	}
}

// --- write with Path and options ---

func TestWriteWithPathAndOptions(t *testing.T) {
	r, _ := engine.DefaultRegistry(native.Register)
	mem := setupMemFSForIO(t, r)

	path := engine.NewPath([]string{"log.txt"}, false)
	opts := engine.NewOrderedMap()
	opts.Set("mode", engine.NewString("write"))
	result := runAQL(t, r, []engine.Value{
		engine.NewWord("write"), path, engine.NewString("line1"), engine.NewMap(opts),
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
	r, _ := engine.DefaultRegistry(native.Register)
	mem := setupMemFSForIO(t, r)
	mem.Files["data.txt"] = []byte("content here")

	path := engine.NewPath([]string{"data.txt"}, false)
	opts := engine.NewOrderedMap()
	opts.Set("fmt", engine.NewString("text"))
	result := runAQL(t, r, []engine.Value{
		engine.NewWord("read"), path, engine.NewMap(opts),
	})
	_as5, _ := result[0].AsString()
	if len(result) != 1 || _as5 != "content here" {
		t.Fatalf("got %v, want 'content here'", result)
	}
}

// --- String paths still work (backward compat) ---

func TestWriteStringPathStillWorks(t *testing.T) {
	r, _ := engine.DefaultRegistry(native.Register)
	mem := setupMemFSForIO(t, r)

	result := runAQL(t, r, []engine.Value{
		engine.NewWord("write"), engine.NewString("old.txt"), engine.NewString("old style"),
	})
	_as6, _ := result[0].AsString()
	if len(result) != 1 || _as6 != "old.txt" {
		t.Fatalf("got %v, want 'old.txt'", result)
	}
	resolved, _ := mem.ResolvePath("old.txt")
	if string(mem.Files[resolved]) != "old style" {
		t.Errorf("content = %q", string(mem.Files[resolved]))
	}
}

func TestReadStringPathStillWorks(t *testing.T) {
	r, _ := engine.DefaultRegistry(native.Register)
	mem := setupMemFSForIO(t, r)
	mem.Files["compat.txt"] = []byte("compat")

	result := runAQL(t, r, []engine.Value{
		engine.NewWord("read"), engine.NewString("compat.txt"),
	})
	_as7, _ := result[0].AsString()
	if len(result) != 1 || _as7 != "compat" {
		t.Fatalf("got %v, want 'compat'", result)
	}
}
