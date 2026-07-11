package buildrt

import (
	"bytes"
	"reflect"
	"strings"
	"testing"

	lang "github.com/aql-lang/aql/lang/go"
)

// langOpts returns a default (no-policy, allow-everything) options value for
// the eval tests.
func langOpts() lang.Options { return lang.Options{} }

// --- Eval / runAndPrint contract ---

func TestEvalPrintsResidual(t *testing.T) {
	var out bytes.Buffer
	if err := Eval(&out, "add 1 2", langOpts(), CompileOff); err != nil {
		t.Fatalf("Eval: %v", err)
	}
	if got := strings.TrimSpace(out.String()); got != "3" {
		t.Fatalf("got %q, want 3", got)
	}
}

func TestEvalErrorOnUndefinedWord(t *testing.T) {
	var out bytes.Buffer
	err := Eval(&out, "nope-not-a-word", langOpts(), CompileOff)
	if err == nil {
		t.Fatal("expected an error for an undefined word")
	}
	if out.Len() != 0 {
		t.Fatalf("expected no stdout on error, got %q", out.String())
	}
}

func TestEvalForceCompileSurfacesRefusal(t *testing.T) {
	// A program the bytecode emitter cannot lower must error under
	// CompileForce (rather than silently falling back).
	var out bytes.Buffer
	err := Eval(&out, "(size (for 5 [i]))", langOpts(), CompileForce)
	if err == nil {
		t.Fatal("expected force-compile to refuse an uncompilable program")
	}
}

// --- Main: exit codes, stdout/stderr split, bundled files ---

func TestMainSuccess(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Main(Config{Source: "add 1 2"}, nil, nil, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit=%d, want 0 (stderr=%q)", code, stderr.String())
	}
	if got := strings.TrimSpace(stdout.String()); got != "3" {
		t.Fatalf("stdout=%q, want 3", got)
	}
}

func TestMainErrorExitsNonZero(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Main(Config{Source: "nope-not-a-word"}, nil, nil, &stdout, &stderr)
	if code == 0 {
		t.Fatal("expected non-zero exit for a runtime error")
	}
	if stderr.Len() == 0 {
		t.Fatal("expected a diagnostic on stderr")
	}
}

func TestMainBundledFileImportResolvesInMemory(t *testing.T) {
	// The entry imports a sibling file that exists only in the embedded
	// Files map — never on disk — proving the in-memory FS is wired up.
	entryDir := "/virtual/proj"
	cfg := Config{
		Source:   `import "./lib.aql"` + "\n" + `Lib.x`,
		EntryDir: entryDir,
		Files: map[string][]byte{
			entryDir + "/lib.aql": []byte(`export "Lib" {x:7}`),
		},
	}
	var stdout, stderr bytes.Buffer
	code := Main(cfg, nil, nil, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit=%d, want 0 (stderr=%q)", code, stderr.String())
	}
	if got := strings.TrimSpace(stdout.String()); got != "7" {
		t.Fatalf("stdout=%q, want 7", got)
	}
}

func TestMainBadOptionsBlobErrors(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Main(Config{Source: "add 1 2", OptionsBlob: "tape:boguskey:10"}, nil, nil, &stdout, &stderr)
	if code == 0 {
		t.Fatal("expected a non-zero exit for an invalid --options blob")
	}
}

// --- Payload encode/decode round-trip ---

func TestPayloadRoundTrip(t *testing.T) {
	cfg := Config{
		Source:   "add 1 2",
		EntryDir: "/x/y",
		Files:    map[string][]byte{"/x/y/lib.aql": []byte("export \"L\" {a:1}")},
		Registry: "reg",
		Seed:     99,
		Compile:  CompileTry,
	}
	payload, err := EncodePayload(cfg)
	if err != nil {
		t.Fatalf("EncodePayload: %v", err)
	}
	// Simulate appending to a host binary image.
	image := append([]byte("PRETEND-BINARY-CONTENT"), payload...)

	got, ok, err := DecodePayload(image)
	if err != nil {
		t.Fatalf("DecodePayload: %v", err)
	}
	if !ok {
		t.Fatal("expected ok=true for an image with a payload")
	}
	if !reflect.DeepEqual(got, cfg) {
		t.Fatalf("round-trip mismatch:\n got %+v\nwant %+v", got, cfg)
	}
}

func TestDecodePayloadNoMagic(t *testing.T) {
	_, ok, err := DecodePayload([]byte("just a plain binary with no trailer"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ok {
		t.Fatal("expected ok=false for an image without a payload")
	}
}

func TestDecodePayloadShortImage(t *testing.T) {
	_, ok, _ := DecodePayload([]byte("tiny"))
	if ok {
		t.Fatal("expected ok=false for an image smaller than the footer")
	}
}

func TestDecodePayloadCorruptLength(t *testing.T) {
	// Magic present but the declared length overruns the image.
	footer := make([]byte, footerSize)
	copy(footer[:magicSize], magic)
	// length far larger than the body
	for i := magicSize; i < footerSize; i++ {
		footer[i] = 0xff
	}
	image := append([]byte("body"), footer...)
	_, ok, err := DecodePayload(image)
	if err == nil {
		t.Fatal("expected an error for a corrupt (overrunning) length")
	}
	if ok {
		t.Fatal("expected ok=false on corruption")
	}
}

// EvalColor renders a structured runtime error through the ANSI
// renderer when color is on, and stays byte-plain when off.
func TestEvalColorErrorRendering(t *testing.T) {
	var out bytes.Buffer
	err := EvalColor(&out, "99 uppr", langOpts(), CompileOff, true)
	if err == nil || !strings.Contains(err.Error(), "\x1b[") {
		t.Fatalf("color=true must render ANSI, got %v", err)
	}
	if !strings.Contains(err.Error(), "error: ") {
		t.Errorf("the error: prefix must survive the colored path: %v", err)
	}
	err = EvalColor(&out, "99 uppr", langOpts(), CompileOff, false)
	if err == nil || strings.Contains(err.Error(), "\x1b[") {
		t.Fatalf("color=false must stay plain, got %v", err)
	}
}
