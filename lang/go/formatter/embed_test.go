package formatter

import (
	"strings"
	"testing"
)

// TestFormatMarkdown reformats AQL inside ```aql fences and marker
// regions, exercises a longer closing fence and a tilde fence, and leaves
// non-aql fences, prose, and an empty region untouched.
func TestFormatMarkdown(t *testing.T) {
	src := strings.Join([]string{
		"# Title",
		"",
		"```aql",
		"def  square  fn  [[x:Integer]  [Integer]  [x mul x]]",
		"````", // a longer closing fence still matches
		"",
		"~~~aql",
		"def  a   1",
		"~~~",
		"",
		"```text",
		"def  not  aql",
		"```",
		"",
		"<!-- aqlfmt -->",
		"def  y   2",
		"<!-- /aqlfmt -->",
		"",
		"<!-- aqlfmt -->",
		"<!-- /aqlfmt -->",
		"end",
	}, "\n")
	want := strings.Join([]string{
		"# Title",
		"",
		"```aql",
		"def square fn x:Integer Integer [x mul x]",
		"````",
		"",
		"~~~aql",
		"def a 1",
		"~~~",
		"",
		"```text",
		"def  not  aql",
		"```",
		"",
		"<!-- aqlfmt -->",
		"def y 2",
		"<!-- /aqlfmt -->",
		"",
		"<!-- aqlfmt -->",
		"<!-- /aqlfmt -->",
		"end",
	}, "\n")
	if got := FormatMarkdown(src); got != want {
		t.Errorf("FormatMarkdown mismatch:\n got:  %q\n want: %q", got, want)
	}
}

// TestFormatHTML reformats only marker regions: fences are not special in
// HTML, and an unclosed region running to end-of-input is still
// reformatted.
func TestFormatHTML(t *testing.T) {
	src := strings.Join([]string{
		"<pre>",
		"<!-- aqlfmt -->",
		"def  z   3",
		"<!-- /aqlfmt -->",
		"</pre>",
		"```aql",
		"def  w  4",
		"```",
		"<!-- aqlfmt -->",
		"def  tail   5",
	}, "\n")
	want := strings.Join([]string{
		"<pre>",
		"<!-- aqlfmt -->",
		"def z 3",
		"<!-- /aqlfmt -->",
		"</pre>",
		"```aql",
		"def  w  4",
		"```",
		"<!-- aqlfmt -->",
		"def tail 5",
	}, "\n")
	if got := FormatHTML(src); got != want {
		t.Errorf("FormatHTML mismatch:\n got:  %q\n want: %q", got, want)
	}
}

// TestFenceHelpers pins the branch behaviour of the fence predicates.
func TestFenceHelpers(t *testing.T) {
	if got := leadingFenceRun(""); got != "" {
		t.Errorf("leadingFenceRun empty = %q", got)
	}
	if got := leadingFenceRun("text"); got != "" {
		t.Errorf("leadingFenceRun non-fence = %q", got)
	}
	if got := leadingFenceRun("``x"); got != "" {
		t.Errorf("leadingFenceRun short = %q", got)
	}
	if got := leadingFenceRun("```aql"); got != "```" {
		t.Errorf("leadingFenceRun backtick = %q", got)
	}
	if got := leadingFenceRun("~~~~"); got != "~~~~" {
		t.Errorf("leadingFenceRun tilde = %q", got)
	}

	if _, ok := aqlFenceOpen("```"); ok {
		t.Error("bare fence should not be an aql open")
	}
	if _, ok := aqlFenceOpen("```go"); ok {
		t.Error("go fence should not be an aql open")
	}
	if f, ok := aqlFenceOpen("```aql"); !ok || f != "```" {
		t.Errorf("aql fence = (%q, %v)", f, ok)
	}

	if !isFenceClose("```", "```") {
		t.Error("equal fence should close")
	}
	if !isFenceClose("````  ", "```") {
		t.Error("longer fence with trailing spaces should close")
	}
	if isFenceClose("~~~", "```") {
		t.Error("different fence char should not close")
	}
	if isFenceClose("``", "```") {
		t.Error("shorter fence should not close")
	}
	if isFenceClose("``` x", "```") {
		t.Error("fence with trailing text should not close")
	}
}
