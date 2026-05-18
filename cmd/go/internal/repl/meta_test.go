package repl

import (
	"bytes"
	"strings"
	"testing"

	"github.com/aql-lang/aql/lang/go/engine"
)

// --- MetaRegistry unit tests ---

func TestNewMetaRegistryHasBuiltins(t *testing.T) {
	mr := NewMetaRegistry()
	names := mr.Names()
	if len(names) < 2 {
		t.Fatalf("expected at least 2 built-in commands, got %d", len(names))
	}
	if mr.Lookup("help") == nil {
		t.Error("expected /help to be registered")
	}
	if mr.Lookup("stack") == nil {
		t.Error("expected /stack to be registered")
	}
}

func TestMetaRegistryRegisterAndLookup(t *testing.T) {
	mr := NewMetaRegistry()
	mr.Register(&MetaCommand{
		Name:    "test",
		Summary: "A test command",
		Handler: func(args []any, ctx *MetaContext) error {
			return nil
		},
	})
	cmd := mr.Lookup("test")
	if cmd == nil {
		t.Fatal("expected /test to be registered")
	}
	if cmd.Summary != "A test command" {
		t.Errorf("summary = %q, want %q", cmd.Summary, "A test command")
	}
}

func TestMetaRegistryNames(t *testing.T) {
	mr := NewMetaRegistry()
	names := mr.Names()
	// Should be sorted.
	for i := 1; i < len(names); i++ {
		if names[i] < names[i-1] {
			t.Errorf("names not sorted: %v", names)
			break
		}
	}
}

// --- ParseAndRun tests ---

