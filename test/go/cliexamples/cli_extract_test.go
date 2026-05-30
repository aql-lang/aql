package cliexamples

import (
	"reflect"
	"testing"
)

func TestCLIExtract_PrintsForm(t *testing.T) {
	src := "```bash\naql do '1 add 2'   # prints 3\n```\n"
	got := Extract("CLI.md", src)
	if len(got) != 1 {
		t.Fatalf("got %d, want 1: %+v", len(got), got)
	}
	if !reflect.DeepEqual(got[0].Args, []string{"do", "1 add 2"}) {
		t.Errorf("Args = %#v", got[0].Args)
	}
	if got[0].Expected != "3" {
		t.Errorf("Expected = %q", got[0].Expected)
	}
}

func TestCLIExtract_QuoteStripped(t *testing.T) {
	src := "```bash\naql do '1 add 2'   # prints \"3\"\n```\n"
	got := Extract("CLI.md", src)
	if len(got) != 1 || got[0].Expected != "3" {
		t.Fatalf("Expected = %q", got[0].Expected)
	}
}

func TestCLIExtract_ArrowForm(t *testing.T) {
	src := "```bash\naql do '\"hi\" upper'   # => HI\n```\n"
	got := Extract("CLI.md", src)
	if len(got) != 1 {
		t.Fatalf("got %d: %+v", len(got), got)
	}
	if !reflect.DeepEqual(got[0].Args, []string{"do", `"hi" upper`}) {
		t.Errorf("Args = %#v", got[0].Args)
	}
	if got[0].Expected != "HI" {
		t.Errorf("Expected = %q", got[0].Expected)
	}
}

func TestCLIExtract_NoAssertionIgnored(t *testing.T) {
	src := "```bash\naql                  # REPL\naql script.aql       # runs the file\n```\n"
	// "REPL" / "runs the file" don't start with a known output keyword,
	// so neither line is an assertion.
	if got := Extract("CLI.md", src); len(got) != 0 {
		t.Errorf("got %+v, want none", got)
	}
}

func TestCLIExtract_NonAQLIgnored(t *testing.T) {
	src := "```bash\nls -la   # prints files\n```\n"
	if got := Extract("CLI.md", src); len(got) != 0 {
		t.Errorf("non-aql command should be ignored, got %+v", got)
	}
}

func TestCLIExtract_AqlFenceIgnored(t *testing.T) {
	// A ```aql block is AQL source, not shell — not a CLI example.
	src := "```aql\naql do '1 add 2'   # prints 3\n```\n"
	if got := Extract("CLI.md", src); len(got) != 0 {
		t.Errorf("aql fence should be ignored, got %+v", got)
	}
}

func TestCLIExtract_SkipMarker(t *testing.T) {
	src := skipMarker + "\n```bash\naql do '1 add 2'   # prints 3\n```\n"
	if got := Extract("CLI.md", src); len(got) != 0 {
		t.Errorf("skip-marked block ignored, got %+v", got)
	}
}

func TestCLIExtract_HashInQuotesNotComment(t *testing.T) {
	// A '#' inside the AQL string is not the output comment.
	src := "```bash\naql do '\"a#b\" upper'   # prints A#B\n```\n"
	got := Extract("CLI.md", src)
	if len(got) != 1 {
		t.Fatalf("got %d: %+v", len(got), got)
	}
	if !reflect.DeepEqual(got[0].Args, []string{"do", `"a#b" upper`}) {
		t.Errorf("Args = %#v", got[0].Args)
	}
	if got[0].Expected != "A#B" {
		t.Errorf("Expected = %q", got[0].Expected)
	}
}

func TestCLIExtract_PromptStripped(t *testing.T) {
	src := "```bash\n$ aql do '1 add 2'   # prints 3\n```\n"
	got := Extract("CLI.md", src)
	if len(got) != 1 || !reflect.DeepEqual(got[0].Args, []string{"do", "1 add 2"}) {
		t.Fatalf("got %+v", got)
	}
}

func TestShellSplit_UnterminatedQuote(t *testing.T) {
	if _, ok := shellSplit(`aql do '1 add 2`); ok {
		t.Error("unterminated quote should return ok=false")
	}
}
