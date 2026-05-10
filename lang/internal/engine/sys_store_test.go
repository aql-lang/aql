package engine_test
import (
	"github.com/metsitaba/voxgig-exp/lang/internal/engine"
	"github.com/metsitaba/voxgig-exp/lang/internal/native"
	"testing"

	"github.com/metsitaba/voxgig-exp/lang/internal/fileops"
)

// --- __sys structure: all containers are Stores ---

func TestSysStoreStructure(t *testing.T) {
	r, _ := engine.DefaultRegistry(native.Register)
	store := r.ContextStore()
	if store == nil {
		t.Fatal("no context store")
	}

	// __sys exists and is a Store
	sysVal, ok := store.Get("__sys")
	if !ok {
		t.Fatal("__sys not found in root context")
	}
	sysStore, ok := sysVal.Data.(*engine.StoreInstanceInfo)
	if !ok {
		t.Fatalf("__sys is %T, want *StoreInstanceInfo", sysVal.Data)
	}

	// __sys.fs exists and is a Store
	fsVal, ok := sysStore.Get("fs")
	if !ok {
		t.Fatal("__sys.fs not found")
	}
	fsStore, ok := fsVal.Data.(*engine.StoreInstanceInfo)
	if !ok {
		t.Fatalf("__sys.fs is %T, want *StoreInstanceInfo", fsVal.Data)
	}

	// __sys.fs.mem = false
	memVal, ok := fsStore.Get("mem")
	if !ok {
		t.Fatal("__sys.fs.mem not found")
	}
	_as0, _ := memVal.AsBoolean()
	if _as0 != false {
		_as1, _ := memVal.AsBoolean()
		t.Errorf("__sys.fs.mem = %v, want false", _as1)
	}

	// __sys.fs.impl = None
	implVal, ok := fsStore.Get("impl")
	if !ok {
		t.Fatal("__sys.fs.impl not found")
	}
	if !implVal.VType.Equal(engine.TNone) {
		t.Errorf("__sys.fs.impl type = %s, want None", implVal.VType)
	}

	// __sys.__val exists and is a Store
	valVal, ok := sysStore.Get("__val")
	if !ok {
		t.Fatal("__sys.__val not found")
	}
	if _, ok := valVal.Data.(*engine.StoreInstanceInfo); !ok {
		t.Fatalf("__sys.__val is %T, want *StoreInstanceInfo", valVal.Data)
	}
}

// --- fs.mem flag switches between OS and in-memory file ops ---

func TestEffectiveFileOpsDefaultIsOS(t *testing.T) {
	r, _ := engine.DefaultRegistry(native.Register)
	ops := engine.EffectiveFileOps(r)
	if _, ok := ops.(*fileops.OSFileOps); !ok {
		t.Fatalf("default EffectiveFileOps is %T, want *OSFileOps", ops)
	}
}

func TestEffectiveFileOpsMemTrue(t *testing.T) {
	r, _ := engine.DefaultRegistry(native.Register)

	// Set __sys.fs.mem = true via AQL
	e := engine.New(r)
	_, err := e.Run([]engine.Value{
		// Get the fs Store from __sys
		engine.NewWord("context"), engine.NewWord("get"), engine.NewWord("__sys"),
		engine.NewWord("get"), engine.NewWord("fs"),
		// Set mem = true on it
		engine.NewWord("set"), engine.NewWord("mem"), engine.NewBoolean(true),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	ops := engine.EffectiveFileOps(r)
	if _, ok := ops.(*fileops.MemFileOps); !ok {
		t.Fatalf("EffectiveFileOps with mem=true is %T, want *MemFileOps", ops)
	}
}

func TestMemFileOpsReadWrite(t *testing.T) {
	r, _ := engine.DefaultRegistry(native.Register)

	// Enable in-memory fs
	e := engine.New(r)
	_, err := e.Run([]engine.Value{
		engine.NewWord("context"), engine.NewWord("get"), engine.NewWord("__sys"),
		engine.NewWord("get"), engine.NewWord("fs"),
		engine.NewWord("set"), engine.NewWord("mem"), engine.NewBoolean(true),
	})
	if err != nil {
		t.Fatalf("error enabling mem: %v", err)
	}

	// Write a file via the in-memory ops
	memOps := engine.EffectiveFileOps(r)
	mem, ok := memOps.(*fileops.MemFileOps)
	if !ok {
		t.Fatal("expected MemFileOps")
	}
	mem.Files["test.txt"] = []byte("hello")

	// Read it back through engine file ops
	data, err := engine.EffectiveFileOps(r).ReadFile("test.txt")
	if err != nil {
		t.Fatalf("ReadFile error: %v", err)
	}
	if string(data) != "hello" {
		t.Errorf("got %q, want %q", string(data), "hello")
	}
}

func TestMemFileOpsPersistsAcrossRuns(t *testing.T) {
	r, _ := engine.DefaultRegistry(native.Register)

	// Enable mem fs
	e := engine.New(r)
	_, err := e.Run([]engine.Value{
		engine.NewWord("context"), engine.NewWord("get"), engine.NewWord("__sys"),
		engine.NewWord("get"), engine.NewWord("fs"),
		engine.NewWord("set"), engine.NewWord("mem"), engine.NewBoolean(true),
	})
	if err != nil {
		t.Fatal(err)
	}

	// EffectiveFileOps should return the same MemFileOps instance
	ops1 := engine.EffectiveFileOps(r)
	ops2 := engine.EffectiveFileOps(r)
	if ops1 != ops2 {
		t.Fatal("MemFileOps instance should be reused across calls")
	}
}

// --- __val Store ---

func TestSysValStoreSetGet(t *testing.T) {
	r, _ := engine.DefaultRegistry(native.Register)
	e := engine.New(r)

	// Set a value in __sys.__val
	_, err := e.Run([]engine.Value{
		engine.NewWord("context"), engine.NewWord("get"), engine.NewWord("__sys"),
		engine.NewWord("get"), engine.NewWord("__val"),
		engine.NewWord("set"), engine.NewWord("x"), engine.NewInteger(42),
	})
	if err != nil {
		t.Fatalf("error setting __val.x: %v", err)
	}

	// Read it back
	result, err := e.Run([]engine.Value{
		engine.NewWord("context"), engine.NewWord("get"), engine.NewWord("__sys"),
		engine.NewWord("get"), engine.NewWord("__val"),
		engine.NewWord("get"), engine.NewWord("x"),
	})
	if err != nil {
		t.Fatalf("error getting __val.x: %v", err)
	}
	_as2, _ := result[0].AsInteger()
	if len(result) != 1 || _as2 != 42 {
		t.Fatalf("got %v, want [42]", result)
	}
}
