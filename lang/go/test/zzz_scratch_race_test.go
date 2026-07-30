package test

import (
	"io"
	"strings"
	"sync"
	"testing"

	lang "github.com/aql-lang/aql/lang/go"
)

type safeSrc struct {
	mu sync.Mutex
	r  io.Reader
}

func (s *safeSrc) Read(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.r.Read(p)
}

// Reachability from ORDINARY AQL SOURCE: TimeUtil.await runs branches on
// ForkConcurrent registries that share the parent's stdin holder.
func TestScratchAwaitReadLineRace(t *testing.T) {
	a, err := lang.New()
	if err != nil {
		t.Fatal(err)
	}
	var sb strings.Builder
	for i := 0; i < 200000; i++ {
		sb.WriteString("a-line-of-input-text-here\n")
	}
	a.NativeRegistry().Input = &safeSrc{r: strings.NewReader(sb.String())}

	body := strings.Repeat("IO.read-line (IO.stdin) drop\n", 4000)
	src := "import \"aql:io\"\nimport \"aql:time-util\"\nTimeUtil.await [[" +
		body + "] [" + body + "] [" + body + "]]\ndrop\n"
	if _, err := a.Run(src); err != nil {
		t.Fatalf("run: %v", err)
	}
}
