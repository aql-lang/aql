package vault

import (
	"strings"
	"testing"
	"time"
)

func TestHumanizeDurRounds(t *testing.T) {
	cases := []struct {
		d    time.Duration
		want string
	}{
		// The reported bug: an expiry set to now+90d, read a moment later.
		{90*24*time.Hour - 5*time.Millisecond, "90d"},
		{90 * 24 * time.Hour, "90d"},
		{24 * time.Hour, "1d"},
		{36 * time.Hour, "2d"}, // 1.5d rounds up
		{34 * time.Hour, "1d"}, // 1.42d rounds down
		// No boundary artifacts: just under a day stays in hours, just
		// under an hour stays in minutes.
		{23*time.Hour + 45*time.Minute, "1d"}, // rounds up into days, not "24h"
		{23*time.Hour + 20*time.Minute, "23h"},
		{59*time.Minute + 45*time.Second, "1h"}, // rounds up into hours, not "60m"
		{59*time.Minute + 20*time.Second, "59m"},
		{45 * time.Second, "1m"},
		{20 * time.Second, "<1m"},
	}
	for _, c := range cases {
		if got := humanizeDur(c.d); got != c.want {
			t.Errorf("humanizeDur(%s) = %q, want %q", c.d, got, c.want)
		}
	}
}

func TestExpiryStatusFreshlySet(t *testing.T) {
	// A key added with `--expiry=90d` and rendered immediately must read
	// "in 90d", not "in 89d".
	now := time.Now()
	expires := now.Add(90 * 24 * time.Hour)
	if got := expiryStatus(expires, now.Add(8*time.Millisecond)); got != "in 90d" {
		t.Errorf("freshly-set 90d expiry = %q, want %q", got, "in 90d")
	}
}

