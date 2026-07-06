package patrun

import (
	"strings"
	"testing"
)

// Coverage-focused unit tests for the vendored patrun trie (patrun.go) and
// the stdlib interval matcher (interval.go). The package previously had no
// tests at all; the API contract is exercised both positively (what must
// match) and negatively (what must NOT match), per repo discipline.

func covStr(s string) *string { return &s }

func covFind(t *testing.T, p *Patrun, pat map[string]string) (string, bool) {
	t.Helper()
	d, ok := p.Find(pat)
	if !ok {
		return "", false
	}
	return *d, true
}

// ---- patrun.go: Add / Find / FindExact ------------------------------------

func TestPatrunCovAddFindBasics(t *testing.T) {
	p := New()
	if p.Top() == nil {
		t.Fatal("Top() must not be nil")
	}

	// Empty matcher: nothing matches, nil pattern short-circuits.
	if _, ok := p.Find(map[string]string{"a": "1"}); ok {
		t.Error("empty patrun must not match")
	}
	if d, ok := p.Find(nil); ok || d != nil {
		t.Error("Find(nil) must return nil,false")
	}

	// Root catch-all (empty pattern).
	p.Add(map[string]string{}, covStr("ROOT"))
	if d, ok := covFind(t, p, map[string]string{}); !ok || d != "ROOT" {
		t.Errorf("root pattern = %q,%v; want ROOT", d, ok)
	}
	if d, ok := covFind(t, p, map[string]string{"z": "9"}); !ok || d != "ROOT" {
		t.Errorf("unmatched query must fall to root, got %q,%v", d, ok)
	}

	p.Add(map[string]string{"a": "1"}, covStr("A1"))
	p.Add(map[string]string{"a": "1", "b": "2"}, covStr("A1B2"))

	if d, _ := covFind(t, p, map[string]string{"a": "1"}); d != "A1" {
		t.Errorf("a=1 -> %q, want A1", d)
	}
	// Most specific wins.
	if d, _ := covFind(t, p, map[string]string{"a": "1", "b": "2"}); d != "A1B2" {
		t.Errorf("a=1,b=2 -> %q, want A1B2", d)
	}
	// Extra query keys are ignored.
	if d, _ := covFind(t, p, map[string]string{"a": "1", "b": "2", "c": "x"}); d != "A1B2" {
		t.Errorf("extra keys -> %q, want A1B2", d)
	}
	// A sibling value falls back to the wider match.
	if d, _ := covFind(t, p, map[string]string{"a": "1", "b": "3"}); d != "A1" {
		t.Errorf("a=1,b=3 -> %q, want A1", d)
	}
	// Wrong value falls to root.
	if d, _ := covFind(t, p, map[string]string{"a": "2"}); d != "ROOT" {
		t.Errorf("a=2 -> %q, want ROOT", d)
	}

	// Re-adding a pattern overwrites its data.
	p.Add(map[string]string{"a": "1"}, covStr("A1v2"))
	if d, _ := covFind(t, p, map[string]string{"a": "1"}); d != "A1v2" {
		t.Errorf("re-add a=1 -> %q, want A1v2", d)
	}
}

func TestPatrunCovFindExact(t *testing.T) {
	p := New()
	p.Add(map[string]string{"a": "1"}, covStr("A1"))
	p.Add(map[string]string{"a": "1", "b": "2"}, covStr("A1B2"))

	if d, ok := p.FindExact(map[string]string{"a": "1", "b": "2"}); !ok || *d != "A1B2" {
		t.Errorf("FindExact full = %v,%v; want A1B2", d, ok)
	}
	if d, ok := p.FindExact(map[string]string{"a": "1"}); !ok || *d != "A1" {
		t.Errorf("FindExact a=1 = %v,%v; want A1", d, ok)
	}
	// A key the trie never saw must NOT exact-match.
	if _, ok := p.FindExact(map[string]string{"a": "1", "c": "9"}); ok {
		t.Error("FindExact with unknown key must fail")
	}
	// A wrong value on a known key must NOT exact-match.
	if _, ok := p.FindExact(map[string]string{"a": "1", "b": "3"}); ok {
		t.Error("FindExact with wrong value must fail")
	}

	// An intermediate node without data must not exact-match.
	p2 := New()
	p2.Add(map[string]string{"a": "1", "b": "2"}, covStr("DEEP"))
	if _, ok := p2.FindExact(map[string]string{"a": "1"}); ok {
		t.Error("FindExact must not return data from a deeper pattern")
	}
}

