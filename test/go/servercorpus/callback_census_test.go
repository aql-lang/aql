// The server-callback census — case 1 of the server-concurrency corpus
// (design/FULL-COMPILATION.0.md section 13).
//
// The corpus's bar is that every callback case compiles FULLY: zero
// runtime interpreter entries, a transcript identical to the interpreter's,
// and race-clean. Nothing in the tree measured the first of those for a
// server, so "does a callback-architected server compile completely?" had
// no numeric answer — only the whole-program compile/refuse verdict, which
// says nothing about what the per-connection handler does at runtime.
//
// This case measures it. A boru program starts a TCP echo server, connects
// to itself, exchanges messages over the callback, and closes. It runs
// compiled with the interpreter-entry hook armed, and interpreted for the
// transcript oracle. The assertions are: identical results, and the
// unattributed-entry count at or below its ceiling.
//
// Like the other Stage-1 censuses this RECORDS a ratchet rather than
// asserting the end state: the count is what it is today, it may only
// fall, and it reaches zero when the callback path stops re-entering the
// tree-walker (Stage 9).
package servercorpus

import (
	"strings"
	"sync"
	"testing"

	lang "github.com/boru-lang/boru/lang/go"
)

// echoServerProgram starts a server whose per-connection handler echoes
// newline-framed lines, drives it over a real socket, and yields the last
// reply. The handler is the callback under measurement: it is stored by
// serve-raw and invoked by the Go accept loop after the program's own
// statements have run.
const echoServerProgram = `
import "boru:net"
def ln
(Net.serve-raw {tcp:0}
  (fn sock:Socket Any
    [def nl (convert Bytes "\n")
      for 3
        [def line (Net.recv-until sock nl)
          Net.send-bytes (convert Bytes (join "" [(convert String line) "\n"])) sock
        ]
    ]
  )
)
def port ((Net.addr ln).port)
def cli (Net.connect-raw {tcp:(join "" ["127.0.0.1:" (convert String port)])})
def nl (convert Bytes "\n")
Net.send-bytes (convert Bytes "one\n") cli
def a (convert String (Net.recv-until cli nl))
Net.send-bytes (convert Bytes "two\n") cli
def b (convert String (Net.recv-until cli nl))
Net.send-bytes (convert Bytes "three\n") cli
def c (convert String (Net.recv-until cli nl))
Net.close cli
join "" [a b c]
`

// callbackEntryCeiling is the number of unattributed interpreter entries
// the compiled echo case may produce. Monotone DOWN only; 0 when the
// callback path runs entirely on the VM (Stage 9). Measured, not chosen.
const callbackEntryCeiling = 0 // measured 2026-08-25 — see the log line

func TestServerCallbackCensus(t *testing.T) {
	var (
		mu      sync.Mutex
		entries []lang.InterpEntry
	)

	ac, err := lang.New()
	if err != nil {
		t.Fatalf("lang.New: %v", err)
	}
	disarm := ac.ArmInterpEntryHook(func(e lang.InterpEntry) {
		mu.Lock()
		defer mu.Unlock()
		entries = append(entries, e)
	})
	gotC, wasCompiled, errC := ac.RunCompiled(echoServerProgram)
	disarm()
	if errC != nil {
		t.Fatalf("compiled run: %v", errC)
	}

	ai, err := lang.New()
	if err != nil {
		t.Fatalf("lang.New: %v", err)
	}
	gotI, errI := ai.RunInterp(echoServerProgram)
	if errI != nil {
		t.Fatalf("interpreted run: %v", errI)
	}

	// Transcript parity: the callback's observable output must not depend
	// on which lane ran the program.
	if renderResult(gotC) != renderResult(gotI) {
		t.Errorf("echo transcript diverged:\n  compiled    = %q\n  interpreted = %q",
			renderResult(gotC), renderResult(gotI))
	}
	if want := "onetwothree"; !strings.Contains(strings.ReplaceAll(renderResult(gotI), "\n", ""), want) {
		t.Errorf("echo did not round-trip: interpreted result %q, want it to contain %q",
			renderResult(gotI), want)
	}

	mu.Lock()
	defer mu.Unlock()
	bySeam := map[string]int{}
	runs := 0
	for _, e := range entries {
		if e.Attribution != "" {
			continue
		}
		bySeam[e.Seam]++
		if e.Seam == "Engine.Run" {
			runs++
		}
	}
	t.Logf("server-callback census: compiled=%v, %d unattributed interpreter runs (routes: %v)",
		wasCompiled, runs, bySeam)

	if runs > callbackEntryCeiling {
		t.Errorf("server-callback census %d exceeds ceiling %d — the per-connection handler re-entered the interpreter: %v",
			runs, callbackEntryCeiling, bySeam)
	}
}

func renderResult(vals []any) string {
	parts := make([]string, len(vals))
	for i, v := range vals {
		if s, ok := v.(string); ok {
			parts[i] = s
			continue
		}
		parts[i] = "?"
	}
	return strings.Join(parts, "")
}
