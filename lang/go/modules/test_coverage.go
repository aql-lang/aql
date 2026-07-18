package modules

import (
	"sort"
	"sync"

	eng "github.com/aql-lang/aql/eng/go"
	"github.com/aql-lang/aql/lang/go/native"
)

// aql:test line-coverage feature. `Test.cover [body]` runs a test body with the
// engine coverage hook (eng/go/coverage.go) armed, accumulating the source rows
// the module-under-test executes; `Test.coverage <id>` reports the percentage
// against the module's DENOMINATOR — every executable row of its registered
// source (a module loader calls RegisterCoverSource + SetCoverID; aql:sift
// does). Coverage is an INTERPRETER measurement: run the suite no-compile so the
// module's fn bodies tree-walk through the single hook (compiled == interpreted
// by the differential gates, so the executed rows are the same).

const capTestCover = "Test.cover.active"

// testCover accumulates covered rows per cover id across a coverage run.
type testCover struct {
	mu   sync.Mutex
	rows map[string]map[int]bool
}

func (c *testCover) record(id string, row int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.rows == nil {
		c.rows = map[string]map[int]bool{}
	}
	m := c.rows[id]
	if m == nil {
		m = map[int]bool{}
		c.rows[id] = m
	}
	m[row] = true
}

// coveredRows returns a COPY of the rows recorded for id (empty when none).
func (c *testCover) coveredRows(id string) map[int]bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := map[int]bool{}
	for row := range c.rows[id] {
		out[row] = true
	}
	return out
}

// activeCover returns the testCover on the parent registry, lazily creating it.
func activeCover(parent *native.Registry) *testCover {
	if c, ok, _ := eng.Cap[*testCover](parent, capTestCover); ok && c != nil {
		return c
	}
	c := &testCover{}
	_ = parent.Capabilities.Set(capTestCover, c)
	return c
}

// coverDenominator parses src and returns the set of EXECUTABLE rows: every
// token's row, with fn param/return signature declarations excluded (they are
// never stepped, so they are not coverable). Returns nil on a parse failure.
func coverDenominator(parent *native.Registry, src string) map[int]bool {
	if parent.ParseFunc == nil {
		return nil
	}
	toks, err := parent.ParseFunc(src)
	if err != nil {
		return nil
	}
	rows := map[int]bool{}
	coverWalkSeq(toks, rows)
	return rows
}

// coverWalkSeq collects the row of every executable token in seq. A `fn`/`afn`
// word's following list is a signature: its param/return sub-lists are skipped
// and only the bodies are walked (recursively — nested fns included).
func coverWalkSeq(seq []native.Value, rows map[int]bool) {
	for i := 0; i < len(seq); i++ {
		v := seq[i]
		if native.IsWord(v) {
			if r := v.Pos().Row; r != 0 {
				rows[r] = true
			}
			w, _ := native.AsWord(v)
			if (w.Name == "fn" || w.Name == "afn") && i+1 < len(seq) {
				if lst, err := native.AsList(seq[i+1]); err == nil && !lst.IsNil() {
					coverWalkFnSig(lst.Slice(), rows)
					i++
				}
			}
			continue
		}
		coverWalkValue(v, rows)
	}
}

// coverWalkFnSig walks a fn signature [[params][returns][body] ...]: only the
// body (index 2 mod 3) is executable.
func coverWalkFnSig(sig []native.Value, rows map[int]bool) {
	for i := 0; i < len(sig); i++ {
		if i%3 != 2 {
			continue
		}
		if body, err := native.AsList(sig[i]); err == nil && !body.IsNil() {
			coverWalkSeq(body.Slice(), rows)
		}
	}
}

func coverWalkValue(v native.Value, rows map[int]bool) {
	if native.IsParenExpr(v) {
		toks, _ := native.AsParenExpr(v)
		coverWalkSeq(toks, rows)
		return
	}
	if lst, err := native.AsList(v); err == nil && !lst.IsNil() {
		coverWalkSeq(lst.Slice(), rows)
		return
	}
	if m, err := native.AsMap(v); err == nil && native.IsConcrete(v) {
		for _, k := range m.Keys() {
			val, _ := m.Get(k)
			coverWalkValue(val, rows)
		}
		return
	}
	if r := v.Pos().Row; r != 0 {
		rows[r] = true
	}
}

// coverNatives builds the Go-implemented coverage words. `parent` is the
// importing registry, whose shared hook holder every module sub-registry
// inherits — so arming it captures the module-under-test's executed rows.
func coverNatives(parent *native.Registry) []native.NativeFunc {
	return []native.NativeFunc{
		{
			// Test.cover [body] — run body with the coverage hook armed. Coverage
			// is a pure INTERPRETER measurement (the body tree-walks here), so the
			// body rides as a raw NoEvalArgs list and needs no closure/compile
			// shape — a compiled caller falls back to the interpreter, which is the
			// only mode that fires the coverage hook anyway.
			Name: "test-cover",
			Signatures: []native.Signature{{
				Args:       []*native.Type{native.TList},
				NoEvalArgs: map[int]bool{0: true},
				Returns:    []*native.Type{}, BarrierPos: -1,
				Impl: native.Go(func(args []native.Value, _ map[string]native.Value, _ []native.Value, r *native.Registry) ([]native.Value, error) {
					body, err := native.RequireConcreteList(args[0], "Test.cover")
					if err != nil {
						return nil, err
					}
					cover := activeCover(parent)
					disarm := parent.ArmCoverageHook(cover.record)
					defer disarm()
					_, e := native.New(r).Run(body.Slice())
					return nil, e
				}),
			}},
		},
		{
			// Test.coverage <id> — report {covered,total,percent,uncovered} for the
			// registered source id against the rows a prior Test.cover recorded.
			Name: "test-coverage",
			Signatures: []native.Signature{{
				Args:       []*native.Type{native.TString},
				Returns:    []*native.Type{native.TMap}, BarrierPos: -1,
				Impl: native.Go(func(args []native.Value, _ map[string]native.Value, _ []native.Value, r *native.Registry) ([]native.Value, error) {
					id, err := args[0].AsConcreteString()
					if err != nil {
						return nil, err
					}
					src, ok := parent.CoverSource(id)
					if !ok {
						return nil, r.AqlError("test_cover_no_source", "no coverage source registered for id: "+id, "test-coverage")
					}
					denom := coverDenominator(parent, src)
					if denom == nil {
						return nil, r.AqlError("test_cover_bad_source", "coverage source failed to parse for id: "+id, "test-coverage")
					}
					covered := activeCover(parent).coveredRows(id)
					var uncovered []int
					hit := 0
					for row := range denom {
						if covered[row] {
							hit++
						} else {
							uncovered = append(uncovered, row)
						}
					}
					sort.Ints(uncovered)
					total := len(denom)
					pct := 100.0
					if total > 0 {
						pct = float64(hit) / float64(total) * 100.0
					}
					uncov := make([]native.Value, len(uncovered))
					for i, row := range uncovered {
						uncov[i] = native.NewInteger(int64(row))
					}
					out := native.NewOrderedMap()
					out.Set("covered", native.NewInteger(int64(hit)))
					out.Set("total", native.NewInteger(int64(total)))
					out.Set("percent", native.NewFloat(pct))
					out.Set("uncovered", native.NewList(uncov))
					return []native.Value{native.NewMap(out)}, nil
				}),
			}},
		},
	}
}
