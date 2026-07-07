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

// exclusions reports how the allowlist interacted with the merged blocks:
// how many statements it removed from the denominator, and the block keys
// that the allowlist named but that are either already covered (the
// exclusion is now unnecessary — the guard became reachable) or absent
// from every profile (a stale entry). Both are surfaced so the allowlist
// cannot silently rot.
type exclusions struct {
	stmts      int      // statements excluded from the denominator
	nowCovered []string // allowlisted keys whose block is actually covered
	stale      []string // allowlisted keys not present in any profile
}

// tallyModules aggregates merged blocks into per-module and total
// counters, skipping any block whose key is in allow (excluded from both
// numerator and denominator). It also computes allowlist bookkeeping.
func tallyModules(blocks map[string]block, allow map[string]struct{}) (map[string]tally, tally, exclusions) {
	perModule := map[string]tally{}
	var all tally
	var ex exclusions
	seen := map[string]bool{}
	for key, b := range blocks {
		if _, ok := allow[key]; ok {
			seen[key] = true
			ex.stmts += b.stmts
			if b.count > 0 {
				ex.nowCovered = append(ex.nowCovered, key)
			}
			continue
		}
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
	for key := range allow {
		if !seen[key] {
			ex.stale = append(ex.stale, key)
		}
	}
	sort.Strings(ex.nowCovered)
	sort.Strings(ex.stale)
	return perModule, all, ex
}

// loadAllowlist reads a covergate allowlist: one profile block key
// (file:sl.sc,el.ec) per line, optionally followed by whitespace and a
// human-readable reason (ignored here — the reason documents the proof
// for reviewers). Blank lines and #-comments are skipped.
func loadAllowlist(path string) (map[string]struct{}, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("covergate: allowlist: %w", err)
	}
	allow := map[string]struct{}{}
	sc := bufio.NewScanner(bytes.NewReader(data))
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key := line
		if i := strings.IndexAny(line, " \t"); i >= 0 {
			key = line[:i]
		}
		allow[key] = struct{}{}
	}
	return allow, sc.Err()
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
	allowPath := fs.String("allow", "", "path to an allowlist of provably-unreachable block keys to exclude")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() == 0 {
		fmt.Fprintln(stderr, "covergate: no profiles given (usage: covergate [-threshold N] [-allow FILE] profile...)")
		return 2
	}
	allow := map[string]struct{}{}
	if *allowPath != "" {
		loaded, err := loadAllowlist(*allowPath)
		if err != nil {
			fmt.Fprintln(stderr, err)
			return 2
		}
		allow = loaded
	}
	blocks, err := mergeProfiles(fs.Args())
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 2
	}
	perModule, all, ex := tallyModules(blocks, allow)

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
	if ex.stmts > 0 || len(allow) > 0 {
		fmt.Fprintf(stdout, "allowlisted (excluded): %d statements across %d blocks\n", ex.stmts, len(allow)-len(ex.stale))
	}

	// A stale or now-covered allowlist entry is a soft failure: it must be
	// removed (the block is gone) or graduated (the guard is now reachable,
	// so cover it and drop the exclusion). Keeping the allowlist honest is
	// the whole point of the mechanism.
	for _, k := range ex.nowCovered {
		fmt.Fprintf(stderr, "covergate: allowlisted block is now covered — remove it from the allowlist: %s\n", k)
	}
	for _, k := range ex.stale {
		fmt.Fprintf(stderr, "covergate: allowlist entry matches no profiled block (stale) — remove it: %s\n", k)
	}
	if len(ex.nowCovered) > 0 || len(ex.stale) > 0 {
		return 1
	}

	if pct(all) < *threshold {
		fmt.Fprintf(stderr, "covergate: FAIL — total %.1f%% is below the %.1f%% floor (%d statements uncovered)\n",
			pct(all), *threshold, all.total-all.covered)
		return 1
	}
	fmt.Fprintf(stdout, "covergate: PASS (floor %.1f%%, %d statements allowlisted)\n", *threshold, ex.stmts)
	return 0
}

func main() {
	osExit(run(os.Args[1:], os.Stdout, os.Stderr))
}
