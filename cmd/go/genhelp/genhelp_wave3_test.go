package main

import (
	"testing"

	"github.com/boru-lang/boru/lang/go/capabilities"
	"github.com/boru-lang/boru/lang/go/native"
)

// TestSeedMemFS verifies seedMemFS installs the in-memory filesystem
// capability with the five seed files and flips __sys.fs.mem in the
// root context. (main() itself is not run here: it writes the generated
// examples file into the repo tree and exits the process on failure.)
func TestSeedMemFS(t *testing.T) {
	reg := newRegistry(t)
	if err := seedMemFS(reg); err != nil {
		t.Fatalf("seedMemFS: %v", err)
	}

	capVal, ok, err := reg.Capabilities.Get(native.CapMemFileOps)
	if err != nil || !ok {
		t.Fatalf("mem fileops capability missing (ok=%v err=%v)", ok, err)
	}
	ops, ok := capVal.(capabilities.FileOps)
	if !ok {
		t.Fatalf("capability is %T, want capabilities.FileOps", capVal)
	}
	for _, name := range []string{"a", "b", "c", "d", "e"} {
		data, err := ops.ReadFile(name)
		if err != nil {
			t.Fatalf("ReadFile(%s): %v", name, err)
		}
		if got, want := string(data), "file-"+name+"-content"; got != want {
			t.Errorf("file %s = %q, want %q", name, got, want)
		}
	}
	if _, err := ops.ReadFile("zzz"); err == nil {
		t.Error("expected an error for an unseeded file")
	}

	// The seed program set __sys.fs.mem = true in the root context.
	got, err := evalExpr(reg, "context dot __sys dot fs dot mem")
	if err != nil {
		t.Fatalf("read __sys.fs.mem: %v", err)
	}
	if got != "true" {
		t.Errorf("__sys.fs.mem = %q, want true", got)
	}
}
