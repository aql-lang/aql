package debugcmd

// debugcmd_seam6e_test.go — seam-arm tests (design/TEST-SEAMS.10.md) for
// runServe's init-error and listen-error arms and writeDiscovery's
// marshal-error arm.

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	lang "github.com/aql-lang/aql/lang/go"
)

func TestServeInitError(t *testing.T) {
	orig := langNew
	langNew = func(...lang.Options) (*lang.AQL, error) { return nil, errors.New("init boom") }
	t.Cleanup(func() { langNew = orig })
	var stdout, stderr bytes.Buffer
	code := runServe(nil, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("exit = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "init boom") {
		t.Errorf("stderr = %q, want init boom", stderr.String())
	}
}

func TestServeListenError(t *testing.T) {
	t.Setenv("TMPDIR", t.TempDir()) // keep the discovery file out of the real tmp
	var stdout, stderr bytes.Buffer
	// A non-loopback bind without --allow-public is refused before listening.
	code := runServe([]string{"--bind", "203.0.113.1:0"}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("exit = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "debug serve:") {
		t.Errorf("stderr = %q, want serve error", stderr.String())
	}
	if !strings.Contains(stdout.String(), "serving introspection") {
		t.Errorf("stdout = %q, want serving line", stdout.String())
	}
}

func TestWriteDiscoveryMarshalError(t *testing.T) {
	orig := jsonMarshal
	jsonMarshal = func(any) ([]byte, error) { return nil, errors.New("marshal boom") }
	t.Cleanup(func() { jsonMarshal = orig })
	path := filepath.Join(t.TempDir(), "aql-debug.json")
	writeDiscovery(path, "127.0.0.1:1", "")
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("discovery file written despite marshal error (stat err=%v)", err)
	}
}
