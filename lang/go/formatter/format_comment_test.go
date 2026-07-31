package formatter

import (
	"strings"
	"testing"
)

// TestTrailingCommentNotDetached pins that a statement ending in a
// trailing comment is never wrapped onto multiple lines, so the comment
// stays attached to what it annotates — the property `expr # returns X`
// doc examples rely on. The comment's leading padding is still collapsed.
func TestTrailingCommentNotDetached(t *testing.T) {
	// A long annotated statement stays on one line.
	src := "import \"boru:time-util\" TimeUtil.await [[add 1 2] [add 3 4]]    # returns [3 7]\n"
	got := Format(src)
	if lines := strings.Split(strings.TrimSuffix(got, "\n"), "\n"); len(lines) != 1 {
		t.Errorf("annotated statement was wrapped into %d lines:\n%s", len(lines), got)
	}
	if !strings.HasSuffix(strings.TrimSuffix(got, "\n"), "# returns [3 7]") {
		t.Errorf("trailing comment lost or moved: %q", got)
	}
	// Alignment padding before the comment is collapsed to one space.
	if strings.Contains(got, "]]    #") {
		t.Errorf("comment padding not collapsed: %q", got)
	}
}
