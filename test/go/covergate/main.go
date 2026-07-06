// Command covergate merges Go cover profiles produced with
// -coverpkg=github.com/aql-lang/aql/... across every module's test run
// and enforces the repo's coverage floor (ADR-008: 100% at all times).
//
// Each input profile may cover an overlapping statement set (lang's
// tests instrument eng, cmd's instrument both, …); covergate dedupes
// by source block, taking the maximum hit count, so a statement counts
// as covered when ANY suite reaches it. It prints a per-module table
// and fails (exit 1) when total coverage is below the threshold.
//
// Usage (normally via `make cover-gate`):
//
//	covergate -threshold 100 coverage/*.xout
package main

import (
	"bufio"
	"bytes"
	"flag"
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"
	"strings"
)

// osExit is a test seam (design/TEST-SEAMS.10.md); tests swap it to
// observe exit codes without killing the process.
var osExit = os.Exit

const modulePrefix = "github.com/aql-lang/aql/"

// block is one profile entry's statement weight and best-seen hit count.
type block struct {
	stmts int
	count int
}

// mergeProfiles reads cover profiles and dedupes blocks by their
// file:range key, keeping the maximum count. Malformed lines are
// reported as errors — a truncated profile must fail the gate loudly,
// not shrink the denominator.
func mergeProfiles(paths []string) (map[string]block, error) {
	blocks := map[string]block{}
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("covergate: %w", err)
		}
		if err := mergeReader(bytes.NewReader(data), blocks); err != nil {
			return nil, fmt.Errorf("covergate: %s: %w", path, err)
		}
	}
	return blocks, nil
}

// mergeReader merges one profile stream into blocks.
func mergeReader(r io.Reader, blocks map[string]block) error {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		line := sc.Text()
		if line == "" || strings.HasPrefix(line, "mode:") {
			continue
		}
		// file.go:sl.sc,el.ec numStmts count
		lastSp := strings.LastIndexByte(line, ' ')
		prevSp := -1
		if lastSp > 0 {
			prevSp = strings.LastIndexByte(line[:lastSp], ' ')
		}
		if lastSp < 0 || prevSp < 0 {
			return fmt.Errorf("malformed profile line %q", line)
		}
		stmts, err := strconv.Atoi(line[prevSp+1 : lastSp])
		if err != nil {
			return fmt.Errorf("malformed statement count in %q", line)
		}
		count, err := strconv.Atoi(line[lastSp+1:])
		if err != nil {
			return fmt.Errorf("malformed hit count in %q", line)
		}
		key := line[:prevSp]
		b := blocks[key]
		b.stmts = stmts
		if count > b.count {
			b.count = count
		}
		blocks[key] = b
	}
	return sc.Err()
}

// moduleOf maps an instrumented file path to its reporting bucket
// (the repo module, e.g. "eng/go").
func moduleOf(file string) string {
	rest, ok := strings.CutPrefix(file, modulePrefix)
	if !ok {
		return "other"
	}
	parts := strings.SplitN(rest, "/", 3)
	if len(parts) >= 2 {
		return parts[0] + "/" + parts[1]
	}
	return parts[0]
}

// tally aggregates merged blocks into per-module and total counters.
type tally struct{ covered, total int }

func tallyModules(blocks map[string]block) (map[string]tally, tally) {
	perModule := map[string]tally{}
	var all tally
	for key, b := range blocks {
		file := key[:strings.LastIndexByte(key, ':')]
		m := moduleOf(file)
		t := perModule[m]
		t.total += b.stmts
		all.total += b.stmts
		if b.count > 0 {
			t.covered += b.stmts
			all.covered += b.stmts
		}
		perModule[m] = t
	}
	return perModule, all
}

func pct(t tally) float64 {
	if t.total == 0 {
		return 100
	}
	return 100 * float64(t.covered) / float64(t.total)
}

// run executes the gate and returns the process exit code.
func run(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("covergate", flag.ContinueOnError)
	fs.SetOutput(stderr)
	threshold := fs.Float64("threshold", 100, "minimum total coverage percentage")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() == 0 {
		fmt.Fprintln(stderr, "covergate: no profiles given (usage: covergate [-threshold N] profile...)")
		return 2
	}
	blocks, err := mergeProfiles(fs.Args())
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 2
	}
	perModule, all := tallyModules(blocks)

	names := make([]string, 0, len(perModule))
	for m := range perModule {
		names = append(names, m)
	}
	sort.Strings(names)
	for _, m := range names {
		t := perModule[m]
		fmt.Fprintf(stdout, "%-20s %6.1f%%  (%d/%d stmts)\n", m, pct(t), t.covered, t.total)
	}
	fmt.Fprintf(stdout, "%-20s %6.1f%%  (%d/%d stmts)\n", "TOTAL", pct(all), all.covered, all.total)

	if pct(all) < *threshold {
		fmt.Fprintf(stderr, "covergate: FAIL — total %.1f%% is below the %.1f%% floor (%d statements uncovered)\n",
			pct(all), *threshold, all.total-all.covered)
		return 1
	}
	fmt.Fprintf(stdout, "covergate: PASS (floor %.1f%%)\n", *threshold)
	return 0
}

func main() {
	osExit(run(os.Args[1:], os.Stdout, os.Stderr))
}