func TestPatrunCovCollect(t *testing.T) {
	p := New()
	p.Add(map[string]string{}, covStr("ROOT"))
	p.Add(map[string]string{"a": "1"}, covStr("A1"))
	p.Add(map[string]string{"a": "1", "b": "2"}, covStr("A1B2"))
	p.Add(map[string]string{"b": "2"}, covStr("B2"))

	got := p.Collect(map[string]string{"a": "1", "b": "2"})
	want := []string{"ROOT", "A1", "A1B2", "B2"}
	if len(got) != len(want) {
		t.Fatalf("Collect = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("Collect[%d] = %q, want %q (all: %v)", i, got[i], want[i], got)
		}
	}

	// An unmatched query collects only the root.
	got = p.Collect(map[string]string{"z": "9"})
	if len(got) != 1 || got[0] != "ROOT" {
		t.Errorf("Collect(z=9) = %v, want [ROOT]", got)
	}
}

// Insertion in reverse key order exercises the insert-before branch, and a
// third key exercises the star-path append.
func TestPatrunCovKeyOrderInsertion(t *testing.T) {
	p := New()
	p.Add(map[string]string{"b": "1"}, covStr("B1"))
	p.Add(map[string]string{"a": "2"}, covStr("A2")) // a < b: insert before
	p.Add(map[string]string{"c": "3"}, covStr("C3")) // c > b: star descent
	p.Add(map[string]string{"a0": "9"}, covStr("A09"))

	for _, tc := range []struct {
		pat  map[string]string
		want string
	}{
		{map[string]string{"b": "1"}, "B1"},
		{map[string]string{"a": "2"}, "A2"},
		{map[string]string{"c": "3"}, "C3"},
		{map[string]string{"a0": "9"}, "A09"},
	} {
		if d, ok := covFind(t, p, tc.pat); !ok || d != tc.want {
			t.Errorf("Find(%v) = %q,%v; want %q", tc.pat, d, ok, tc.want)
		}
	}
	// No catch-all: a miss is a miss.
	if _, ok := p.Find(map[string]string{"b": "9"}); ok {
		t.Error("b=9 must not match")
	}

	// Multi-key across the reordered trie.
	p.Add(map[string]string{"a": "2", "c": "3"}, covStr("A2C3"))
	if d, _ := covFind(t, p, map[string]string{"a": "2", "c": "3"}); d != "A2C3" {
		t.Errorf("a=2,c=3 -> %q, want A2C3", d)
	}
}

func TestPatrunCovRemove(t *testing.T) {
	p := New()
	p.Add(map[string]string{}, covStr("ROOT"))
	p.Add(map[string]string{"a": "1"}, covStr("A1"))
	p.Add(map[string]string{"a": "1", "b": "2"}, covStr("A1B2"))

	p.Remove(map[string]string{"a": "1", "b": "2"})
	if d, _ := covFind(t, p, map[string]string{"a": "1", "b": "2"}); d != "A1" {
		t.Errorf("after remove, a=1,b=2 -> %q, want A1 (wider)", d)
	}

	p.Remove(map[string]string{"a": "1"})
	if d, _ := covFind(t, p, map[string]string{"a": "1"}); d != "ROOT" {
		t.Errorf("after remove, a=1 -> %q, want ROOT", d)
	}

	// Removing a never-added pattern must not panic or disturb others.
	p.Remove(map[string]string{"zz": "99"})
	p.Remove(map[string]string{})
	if d, _ := covFind(t, p, map[string]string{}); d != "ROOT" {
		t.Errorf("root survived remove of unknown patterns; got %q", d)
	}
}

