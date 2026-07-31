// Wave-4 coverage for the help subcommand: the overview (commands and
// services grouped separately), per-command help, the unknown-command
// hint, and the Command wrapper methods.
package help

import (
	"bytes"
	"io"
	"strings"
	"testing"

	"github.com/boru-lang/boru/cmd/go/internal/command"
)

type fakeCmd struct {
	name     string
	synopsis string
}

func (f *fakeCmd) Name() string     { return f.name }
func (f *fakeCmd) Synopsis() string { return f.synopsis }
func (f *fakeCmd) Run([]string, io.Reader, io.Writer, io.Writer) int {
	return 0
}

func testProvider() Provider {
	reg := command.New()
	reg.Register(&fakeCmd{name: "runner", synopsis: "run things"})
	reg.Register(&fakeCmd{name: "server", synopsis: "serve things"})
	services := map[string]bool{"server": true}
	return func() (*command.Registry, map[string]bool) { return reg, services }
}

func TestW4CommandMethods(t *testing.T) {
	c := New(testProvider())
	if c.Name() != "help" {
		t.Errorf("Name() = %q, want help", c.Name())
	}
	if c.Synopsis() == "" {
		t.Error("Synopsis() is empty")
	}
}

func TestW4OverviewGroupsCommandsAndServices(t *testing.T) {
	c := New(testProvider())
	var out bytes.Buffer
	if code := c.Run(nil, nil, &out, io.Discard); code != 0 {
		t.Fatalf("Run(nil) = %d, want 0", code)
	}
	s := out.String()

	for _, want := range []string{
		"boru — command-line tool",
		"Usage:",
		"Commands:",
		"Services (long-running",
		"runner",
		"run things",
		"server",
		"serve things",
		"Docs: ",
	} {
		if !strings.Contains(s, want) {
			t.Errorf("overview missing %q", want)
		}
	}

	// The one-shot command must appear in the Commands block, the
	// service in the Services block.
	cmdIdx := strings.Index(s, "Commands:")
	svcIdx := strings.Index(s, "Services (long-running")
	runnerIdx := strings.Index(s, "runner")
	serverIdx := strings.Index(s, "server ")
	if !(cmdIdx < runnerIdx && runnerIdx < svcIdx) {
		t.Errorf("runner not inside the Commands block (cmd=%d runner=%d svc=%d)", cmdIdx, runnerIdx, svcIdx)
	}
	if serverIdx < svcIdx {
		t.Errorf("server not inside the Services block (server=%d svc=%d)", serverIdx, svcIdx)
	}
}

func TestW4HelpKnownCommand(t *testing.T) {
	c := New(testProvider())
	var out bytes.Buffer
	if code := c.Run([]string{"runner"}, nil, &out, io.Discard); code != 0 {
		t.Fatalf("Run(runner) = %d, want 0", code)
	}
	s := out.String()
	if !strings.Contains(s, "boru runner — run things") {
		t.Errorf("missing summary line: %q", s)
	}
	if !strings.Contains(s, "Run 'boru runner -h'") {
		t.Errorf("missing -h hint: %q", s)
	}
}

func TestW4HelpUnknownCommand(t *testing.T) {
	c := New(testProvider())
	var out bytes.Buffer
	if code := c.Run([]string{"zz-nope"}, nil, &out, io.Discard); code != 1 {
		t.Fatalf("Run(zz-nope) = %d, want 1", code)
	}
	s := out.String()
	if !strings.Contains(s, `unknown command "zz-nope"`) {
		t.Errorf("missing unknown-command line: %q", s)
	}
	if !strings.Contains(s, "boru describe zz-nope") {
		t.Errorf("missing describe hint: %q", s)
	}
}
