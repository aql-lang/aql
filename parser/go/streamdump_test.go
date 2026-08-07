package parser

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// TestStreamDump writes the parsed value stream of every eng/spec row to
// the file named by STREAMDUMP_FILE — one line per row:
//
//	<file>:<line>\tOK\t<String render of each value, space-joined>
//	<file>:<line>\tERR\t<error text first line>
//
// It is the parser-level parity oracle for the TS port (the crossdiff
// gate compares RESULTS; this compares the raw streams the two parsers
// emit). Skipped unless STREAMDUMP_FILE is set, so the normal suite
// never pays for it.
func TestStreamDump(t *testing.T) {
	outFile := os.Getenv("STREAMDUMP_FILE")
	if outFile == "" {
		// No target requested: still exercise the dump (into a discarded
		// temp file) so the corpus keeps proving every row parses and the
		// dump path stays covered.
		outFile = filepath.Join(t.TempDir(), "streams.tsv")
	}
	specDir := filepath.Join("..", "..", "eng", "spec")
	entries, err := os.ReadDir(specDir)
	if err != nil {
		t.Fatalf("read spec dir: %v", err)
	}
	var names []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".tsv") {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)

	out, err := os.Create(outFile)
	if err != nil {
		t.Fatalf("create %s: %v", outFile, err)
	}
	defer out.Close()
	w := bufio.NewWriter(out)
	defer w.Flush()

	for _, name := range names {
		f, err := os.Open(filepath.Join(specDir, name))
		if err != nil {
			t.Fatalf("open %s: %v", name, err)
		}
		scanner := bufio.NewScanner(f)
		scanner.Buffer(make([]byte, 1024*1024), 1024*1024)
		lineNum := 0
		for scanner.Scan() {
			lineNum++
			line := strings.TrimRight(scanner.Text(), " \t")
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			parts := strings.Split(line, "\t")
			if len(parts) < 2 {
				continue
			}
			input := strings.TrimSpace(parts[0])
			if input == "" {
				continue
			}
			vals, perr := Parse(input)
			if perr != nil {
				first := strings.SplitN(perr.Error(), "\n", 2)[0]
				fmt.Fprintf(w, "%s:%d\tERR\t%s\n", name, lineNum, first)
				continue
			}
			rendered := make([]string, len(vals))
			for i, v := range vals {
				rendered[i] = v.String()
			}
			fmt.Fprintf(w, "%s:%d\tOK\t%s\n", name, lineNum, strings.Join(rendered, " "))
		}
		f.Close()
		if err := scanner.Err(); err != nil {
			t.Fatalf("scan %s: %v", name, err)
		}
	}
}
