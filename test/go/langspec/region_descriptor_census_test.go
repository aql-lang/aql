// Region-descriptor census — has the Stage 4 descriptor model ever seen a
// real program? (design/FULL-COMPILATION.0.md §6.2, Stage 4.)
//
// WHY THIS EXISTS. The flag-day's central assumption is that a region
// descriptor can express the statements a real program is made of. Nothing
// tested it: RegionEnd, RegionSlotCount and CaptureRegionSlots have no
// production caller, so until this file they were exercised by their own unit
// tests alone — on windows those tests built. A model validated only against
// its author's examples is the shape of assumption this project converts into
// a number before betting a flag-day on it, exactly as the interp-entry census
// converted "0 islanded".
//
// WHAT IT MEASURES, and what it deliberately does not. It walks each corpus
// program's REAL token stream and partitions it into regions, checking the
// three properties that make an extent usable as a descriptor unit:
//
//	TOTAL      the walk reaches the end — every step advances, so no
//	           partition spins, and none runs past the window.
//	EXACT      every token lands in exactly one region or is the DELIMITER
//	           between two: the slot counts plus the delimiters sum to the
//	           stream length, so there is no gap a dispatch could fall into
//	           and no token claimed twice.
//	AGREED     CaptureRegionSlots and RegionEnd agree about where each
//	           region is — the captured slot count equals the extent.
//
// A region of ZERO width is not a failure when the walk is standing ON a
// hard delimiter: `def k 'b' end {a:1 b:2} get k` has `end` at index 3, and
// RegionEnd reports the region [0,3) then answers 3 again from 3, because a
// delimiter belongs to no region. The caller steps over it. Getting that
// wrong was this test's first result — 825 "zero width" failures that were
// the walk's bug, not the model's.
//
// TWO THINGS THIS TEST GOT WRONG FIRST, both worth keeping. Its opening
// measurement said 39.87%, and every one of those failures was the
// instrument. 825 were "zero width" regions that were the walk standing on a
// hard delimiter and not stepping over it. The other 4013 came from checking
// each captured slot against the tape with core.DeepEqual — which is NOT
// REFLEXIVE for an unevaluated ParenExprPayload, so `{a:1 b:2} get ('a')`
// compared unequal to itself. That is not a user-visible defect (`deq ('x')
// ('x')` is true, because the parens evaluate before deq sees them) and it is
// not what this test is for; the check was near-vacuous anyway, since
// CaptureRegionSlots reads the very indices it is compared against. Both are
// recorded because a measurement whose first answer is dramatic is a
// measurement to re-read before believing.
//
// It does NOT call RegionDesc.Validate, and that is not an omission.
// CaptureRegionSlots leaves every slot's Source at SlotNone on purpose (the
// operand model belongs to the lowerer, not the tape), so validating a raw
// capture would assert the sentinel it exists to preserve. Extent and written
// order are what the tape alone can establish, and they are what this
// measures.
//
// THE ANSWER, 2026-08-29: 7585 of 7585 parsed corpus programs, 100.00%. The
// extent half of the descriptor model holds against every real program the
// corpus has. So the flag-day's risk is NOT in the region extents; it sits in
// the collectHost adapter and the bind twins, which is where task #15 should
// spend its care. What remains unmeasured is the SOURCE half — the operand
// model the lowerer fills in — because nothing can measure that until a
// lowerer exists to be measured.
package langspec

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
	"testing"

	compiler "github.com/boru-lang/boru/compiler/go"
	core "github.com/boru-lang/boru/core/go"
	lang "github.com/boru-lang/boru/lang/go"
)

// regionWalkFailure names one way a program's region partition is unusable.
type regionWalkFailure struct {
	kind string
	row  string
}

// walkRegions partitions vals into regions and returns the failure kinds
// found. An empty result means the partition is total, exact and ordered.
func walkRegions(vals []core.Value) []string {
	if len(vals) == 0 {
		return nil
	}
	var bad []string
	w := core.NewTape(vals, 0)
	n := w.Len()
	covered, at, guard := 0, 0, 0
	for at < n {
		guard++
		if guard > n+1 {
			bad = append(bad, "walk did not terminate")
			break
		}
		end := core.RegionEnd(w, at)
		if end == at {
			// Standing on a hard delimiter: it belongs to no region, so step
			// over it and keep the partition total.
			if v := w.At(at); core.IsEnd(v) || core.IsCloseParen(v) {
				covered++
				at++
				continue
			}
			bad = append(bad, "zero-width region on a non-delimiter token")
			break
		}
		if end < at {
			bad = append(bad, "region end before its start")
			break
		}
		if end > n {
			bad = append(bad, "region past the window end")
			break
		}
		slots := compiler.CaptureRegionSlots(w, at)
		if len(slots) != end-at {
			bad = append(bad, "slot count disagrees with the extent")
			break
		}
		covered += end - at
		at = end
	}
	if len(bad) == 0 && covered != n {
		bad = append(bad, "partition does not cover the stream")
	}
	return bad
}

// TestRegionDescriptorCensus reports the share of corpus programs whose token
// stream partitions into well-formed regions, and the failure modes of the
// rest. It is a MEASUREMENT with a floor, not a pin: the floor only forbids
// the share going backwards.
func TestRegionDescriptorCensus(t *testing.T) {
	specDir := filepath.Join("..", "..", "..", "lang", "spec")
	entries, err := os.ReadDir(specDir)
	if err != nil {
		t.Fatal(err)
	}
	var rows, parsed, clean int
	kinds := map[string]int{}
	examples := map[string]regionWalkFailure{}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".tsv") {
			continue
		}
		f, err := os.Open(filepath.Join(specDir, e.Name()))
		if err != nil {
			t.Fatal(err)
		}
		sc := bufio.NewScanner(f)
		sc.Buffer(make([]byte, 1024*1024), 1024*1024)
		for sc.Scan() {
			line := sc.Text()
			if strings.TrimSpace(line) == "" || strings.HasPrefix(line, "#") {
				continue
			}
			parts := strings.Split(line, "\t")
			if len(parts) < 2 {
				continue
			}
			rows++
			src := strings.TrimSpace(parts[0])
			vals, err := lang.Parse(src)
			if err != nil {
				continue // a parse-error row is the parser's business, not the model's
			}
			parsed++
			bad := walkRegions(vals)
			if len(bad) == 0 {
				clean++
				continue
			}
			for _, k := range bad {
				kinds[k]++
				if _, seen := examples[k]; !seen {
					examples[k] = regionWalkFailure{kind: k, row: e.Name() + ": " + src}
				}
			}
		}
		f.Close()
	}

	share := 0.0
	if parsed > 0 {
		share = 100 * float64(clean) / float64(parsed)
	}
	t.Logf("region-descriptor census: %d corpus rows, %d parsed, %d partition cleanly (%.2f%%)",
		rows, parsed, clean, share)
	for _, k := range sortedKeys(kinds) {
		t.Logf("   failure %-52s %d   e.g. %s", k, kinds[k], examples[k].row)
	}

	if clean < parsed {
		t.Errorf("region-descriptor census: %d of %d parsed programs do not partition into "+
			"well-formed regions. The Stage 4 flag-day rests on descriptors expressing real "+
			"statements; a failure here is the descriptor model, and it wants fixing BEFORE "+
			"OpCollect rather than after", parsed-clean, parsed)
	}
}