func TestPatrunCovList(t *testing.T) {
	p := New()
	p.Add(map[string]string{}, covStr("ROOT"))
	p.Add(map[string]string{"a": "1"}, covStr("A1"))
	p.Add(map[string]string{"a": "1", "b": "2"}, covStr("A1B2"))
	p.Add(map[string]string{"b": "2"}, covStr("B2"))

	all := p.List(nil)
	if len(all) != 4 {
		t.Fatalf("List(nil) = %d entries, want 4: %v", len(all), all)
	}
	if all[0].Data != "ROOT" || len(all[0].Match) != 0 {
		t.Errorf("first entry must be the root, got %v", all[0])
	}

	// Filtered list: only patterns carrying a=1 (plus the root entry).
	filtered := p.List(map[string]string{"a": "1"})
	var datas []string
	for _, e := range filtered {
		datas = append(datas, e.Data)
	}
	joined := strings.Join(datas, ",")
	if !strings.Contains(joined, "A1") || !strings.Contains(joined, "A1B2") {
		t.Errorf("List(a=1) missing a=1 entries: %v", datas)
	}
	for _, d := range datas {
		if d == "B2" {
			t.Errorf("List(a=1) must not contain the b-only pattern: %v", datas)
		}
	}

	// A filter matching nothing yields only the root.
	none := p.List(map[string]string{"a": "no-such"})
	if len(none) != 1 || none[0].Data != "ROOT" {
		t.Errorf("List(a=no-such) = %v, want only root", none)
	}

	// Match maps carry the pattern keys.
	deep := p.List(map[string]string{"b": "2"})
	found := false
	for _, e := range deep {
		if e.Data == "A1B2" && e.Match["a"] == "1" && e.Match["b"] == "2" {
			found = true
		}
	}
	if !found {
		t.Errorf("List(b=2) must include A1B2 with full match map: %v", deep)
	}
}

func TestPatrunCovStringAndToJSON(t *testing.T) {
	p := New()
	p.Add(map[string]string{"a": "1"}, covStr("A1"))
	p.Add(map[string]string{"b": "2"}, covStr("B2"))
	s := p.String()
	if !strings.Contains(s, "a=1 -> <A1>") {
		t.Errorf("String() missing a=1 entry: %q", s)
	}
	if !strings.Contains(s, "b=2 -> <B2>") {
		t.Errorf("String() missing star-path b=2 entry: %q", s)
	}
	if js := p.ToJSON(); js == "" {
		t.Error("ToJSON() must return a non-empty string")
	}
}

// ---- interval matcher wired through the trie -------------------------------

func TestPatrunCovIntervalMatching(t *testing.T) {
	p := New(Options{Interval: true})
	p.Add(map[string]string{"n": ">10"}, covStr("GT10"))

	if d, ok := covFind(t, p, map[string]string{"n": "15"}); !ok || d != "GT10" {
		t.Errorf("n=15 -> %q,%v; want GT10", d, ok)
	}
	// Boundary is exclusive; below and non-numeric must NOT match.
	for _, bad := range []string{"10", "5", "abc", ""} {
		if _, ok := p.Find(map[string]string{"n": bad}); ok {
			t.Errorf("n=%q must not match >10", bad)
		}
	}

	// Same interval re-added: matcher deduped, data overwritten.
	p.Add(map[string]string{"n": ">10"}, covStr("GT10v2"))
	if d, _ := covFind(t, p, map[string]string{"n": "15"}); d != "GT10v2" {
		t.Errorf("re-added interval -> %q, want GT10v2", d)
	}

	// A second, different interval on the same key coexists.
	p.Add(map[string]string{"n": "<5"}, covStr("LT5"))
	if d, _ := covFind(t, p, map[string]string{"n": "3"}); d != "LT5" {
		t.Errorf("n=3 -> %q, want LT5", d)
	}
	if _, ok := p.Find(map[string]string{"n": "7"}); ok {
		t.Error("n=7 must match neither interval")
	}

	// String() renders matchers with the ~ marker.
	if s := p.String(); !strings.Contains(s, "n~>10") || !strings.Contains(s, "n~<5") {
		t.Errorf("String() missing interval entries: %q", s)
	}

	// Remove by the interval's fix string.
	p.Remove(map[string]string{"n": ">10"})
	if _, ok := p.Find(map[string]string{"n": "15"}); ok {
		t.Error("removed interval must no longer match")
	}
	if d, _ := covFind(t, p, map[string]string{"n": "3"}); d != "LT5" {
		t.Errorf("sibling interval must survive removal, got %q", d)
	}
}

