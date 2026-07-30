package native

import (
	"fmt"
	"io"
	"strings"
	"sync"
	"testing"
)

type safeSrc2 struct {
	mu sync.Mutex
	r  io.Reader
}

func (s *safeSrc2) Read(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.r.Read(p)
}

// Does the race actually CORRUPT / DUPLICATE / LOSE input, or is it benign?
// Numbered lines make every anomaly detectable.
func TestScratchStdinLineCorruption(t *testing.T) {
	const N = 200000
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	var sb strings.Builder
	for i := 0; i < N; i++ {
		fmt.Fprintf(&sb, "L%07d-padpadpadpadpadpad\n", i)
	}
	r.Input = &safeSrc2{r: strings.NewReader(sb.String())}

	stdin := NewAtom("stdin")
	var mu sync.Mutex
	seen := make(map[string]int, N)
	var malformed []string
	var wg sync.WaitGroup
	for g := 0; g < 4; g++ {
		fork := r.ForkConcurrent()
		wg.Add(1)
		go func(reg *Registry) {
			defer wg.Done()
			local := []string{}
			for {
				out, herr := readLineHandler([]Value{stdin}, nil, nil, reg)
				if herr != nil || len(out) != 1 {
					break
				}
				s, aerr := out[0].AsConcreteString()
				if aerr != nil {
					break // none → EOF
				}
				local = append(local, s)
			}
			mu.Lock()
			for _, s := range local {
				seen[s]++
				if len(s) != 26 || !strings.HasPrefix(s, "L") {
					malformed = append(malformed, s)
				}
			}
			mu.Unlock()
		}(fork)
	}
	wg.Wait()

	dups, missing := 0, 0
	for i := 0; i < N; i++ {
		k := fmt.Sprintf("L%07d-padpadpadpadpadpad", i)
		switch c := seen[k]; {
		case c == 0:
			missing++
		case c > 1:
			dups += c - 1
		}
	}
	t.Logf("lines written=%d distinct-read=%d missing=%d duplicated=%d malformed=%d",
		N, len(seen), missing, dups, len(malformed))
	for i, m := range malformed {
		if i >= 5 {
			break
		}
		t.Logf("  malformed sample %d: %q", i, m)
	}
}
