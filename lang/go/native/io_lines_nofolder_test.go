package native

import (
	"bufio"
	"strings"
	"testing"
)

// The stdin line/drain paths all have a fallback for a registry with no
// shared-reader holder installed. DefaultRegistryWithPolicy installs one
// eagerly, so no production path reaches these arms — but a host assembling a
// registry by hand can, and "a host can reach it" is the difference between a
// fallback and dead code.
//
// The fallback's contract is narrower than the holder's and worth stating: a
// line read still WORKS, it just cannot share its buffer with a later
// whole-stream read, because each call builds its own. These tests pin that,
// so a future change that quietly made the holder mandatory would fail here
// rather than in a host's program.

// withoutHolder returns a standard registry with the shared-reader capability
// removed, which is the only way to reach the fallbacks.
func withoutHolder(t *testing.T, input string) *Registry {
	t.Helper()
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	if _, derr := r.Capabilities.Delete(CapStdinLines); derr != nil {
		t.Fatalf("removing %s: %v", CapStdinLines, derr)
	}
	if holder, ok, _ := r.Capabilities.Get(CapStdinLines); ok && holder != nil {
		t.Fatalf("the holder is still installed: %v", holder)
	}
	r.Input = strings.NewReader(input)
	return r
}

func TestStdinLineSourceFallsBackWithoutAHolder(t *testing.T) {
	r := withoutHolder(t, "alpha\nbravo\n")
	src := stdinLineSource{r}

	line, ok, err := src.ReadLine()
	if err != nil || !ok {
		t.Fatalf("ReadLine: %q ok=%v err=%v", line, ok, err)
	}
	if line != "alpha" {
		t.Errorf("read %q, want %q", line, "alpha")
	}
}

// Each fallback call builds its OWN buffer over r.Input, so a second read does
// not continue where the first stopped — it re-reads whatever the underlying
// reader has left. That is the documented cost of running without a holder,
// and pinning it keeps the difference from the shared path explicit.
func TestStdinLineSourceFallbackDoesNotShareItsBuffer(t *testing.T) {
	r := withoutHolder(t, "one\ntwo\nthree\n")
	src := stdinLineSource{r}

	first, _, err := src.ReadLine()
	if err != nil {
		t.Fatal(err)
	}
	if first != "one" {
		t.Fatalf("first read %q, want %q", first, "one")
	}
	// A fresh buffer over the SAME underlying reader: bufio read ahead on the
	// first call, so what is left is whatever it did not buffer. The property
	// asserted is only that the call succeeds and does not repeat "one" — the
	// exact remainder is a bufio implementation detail.
	second, ok, err := src.ReadLine()
	if err != nil {
		t.Fatalf("second ReadLine: %v", err)
	}
	if ok && second == "one" {
		t.Error("the fallback re-read the first line — each call must take a " +
			"fresh view of the remaining input, not restart the stream")
	}
}

func TestStdinReadAllFallsBackWithoutAHolder(t *testing.T) {
	r := withoutHolder(t, "payload\nrest\n")
	got, err := stdinReadAll(r)
	if err != nil {
		t.Fatalf("stdinReadAll: %v", err)
	}
	if string(got) != "payload\nrest\n" {
		t.Errorf("drained %q, want the whole input", got)
	}
}

// The holder path is the one production uses, and it DOES share: a line read
// followed by a whole-stream read yields the rest of the stream rather than
// re-reading from the start. Stated here beside the fallback so the two
// contracts sit together.
func TestStdinHolderPathSharesItsBuffer(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	r.Input = strings.NewReader("first\nsecond\nthird\n")

	line, ok, rerr := stdinLineSource{r}.ReadLine()
	if rerr != nil || !ok || line != "first" {
		t.Fatalf("ReadLine: %q ok=%v err=%v", line, ok, rerr)
	}
	rest, aerr := stdinReadAll(r)
	if aerr != nil {
		t.Fatalf("stdinReadAll: %v", aerr)
	}
	if string(rest) != "second\nthird\n" {
		t.Errorf("the drain returned %q, want the REST of the stream — the line "+
			"read and the drain must share one buffer", rest)
	}
}

// Registry.Input is a public io.Reader field, so a host may assign a reader
// that is ALREADY buffered. The fallback recognises that and reuses it rather
// than wrapping a second bufio.Reader around it — double buffering would put
// bytes in an intermediate layer that the next read never looks at, which is
// the same class of bug as the un-shared buffer this whole path exists to
// avoid.
func TestStdinFallbackReusesAnAlreadyBufferedInput(t *testing.T) {
	r := withoutHolder(t, "")
	inner := bufio.NewReader(strings.NewReader("buffered\nsecond\n"))
	r.Input = inner

	if got := lineReaderForStdin(r); got != inner {
		t.Fatalf("lineReaderForStdin wrapped an already-buffered reader (%p, want %p) "+
			"— the second layer would hold bytes the first never sees", got, inner)
	}

	line, ok, err := stdinLineSource{r}.ReadLine()
	if err != nil || !ok || line != "buffered" {
		t.Errorf("ReadLine over a pre-buffered Input = %q ok=%v err=%v", line, ok, err)
	}
}
