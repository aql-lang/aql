package test

import (
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/aql-lang/aql/eng/go/parser"
	"github.com/aql-lang/aql/lang/go/modules"
	"github.com/aql-lang/aql/lang/go/native"
	"github.com/aql-lang/aql/lang/go/tuikit"
)

// End-to-end test for the AQL vault TUI (lang/go/modules/vault_tui.aql,
// the design/VAULT-TUI-PORT.0.md application): the aql:vault-tui native
// module runs a real Tui.run session against a scripted virtual
// terminal and a scripted fake vault backend — no TTY and no vault
// anywhere, the §6.2 harness.

// runVaultTuiSteps mirrors runTuiAppSteps with BOTH host seams
// registered: a virtual terminal and a scripted vault backend. Both
// registrations happen before the import, per the ModuleInheritedCaps
// snapshot contract. do == nil leaves the vault backend unregistered;
// vb == nil leaves the terminal unregistered.
func runVaultTuiSteps(t *testing.T, vb *tuikit.VirtualBackend,
	do func(op string, params map[string]any) (any, error), steps []string) ([]native.Value, error) {
	t.Helper()
	reg, err := native.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	reg.ParseFunc = parser.Parse
	modules.InstallResolver(reg)
	if vb != nil {
		if rErr := modules.RegisterHostTui(reg, modules.TuiSpec{
			Name: "virtual",
			Open: func() (tuikit.Backend, error) { return vb, nil },
		}); rErr != nil {
			t.Fatal(rErr)
		}
	}
	if do != nil {
		if rErr := modules.RegisterHostVault(reg, modules.VaultSpec{Name: "fake", Do: do}); rErr != nil {
			t.Fatal(rErr)
		}
	}
	engine := native.NewTop(reg)
	var result []native.Value
	for _, step := range steps {
		vals, pErr := parser.Parse(step)
		if pErr != nil {
			return nil, pErr
		}
		result, err = engine.Run(vals)
		if err != nil {
			return nil, err
		}
	}
	return result, nil
}

// The vault TUI skeleton session, driven live (events land between
// repaints, so intermediate frames are observable): launch onto Home
// over a live status read, move the menu cursor, open a status pager (a
// real bridge round trip), navigate back, quit — the chrome, the screen
// stack, and the bridge all end to end.
func TestAppVaultTUI(t *testing.T) {
	vb := tuikit.NewVirtualBackend(60, 14)
	var mu sync.Mutex
	var ops []string
	do := func(op string, params map[string]any) (any, error) {
		mu.Lock()
		ops = append(ops, op)
		mu.Unlock()
		switch op {
		case "status":
			return map[string]any{
				"ok": true, "path": "/tmp/v/vault.jsonic", "backend": "file",
				"locked": false, "aliases": 2,
			}, nil
		case "status-text":
			return "BACKEND file\nGENERATION 7", nil
		}
		return nil, nil
	}
	type result struct {
		out []native.Value
		err error
	}
	done := make(chan result, 1)
	go func() {
		out, err := runVaultTuiSteps(t, vb, do, []string{
			`import "aql:vault-tui"`,
			`def final (VaultTui.run {dark: true})`,
			`join "|" [(convert String (size final.screens)) (convert String final.vault.ok)
			           (final.vault.backend) ((final.screens get 0).kind)]`,
		})
		done <- result{out, err}
	}()

	// the init frame renders the chrome over the fake vault's status
	pollScreen(t, vb, "vault: /tmp/v/vault.jsonic")
	first := strings.Join(vb.Screen(), "\n")
	for _, want := range []string{"aql vault", "Secrets", "enter open"} {
		if !strings.Contains(first, want) {
			t.Errorf("init frame is missing %q:\n%s", want, first)
		}
	}
	// move to Access and open its pager — a live bridge round trip
	vb.Inject(tuikit.Event{Tag: "key", Key: "down", Char: ""})
	vb.Inject(tuikit.Event{Tag: "key", Key: "enter", Char: ""})
	pollScreen(t, vb, "BACKEND file")
	pollScreen(t, vb, "Home › Access")
	// back to Home, then quit
	vb.Inject(tuikit.Event{Tag: "key", Key: "esc", Char: ""})
	vb.Inject(tuikit.Event{Tag: "key", Key: "q", Char: "q"})

	select {
	case res := <-done:
		if res.err != nil {
			t.Fatal(res.err)
		}
		if got := appLastString(t, res.out); got != "1|true|file|menu" {
			t.Fatalf("final state = %q, want 1|true|file|menu", got)
		}
	case <-time.After(15 * time.Second):
		t.Fatal("the vault TUI never returned")
	}
	// the backend saw the ops in order: the launch status load, then
	// the pager's status-text capture
	mu.Lock()
	joined := strings.Join(ops, ",")
	mu.Unlock()
	if !strings.HasPrefix(joined, "status,status-text") {
		t.Errorf("backend ops = %q", joined)
	}
}

// A backend failure at launch folds into the header + status line
// instead of aborting the app; a backend failure inside a pager builder
// folds into the pager text. Driven live with NO vault backend at all:
// every bridge word raises no_backend and the app keeps running.
func TestAppVaultTUIBackendErrorsFold(t *testing.T) {
	vb := tuikit.NewVirtualBackend(60, 12)
	type result struct {
		out []native.Value
		err error
	}
	done := make(chan result, 1)
	go func() {
		out, err := runVaultTuiSteps(t, vb, nil, []string{
			`import "aql:vault-tui"`,
			`def final (VaultTui.run {dark: false})`,
			`join "|" [(convert String final.vault.ok) (convert String final.status.err)]`,
		})
		done <- result{out, err}
	}()

	// launch folded the status failure into the header + status line
	pollScreen(t, vb, "no vault open")
	pollScreen(t, vb, "no vault backend registered")
	// a pager built over the dead backend folds the failure into its text
	vb.Inject(tuikit.Event{Tag: "key", Key: "enter", Char: ""})
	pollScreen(t, vb, "error: ")
	// pop the pager, then quit
	vb.Inject(tuikit.Event{Tag: "key", Key: "q", Char: "q"})
	vb.Inject(tuikit.Event{Tag: "key", Key: "q", Char: "q"})

	select {
	case res := <-done:
		if res.err != nil {
			t.Fatal(res.err)
		}
		if got := appLastString(t, res.out); got != "false|true" {
			t.Fatalf("folded state = %q, want false|true", got)
		}
	case <-time.After(15 * time.Second):
		t.Fatal("the vault TUI never returned")
	}
}

// Without a terminal backend the app cannot start: Tui.run raises the
// first-class no_backend error.
func TestAppVaultTUINoTerminal(t *testing.T) {
	_, err := runVaultTuiSteps(t, nil,
		func(string, map[string]any) (any, error) { return nil, nil },
		[]string{
			`import "aql:vault-tui"`,
			`VaultTui.run {}`,
		})
	if err == nil || !strings.Contains(err.Error(), "no terminal backend registered") {
		t.Fatalf("terminal-less run = %v", err)
	}
}
