package test

import (
	"fmt"
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
	// the backend saw the ops in order: the launch status load, the
	// saved-prefs read, then the pager's status-text capture
	mu.Lock()
	joined := strings.Join(ops, ",")
	mu.Unlock()
	if !strings.HasPrefix(joined, "status,prefs,status-text") {
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
	// a pager built over the dead backend folds the failure into its
	// text (Access still opens the status pager in this phase)
	vb.Inject(tuikit.Event{Tag: "key", Key: "down", Char: ""})
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

// pollScreenGone waits until the virtual screen no longer shows want.
func pollScreenGone(t *testing.T, vb *tuikit.VirtualBackend, want string) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if !strings.Contains(strings.Join(vb.Screen(), "\n"), want) {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("screen never dropped %q:\n%s", want, strings.Join(vb.Screen(), "\n"))
}

func typeKeys(vb *tuikit.VirtualBackend, s string) {
	for _, r := range s {
		vb.Inject(tuikit.Event{Tag: "key", Key: string(r), Char: string(r)})
	}
}

func key(vb *tuikit.VirtualBackend, name string) {
	vb.Inject(tuikit.Event{Tag: "key", Key: name, Char: ""})
}

// secretsFake is the scripted backend for the secrets flows: two
// aliases, canned recipes, a recorded call log, and a passphrase state
// machine ("pw" authenticates).
type secretsFake struct {
	mu        sync.Mutex
	ops       []string
	params    map[string]map[string]any
	needsPass bool
	authed    bool
	removed   bool
}

func (f *secretsFake) do(op string, params map[string]any) (any, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.ops = append(f.ops, op)
	if f.params == nil {
		f.params = map[string]map[string]any{}
	}
	f.params[op] = params
	row := map[string]any{"name": "gh_token", "provider": "github", "namespace": "", "expires-at": ""}
	switch op {
	case "status":
		return map[string]any{"ok": true, "path": "/tmp/v/vault.jsonic", "backend": "file", "locked": false}, nil
	case "needs-passphrase":
		return f.needsPass, nil
	case "authed":
		return f.authed, nil
	case "authenticate":
		if params["pass"] == "pw" {
			f.authed = true
			return true, nil
		}
		return nil, fmt.Errorf("bad passphrase")
	case "secrets":
		if f.removed {
			return []any{}, nil
		}
		return []any{row, map[string]any{"name": "aws_key", "provider": "", "namespace": "", "expires-at": ""}}, nil
	case "secret":
		return row, nil
	case "recipes":
		return []any{
			map[string]any{"cmd": "aql vault exec gh_token -- <cmd>", "desc": "inject as env"},
			map[string]any{"cmd": "aql vault exec --for=github gh_token -- gh", "desc": "provider recipe"},
		}, nil
	case "reveal":
		return "sekrit-v1", nil
	case "copy":
		return "clip-tool", nil
	case "grant":
		return "token: tok-12345 (shown once)", nil
	case "status-text":
		return "STATUS PANEL", nil
	case "audit":
		return "AUDIT LOG LINES", nil
	case "history":
		return "HISTORY LINES", nil
	case "providers-text":
		return "PROVIDER CATALOG", nil
	case "config-text":
		return "CONFIG LISTING", nil
	case "prefs":
		return map[string]any{"theme": ""}, nil
	case "remove":
		f.removed = true
	}
	return nil, nil
}

func (f *secretsFake) opAndParams(t *testing.T, op string) map[string]any {
	t.Helper()
	f.mu.Lock()
	defer f.mu.Unlock()
	p, ok := f.params[op]
	if !ok {
		t.Fatalf("backend never saw op %q (ops: %v)", op, f.ops)
	}
	return p
}

// The secrets browse flow: table rows over the bridge, `/` filtering,
// the detail page with recipes, reveal → the value is on screen,
// hide → it is gone again, copy → the clipboard label lands in the
// status line.
func TestAppVaultTUISecretsBrowse(t *testing.T) {
	vb := tuikit.NewVirtualBackend(72, 18)
	fake := &secretsFake{}
	type result struct {
		out []native.Value
		err error
	}
	done := make(chan result, 1)
	go func() {
		out, err := runVaultTuiSteps(t, vb, fake.do, []string{
			`import "aql:vault-tui"`,
			`def final (VaultTui.run {dark: true})`,
			`convert String (size final.screens)`,
		})
		done <- result{out, err}
	}()

	pollScreen(t, vb, "Secrets")
	key(vb, "enter") // Home → Secrets table
	pollScreen(t, vb, "gh_token")
	pollScreen(t, vb, "aws_key")
	// filter down to the github row
	key(vb, "/")
	pollScreen(t, vb, "filter aliases")
	typeKeys(vb, "gh")
	key(vb, "enter")
	pollScreenGone(t, vb, "aws_key")
	pollScreen(t, vb, "gh_token")
	// open the detail page: recipes + masked value
	key(vb, "enter")
	pollScreen(t, vb, "exec recipes")
	pollScreen(t, vb, "●●●")
	// reveal, then hide again (hygiene: the value must leave the frame)
	key(vb, "r")
	pollScreen(t, vb, "sekrit-v1")
	key(vb, "r")
	pollScreen(t, vb, "●●●")
	if strings.Contains(strings.Join(vb.Screen(), "\n"), "sekrit-v1") {
		t.Fatalf("hidden value still on screen:\n%s", strings.Join(vb.Screen(), "\n"))
	}
	// copy the selected recipe
	key(vb, "c")
	pollScreen(t, vb, "copied via clip-tool")
	// back out and quit
	key(vb, "esc") // detail → table
	key(vb, "esc") // clear the applied filter
	key(vb, "esc") // table → home
	key(vb, "q")

	select {
	case res := <-done:
		if res.err != nil {
			t.Fatal(res.err)
		}
		if got := appLastString(t, res.out); got != "1" {
			t.Fatalf("final stack depth = %q", got)
		}
	case <-time.After(15 * time.Second):
		t.Fatal("the vault TUI never returned")
	}
	if p := fake.opAndParams(t, "copy"); !strings.Contains(p["text"].(string), "exec gh_token") {
		t.Fatalf("copied text = %#v", p)
	}
	if p := fake.opAndParams(t, "reveal"); p["alias"] != "gh_token" {
		t.Fatalf("reveal params = %#v", p)
	}
}

// The secrets mutation flows: the add form (masked value field,
// defaults memory), rotate, the typed-confirm delete, and grant's
// one-time token pager.
func TestAppVaultTUISecretsMutate(t *testing.T) {
	vb := tuikit.NewVirtualBackend(72, 20)
	fake := &secretsFake{}
	type result struct {
		out []native.Value
		err error
	}
	done := make(chan result, 1)
	go func() {
		out, err := runVaultTuiSteps(t, vb, fake.do, []string{
			`import "aql:vault-tui"`,
			`def final (VaultTui.run {dark: true})`,
			`(final.add-defaults).provider`,
		})
		done <- result{out, err}
	}()

	pollScreen(t, vb, "Secrets")
	key(vb, "enter")
	pollScreen(t, vb, "gh_token")

	// add: alias, masked value, provider; submit from the last field
	key(vb, "a")
	pollScreen(t, vb, "Add secret")
	typeKeys(vb, "n1")
	key(vb, "tab")
	typeKeys(vb, "v1")
	pollScreen(t, vb, "••")
	if strings.Contains(strings.Join(vb.Screen(), "\n"), "v1") {
		t.Fatalf("secret value echoed unmasked:\n%s", strings.Join(vb.Screen(), "\n"))
	}
	key(vb, "tab")
	typeKeys(vb, "openai")
	key(vb, "tab")
	key(vb, "tab")
	key(vb, "enter")
	pollScreen(t, vb, "added n1")
	p := fake.opAndParams(t, "add")
	if p["alias"] != "n1" || p["value"] != "v1" || p["provider"] != "openai" {
		t.Fatalf("add params = %#v", p)
	}

	// rotate from the detail page
	key(vb, "enter")
	pollScreen(t, vb, "exec recipes")
	key(vb, "R")
	pollScreen(t, vb, "Rotate gh_token")
	typeKeys(vb, "v2")
	key(vb, "enter")
	pollScreen(t, vb, "rotated gh_token")
	if p := fake.opAndParams(t, "rotate"); p["alias"] != "gh_token" || p["value"] != "v2" {
		t.Fatalf("rotate params = %#v", p)
	}

	// grant: agent + ttl, then the one-time token pager
	key(vb, "g")
	pollScreen(t, vb, "Grant for gh_token")
	typeKeys(vb, "a1")
	key(vb, "tab")
	typeKeys(vb, "2h")
	key(vb, "enter")
	pollScreen(t, vb, "token: tok-12345")
	if p := fake.opAndParams(t, "grant"); p["alias"] != "gh_token" || p["agent"] != "a1" || p["ttl"] != "2h" {
		t.Fatalf("grant params = %#v", p)
	}
	key(vb, "esc") // pager → detail

	// delete: the typed confirmation must match before the op runs
	key(vb, "D")
	pollScreen(t, vb, "Delete gh_token")
	typeKeys(vb, "zz")
	key(vb, "enter")
	pollScreen(t, vb, "confirmation does not match")
	key(vb, "backspace")
	key(vb, "backspace")
	typeKeys(vb, "gh_token")
	key(vb, "enter")
	pollScreen(t, vb, "removed gh_token")
	if p := fake.opAndParams(t, "remove"); p["alias"] != "gh_token" {
		t.Fatalf("remove params = %#v", p)
	}

	key(vb, "esc")
	key(vb, "q")
	select {
	case res := <-done:
		if res.err != nil {
			t.Fatal(res.err)
		}
		// the add form's inputs became the sticky defaults
		if got := appLastString(t, res.out); got != "openai" {
			t.Fatalf("add-defaults.provider = %q", got)
		}
	case <-time.After(15 * time.Second):
		t.Fatal("the vault TUI never returned")
	}
}

// The passphrase gate: a secret action on a passphrase-guarded vault
// detours through the masked passphrase form; a wrong passphrase fails
// inline and clears the field, the right one authenticates and the
// original action proceeds.
func TestAppVaultTUIPassphraseGate(t *testing.T) {
	vb := tuikit.NewVirtualBackend(72, 18)
	fake := &secretsFake{needsPass: true}
	type result struct {
		out []native.Value
		err error
	}
	done := make(chan result, 1)
	go func() {
		out, err := runVaultTuiSteps(t, vb, fake.do, []string{
			`import "aql:vault-tui"`,
			`(VaultTui.run {dark: true}) drop`,
		})
		done <- result{out, err}
	}()

	pollScreen(t, vb, "Secrets")
	key(vb, "enter")
	pollScreen(t, vb, "gh_token")
	key(vb, "enter")
	pollScreen(t, vb, "exec recipes")
	// reveal detours through the passphrase form
	key(vb, "r")
	pollScreen(t, vb, "Passphrase")
	typeKeys(vb, "x")
	pollScreen(t, vb, "•")
	if strings.Contains(strings.Join(vb.Screen(), "\n"), " x ") {
		t.Fatalf("passphrase echoed unmasked:\n%s", strings.Join(vb.Screen(), "\n"))
	}
	key(vb, "enter")
	pollScreen(t, vb, "bad passphrase")
	typeKeys(vb, "pw")
	key(vb, "enter")
	// authenticated → the gated reveal ran
	pollScreen(t, vb, "sekrit-v1")
	key(vb, "esc")
	key(vb, "esc")
	key(vb, "q")
	select {
	case res := <-done:
		if res.err != nil {
			t.Fatal(res.err)
		}
	case <-time.After(15 * time.Second):
		t.Fatal("the vault TUI never returned")
	}
	fake.mu.Lock()
	auths := 0
	for _, op := range fake.ops {
		if op == "authenticate" {
			auths++
		}
	}
	fake.mu.Unlock()
	if auths != 2 {
		t.Fatalf("authenticate calls = %d, want 2", auths)
	}
}

// The `:` palette, the `?` help overlay, and the `T` theme cycle: the
// palette navigates and runs ops, unknown words land in the status
// line, help swaps the body and closes on any key, and the theme
// persists through Vault.set-prefs.
func TestAppVaultTUIPaletteHelpTheme(t *testing.T) {
	vb := tuikit.NewVirtualBackend(76, 20)
	fake := &secretsFake{}
	type result struct {
		out []native.Value
		err error
	}
	done := make(chan result, 1)
	go func() {
		out, err := runVaultTuiSteps(t, vb, fake.do, []string{
			`import "aql:vault-tui"`,
			`def final (VaultTui.run {dark: true})`,
			`final.theme`,
		})
		done <- result{out, err}
	}()

	pollScreen(t, vb, "Secrets")
	// palette → audit pager (a real bridge capture)
	key(vb, ":")
	pollScreen(t, vb, ":command")
	typeKeys(vb, "audit")
	key(vb, "enter")
	pollScreen(t, vb, "Home › Audit")
	key(vb, "esc")
	// palette → lock runs the op and reports
	key(vb, ":")
	pollScreen(t, vb, ":command")
	typeKeys(vb, "lock")
	key(vb, "enter")
	pollScreen(t, vb, "locked")
	// palette → unknown word lands in the status line
	key(vb, ":")
	pollScreen(t, vb, ":command")
	typeKeys(vb, "zzz")
	key(vb, "enter")
	pollScreen(t, vb, "unknown command: zzz")
	// palette → secrets navigates from anywhere
	key(vb, ":")
	pollScreen(t, vb, ":command")
	typeKeys(vb, "secrets")
	key(vb, "enter")
	pollScreen(t, vb, "gh_token")
	// help overlay swaps the body; any key closes it
	key(vb, "?")
	pollScreen(t, vb, "palette words")
	key(vb, "x")
	pollScreenGone(t, vb, "palette words")
	// theme cycle persists and reports (auto → dark)
	key(vb, "T")
	pollScreen(t, vb, "theme: dark")
	if p := fake.opAndParams(t, "set-prefs"); p["theme"] != "dark" {
		t.Fatalf("set-prefs params = %#v", p)
	}
	// palette → quit ends the app
	key(vb, ":")
	pollScreen(t, vb, ":command")
	typeKeys(vb, "quit")
	key(vb, "enter")

	select {
	case res := <-done:
		if res.err != nil {
			t.Fatal(res.err)
		}
		if got := appLastString(t, res.out); got != "dark" {
			t.Fatalf("final theme = %q", got)
		}
	case <-time.After(15 * time.Second):
		t.Fatal("the vault TUI never returned")
	}
	fake.opAndParams(t, "audit")
	fake.opAndParams(t, "lock")
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
