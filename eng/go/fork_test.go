package eng

import (
	"sync"
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// TestForkConcurrentIsolation runs many forks concurrently, each
// mutating its own def table, context, and args scope, and asserts the
// writes stay private. Run under `go test -race` it also guards against
// the shared-registry data race that ForkConcurrent exists to remove.
func TestForkConcurrentIsolation(t *testing.T) {
	r, err := core.NewRegistry()
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	r.Defs.Push("shared", core.NewInteger(1))

	const n = 32
	var wg sync.WaitGroup
	wg.Add(n)
	results := make([]int64, n)
	for i := 0; i < n; i++ {
		f := r.ForkConcurrent()
		go func(idx int, reg *core.Registry) {
			defer wg.Done()
			// Each fork writes a distinct private binding and a context key.
			reg.Defs.Push("local", core.NewInteger(int64(idx)))
			reg.Contexts.Push(reg.Contexts.Top())
			if v, ok := reg.Defs.Top("local"); ok {
				n, _ := core.AsInteger(v)
				results[idx] = n
			}
			// Inherited binding is still visible.
			if v, ok := reg.Defs.Top("shared"); ok {
				if s, _ := core.AsInteger(v); s != 1 {
					t.Errorf("fork %d: shared = %d, want 1", idx, s)
				}
			} else {
				t.Errorf("fork %d: shared binding not inherited", idx)
			}
		}(i, f)
	}
	wg.Wait()

	for i := 0; i < n; i++ {
		if results[i] != int64(i) {
			t.Errorf("fork %d saw local = %d, want %d (scope leaked)", i, results[i], i)
		}
	}
	// Parent must be unaffected by any fork's writes.
	if _, ok := r.Defs.Top("local"); ok {
		t.Error("parent registry saw a fork's local binding (isolation broken)")
	}
}

func TestForkConcurrentInheritsDefsSnapshot(t *testing.T) {
	r, err := core.NewRegistry()
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	r.Defs.Push("x", core.NewInteger(7))
	f := r.ForkConcurrent()

	// Fork sees the snapshot.
	if v, ok := f.Defs.Top("x"); !ok {
		t.Fatal("fork did not inherit x")
	} else if n, _ := core.AsInteger(v); n != 7 {
		t.Errorf("fork x = %d, want 7", n)
	}

	// A later parent rebind does not retroactively change the fork.
	r.Defs.Push("x", core.NewInteger(99))
	if v, _ := f.Defs.Top("x"); func() int64 { n, _ := core.AsInteger(v); return n }() != 7 {
		t.Error("fork x changed after parent rebind (snapshot not isolated)")
	}
}
