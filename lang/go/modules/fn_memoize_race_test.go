package modules

import (
	"sync"
	"testing"

	"github.com/boru-lang/boru/lang/go/native"
)

// TestFnUtilMemoizeConcurrent pins the P1 review finding on PR #397: the
// memoize cache is captured by the returned handler, and that handler rides
// a Function VALUE — which a ForkConcurrent registry inherits, so `await`,
// timers, services and network callbacks can drive the SAME closure from
// several goroutines. An unsynchronised map there is not merely racy: Go
// terminates the process on a concurrent map read/write, so the failure mode
// is a killed program rather than a wrong answer.
//
// Run under -race this fails loudly without the mutex; without -race it can
// still trip the runtime's own concurrent-map detector. Both cold keys (a
// write while others read) and hot keys (concurrent reads) are exercised.
func TestFnUtilMemoizeConcurrent(t *testing.T) {
	r := fnUtilReg(t)
	var mu sync.Mutex
	calls := 0
	counted := fnTestFn(1, func(a []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
		mu.Lock()
		calls++
		mu.Unlock()
		return []native.Value{a[0]}, nil
	})
	out, err := fnUtilHandler(t, "memoize")([]native.Value{counted}, nil, nil, r)
	if err != nil {
		t.Fatal(err)
	}
	m := out[0]

	// Warm one key so readers race an existing entry while writers add new
	// ones — the read/write pairing that kills the process.
	if _, err := applyWrapper(t, r, m, native.NewInteger(0)); err != nil {
		t.Fatal(err)
	}

	const workers = 8
	var wg sync.WaitGroup
	errs := make([]error, workers)
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			for i := 0; i < 40; i++ {
				key := int64(i % 10) // shared cold keys, then hot ones
				if _, err := applyWrapper(t, r, m, native.NewInteger(key)); err != nil {
					errs[w] = err
					return
				}
				if _, err := applyWrapper(t, r, m, native.NewInteger(0)); err != nil {
					errs[w] = err
					return
				}
			}
		}(w)
	}
	wg.Wait()
	for w, err := range errs {
		if err != nil {
			t.Fatalf("worker %d: %v", w, err)
		}
	}
}
