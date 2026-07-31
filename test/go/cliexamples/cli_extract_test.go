package cliexamples

import (
	"reflect"
	"testing"
)

func TestCLIExtract_PrintsForm(t *testing.T) {
	src := "```bash\nboru do '1 add 2'   # prints 3\n```\n"
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
	src := "```bash\nboru do '1 add 2'   # prints \"3\"\n```\n"
	got := Extract("CLI.md", src)
	if len(got) != 1 || got[0].Expected != "3" {
		t.Fatalf("Expected = %q", got[0].Expected)
	}
}

func TestCLIExtract_ArrowForm(t *testing.T) {
	src := "```bash\nboru do '\"hi\" upper'   # => HI\n```\n"
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
	src := "```bash\nboru                  # REPL\nboru script.boru       # runs the file\n```\n"
	// "REPL" / "runs the file" don't start with a known output keyword,
	// so neither line is an assertion.
	if got := Extract("CLI.md", src); len(got) != 0 {
		t.Errorf("got %+v, want none", got)
	}
}

func TestCLIExtract_NonBoruIgnored(t *testing.T) {
	src := "```bash\nls -la   # prints files\n```\n"
	if got := Extract("CLI.md", src); len(got) != 0 {
		t.Errorf("non-boru command should be ignored, got %+v", got)
	}
}

func TestCLIExtract_BoruFenceIgnored(t *testing.T) {
	// A ```boru block is boru source, not shell — not a CLI example.
	src := "```boru\nboru do '1 add 2'   # prints 3\n```\n"
	if got := Extract("CLI.md", src); len(got) != 0 {
		t.Errorf("boru fence should be ignored, got %+v", got)
	}
}

func TestCLIExtract_SkipMarker(t *testing.T) {
	src := skipMarker + "\n```bash\nboru do '1 add 2'   # prints 3\n```\n"
	if got := Extract("CLI.md", src); len(got) != 0 {
		t.Errorf("skip-marked block ignored, got %+v", got)
	}
}

func TestCLIExtract_HashInQuotesNotComment(t *testing.T) {
	// A '#' inside the boru string is not the output comment.
	src := "```bash\nboru do '\"a#b\" upper'   # prints A#B\n```\n"
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
	src := "```bash\n$ boru do '1 add 2'   # prints 3\n```\n"
	got := Extract("CLI.md", src)
	if len(got) != 1 || !reflect.DeepEqual(got[0].Args, []string{"do", "1 add 2"}) {
		t.Fatalf("got %+v", got)
	}
}

func TestShellSplit_UnterminatedQuote(t *testing.T) {
	if _, ok := shellSplit(`boru do '1 add 2`); ok {
		t.Error("unterminated quote should return ok=false")
	}
}
