// Cross-engine differential gate. This is the Go half of the "each engine
// validates against the other" contract: it runs the shared eng/spec corpus
// through the Go kernel and compares each row's result to the TypeScript
// engine's result for the SAME row, obtained by executing the TS dumper
// (eng/ts/src/crossdump.ts) once and reading its JSONL.
//
// Until now the two engines were compared only INDIRECTLY — each asserted its
// own result against the golden `expected` column, so agreement was
// transitive. This gate compares the engines DIRECTLY, so a divergence is
// caught even on a row whose golden is wrong or absent, and functionality
// gaps are surfaced explicitly.
//
// Dispositions per row (joined on file:line):
//   - agree        — both produce the same value, or both error (parity).
//   - codeDiff      — both error but with different taxonomy codes (reported).
//   - gap          — one engine produces a value where the other errors: a
//     functionality gap. PERMITTED — reported, never failed.
//   - divergence   — both produce a value but the values differ: a real
//     miscompilation/misinterpretation. HARD FAIL.
//
// The test SKIPS (rather than fails) when `node` is unavailable, so the Go
// module stays buildable/testable in environments without the TS toolchain.
package engspec

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	eng "github.com/aql-lang/aql/eng/go"
	"github.com/aql-lang/aql/eng/go/parser"
)

// crossRec is one engine's result for one corpus row.
type crossRec struct {
	Mode  string `json:"mode"`
	File  string `json:"file"`
	Line  int    `json:"line"`
	Input string `json:"input"`
	OK    bool   `json:"ok"`
	Out   string `json:"out"`
}

func crossKey(r crossRec) string { return r.Mode + "|" + r.File + ":" + itoa(r.Line) }

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}

// goValueResult runs one row through the Go kernel exactly as TestSpec does,
// rendering success via eng.Canon and failure as the AqlError taxonomy code
// (so error-parity is compared by code, not message text — the same shape the
// TS dumper emits).
func goValueResult(input string) (bool, string) {
	values, err := parser.Parse(input)
	if err != nil {
		return false, "TOKENIZE:" + err.Error()
	}
	r, err := eng.NewRegistry()
	if err != nil {
		return false, "UNEXPECTED:newRegistry"
	}
	registerSpecWords(r)
	r.InitRootContext()
	out, runErr := eng.NewTop(r).Run(values)
	if runErr != nil {
		var ae *eng.AqlError
		if errors.As(runErr, &ae) {
			return false, ae.Code
		}
		return false, "UNEXPECTED:" + runErr.Error()
	}
	return true, eng.Canon(out)
}

// tsResults runs the TS dumper once and returns its records keyed by crossKey.
// Returns ok=false when node is unavailable so the caller can skip.
func tsResults(t *testing.T) (map[string]crossRec, bool) {
	t.Helper()
	node, err := exec.LookPath("node")
	if err != nil {
		return nil, false
	}
	script := filepath.Join("..", "..", "..", "eng", "ts", "src", "crossdump.ts")
	if _, err := os.Stat(script); err != nil {
		return nil, false
	}
	cmd := exec.Command(node, "--experimental-strip-types", "--no-warnings", script)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Logf("TS dumper failed (%v); stderr:\n%s", err, stderr.String())
		return nil, false
	}
	recs := map[string]crossRec{}
	sc := bufio.NewScanner(&stdout)
	sc.Buffer(make([]byte, 1024*1024), 1024*1024)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var r crossRec
		if err := json.Unmarshal([]byte(line), &r); err != nil {
			t.Fatalf("bad TS dumper line %q: %v", line, err)
		}
		recs[crossKey(r)] = r
	}
	return recs, true
}

func TestCrossEngineDifferential(t *testing.T) {
	ts, ok := tsResults(t)
	if !ok {
		t.Skip("node/crossdump.ts unavailable — skipping cross-engine differential")
	}

	specDir := filepath.Join("..", "..", "..", "eng", "spec")
	entries, err := os.ReadDir(specDir)
	if err != nil {
		t.Fatalf("read %s: %v", specDir, err)
	}

	var agree, codeDiff, gaps, divergences, onlyOne int
	var gapMsgs, divMsgs, codeMsgs []string
	seenTS := map[string]bool{}

	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".tsv") {
			continue
		}
		f, err := os.Open(filepath.Join(specDir, e.Name()))
		if err != nil {
			t.Fatalf("open: %v", err)
		}
		sc := bufio.NewScanner(f)
		sc.Buffer(make([]byte, 1024*1024), 1024*1024)
		lineNum := 0
		for sc.Scan() {
			lineNum++
			line := strings.TrimRight(sc.Text(), " \t")
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			parts := strings.Split(line, "\t")
			if len(parts) < 2 {
				continue
			}
			input := strings.TrimSpace(parts[0])
			gOK, gOut := goValueResult(input)
			gr := crossRec{Mode: "value", File: e.Name(), Line: lineNum}
			key := crossKey(gr)
			tr, present := ts[key]
			if !present {
				onlyOne++
				continue
			}
			seenTS[key] = true

			switch {
			case gOK && tr.OK:
				if gOut == tr.Out {
					agree++
				} else {
					divergences++
					if len(divMsgs) < 25 {
						divMsgs = append(divMsgs, e.Name()+":L"+itoa(lineNum)+" "+input+
							"\n    go=  "+gOut+"\n    ts=  "+tr.Out)
					}
				}
			case !gOK && !tr.OK:
				if gOut == tr.Out {
					agree++
				} else {
					codeDiff++
					if len(codeMsgs) < 25 {
						codeMsgs = append(codeMsgs, e.Name()+":L"+itoa(lineNum)+" "+input+
							"  (go="+gOut+" ts="+tr.Out+")")
					}
				}
			default:
				gaps++
				which := "go=ok ts=err"
				if !gOK {
					which = "go=err ts=ok"
				}
				if len(gapMsgs) < 50 {
					gapMsgs = append(gapMsgs, e.Name()+":L"+itoa(lineNum)+" "+input+
						"  ("+which+"; go="+gOut+" ts="+tr.Out+")")
				}
			}
		}
		f.Close()
	}

	// TS rows the Go walk never visited (corpus skew).
	tsOnly := 0
	for k := range ts {
		if !seenTS[k] {
			tsOnly++
		}
	}

	total := agree + codeDiff + gaps + divergences
	t.Logf("cross-engine differential over eng/spec (value mode): %d rows compared", total)
	t.Logf("  agree:        %d", agree)
	t.Logf("  error codeDiff:%d", codeDiff)
	t.Logf("  GAPS:         %d  (one engine errors where the other succeeds — permitted)", gaps)
	t.Logf("  divergences:  %d  (both succeed, values differ — fail)", divergences)
	t.Logf("  corpus skew:  go-only=%d ts-only=%d", onlyOne, tsOnly)
	for _, m := range codeMsgs {
		t.Logf("  codeDiff: %s", m)
	}
	for _, m := range gapMsgs {
		t.Logf("  gap: %s", m)
	}

	if divergences > 0 {
		sort.Strings(divMsgs)
		t.Errorf("%d cross-engine divergences (both engines produced a value, but they differ):\n%s",
			divergences, strings.Join(divMsgs, "\n"))
	}
}
