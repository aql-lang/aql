package modules

import (
	"strings"
	"testing"
)

// boru:repl — the socket REPL written in boru (repl.go's preamble) —
// exercised end to end over a real loopback socket: evaluation,
// session-history persistence across sandboxed one-shot evals, error
// replies, and /reset.
func TestReplModuleEndToEnd(t *testing.T) {
	out, err := runNetSteps(t, []string{
		`import "boru:repl"`,
		`import "boru:net"`,
		`def ln (Repl.serve {port: 0})`,
		`def lna (Net.addr ln)`,
		`def port lna.port`,
		`def ep (Repl.connect (join "" ["127.0.0.1:" (convert String port)]))`,
		`def r1 (Repl.eval ep "1 add 2")`,
		`Repl.eval ep "def x 21" drop`,
		`def r2 (Repl.eval ep "x mul 2")`,
		`def r3 (Repl.eval ep "no-such-word")`,
		`def r4 (Repl.eval ep "/reset")`,
		`def r5 (Repl.eval ep "x")`,
		`Repl.close ep;`,
		`Repl.close ln;`,
		`join "|" [r1 r2 r3 r4 r5]`,
	})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	got := lastString(t, out)
	parts := strings.Split(got, "|")
	if len(parts) != 5 {
		t.Fatalf("expected 5 replies, got %q", got)
	}
	if parts[0] != "3" {
		t.Errorf("eval '1 add 2' = %q, want 3", parts[0])
	}
	if parts[1] != "42" {
		t.Errorf("history replay 'x mul 2' = %q, want 42 (def x 21 must persist)", parts[1])
	}
	if !strings.Contains(parts[2], "error") || !strings.Contains(parts[2], "no-such-word") {
		t.Errorf("bad input reply = %q, want an error line naming the word", parts[2])
	}
	if !strings.Contains(parts[3], "reset") {
		t.Errorf("/reset reply = %q", parts[3])
	}
	if !strings.Contains(parts[4], "error") {
		t.Errorf("post-reset 'x' = %q, want an undefined-word error (history cleared)", parts[4])
	}
}

// Repl.serve on a boru:net-visible port also needs `import "boru:repl"`
// alone (Net words are used inside the module, not by the caller):
// the module preamble's own imports must not leak requirements out.
func TestReplModuleSelfContainedImports(t *testing.T) {
	_, err := runNetSteps(t, []string{
		`import "boru:repl"`,
		`def ln (Repl.serve {port: 0})`,
		`Repl.close ln;`,
		`"ok"`,
	})
	if err != nil {
		t.Fatalf("boru:repl must work without the caller importing boru:net/boru:vm: %v", err)
	}
}
