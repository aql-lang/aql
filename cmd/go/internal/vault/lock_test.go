package vault

import (
	"fmt"
	"sync"
	"testing"
)

// TestMutateStoreConcurrentNoLostUpdates is the cross-writer guard:
// many goroutines each commit a distinct alias through mutateStore;
// because each reloads under the lock, every alias must survive. The
// flock excludes across separate open descriptions, so this exercises
// the same path two aql processes would take. Without the lock (or the
// reload), concurrent read-modify-write loses most of the additions.
func TestMutateStoreConcurrentNoLostUpdates(t *testing.T) {
	home := testHome(t)
	mustInit(t)

	const n = 48
	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func(i int) {
			defer wg.Done()
			name := fmt.Sprintf("k%02d", i)
			_ = mutateStore(home, func(s *Store) error {
				s.UpsertAlias(Alias{Name: name, Source: "concurrent"})
				return nil
			})
		}(i)
	}
	wg.Wait()

	s, _ := LoadStore(home)
	if len(s.Aliases) != n {
		t.Errorf("got %d aliases, want %d (lost updates)", len(s.Aliases), n)
	}
	for i := 0; i < n; i++ {
		if a, _ := s.FindAlias(fmt.Sprintf("k%02d", i)); a == nil {
			t.Errorf("alias k%02d was lost", i)
		}
	}
}

// TestMutateStoreErrorAborts confirms a closure error aborts the save.
func TestMutateStoreErrorAborts(t *testing.T) {
	home := testHome(t)
	mustInit(t)
	sentinel := fmt.Errorf("boom")
	if err := mutateStore(home, func(s *Store) error {
		s.UpsertAlias(Alias{Name: "shouldnotpersist"})
		return sentinel
	}); err != sentinel {
		t.Fatalf("err = %v, want sentinel", err)
	}
	s, _ := LoadStore(home)
	if a, _ := s.FindAlias("shouldnotpersist"); a != nil {
		t.Error("mutation persisted despite closure error")
	}
}