func TestParseAndRunNonMeta(t *testing.T) {
	mr := NewMetaRegistry()
	ctx := &MetaContext{Out: &bytes.Buffer{}}
	handled, err := mr.ParseAndRun("1 add 2", ctx)
	if handled {
		t.Error("expected non-meta line to not be handled")
	}
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestParseAndRunEmptyCommand(t *testing.T) {
	mr := NewMetaRegistry()
	ctx := &MetaContext{Out: &bytes.Buffer{}}
	handled, err := mr.ParseAndRun("/", ctx)
	if !handled {
		t.Error("expected '/' to be handled")
	}
	if err == nil || !strings.Contains(err.Error(), "empty meta command") {
		t.Errorf("expected empty meta command error, got: %v", err)
	}
}

func TestParseAndRunUnknownCommand(t *testing.T) {
	mr := NewMetaRegistry()
	ctx := &MetaContext{Out: &bytes.Buffer{}}
	handled, err := mr.ParseAndRun("/nope", ctx)
	if !handled {
		t.Error("expected '/nope' to be handled")
	}
	if err == nil || !strings.Contains(err.Error(), "unknown meta command") {
		t.Errorf("expected unknown command error, got: %v", err)
	}
}

func TestParseAndRunCustomCommand(t *testing.T) {
	mr := NewMetaRegistry()
	var gotArgs []any
	mr.Register(&MetaCommand{
		Name:    "echo",
		Summary: "Echo args",
		Handler: func(args []any, ctx *MetaContext) error {
			gotArgs = args
			return nil
		},
	})
	ctx := &MetaContext{Out: &bytes.Buffer{}}
	handled, err := mr.ParseAndRun("/echo hello 42", ctx)
	if !handled {
		t.Error("expected handled")
	}
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(gotArgs) != 2 {
		t.Fatalf("expected 2 args, got %d: %v", len(gotArgs), gotArgs)
	}
	if gotArgs[0] != "hello" {
		t.Errorf("arg[0] = %v, want 'hello'", gotArgs[0])
	}
}

func TestParseAndRunNoArgs(t *testing.T) {
	mr := NewMetaRegistry()
	var gotArgs []any
	mr.Register(&MetaCommand{
		Name:    "noop",
		Summary: "No-op",
		Handler: func(args []any, ctx *MetaContext) error {
			gotArgs = args
			return nil
		},
	})
	ctx := &MetaContext{Out: &bytes.Buffer{}}
	handled, err := mr.ParseAndRun("/noop", ctx)
	if !handled || err != nil {
		t.Fatalf("handled=%v err=%v", handled, err)
	}
	if len(gotArgs) != 0 {
		t.Errorf("expected 0 args, got %d: %v", len(gotArgs), gotArgs)
	}
}

// --- parseMetaArgs tests ---

func TestParseMetaArgsEmpty(t *testing.T) {
	args, err := parseMetaArgs("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(args) != 0 {
		t.Errorf("expected 0 args, got %d", len(args))
	}
}

func TestParseMetaArgsSingleString(t *testing.T) {
	args, err := parseMetaArgs("hello")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(args) != 1 {
		t.Fatalf("expected 1 arg, got %d", len(args))
	}
	if args[0] != "hello" {
		t.Errorf("arg[0] = %v, want 'hello'", args[0])
	}
}

func TestParseMetaArgsMultiple(t *testing.T) {
	args, err := parseMetaArgs("foo 42 true")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(args) != 3 {
		t.Fatalf("expected 3 args, got %d: %v", len(args), args)
	}
}

func TestParseMetaArgsQuotedString(t *testing.T) {
	args, err := parseMetaArgs(`"hello world"`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(args) != 1 {
		t.Fatalf("expected 1 arg, got %d", len(args))
	}
	if args[0] != "hello world" {
		t.Errorf("arg[0] = %v, want 'hello world'", args[0])
	}
}

// --- /help tests ---

func TestMetaHelpNoArgs(t *testing.T) {
	mr := NewMetaRegistry()
	out := &bytes.Buffer{}
	ctx := &MetaContext{Out: out}
	_, err := mr.ParseAndRun("/help", ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	output := out.String()
	if !strings.Contains(output, "/help") {
		t.Error("expected /help in output")
	}
	if !strings.Contains(output, "/stack") {
		t.Error("expected /stack in output")
	}
	if !strings.Contains(output, "Meta commands") {
		t.Error("expected 'Meta commands' header")
	}
}

func TestMetaHelpWithWord(t *testing.T) {
	mr := NewMetaRegistry()
	out := &bytes.Buffer{}
	ctx := &MetaContext{Out: out}
	_, err := mr.ParseAndRun("/help add", ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	output := out.String()
	if !strings.Contains(output, "add") {
		t.Error("expected 'add' in help output")
	}
	if !strings.Contains(output, "Description") {
		t.Error("expected 'Description' section")
	}
}

func TestMetaHelpUnknownWord(t *testing.T) {
	mr := NewMetaRegistry()
	out := &bytes.Buffer{}
	ctx := &MetaContext{Out: out}
	_, err := mr.ParseAndRun("/help nonexistent", ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out.String(), "no help available") {
		t.Error("expected 'no help available' message")
	}
}

// --- /stack tests ---

func TestMetaStackEmpty(t *testing.T) {
	mr := NewMetaRegistry()
	out := &bytes.Buffer{}
	ctx := &MetaContext{Out: out, Stack: nil}
	_, err := mr.ParseAndRun("/stack", ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out.String(), "empty stack") {
		t.Errorf("expected 'empty stack', got %q", out.String())
	}
}

func TestMetaStackWithValues(t *testing.T) {
	mr := NewMetaRegistry()
	out := &bytes.Buffer{}
	stack := []engine.Value{
		engine.NewInteger(1),
		engine.NewString("hello"),
		engine.NewBoolean(true),
	}
	ctx := &MetaContext{Out: out, Stack: stack}
	_, err := mr.ParseAndRun("/stack", ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	output := out.String()
	if !strings.Contains(output, "[0] 1") {
		t.Error("expected [0] 1 in output")
	}
	if !strings.Contains(output, "[1] 'hello'") {
		t.Error("expected [1] 'hello' in output")
	}
	if !strings.Contains(output, "[2] true") {
		t.Error("expected [2] true in output")
	}
}

func TestMetaStackTopN(t *testing.T) {
	mr := NewMetaRegistry()
	out := &bytes.Buffer{}
	stack := []engine.Value{
		engine.NewInteger(10),
		engine.NewInteger(20),
		engine.NewInteger(30),
		engine.NewInteger(40),
	}
	ctx := &MetaContext{Out: out, Stack: stack}
	_, err := mr.ParseAndRun("/stack 2", ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	output := out.String()
	// Should show only the top 2: [2] 30 and [3] 40
	if strings.Contains(output, "[0]") || strings.Contains(output, "[1]") {
		t.Errorf("should not contain [0] or [1], got %q", output)
	}
	if !strings.Contains(output, "[2] 30") {
		t.Errorf("expected [2] 30, got %q", output)
	}
	if !strings.Contains(output, "[3] 40") {
		t.Errorf("expected [3] 40, got %q", output)
	}
}

func TestMetaStackTopNExceedsDepth(t *testing.T) {
	mr := NewMetaRegistry()
	out := &bytes.Buffer{}
	stack := []engine.Value{engine.NewInteger(1), engine.NewInteger(2)}
	ctx := &MetaContext{Out: out, Stack: stack}
	// n > stack depth → show all
	_, err := mr.ParseAndRun("/stack 99", ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	output := out.String()
	if !strings.Contains(output, "[0] 1") || !strings.Contains(output, "[1] 2") {
		t.Errorf("expected all entries, got %q", output)
	}
}

func TestMetaStackTopZero(t *testing.T) {
	mr := NewMetaRegistry()
	out := &bytes.Buffer{}
	stack := []engine.Value{engine.NewInteger(1)}
	ctx := &MetaContext{Out: out, Stack: stack}
	_, err := mr.ParseAndRun("/stack 0", ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// n=0 → nothing shown
	if strings.Contains(out.String(), "[") {
		t.Errorf("expected no output for /stack 0, got %q", out.String())
	}
}

func TestMetaStackNegative(t *testing.T) {
	mr := NewMetaRegistry()
	out := &bytes.Buffer{}
	stack := []engine.Value{engine.NewInteger(1)}
	ctx := &MetaContext{Out: out, Stack: stack}
	_, err := mr.ParseAndRun("/stack -1", ctx)
	if err == nil || !strings.Contains(err.Error(), "non-negative") {
		t.Errorf("expected non-negative error, got: %v", err)
	}
}

func TestMetaStackBadArg(t *testing.T) {
	mr := NewMetaRegistry()
	out := &bytes.Buffer{}
	ctx := &MetaContext{Out: out, Stack: []engine.Value{engine.NewInteger(1)}}
	_, err := mr.ParseAndRun("/stack foo", ctx)
	if err == nil || !strings.Contains(err.Error(), "expected integer") {
		t.Errorf("expected integer arg error, got: %v", err)
	}
}

// --- splitCommand tests ---

func TestSplitCommand(t *testing.T) {
	tests := []struct {
		input    string
		wantName string
		wantArgs string
	}{
		{"help", "help", ""},
		{"help add", "help", "add"},
		{"stack  ", "stack", ""},
		{"echo foo bar", "echo", "foo bar"},
		{"", "", ""},
	}
	for _, tc := range tests {
		name, args := splitCommand(tc.input)
		if name != tc.wantName || args != tc.wantArgs {
			t.Errorf("splitCommand(%q) = (%q, %q), want (%q, %q)",
				tc.input, name, args, tc.wantName, tc.wantArgs)
		}
	}
}

// --- REPL integration test with meta commands ---

func TestReplMetaHelpIntegration(t *testing.T) {
	in := strings.NewReader("/help\n")
	out := &bytes.Buffer{}
	Start(in, out, "")
	output := out.String()
	if !strings.Contains(output, "Meta commands") {
		t.Errorf("expected 'Meta commands' in REPL output, got %q", output)
	}
}

func TestReplMetaStackIntegration(t *testing.T) {
	// Push values then check /stack
	in := strings.NewReader("1 2 3\n/stack\n")
	out := &bytes.Buffer{}
	Start(in, out, "")
	output := out.String()
	if !strings.Contains(output, "[0] 1") {
		t.Errorf("expected [0] 1 in output, got %q", output)
	}
	if !strings.Contains(output, "[2] 3") {
		t.Errorf("expected [2] 3 in output, got %q", output)
	}
}

func TestReplMetaUnknownCommand(t *testing.T) {
	in := strings.NewReader("/bogus\n")
	out := &bytes.Buffer{}
	Start(in, out, "")
	output := out.String()
	if !strings.Contains(output, "unknown meta command") {
		t.Errorf("expected unknown meta command error, got %q", output)
	}
}
