package docexamples

import "testing"

// A bare `# returns …` line with NO preceding code line in the block is
// a stray result — skipped, never evaluated.
func TestExtract_StrayResultLineSkipped(t *testing.T) {
	src := "```\n# returns 6\n```\n"
	if got := Extract("X.md", src); len(got) != 0 {
		t.Fatalf("stray result extracted %d examples, want 0: %+v", len(got), got)
	}
}

// An error-code form whose bracket never closes (`[aql/foo` with no
// `]`) keeps the wantErr flag but carries no error code to match.
func TestExtract_UnterminatedErrorBracket(t *testing.T) {
	exp, wantErr, errSubstr := classifyRHS("[aql/undefined_word return value")
	if !wantErr {
		t.Error("unterminated error bracket: wantErr = false, want true")
	}
	if exp != "" || errSubstr != "" {
		t.Errorf("classifyRHS = (%q, %q), want empty expected and code", exp, errSubstr)
	}
}
