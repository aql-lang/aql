package native

import (
	"io"
	"strings"
	"sync"
	"testing"

	eng "github.com/aql-lang/aql/eng/go"
)

// safeReader makes the UNDERLYING source concurrency-safe, so any race the
// detector reports must be inside the shared *bufio.Reader itself.
type safeReader struct {
	mu sync.Mutex
	r  io.Reader
}

func (s *safeReader) Read(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.r.Read(p)
}

func TestScratchStdinLineRace(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	var sb strings.Builder
	for i := 0; i < 400000; i++ {
		sb.WriteString("a-line-of-input-text-here\n")
	}
	r.Input = &safeReader{r: strings.NewReader(sb.String())}

	f1 := r.ForkConcurrent()
	f2 := r.ForkConcurrent()
	h1, _, _ := eng.Cap[*stdinLines](f1, CapStdinLines)
	h2, _, _ := eng.Cap[*stdinLines](f2, CapStdinLines)
	if h1 == nil || h1 != h2 {
		t.Fatalf("forks do not share the stdinLines holder: %p vs %p", h1, h2)
	}
	if lineReaderForStdin(f1) != lineReaderForStdin(f2) {
		t.Fatal("forks do not share the *bufio.Reader")
	}

	stdin := NewAtom("stdin")
	var wg sync.WaitGroup
	for g := 0; g < 4; g++ {
		fork := r.ForkConcurrent()
		wg.Add(1)
		go func(reg *Registry) {
			defer wg.Done()
			for i := 0; i < 50000; i++ {
				out, herr := readLineHandler([]Value{stdin}, nil, nil, reg)
				if herr != nil || len(out) != 1 {
					return
				}
			}
		}(fork)
	}
	wg.Wait()
}
