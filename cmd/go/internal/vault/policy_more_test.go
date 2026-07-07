package vault

// policy_more_test.go — policy verb dispatch and TTL parsing edges.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

func TestPolicyDispatchAndApplyEdges(t *testing.T) {
	home := testHome(t)
	mustInit(t)
	if code, _, errOut := runVault(t, "", "policy"); code == 0 || !strings.Contains(errOut, "usage") {
		t.Errorf("bare policy: %q", errOut)
	}
	if code, _, errOut := runVault(t, "", "policy", "destroy"); code == 0 ||
		!strings.Contains(errOut, "unknown policy verb") {
		t.Errorf("unknown verb: %q", errOut)
	}
	if code, _, errOut := runVault(t, "", "policy", "apply"); code == 0 ||
		!strings.Contains(errOut, "usage") {
		t.Errorf("apply without a file: %q", errOut)
	}
	if code, _, errOut := runVault(t, "", "policy", "apply", filepath.Join(home, "nope.json")); code == 0 ||
		errOut == "" {
		t.Errorf("missing policy file: %q", errOut)
	}
	bad := filepath.Join(home, "bad.json")
	if err := os.WriteFile(bad, []byte("{oops"), 0o600); err != nil {
		t.Fatal(err)
	}
	if code, _, errOut := runVault(t, "", "policy", "apply", bad); code == 0 ||
		!strings.Contains(errOut, "parsing") {
		t.Errorf("unparseable policy: %q", errOut)
	}
}

func TestParsePolicyTTL(t *testing.T) {
	if d, err := parsePolicyTTL(""); err != nil || d != 2*time.Hour {
		t.Errorf("default = %v, %v", d, err)
	}
	if d, err := parsePolicyTTL("30m"); err != nil || d != 30*time.Minute {
		t.Errorf("30m = %v, %v", d, err)
	}
	if _, err := parsePolicyTTL("soon"); err == nil {
		t.Error("garbage ttl should error")
	}
	if _, err := parsePolicyTTL("-1h"); err == nil {
		t.Error("negative ttl should error")
	}
	if _, err := parsePolicyTTL("0s"); err == nil {
		t.Error("zero ttl should error")
	}
}

func TestScanFormSubmitOpensPager(t *testing.T) {
	ctl, _ := newTestController(t)
	m := newRootModel(ctl)
	fs := m.buildScanForm().(*formScreen)
	// Point the scan at an empty temp dir (ctrl+u clears the "." default).
	dir := t.TempDir()
	msgs := []tea.Msg{
		tea.WindowSizeMsg{Width: 80, Height: 24},
		tea.KeyMsg{Type: tea.KeyCtrlU},
	}
	for _, r := range dir {
		msgs = append(msgs, keyMsg(string(r)))
	}
	msgs = append(msgs, tea.KeyMsg{Type: tea.KeyEnter})
	_, seen := pumpScreen(t, fs, msgs...)
	var opened bool
	for _, msg := range seen {
		if pm, ok := msg.(pushMsg); ok && pm.s.Title() == "scan" {
			opened = true
		}
	}
	if !opened {
		t.Fatalf("scan submit should open the scan pager: %+v", seen)
	}
}
