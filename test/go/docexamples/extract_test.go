package docexamples

import "testing"

func TestExtract_Inline(t *testing.T) {
	src := "intro\n\n```\n2 mul 3   # returns 6\n```\n"
	got := Extract("X.md", src)
	if len(got) != 1 {
		t.Fatalf("got %d examples, want 1: %+v", len(got), got)
	}
	ex := got[0]
	if ex.Expr != "2 mul 3" {
		t.Errorf("Expr = %q, want %q", ex.Expr, "2 mul 3")
	}
	if ex.Program != "2 mul 3" {
		t.Errorf("Program = %q, want %q", ex.Program, "2 mul 3")
	}
	if ex.Expected != "6" || ex.WantErr {
		t.Errorf("Expected = %q wantErr=%v, want 6/false", ex.Expected, ex.WantErr)
	}
	if ex.File != "X.md" || ex.Line != 4 {
		t.Errorf("File/Line = %s:%d, want X.md:4", ex.File, ex.Line)
	}
}

func TestExtract_ReplPromptStripped(t *testing.T) {
	src := "```\naql> 1 2 add   # returns 3\n```\n"
	got := Extract("X.md", src)
	if len(got) != 1 || got[0].Expr != "1 2 add" || got[0].Expected != "3" {
		t.Fatalf("got %+v", got)
	}
}

func TestExtract_SharedSetupState(t *testing.T) {
	// def lines (no result) become setup prepended to the later
	// `# returns` line.
	src := "```\naql> def x 1\naql> def y 2\naql> {x y}   # returns {x:1 y:2}\n```\n"
	got := Extract("X.md", src)
	if len(got) != 1 {
		t.Fatalf("got %d examples, want 1: %+v", len(got), got)
	}
	want := "def x 1\ndef y 2\n{x y}"
	if got[0].Program != want {
		t.Errorf("Program = %q, want %q", got[0].Program, want)
	}
	if got[0].Expected != "{x:1 y:2}" {
		t.Errorf("Expected = %q", got[0].Expected)
	}
}

func TestExtract_PriorResultLinesNotInProgram(t *testing.T) {
	// Two independent result lines in one block: the second must NOT
	// carry the first as setup (it was an asserted result, not state).
	src := "```\naql> 5 dup    # returns 5 5\naql> 1 2 swap  # returns 2 1\n```\n"
	got := Extract("X.md", src)
	if len(got) != 2 {
		t.Fatalf("got %d examples, want 2", len(got))
	}
	if got[1].Program != "1 2 swap" {
		t.Errorf("second Program = %q, want %q", got[1].Program, "1 2 swap")
	}
}

func TestExtract_DescriptionAfterEmDashStripped(t *testing.T) {
	src := "```\nadd 1 2   # returns 3 — classic prefix\n```\n"
	got := Extract("X.md", src)
	if len(got) != 1 || got[0].Expected != "3" {
		t.Fatalf("Expected = %q (want 3)", got[0].Expected)
	}
}

func TestExtract_MultiTokenValue(t *testing.T) {
	// The value can span spaces (a multi-value stack render).
	src := "```\n5 dup   # returns 5 5 — duplicate top\n```\n"
	got := Extract("X.md", src)
	if len(got) != 1 || got[0].Expected != "5 5" {
		t.Fatalf("Expected = %q (want '5 5')", got[0].Expected)
	}
}

func TestExtract_HashInsideStringIsNotTheComment(t *testing.T) {
	// A `#` inside the expression's string must not be mistaken for the
	// result comment.
	src := "```\n\"a#b\" foo   # returns 'a#b'\n```\n"
	got := Extract("X.md", src)
	if len(got) != 1 {
		t.Fatalf("got %d examples, want 1: %+v", len(got), got)
	}
	if got[0].Expr != `"a#b" foo` {
		t.Errorf("Expr = %q, want %q", got[0].Expr, `"a#b" foo`)
	}
	if got[0].Expected != "'a#b'" {
		t.Errorf("Expected = %q, want %q", got[0].Expected, "'a#b'")
	}
}

func TestExtract_DescriptiveCommentNotAsserted(t *testing.T) {
	// A trailing comment that does not start with `returns` is just a
	// description; the line is not an asserted example.
	src := "```\n\"x,y\" StringUtil.split \",\"   # split on comma\n```\n"
	if got := Extract("X.md", src); len(got) != 0 {
		t.Errorf("descriptive comment must not assert, got %+v", got)
	}
}

func TestExtract_ErrorForms(t *testing.T) {
	cases := []struct {
		rhs       string
		wantErr   bool
		errSubstr string
		expected  string
	}{
		{"error", true, "", ""},
		{"build error", true, "", ""},
		{"error: missing key 'y'", true, "missing key 'y'", ""},
		{"Error", true, "", ""},
		{"[aql/type_error] return value 1: expected Integer got X", true, "aql/type_error", ""},
		{"6", false, "", "6"},
	}
	for _, c := range cases {
		exp, we, es := classifyRHS(c.rhs)
		if we != c.wantErr || es != c.errSubstr || exp != c.expected {
			t.Errorf("classifyRHS(%q) = (%q,%v,%q), want (%q,%v,%q)",
				c.rhs, exp, we, es, c.expected, c.wantErr, c.errSubstr)
		}
	}
}

func TestExtract_BashFenceIgnored(t *testing.T) {
	src := "```bash\naql do '2 mul 3'   # returns 6\n```\n"
	if got := Extract("X.md", src); len(got) != 0 {
		t.Errorf("bash fence should be ignored, got %+v", got)
	}
}

func TestExtract_SkipMarker(t *testing.T) {
	src := skipMarker + "\n```\nnow   # returns 2026-01-01\n```\n"
	if got := Extract("X.md", src); len(got) != 0 {
		t.Errorf("skip-marked block should be ignored, got %+v", got)
	}
}

func TestExtract_AqlTaggedFenceRuns(t *testing.T) {
	src := "```aql\n2 mul 3   # returns 6\n```\n"
	if got := Extract("X.md", src); len(got) != 1 {
		t.Errorf("```aql fence should run, got %+v", got)
	}
}

func TestExtract_ResultOnOwnLine(t *testing.T) {
	// The expression is one line; its `# returns result` is the next line.
	src := "```\naql> make Inventory [[1] [2]]\n# returns [{a:1} {a:2}]\n```\n"
	got := Extract("X.md", src)
	if len(got) != 1 {
		t.Fatalf("got %d examples, want 1: %+v", len(got), got)
	}
	if got[0].Expr != "make Inventory [[1] [2]]" {
		t.Errorf("Expr = %q", got[0].Expr)
	}
	if got[0].Expected != "[{a:1} {a:2}]" {
		t.Errorf("Expected = %q", got[0].Expected)
	}
	if got[0].Line != 2 {
		t.Errorf("Line = %d, want 2 (the expr line)", got[0].Line)
	}
}

func TestExtract_ProseOutsideFenceIgnored(t *testing.T) {
	src := "this sentence says foo returns bar in prose, not code\n"
	if got := Extract("X.md", src); len(got) != 0 {
		t.Errorf("prose should be ignored, got %+v", got)
	}
}
