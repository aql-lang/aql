package native

import (
	"bufio"
	"strings"
	"sync"
	"testing"
)

// The shared stdin buffer is reachable from more than one goroutine:
// eng.ForkConcurrent leaves Capabilities and Input alone, so every await/spawn
// fork shares one *stdinLines holder, and a File handle can be passed between
// them too. bufio.Reader is documented as unsafe for concurrent use.
//
// The mutex on both holders originally guarded only the buffer's CREATION —
// the accessor returned the *bufio.Reader and the caller read from it after
// unlocking — so concurrent reads were unsynchronised. The symptom is not a
// crash: it is duplicated, interleaved or lost input, which reaches a user as
// corrupt data.
//
// These tests are meaningless without -race (the repo runs lang/go under it:
// Makefile's RACE_MODULES). Under -race they fail on the pre-fix code and pass
// on the fixed code, which is the whole point — verified by reverting the lock
// scope and watching them report a data race inside bufio.Reader.

func TestStdinLinesConcurrentReadLineIsSynchronised(t *testing.T) {
	var sb strings.Builder
	for i := 0; i < 2000; i++ {
		sb.WriteString("line\n")
	}
	h := &stdinLines{}
	src := strings.NewReader(sb.String())

	var wg sync.WaitGroup
	var mu sync.Mutex
	got := 0
	for g := 0; g < 8; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				line, ok, err := h.readLine(src)
				if err != nil || !ok {
					return
				}
				if line != "line" {
					mu.Lock()
					t.Errorf("torn read: got %q, want %q", line, "line")
					mu.Unlock()
					return
				}
				mu.Lock()
				got++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()

	// Every line is read exactly once across all readers: the count is the
	// property a torn read breaks, and it is checked as well as the content
	// because a race can duplicate a well-formed line as easily as split one.
	if got != 2000 {
		t.Errorf("read %d lines in total, want 2000 — lines were lost or duplicated", got)
	}
}

// The whole-stream drain shares the same buffer, so it must take the same
// lock. This runs it against concurrent line reads and asserts that between
// them they consume the input exactly once.
func TestStdinLinesReadAllSharesTheLock(t *testing.T) {
	var sb strings.Builder
	for i := 0; i < 500; i++ {
		sb.WriteString("x\n")
	}
	h := &stdinLines{}
	src := strings.NewReader(sb.String())

	var wg sync.WaitGroup
	var mu sync.Mutex
	lines, drained := 0, 0
	for g := 0; g < 4; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 50; i++ {
				if _, ok, err := h.readLine(src); err != nil || !ok {
					return
				}
				mu.Lock()
				lines++
				mu.Unlock()
			}
		}()
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		b, err := h.readAll(src)
		if err != nil {
			return
		}
		mu.Lock()
		drained = len(b)
		mu.Unlock()
	}()
	wg.Wait()

	// 500 lines of "x\n" = 1000 bytes. Whatever the interleaving, the line
	// readers' 2 bytes each plus the drain's remainder must be the whole
	// input and no more.
	if lines*2+drained != 1000 {
		t.Errorf("line readers took %d lines (%d bytes) and the drain took %d bytes, "+
			"totalling %d — want exactly 1000", lines, lines*2, drained, lines*2+drained)
	}
}

// A File handle carries its own buffer and its own mutex, with the same
// original defect, so it gets the same guard.
func TestFileHandleConcurrentReadLineIsSynchronised(t *testing.T) {
	var sb strings.Builder
	for i := 0; i < 1000; i++ {
		sb.WriteString("row\n")
	}
	fh := &FileHandleInfo{lines: bufio.NewReader(strings.NewReader(sb.String()))}

	var wg sync.WaitGroup
	var mu sync.Mutex
	got := 0
	for g := 0; g < 8; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				line, ok, err := fh.ReadLine()
				if err != nil || !ok {
					return
				}
				if line != "row" {
					mu.Lock()
					t.Errorf("torn read: got %q, want %q", line, "row")
					mu.Unlock()
					return
				}
				mu.Lock()
				got++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	if got != 1000 {
		t.Errorf("read %d lines in total, want 1000 — lines were lost or duplicated", got)
	}
}
