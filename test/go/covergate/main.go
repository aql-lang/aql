// Command covergate merges Go cover profiles produced with
// -coverpkg=github.com/boru-lang/boru/... across every module's test run
// and enforces the repo's coverage floor (ADR-008: 100% at all times).
//
// Each input profile may cover an overlapping statement set (lang's
// tests instrument eng, cmd's instrument both, …); covergate dedupes
// by source block, taking the maximum hit count, so a statement counts
// as covered when ANY suite reaches it. It prints a per-module table
// and fails (exit 1) when total coverage is below the threshold.
//
// Provably-unreachable defensive guards are excluded from the floor by an
// inline `//covergate:allow <reason>` comment on the guard's OPENING line —
// the exclusion travels WITH the code, so inserting or deleting lines above
// a guard never invalidates it (the historical line-keyed allowlist.tsv went
// stale on every refactor above a guard). A reason is mandatory: it is the
// proof the block is unreachable. See design/COVERAGE-ALLOWLIST.10.md.
//
// Usage (normally via `make cover-gate`):
//
//	covergate -threshold 100 -root . coverage/*.xout
package main

import (
	"bufio"
	"bytes"
	"flag"
	"fmt"
	"go/parser"
	"go/token"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// osExit is a test seam (design/TEST-SEAMS.10.md); tests swap it to
// observe exit codes without killing the process.
var osExit = os.Exit

const modulePrefix = "github.com/boru-lang/boru/"

// pragmaMarker is the inline comment that excludes the coverage block(s)
// opening on its line from the floor. The text after it is the REQUIRED
// reason — the proof the guard is unreachable; covergate rejects a bare
// marker.
const pragmaMarker = "//covergate:allow"

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

// pragmaKey identifies a source line by instrumented file path and 1-based
// line number — the granularity at which a `//covergate:allow` comment
// excludes the coverage block(s) that OPEN on that line.
type pragmaKey struct {
	file string
	line int
}

// blockStart maps a profile block key (file:sl.sc,el.ec) to the pragmaKey of
// its opening line. Go's cover tool anchors a block at the line where it
// begins, which is where the `//covergate:allow` comment sits.
func blockStart(key string) pragmaKey {
	i := strings.LastIndexByte(key, ':')
	rng := key[i+1:] // sl.sc,el.ec
	line, _ := strconv.Atoi(rng[:strings.IndexByte(rng, '.')])
	return pragmaKey{key[:i], line}
}

// scanPragmas parses every source file referenced by the merged blocks and
// records the lines whose trailing comment IS a `//covergate:allow` marker,
// mapped to the reason that follows. Detection is comment-token exact (via
// go/parser), not a substring scan — so covergate's own documentation of the
// marker, and the marker's appearance inside string literals, are correctly
// ignored. A marker with no reason is a hard error: an exclusion must carry
// its proof. Out-of-repo files (bucketed "other") hold no pragmas and are
// skipped.
func scanPragmas(root string, blocks map[string]block) (map[pragmaKey]string, error) {
	files := map[string]struct{}{}
	for key := range blocks {
		files[key[:strings.LastIndexByte(key, ':')]] = struct{}{}
	}
	out := map[pragmaKey]string{}
	for f := range files {
		rel, ok := strings.CutPrefix(f, modulePrefix)
		if !ok {
			continue
		}
		fset := token.NewFileSet()
		af, err := parser.ParseFile(fset, filepath.Join(root, filepath.FromSlash(rel)), nil, parser.ParseComments)
		if err != nil {
			return nil, fmt.Errorf("covergate: pragma scan: %w", err)
		}
		for _, cg := range af.Comments {
			for _, c := range cg.List {
				if !strings.HasPrefix(c.Text, pragmaMarker) {
					continue
				}
				line := fset.Position(c.Slash).Line
				reason := strings.TrimSpace(c.Text[len(pragmaMarker):])
				if reason == "" {
					return nil, fmt.Errorf("covergate: %s:%d: %s needs a reason", f, line, pragmaMarker)
				}
				out[pragmaKey{f, line}] = reason
			}
		}
	}
	return out, nil
}

// tally aggregates merged blocks into per-module and total counters.
type tally struct{ covered, total int }

// exclusions reports how the pragmas interacted with the merged blocks: how
// many statements (across how many blocks) they removed from the denominator,
// and the pragma LINES that are either already fully covered (every block on
// the line is covered — the guard became reachable, so remove the pragma) or
// that no profiled block opens on (stale). Both are surfaced so the
// exclusions cannot silently rot.
type exclusions struct {
	stmts      int      // statements excluded from the denominator
	blocks     int      // number of profiled blocks excluded
	nowCovered []string // pragma lines whose every block is covered
	stale      []string // pragma lines that open no profiled block
}

// tallyModules aggregates merged blocks into per-module and total counters. A
// `//covergate:allow` pragma on a block's opening line excludes that block
// from both numerator and denominator ONLY IF the block is UNCOVERED — a
// covered block sharing the line (e.g. the reaching block of an `if guard {`)
// is counted normally, so line granularity never over-excludes real coverage.
// A pragma line that excluded no uncovered block is reported: fully covered
// (graduate it) if some block opens there, stale if none does.
func tallyModules(blocks map[string]block, pragmas map[pragmaKey]string) (map[string]tally, tally, exclusions) {
	perModule := map[string]tally{}
	var all tally
	var ex exclusions
	guarded := map[pragmaKey]bool{} // pragma excluded >=1 uncovered block
	onLine := map[pragmaKey]bool{}  // pragma line has >=1 profiled block
	for key, b := range blocks {
		pk := blockStart(key)
		if _, ok := pragmas[pk]; ok {
			onLine[pk] = true
			if b.count == 0 {
				guarded[pk] = true
				ex.stmts += b.stmts
				ex.blocks++
				continue
			}
			// A covered block on a pragma line falls through and is tallied.
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
	for pk := range pragmas {
		if guarded[pk] {
			continue
		}
		loc := fmt.Sprintf("%s:%d", pk.file, pk.line)
		if onLine[pk] {
			ex.nowCovered = append(ex.nowCovered, loc)
		} else {
			ex.stale = append(ex.stale, loc)
		}
	}
	sort.Strings(ex.nowCovered)
	sort.Strings(ex.stale)
	return perModule, all, ex
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
	root := fs.String("root", ".", "repository root for resolving //covergate:allow pragmas in source")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() == 0 {
		fmt.Fprintln(stderr, "covergate: no profiles given (usage: covergate [-threshold N] [-root DIR] profile...)")
		return 2
	}
	blocks, err := mergeProfiles(fs.Args())
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 2
	}
	pragmas, err := scanPragmas(*root, blocks)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 2
	}
	perModule, all, ex := tallyModules(blocks, pragmas)

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
	if len(pragmas) > 0 {
		fmt.Fprintf(stdout, "allowlisted (excluded): %d statements across %d blocks\n", ex.stmts, ex.blocks)
	}

	// A stale or now-covered pragma is a soft failure: it must be removed
	// (the guard is gone or moved off the line) or graduated (the guard is
	// now reachable, so cover it and drop the pragma). Keeping the
	// exclusions honest is the whole point of the mechanism.
	for _, k := range ex.nowCovered {
		fmt.Fprintf(stderr, "covergate: guard marked %s is now covered — remove the pragma: %s\n", pragmaMarker, k)
	}
	for _, k := range ex.stale {
		fmt.Fprintf(stderr, "covergate: %s on a line that opens no profiled block (stale) — remove it: %s\n", pragmaMarker, k)
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
