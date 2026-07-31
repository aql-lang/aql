// Tests for the Service-shaped REPL wrapper (service.go). Blocking
// input is modelled with io.Pipe so Stop/Pause/Resume can be driven
// against a live loop; output is read through a mutex-guarded buffer
// because the loop writes from its own goroutine.

package repl

import (
	"bytes"
	"context"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/boru-lang/boru/cmd/go/internal/service"
)

// syncBuffer is a mutex-guarded bytes.Buffer safe to share between
// the REPL goroutine and the test goroutine.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// waitReplState polls Status (atomic-backed, race-free) until it
// reports want or the deadline passes.
func waitReplState(t *testing.T, s *Server, want service.State) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if s.Status() == want {
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatalf("server never reached state %s (currently %s)", want, s.Status())
}

// waitOutput polls the shared output buffer until it contains want.
func waitOutput(t *testing.T, out *syncBuffer, want string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if strings.Contains(out.String(), want) {
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatalf("output never contained %q; got %q", want, out.String())
}

func TestNewServerDefaults(t *testing.T) {
	s := NewServer(strings.NewReader(""), io.Discard, "")
	if s.Name() != "repl" {
		t.Errorf("Name() = %q, want repl", s.Name())
	}
	if !s.UsesStdio() {
		t.Error("UsesStdio() = false, want true")
	}
	if s.Status() != service.StateStopped {
		t.Errorf("initial Status() = %s, want stopped", s.Status())
	}
	if md := s.Metadata(); len(md) != 0 {
		t.Errorf("Metadata() = %v, want empty without a registry path", md)
	}
}

func TestServerMetadataIncludesRegistry(t *testing.T) {
	s := NewServer(strings.NewReader(""), io.Discard, "/some/registry")
	md := s.Metadata()
	if md["registry"] != "/some/registry" {
		t.Errorf("Metadata registry = %q, want /some/registry", md["registry"])
	}
}

func TestServerStartEvaluatesUntilEOF(t *testing.T) {
	out := &syncBuffer{}
	s := NewServer(strings.NewReader("1 add 2\n"), out, "")
	if err := s.Start(context.Background()); err != nil {
		t.Fatalf("Start: %s", err)
	}
	if !strings.Contains(out.String(), "3") {
		t.Errorf("output = %q, want the evaluated result 3", out.String())
	}
	if s.Status() != service.StateStopped {
		t.Errorf("post-Start Status() = %s, want stopped", s.Status())
	}
}

func TestServerStopUnblocksStart(t *testing.T) {
	inR, inW := io.Pipe()
	defer inW.Close()
	out := &syncBuffer{}
	s := NewServer(inR, out, "")

	errCh := make(chan error, 1)
	go func() { errCh <- s.Start(context.Background()) }()

	waitReplState(t, s, service.StateRunning)
	if err := s.Stop(context.Background()); err != nil {
		t.Fatalf("Stop: %s", err)
	}

	select {
	case err := <-errCh:
		if err != nil {
			t.Errorf("Start after Stop = %s, want nil", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Start did not return after Stop")
	}
	if s.Status() != service.StateStopped {
		t.Errorf("Status() = %s, want stopped", s.Status())
	}
}

func TestServerCancelUnblocksStart(t *testing.T) {
	inR, inW := io.Pipe()
	defer inW.Close()
	s := NewServer(inR, &syncBuffer{}, "")

	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() { errCh <- s.Start(ctx) }()

	waitReplState(t, s, service.StateRunning)
	cancel()

	select {
	case err := <-errCh:
		if err != nil {
			t.Errorf("Start after cancel = %s, want nil", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Start did not return after ctx cancel")
	}
}

func TestServerPauseDiscardsInputAndResumeRestoresEval(t *testing.T) {
	inR, inW := io.Pipe()
	out := &syncBuffer{}
	s := NewServer(inR, out, "")

	errCh := make(chan error, 1)
	go func() { errCh <- s.Start(context.Background()) }()
	waitReplState(t, s, service.StateRunning)

	if err := s.Pause(context.Background()); err != nil {
		t.Fatalf("Pause: %s", err)
	}
	if s.Status() != service.StatePaused {
		t.Errorf("Status() after Pause = %s, want paused", s.Status())
	}
	if _, err := io.WriteString(inW, "1 add 2\n"); err != nil {
		t.Fatalf("write while paused: %s", err)
	}
	waitOutput(t, out, "paused")
	if strings.Contains(out.String(), "3") {
		t.Errorf("paused REPL evaluated input anyway: %q", out.String())
	}

	if err := s.Resume(context.Background()); err != nil {
		t.Fatalf("Resume: %s", err)
	}
	if s.Status() != service.StateRunning {
		t.Errorf("Status() after Resume = %s, want running", s.Status())
	}
	if _, err := io.WriteString(inW, "1 add 2\n"); err != nil {
		t.Fatalf("write after resume: %s", err)
	}
	waitOutput(t, out, "3")

	// EOF ends the loop.
	inW.Close()
	select {
	case err := <-errCh:
		if err != nil {
			t.Errorf("Start = %s, want nil", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Start did not return after input EOF")
	}
}

func TestServerExitWordWorksWhilePaused(t *testing.T) {
	// The exit word is checked BEFORE the pause gate, so a paused
	// service can still be shut down from its input stream.
	inR, inW := io.Pipe()
	defer inW.Close()
	out := &syncBuffer{}
	s := NewServer(inR, out, "")

	errCh := make(chan error, 1)
	go func() { errCh <- s.Start(context.Background()) }()
	waitReplState(t, s, service.StateRunning)

	if err := s.Pause(context.Background()); err != nil {
		t.Fatalf("Pause: %s", err)
	}
	if _, err := io.WriteString(inW, "exit\n"); err != nil {
		t.Fatalf("write exit: %s", err)
	}

	select {
	case err := <-errCh:
		if err != nil {
			t.Errorf("Start = %s, want nil", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("exit word did not shut down the paused REPL")
	}
	if strings.Contains(out.String(), "paused") {
		t.Errorf("exit must bypass the pause gate, but got the paused notice: %q", out.String())
	}
}
