package api

import (
	"context"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/aql-lang/aql/cmd/go/internal/service"
)

// seam5Inspector is a minimal service.Inspector for exercising Start.
type seam5Inspector struct{}

func (seam5Inspector) Services() []service.Service { return nil }
func (seam5Inspector) ByName(string) (service.Service, bool) {
	return nil, false
}
func (seam5Inspector) StopService(context.Context, string) error { return nil }

// seam5TempDir points the discovery file at a per-test directory so the
// suite never touches the real $TMPDIR/aql-api.json.
func seam5TempDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	orig := tempDirFunc
	tempDirFunc = func() string { return dir }
	t.Cleanup(func() { tempDirFunc = orig })
	return dir
}

func TestSeam5WriteDiscoveryFileMarshalError(t *testing.T) {
	dir := seam5TempDir(t)
	boom := errors.New("marshal boom")
	orig := jsonMarshal
	jsonMarshal = func(any) ([]byte, error) { return nil, boom }
	t.Cleanup(func() { jsonMarshal = orig })

	s := NewServer("127.0.0.1:0", "", io.Discard)
	if err := s.writeDiscoveryFile(); !errors.Is(err, boom) {
		t.Fatalf("writeDiscoveryFile = %v, want %v", err, boom)
	}
	if _, err := os.Stat(filepath.Join(dir, "aql-api.json")); !os.IsNotExist(err) {
		t.Fatalf("discovery file must not exist after marshal error, stat err = %v", err)
	}
}

func TestSeam5WriteDiscoveryFileChmodError(t *testing.T) {
	dir := seam5TempDir(t)
	boom := errors.New("chmod boom")
	orig := fileChmod
	fileChmod = func(*os.File, os.FileMode) error { return boom }
	t.Cleanup(func() { fileChmod = orig })

	s := NewServer("127.0.0.1:0", "", io.Discard)
	if err := s.writeDiscoveryFile(); !errors.Is(err, boom) {
		t.Fatalf("writeDiscoveryFile = %v, want %v", err, boom)
	}
	seam5AssertNoTempLeft(t, dir)
}

func TestSeam5WriteDiscoveryFileWriteError(t *testing.T) {
	dir := seam5TempDir(t)
	boom := errors.New("write boom")
	orig := fileWrite
	fileWrite = func(*os.File, []byte) (int, error) { return 0, boom }
	t.Cleanup(func() { fileWrite = orig })

	s := NewServer("127.0.0.1:0", "", io.Discard)
	if err := s.writeDiscoveryFile(); !errors.Is(err, boom) {
		t.Fatalf("writeDiscoveryFile = %v, want %v", err, boom)
	}
	seam5AssertNoTempLeft(t, dir)
}

func TestSeam5WriteDiscoveryFileCloseError(t *testing.T) {
	dir := seam5TempDir(t)
	boom := errors.New("close boom")
	orig := fileClose
	fileClose = func(f *os.File) error {
		_ = f.Close()
		return boom
	}
	t.Cleanup(func() { fileClose = orig })

	s := NewServer("127.0.0.1:0", "", io.Discard)
	if err := s.writeDiscoveryFile(); !errors.Is(err, boom) {
		t.Fatalf("writeDiscoveryFile = %v, want %v", err, boom)
	}
	seam5AssertNoTempLeft(t, dir)
}

// seam5AssertNoTempLeft checks the failure arms removed their temp file.
func seam5AssertNoTempLeft(t *testing.T, dir string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if strings.Contains(e.Name(), "aql-api") {
			t.Fatalf("leftover file %s in %s", e.Name(), dir)
		}
	}
}

// TestSeam5StartServeError drives Start's final `return err` arm: closing
// the listener out from under the http server makes Serve return a real
// error that is not http.ErrServerClosed.
func TestSeam5StartServeError(t *testing.T) {
	seam5TempDir(t)

	s := NewServer("127.0.0.1:0", "", io.Discard)
	s.Bind(seam5Inspector{})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	errc := make(chan error, 1)
	go func() { errc <- s.Start(ctx) }()

	deadline := time.Now().Add(5 * time.Second)
	for s.Status() != service.StateRunning {
		if time.Now().After(deadline) {
			t.Fatal("server did not reach Running state")
		}
		time.Sleep(time.Millisecond)
	}
	// Tear the listener down directly: Serve returns "use of closed
	// network connection", which Start must propagate.
	if err := s.ln.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-errc:
		if err == nil {
			t.Fatal("Start must return the Serve error, got nil")
		}
		if errors.Is(err, http.ErrServerClosed) {
			t.Fatalf("Start returned ErrServerClosed, want a raw listener error")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Start did not return after listener close")
	}
}
