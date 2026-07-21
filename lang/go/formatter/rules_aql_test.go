package formatter

import (
	"strings"
	"testing"
)

// TestDefaultRulesFromAQL pins the compiled form of the fmt-rules.aql
// stylesheet: every field carries the canonical value the file declares,
// and the stylesheet parsed without error. This is the guard that makes
// the embedded file's validity a build fact.
func TestDefaultRulesFromAQL(t *testing.T) {
	if err := DefaultRulesInitError(); err != nil {
		t.Fatalf("stylesheet failed to parse: %v", err)
	}
	ru := DefaultRules()
	if ru.Width != maxLineWidth {
		t.Errorf("Width = %d, want %d (the stylesheet and the test-echo const drifted)", ru.Width, maxLineWidth)
	}
	if ru.Indent != 2 {
		t.Errorf("Indent = %d, want 2", ru.Indent)
	}
	if ru.FnWord != "fn" {
		t.Errorf("FnWord = %q, want fn", ru.FnWord)
	}
	if got, want := strings.Join(ru.StmtStartWords, " "), "def refine if for export end make"; got != want {
		t.Errorf("StmtStartWords = %q, want %q", got, want)
	}
	if got, want := strings.Join(ru.Strategies, " "), strings.Join(StrategyNames(), " "); got != want {
		t.Errorf("Strategies = %q, want %q", got, want)
	}
	if !ru.AttachDotSuffix {
		t.Error("AttachDotSuffix = false, want true")
	}
	prev := map[NodeKind]bool{}
	for _, k := range ru.AttachPrev {
		prev[k] = true
	}
	next := map[NodeKind]bool{}
	for _, k := range ru.AttachNext {
		next[k] = true
	}
	for _, k := range []NodeKind{NdComma, NdColon, NdQuestion, NdDot} {
		if !prev[k] {
			t.Errorf("AttachPrev missing %s", NodeKindName(k))
		}
	}
	for _, k := range []NodeKind{NdColon, NdDot} {
		if !next[k] {
			t.Errorf("AttachNext missing %s", NodeKindName(k))
		}
	}
	if ru.ListOpen != "[" || ru.ListClose != "]" ||
		ru.MapOpen != "{" || ru.MapClose != "}" ||
		ru.ParenOpen != "(" || ru.ParenClose != ")" {
		t.Errorf("bracket glyphs drifted: %q%q %q%q %q%q",
			ru.ListOpen, ru.ListClose, ru.MapOpen, ru.MapClose, ru.ParenOpen, ru.ParenClose)
	}
}

// TestRulesStylesheetFmtClean is the dogfood pin: the stylesheet is laid
// out in the very style it defines — formatting it is a no-op. This keeps
// the canonical-style definition from drifting out of the canonical style.
func TestRulesStylesheetFmtClean(t *testing.T) {
	if got := Format(rulesSource); got != rulesSource {
		t.Errorf("fmt-rules.aql is not fmt-clean; run it through fmt:\n--- have ---\n%s--- want ---\n%s", rulesSource, got)
	}
}

// stylesheetSource builds a complete rule-table source, optionally dropping
// one top-level key — the strict-mode fixture generator.
func stylesheetSource(dropKey string) string {
	entries := map[string]string{
		"width":             "width:72",
		"indent":            "indent:2",
		"fn-word":           "fn-word:'fn'",
		"statement-starts":  "statement-starts:['def']",
		"attach":            "attach:{comma:'prev'}",
		"attach-dot-suffix": "attach-dot-suffix:true",
		"brackets":          "brackets:{list:{open:'[' close:']'} map:{open:'{' close:'}'} paren:{open:'(' close:')'}}",
		"strategies":        "strategies:['inline' 'wrap']",
	}
	var b strings.Builder
	b.WriteString("{")
	first := true
	for _, key := range requiredRuleKeys() {
		if key == dropKey {
			continue
		}
		if !first {
			b.WriteString("\n ")
		}
		first = false
		b.WriteString(entries[key])
	}
	b.WriteString("}")
	return b.String()
}

// TestParseRulesSourceStrict pins the stylesheet loader's contract: a
// complete table parses; every malformed shape — unparseable source, no
// table, two tables, a missing top-level key, an incomplete brackets
// section — is refused with a message naming the problem.
func TestParseRulesSourceStrict(t *testing.T) {
	if _, err := parseRulesSource(stylesheetSource("")); err != nil {
		t.Fatalf("complete stylesheet rejected: %v", err)
	}

	bad := []struct{ name, src, want string }{
		{"unparseable", "{width:", "fmt rules stylesheet"},
		{"no table", "# just a comment\n", "no rule table"},
		{"two tables", "{width:72} {indent:2}", "more than one rule table"},
		{"brackets missing paren", strings.Replace(stylesheetSource(""),
			" paren:{open:'(' close:')'}", "", 1), `brackets.paren missing "open"`},
		{"brackets missing close", strings.Replace(stylesheetSource(""),
			"list:{open:'[' close:']'}", "list:{open:'['}", 1), `brackets.list missing "close"`},
	}
	for _, key := range requiredRuleKeys() {
		bad = append(bad, struct{ name, src, want string }{
			"missing " + key, stylesheetSource(key), "missing rule key \"" + key + "\"",
		})
	}
	for _, c := range bad {
		_, err := parseRulesSource(c.src)
		if err == nil {
			t.Errorf("%s: accepted, want error containing %q", c.name, c.want)
			continue
		}
		if !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: error %q does not mention %q", c.name, err.Error(), c.want)
		}
	}
}

// TestBoolRuleFieldWords pins the raw-parse boolean forms: the stylesheet
// path sees `true`/`false` as WORDS (only engine evaluation makes them
// Booleans), and both spellings must read identically — plus the negative.
func TestBoolRuleFieldWords(t *testing.T) {
	on := strings.Replace(stylesheetSource(""), "attach-dot-suffix:true", "attach-dot-suffix:false", 1)
	ru, err := parseRulesSource(on)
	if err != nil {
		t.Fatalf("false-word stylesheet rejected: %v", err)
	}
	if ru.AttachDotSuffix {
		t.Error("attach-dot-suffix:false read as true")
	}
	badBool := strings.Replace(stylesheetSource(""), "attach-dot-suffix:true", "attach-dot-suffix:'yes'", 1)
	if _, err := parseRulesSource(badBool); err == nil || !strings.Contains(err.Error(), "must be a Boolean") {
		t.Errorf("non-boolean attach-dot-suffix: err=%v, want must-be-Boolean", err)
	}
}

// TestSwapDefaultRulesErr pins the test seam: swapping an error in makes
// DefaultRulesInitError report it, and restoring brings back nil (the
// shipped stylesheet parses).
func TestSwapDefaultRulesErr(t *testing.T) {
	if err := DefaultRulesInitError(); err != nil {
		t.Fatalf("shipped stylesheet: %v", err)
	}
	sentinel := &stylesheetErrSentinel{}
	prev := SwapDefaultRulesErr(sentinel)
	if prev != nil {
		t.Errorf("previous init error non-nil: %v", prev)
	}
	if DefaultRulesInitError() != sentinel {
		t.Error("swapped error not reported")
	}
	SwapDefaultRulesErr(prev)
	if err := DefaultRulesInitError(); err != nil {
		t.Errorf("restore failed: %v", err)
	}
}

type stylesheetErrSentinel struct{}

func (*stylesheetErrSentinel) Error() string { return "stylesheet sentinel" }
