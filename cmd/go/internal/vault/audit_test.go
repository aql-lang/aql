package vault

import (
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"testing"
)

// findEvent returns the first audit event whose Action matches.
// Fails the test if no such event exists.
func findEvent(t *testing.T, events []AuditEvent, action string) AuditEvent {
	t.Helper()
	for _, ev := range events {
		if ev.Action == action {
			return ev
		}
	}
	t.Fatalf("no event with action=%q in %v", action, events)
	return AuditEvent{}
}

func TestAuditInitAddRm(t *testing.T) {
	home := testHome(t)
	mustInit(t)

	if code, _, errOut := runVault(t, "v\n", "add",
		"--from-stdin", "--provider=openai", "k"); code != 0 {
		t.Fatalf("add: %s", errOut)
	}
	if code, _, errOut := runVault(t, "", "rm", "k"); code != 0 {
		t.Fatalf("rm: %s", errOut)
	}

	events, err := ReadAudit(home)
	if err != nil {
		t.Fatal(err)
	}
	initEv := findEvent(t, events, "vault.init")
	if !strings.Contains(initEv.Reason, "backend=file") {
		t.Errorf("init event missing backend tag: %+v", initEv)
	}
	addEv := findEvent(t, events, "vault.add")
	if addEv.Alias != "k" || addEv.Provider != "openai" {
		t.Errorf("add event missing fields: %+v", addEv)
	}
	rmEv := findEvent(t, events, "vault.rm")
	if rmEv.Alias != "k" {
		t.Errorf("rm event missing alias: %+v", rmEv)
	}
}

func TestAuditGrantRevokeLockUnlock(t *testing.T) {
	home := testHome(t)
	mustInit(t)

	if code, _, _ := runVault(t, "v\n", "add", "--from-stdin", "--provider=openai", "k"); code != 0 {
		t.Fatal("add")
	}
	if code, out, _ := runVault(t, "", "grant",
		"--agent=claude", "--hosts=api.example.com", "--ttl=1h", "k"); code != 0 {
		t.Fatalf("grant: %s", out)
	}
	if code, _, _ := runVault(t, "", "lock"); code != 0 {
		t.Fatal("lock")
	}
	if code, _, _ := runVault(t, "", "unlock"); code != 0 {
		t.Fatal("unlock")
	}

	events, err := ReadAudit(home)
	if err != nil {
		t.Fatal(err)
	}
	grantEv := findEvent(t, events, "vault.grant")
	if grantEv.Alias != "k" || grantEv.Agent != "claude" || grantEv.Capability == "" {
		t.Errorf("grant event missing fields: %+v", grantEv)
	}
	// Revoke using the capability id from the grant event.
	if code, _, errOut := runVault(t, "", "revoke", grantEv.Capability); code != 0 {
		t.Fatalf("revoke: %s", errOut)
	}
	events, _ = ReadAudit(home)
	revEv := findEvent(t, events, "vault.revoke")
	if revEv.Capability != grantEv.Capability {
		t.Errorf("revoke event cap mismatch: %+v vs %+v", revEv, grantEv)
	}
	findEvent(t, events, "vault.lock")
	findEvent(t, events, "vault.unlock")
}