func TestPatrunCovIntervalMixedKeys(t *testing.T) {
	p := New(Options{Interval: true})
	p.Add(map[string]string{"m": "1", "n": ">10"}, covStr("MIX"))
	if d, ok := covFind(t, p, map[string]string{"m": "1", "n": "12"}); !ok || d != "MIX" {
		t.Errorf("mixed exact+interval -> %q,%v; want MIX", d, ok)
	}
	if _, ok := p.Find(map[string]string{"m": "1", "n": "9"}); ok {
		t.Error("interval part must reject 9")
	}
	if _, ok := p.Find(map[string]string{"m": "2", "n": "12"}); ok {
		t.Error("exact part must reject m=2")
	}

	// Insert-before with a matcher on the incoming key.
	p2 := New(Options{Interval: true})
	p2.Add(map[string]string{"b": "1"}, covStr("B1"))
	p2.Add(map[string]string{"a": ">5"}, covStr("AG5"))
	if d, _ := covFind(t, p2, map[string]string{"a": "7"}); d != "AG5" {
		t.Errorf("a=7 -> %q, want AG5", d)
	}
	if d, _ := covFind(t, p2, map[string]string{"b": "1"}); d != "B1" {
		t.Errorf("b=1 -> %q, want B1", d)
	}

	// Insert-before that must carry existing matcher state onto the star path.
	p3 := New(Options{Interval: true})
	p3.Add(map[string]string{"b": ">5"}, covStr("BG5"))
	p3.Add(map[string]string{"a": "1"}, covStr("A1"))
	if d, _ := covFind(t, p3, map[string]string{"b": "7"}); d != "BG5" {
		t.Errorf("b=7 -> %q, want BG5 (matcher must survive reparent)", d)
	}
	if d, _ := covFind(t, p3, map[string]string{"a": "1"}); d != "A1" {
		t.Errorf("a=1 -> %q, want A1", d)
	}
	if s := p3.String(); !strings.Contains(s, "b~>5") {
		t.Errorf("String() must render reparented matcher: %q", s)
	}
}

// Without the Interval option ">10" is a plain literal.
func TestPatrunCovNoIntervalOptionIsLiteral(t *testing.T) {
	p := New()
	p.Add(map[string]string{"n": ">10"}, covStr("LIT"))
	if d, ok := covFind(t, p, map[string]string{"n": ">10"}); !ok || d != "LIT" {
		t.Errorf("literal >10 -> %q,%v; want LIT", d, ok)
	}
	if _, ok := p.Find(map[string]string{"n": "15"}); ok {
		t.Error("15 must not match the literal \">10\" without Interval option")
	}
}

// ---- interval.go: Make forms ------------------------------------------------