func TestParseExpiryAbsolute(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	cases := []struct {
		in      string
		want    string
		wantErr bool
	}{
		{"2026-12-31", "2026-12-31T00:00:00Z", false},
		{"2026-12-31T23:59:59Z", "2026-12-31T23:59:59Z", false},
		{"2026-12-31T12:00:00+02:00", "2026-12-31T10:00:00Z", false}, // normalized to UTC
		{"", "", true},
		{"garbage", "", true},
		{"-5d", "", true}, // negative duration
		{"0d", "", true},  // zero duration
	}
	for _, c := range cases {
		got, err := parseExpiry(c.in, now)
		if c.wantErr {
			if err == nil {
				t.Errorf("parseExpiry(%q): want error, got %q", c.in, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("parseExpiry(%q): unexpected error %v", c.in, err)
			continue
		}
		if got != c.want {
			t.Errorf("parseExpiry(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestParseExpiryRelative(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	got, err := parseExpiry("90d", now)
	if err != nil {
		t.Fatalf("parseExpiry(90d): %v", err)
	}
	want := now.Add(90 * 24 * time.Hour).Format(time.RFC3339)
	if got != want {
		t.Errorf("parseExpiry(90d) = %q, want %q", got, want)
	}
}

func TestParseDurationDays(t *testing.T) {
	cases := []struct {
		in      string
		want    time.Duration
		wantErr bool
	}{
		{"90d", 90 * 24 * time.Hour, false},
		{"1d", 24 * time.Hour, false},
		{"720h", 720 * time.Hour, false},
		{"30d12h", 30*24*time.Hour + 12*time.Hour, false},
		{"2h30m", 2*time.Hour + 30*time.Minute, false},
		{"garbage", 0, true},
		{"5x", 0, true},
		{"d", 0, true},
		{"1.5d", 0, true},
	}
	for _, c := range cases {
		got, err := parseDurationDays(c.in)
		if c.wantErr {
			if err == nil {
				t.Errorf("parseDurationDays(%q): want error, got %v", c.in, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("parseDurationDays(%q): unexpected error %v", c.in, err)
			continue
		}
		if got != c.want {
			t.Errorf("parseDurationDays(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}

func TestAddWithExpiryShownInList(t *testing.T) {
	home := testHome(t)
	mustInit(t)

	code, out, errOut := runVault(t, "sk-1\n", "add", "--from-stdin", "--expiry=2030-01-15", "openai")
	if code != 0 {
		t.Fatalf("add: %s", errOut)
	}
	if !strings.Contains(out, "expires 2030-01-15T00:00:00Z") {
		t.Errorf("add output missing expiry confirmation: %q", out)
	}

	s, _ := LoadStore(home)
	a, _ := s.FindAlias("openai")
	if a == nil || a.ExpiresAt != "2030-01-15T00:00:00Z" {
		t.Fatalf("expiry not stored: %+v", a)
	}

	code, out, _ = runVault(t, "", "list")
	if code != 0 {
		t.Fatal("list")
	}
	if !strings.Contains(out, "EXPIRES") {
		t.Errorf("list header missing EXPIRES column: %q", out)
	}
	if !strings.Contains(out, "2030-01-15T00:00:00Z") {
		t.Errorf("list missing expiry value: %q", out)
	}
}

func TestAddRejectsBadExpiry(t *testing.T) {
	testHome(t)
	mustInit(t)
	code, _, errOut := runVault(t, "sk\n", "add", "--from-stdin", "--expiry=nonsense", "k")
	if code == 0 {
		t.Fatal("expected non-zero exit for invalid expiry")
	}
	if !strings.Contains(errOut, "invalid expiry") {
		t.Errorf("missing invalid-expiry message: %q", errOut)
	}
}

func TestExpiryListSortedAndNamespaceFiltered(t *testing.T) {
	testHome(t)
	mustInit(t)

	// bravo expires first, then alpha, then charlie.
	if code, _, e := runVault(t, "v\n", "add", "--from-stdin", "--expiry=2030-06-01", "alpha"); code != 0 {
		t.Fatalf("add alpha: %s", e)
	}
	if code, _, e := runVault(t, "v\n", "add", "--from-stdin", "--expiry=2029-06-01", "proj:bravo"); code != 0 {
		t.Fatalf("add bravo: %s", e)
	}
	if code, _, e := runVault(t, "v\n", "add", "--from-stdin", "--expiry=2031-06-01", "proj:charlie"); code != 0 {
		t.Fatalf("add charlie: %s", e)
	}
	// A key with no expiry must never appear in the expiry report.
	if code, _, e := runVault(t, "v\n", "add", "--from-stdin", "noexp"); code != 0 {
		t.Fatalf("add noexp: %s", e)
	}

	code, out, errOut := runVault(t, "", "expiry")
	if code != 0 {
		t.Fatalf("expiry: %s", errOut)
	}
	if strings.Contains(out, "noexp") {
		t.Errorf("expiry listed a key with no expiry: %q", out)
	}
	posB, posA, posC := strings.Index(out, "bravo"), strings.Index(out, "alpha"), strings.Index(out, "charlie")
	if posB < 0 || posA < 0 || posC < 0 {
		t.Fatalf("expiry missing an alias: %q", out)
	}
	if !(posB < posA && posA < posC) {
		t.Errorf("expiry not sorted soonest-first: %q", out)
	}

	// Filter to the proj namespace.
	code, out, _ = runVault(t, "", "expiry", "--namespace=proj")
	if code != 0 {
		t.Fatal("expiry --namespace=proj")
	}
	if strings.Contains(out, "alpha") {
		t.Errorf("namespace filter leaked root alias: %q", out)
	}
	if !strings.Contains(out, "bravo") || !strings.Contains(out, "charlie") {
		t.Errorf("namespace filter dropped proj aliases: %q", out)
	}

	// Filter to root only.
	code, out, _ = runVault(t, "", "expiry", "--namespace=:")
	if code != 0 {
		t.Fatal("expiry --namespace=:")
	}
	if !strings.Contains(out, "alpha") {
		t.Errorf("root filter missing root alias: %q", out)
	}
	if strings.Contains(out, "bravo") || strings.Contains(out, "charlie") {
		t.Errorf("root filter leaked proj aliases: %q", out)
	}
}

func TestExpiryWithinWindow(t *testing.T) {
	testHome(t)
	mustInit(t)
	if code, _, e := runVault(t, "v\n", "add", "--from-stdin", "--expiry=10d", "near"); code != 0 {
		t.Fatalf("add near: %s", e)
	}
	if code, _, e := runVault(t, "v\n", "add", "--from-stdin", "--expiry=400d", "far"); code != 0 {
		t.Fatalf("add far: %s", e)
	}
	code, out, errOut := runVault(t, "", "expiry", "--within=30d")
	if code != 0 {
		t.Fatalf("expiry --within: %s", errOut)
	}
	if !strings.Contains(out, "near") {
		t.Errorf("--within dropped the near key: %q", out)
	}
	if strings.Contains(out, "far") {
		t.Errorf("--within included the far key: %q", out)
	}
}

func TestExpiryStatusOverdue(t *testing.T) {
	testHome(t)
	mustInit(t)
	if code, _, e := runVault(t, "v\n", "add", "--from-stdin", "--expiry=2000-01-01", "stale"); code != 0 {
		t.Fatalf("add stale: %s", e)
	}
	code, out, _ := runVault(t, "", "expiry")
	if code != 0 {
		t.Fatal("expiry")
	}
	if !strings.Contains(out, "expired") {
		t.Errorf("overdue key not marked expired: %q", out)
	}
}

func TestExpirySetAndClear(t *testing.T) {
	home := testHome(t)
	mustInit(t)
	if code, _, e := runVault(t, "v\n", "add", "--from-stdin", "k"); code != 0 {
		t.Fatalf("add: %s", e)
	}

	// Set an expiry on the existing key without rotating its value.
	code, out, errOut := runVault(t, "", "expiry", "set", "k", "2030-01-01")
	if code != 0 {
		t.Fatalf("expiry set: %s", errOut)
	}
	if !strings.Contains(out, "2030-01-01T00:00:00Z") {
		t.Errorf("set output missing expiry: %q", out)
	}
	s, _ := LoadStore(home)
	if a, _ := s.FindAlias("k"); a == nil || a.ExpiresAt != "2030-01-01T00:00:00Z" {
		t.Fatalf("expiry not set: %+v", a)
	}

	// Clear it.
	code, _, errOut = runVault(t, "", "expiry", "clear", "k")
	if code != 0 {
		t.Fatalf("expiry clear: %s", errOut)
	}
	s, _ = LoadStore(home)
	if a, _ := s.FindAlias("k"); a == nil || a.ExpiresAt != "" {
		t.Fatalf("expiry not cleared: %+v", a)
	}
	code, out, _ = runVault(t, "", "expiry")
	if code != 0 {
		t.Fatal("expiry list after clear")
	}
	if !strings.Contains(out, "no pending key expiries") {
		t.Errorf("expected empty listing after clear, got %q", out)
	}
}

func TestRotatePreservesAndUpdatesExpiry(t *testing.T) {
	home := testHome(t)
	mustInit(t)
	if code, _, e := runVault(t, "v1\n", "add", "--from-stdin", "--expiry=2030-01-01", "k"); code != 0 {
		t.Fatalf("add: %s", e)
	}

	// Rotating the value without --expiry keeps the existing reminder.
	if code, _, e := runVault(t, "v2\n", "rotate", "--yes", "--from-stdin", "k"); code != 0 {
		t.Fatalf("rotate: %s", e)
	}
	s, _ := LoadStore(home)
	if a, _ := s.FindAlias("k"); a == nil || a.ExpiresAt != "2030-01-01T00:00:00Z" {
		t.Fatalf("rotate dropped expiry: %+v", a)
	}

	// Rotating with --expiry updates it.
	if code, _, e := runVault(t, "v3\n", "rotate", "--yes", "--from-stdin", "--expiry=2031-02-02", "k"); code != 0 {
		t.Fatalf("rotate --expiry: %s", e)
	}
	s, _ = LoadStore(home)
	if a, _ := s.FindAlias("k"); a == nil || a.ExpiresAt != "2031-02-02T00:00:00Z" {
		t.Fatalf("rotate did not update expiry: %+v", a)
	}
}

func TestAliasExpiryRoundTrip(t *testing.T) {
	dir := t.TempDir()
	s := &Store{Version: storeVersion, Backend: BackendFile}
	s.UpsertAlias(Alias{Name: "k", ExpiresAt: "2030-01-01T00:00:00Z"})
	if err := SaveStore(dir, s); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	if a, _ := loaded.FindAlias("k"); a == nil || a.ExpiresAt != "2030-01-01T00:00:00Z" {
		t.Fatalf("expiry not persisted: %+v", a)
	}
	// An empty incoming expiry must not wipe a stored one.
	loaded.UpsertAlias(Alias{Name: "k", Provider: "openai"})
	if a, _ := loaded.FindAlias("k"); a == nil || a.ExpiresAt != "2030-01-01T00:00:00Z" {
		t.Fatalf("re-upsert wiped expiry: %+v", a)
	}
}