func TestAuditProxyRequest(t *testing.T) {
	home := testHome(t)
	mustInit(t)
	fu := newFakeUpstream(t)
	registerTestProvider(t, "fake", fu, "bearer")

	if code, _, _ := runVault(t, "v\n", "add", "--from-stdin", "--provider=fake", "k"); code != 0 {
		t.Fatal("add")
	}
	tok := grantOK(t, "k", nil, nil)

	base := startProxy(t)
	req, _ := http.NewRequest("GET", base+"/k/foo", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()

	events, err := ReadAudit(home)
	if err != nil {
		t.Fatal(err)
	}
	pr := findEvent(t, events, "proxy.request")
	if pr.Alias != "k" || pr.Method != "GET" || pr.Status != 200 || pr.Outcome != "ok" {
		t.Errorf("proxy.request event missing fields: %+v", pr)
	}
	// Audit log must not contain the capability token or any
	// upstream secret material.
	raw, _ := os.ReadFile(auditPath(home))
	if strings.Contains(string(raw), tok) {
		t.Errorf("audit log leaked capability token")
	}
}

func TestAuditModeFiltersAndJSON(t *testing.T) {
	testHome(t)
	mustInit(t)
	if code, _, _ := runVault(t, "v\n", "add", "--from-stdin", "--provider=openai", "a"); code != 0 {
		t.Fatal("add a")
	}
	if code, _, _ := runVault(t, "v\n", "add", "--from-stdin", "--provider=anthropic", "b"); code != 0 {
		t.Fatal("add b")
	}

	// Default: prints all events including init + two adds.
	code, out, _ := runVault(t, "", "audit")
	if code != 0 {
		t.Fatal("audit")
	}
	if !strings.Contains(out, "vault.init") ||
		!strings.Contains(out, "alias=a") ||
		!strings.Contains(out, "alias=b") {
		t.Errorf("audit default missing events: %q", out)
	}

	// Filter by action.
	code, out, _ = runVault(t, "", "audit", "--action=vault.add")
	if code != 0 {
		t.Fatal("audit --action")
	}
	if strings.Contains(out, "vault.init") {
		t.Errorf("init not filtered out: %q", out)
	}

	// Filter by alias.
	code, out, _ = runVault(t, "", "audit", "--alias=a")
	if code != 0 {
		t.Fatal("audit --alias")
	}
	if strings.Contains(out, "alias=b") {
		t.Errorf("alias filter leaked b: %q", out)
	}

	// JSON output must parse round-trip.
	code, out, _ = runVault(t, "", "audit", "--action=vault.add", "--json")
	if code != 0 {
		t.Fatal("audit --json")
	}
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		var ev AuditEvent
		if err := json.Unmarshal([]byte(line), &ev); err != nil {
			t.Errorf("non-JSON line: %q (%s)", line, err)
		}
	}

	// --last bounds output.
	code, out, _ = runVault(t, "", "audit", "--last=1")
	if code != 0 {
		t.Fatal("audit --last")
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) != 1 {
		t.Errorf("--last=1 expected 1 line, got %d (%q)", len(lines), out)
	}
}

func TestAuditConfigDisable(t *testing.T) {
	home := testHome(t)
	mustInit(t)
	// Disable audit, then perform a mutation that would normally
	// write an event.
	if code, _, _ := runVault(t, "", "config", "--set=audit.enabled=false"); code != 0 {
		t.Fatal("config")
	}
	preCount := func() int {
		evs, _ := ReadAudit(home)
		return len(evs)
	}
	before := preCount()
	if code, _, _ := runVault(t, "v\n", "add", "--from-stdin", "k"); code != 0 {
		t.Fatal("add")
	}
	if got := preCount(); got != before {
		t.Errorf("expected no new audit events while disabled; before=%d after=%d", before, got)
	}
}

func TestAuditModeMissingLog(t *testing.T) {
	testHome(t)
	// Note: no init, no events. The audit mode should report the
	// empty log gracefully.
	code, out, _ := runVault(t, "", "audit")
	if code != 0 {
		t.Fatalf("audit on empty log: exit %d (%s)", code, out)
	}
	if !strings.Contains(out, "no audit") {
		t.Errorf("expected empty-log message, got %q", out)
	}
}

func TestFormatAuditLine(t *testing.T) {
	ev := AuditEvent{
		Timestamp:  "2026-05-24T22:00:00Z",
		Action:     "proxy.request",
		Actor:      "proxy",
		Alias:      "openai",
		Agent:      "claude",
		Capability: "abcdef0123456789",
		Method:     "POST",
		Host:       "api.openai.com",
		Path:       "/v1/chat/completions",
		Status:     200,
		Outcome:    "ok",
	}
	line := formatAuditLine(ev)
	for _, want := range []string{
		"action=proxy.request", "alias=openai", "agent=claude",
		"cap=abcdef01", "method=POST", "host=api.openai.com",
		"status=200", "outcome=ok",
	} {
		if !strings.Contains(line, want) {
			t.Errorf("formatted line missing %q in %q", want, line)
		}
	}
	if strings.Contains(line, ev.Capability) {
		t.Errorf("formatted line leaked full cap id: %q", line)
	}
}