func TestIntervalCovMakeForms(t *testing.T) {
	m := &IntervalMatcher{}
	cases := []struct {
		fix  string
		yes  []string
		no   []string
		desc string
	}{
		{">10", []string{"11", "10.5", "1e3"}, []string{"10", "9", "-11"}, "exclusive lower"},
		{">=10", []string{"10", "11"}, []string{"9.99"}, "inclusive lower"},
		{"<10", []string{"9", "-5"}, []string{"10", "11"}, "exclusive upper"},
		{"<=10", []string{"10", "9"}, []string{"10.01"}, "inclusive upper"},
		{"=10", []string{"10"}, []string{"9", "11"}, "equality"},
		{"10]", []string{"10", "9"}, []string{"11"}, "close-bracket upper"},
		{">10&<20", []string{"15"}, []string{"10", "20", "5", "25"}, "AND range"},
		{"<10||>20", []string{"5", "25"}, []string{"10", "15", "20"}, "OR range"},
		{"10..20", []string{"10", "15", "20"}, []string{"9.9", "20.1"}, "dot-dot range"},
		{"20..10", []string{"10", "15", "20"}, []string{"9", "21"}, "reversed dot-dot"},
		{"[10,20]", []string{"10", "20"}, []string{"9", "21"}, "inclusive brackets"},
		{"(10,20)", []string{"15"}, []string{"10", "20"}, "exclusive parens"},
		{"<20&>10", []string{"15"}, []string{"10", "20"}, "reversed bound order"},
		{"=10&<=10", []string{"10", "9"}, []string{"11"}, "merge eq+lte"},
		{"=10&>=10", []string{"10", "11"}, []string{"9"}, "merge eq+gte"},
		// eq+eq merges to a bare equality op but keeps the AND join with a
		// nil second op, so the combination can never match anything.
		{"=10&=10", nil, []string{"9", "10", "11"}, "merge eq+eq"},
		{">=10&=10", []string{"10", "11"}, []string{"9"}, "merge gte+eq"},
		{"<=10&=10", []string{"10", "9"}, []string{"11"}, "merge lte+eq"},
		// Degenerate operator combinations resolve to never-matching checks
		// rather than errors — nothing may match.
		{"(=10", nil, []string{"9", "10", "11"}, "unresolvable op"},
		{"(10&=10", nil, []string{"9", "10", "11"}, "merge to error"},
		{"=10&10)", nil, []string{"9", "10", "11"}, "merge close-bracket no-op"},
		{"10&<20", nil, []string{"5", "15", "25"}, "missing first op ANDed"},
		// A second number with no operator resolves to an error op, and the
		// one-sided extension then widens the pattern to plain >10.
		{">10&20", []string{"15", "25"}, []string{"5", "10"}, "missing second op"},
	}
	for _, tc := range cases {
		mv, ok := m.Make("k", tc.fix)
		if !ok || mv == nil {
			t.Errorf("%s: Make(%q) failed", tc.desc, tc.fix)
			continue
		}
		for _, v := range tc.yes {
			if !mv.Match(v) {
				t.Errorf("%s: %q must match %q", tc.desc, v, tc.fix)
			}
		}
		for _, v := range tc.no {
			if mv.Match(v) {
				t.Errorf("%s: %q must NOT match %q", tc.desc, v, tc.fix)
			}
		}
		// Non-numeric input never matches any interval.
		if mv.Match("not-a-number") {
			t.Errorf("%s: non-numeric input must not match %q", tc.desc, tc.fix)
		}
	}
}

func TestIntervalCovMakeRejects(t *testing.T) {
	m := &IntervalMatcher{}
	for _, fix := range []string{
		"hello",     // no interval characters at all
		"",          // empty
		"10",        // plain number, no operator chars
		">",         // operator with no number
		">0xff",     // unparseable first number (hex float without exponent)
		">1&<0xff",  // unparseable second number
		"a>b",       // malformed expression
		"10,20",     // join without any operator char
		">>10<<20&", // garbage operators
	} {
		if mv, ok := m.Make("k", fix); ok || mv != nil {
			t.Errorf("Make(%q) = %v,%v; want nil,false", fix, mv, ok)
		}
	}
}

// covFakeMV is a non-interval MatchValue used to prove Same rejects foreign
// matcher kinds.
type covFakeMV struct{ km *keymap }

func (f *covFakeMV) Match(string) bool    { return false }
func (f *covFakeMV) Same(MatchValue) bool { return false }
func (f *covFakeMV) Kind() string         { return "fake" }
func (f *covFakeMV) Fix() string          { return "fake" }
func (f *covFakeMV) Keymap() *keymap      { return f.km }
func (f *covFakeMV) SetKeymap(km *keymap) { f.km = km }

func TestIntervalCovValueAccessors(t *testing.T) {
	m := &IntervalMatcher{}
	mv, ok := m.Make("k", ">10")
	if !ok {
		t.Fatal("Make(>10) failed")
	}
	if mv.Kind() != "interval" {
		t.Errorf("Kind() = %q, want interval", mv.Kind())
	}
	if mv.Fix() != ">10" {
		t.Errorf("Fix() = %q, want >10", mv.Fix())
	}
	if mv.Keymap() != nil {
		t.Error("fresh matcher must have nil keymap")
	}
	km := &keymap{}
	mv.SetKeymap(km)
	if mv.Keymap() != km {
		t.Error("SetKeymap/Keymap round trip failed")
	}

	same, _ := m.Make("k", ">10")
	if !mv.Same(same) {
		t.Error("identical intervals must be Same")
	}
	diff, _ := m.Make("k", "<10")
	if mv.Same(diff) {
		t.Error(">10 and <10 must not be Same")
	}
	if mv.Same(nil) {
		t.Error("Same(nil) must be false")
	}
	if mv.Same(&covFakeMV{}) {
		t.Error("Same(non-interval) must be false")
	}
}
