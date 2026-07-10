package vault

import (
	"net/http"
	"strings"
	"testing"
)

func TestParseIPWhitelist(t *testing.T) {
	if got, err := parseIPWhitelist("  "); err != nil || got != nil {
		t.Errorf("empty should be (nil,nil): got %v err %v", got, err)
	}
	// valid IPs + CIDR, whitespace-trimmed, de-duplicated, insertion order.
	got, err := parseIPWhitelist("203.0.113.7, 10.0.0.0/8 , 203.0.113.7, 2001:db8::1")
	if err != nil {
		t.Fatalf("valid: %v", err)
	}
	want := []string{"203.0.113.7", "10.0.0.0/8", "2001:db8::1"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("got %v, want %v", got, want)
	}
	// CIDR host bits are masked to the canonical network.
	if g, _ := parseIPWhitelist("10.1.2.3/8"); len(g) != 1 || g[0] != "10.0.0.0/8" {
		t.Errorf("CIDR normalize: %v", g)
	}
	// Empty CSV parts are skipped; an all-empty (comma-only) list yields no
	// entries and returns (nil,nil) like an absent whitelist.
	if got, err := parseIPWhitelist(","); err != nil || got != nil {
		t.Errorf("comma-only should be (nil,nil): got %v err %v", got, err)
	}
	if g, _ := parseIPWhitelist("203.0.113.7,,10.0.0.0/8"); len(g) != 2 {
		t.Errorf("empty middle part must be skipped, keeping the rest: %v", g)
	}
	// A typo must be an error, never a silent widening.
	for _, bad := range []string{"not-an-ip", "999.1.1.1", "10.0.0.0/99", "1.2.3.4,garbage"} {
		if _, err := parseIPWhitelist(bad); err == nil {
			t.Errorf("expected error for %q", bad)
		}
	}
}

func TestIPAllowed(t *testing.T) {
	if !ipAllowed("8.8.8.8", nil) {
		t.Error("empty whitelist must allow everything")
	}
	wl := []string{"203.0.113.7", "10.0.0.0/8"}
	for _, c := range []struct {
		ip   string
		want bool
	}{
		{"203.0.113.7", true},  // exact IP
		{"203.0.113.8", false}, // unlisted
		{"10.5.6.7", true},     // inside CIDR
		{"11.0.0.1", false},    // outside CIDR
		{"", false},            // unparseable client IP
		{"garbage", false},
	} {
		if got := ipAllowed(c.ip, wl); got != c.want {
			t.Errorf("ipAllowed(%q) = %v, want %v", c.ip, got, c.want)
		}
	}
	if !ipAllowed("2001:db8::5", []string{"2001:db8::/32"}) {
		t.Error("IPv6 CIDR should match")
	}
}

func TestClientIP(t *testing.T) {
	if got := clientIP("1.2.3.4:5678"); got != "1.2.3.4" {
		t.Errorf("clientIP = %q", got)
	}
	if got := clientIP("[2001:db8::1]:443"); got != "2001:db8::1" {
		t.Errorf("clientIP v6 = %q", got)
	}
	if got := clientIP("no-port"); got != "" {
		t.Errorf("malformed should be empty: %q", got)
	}
}

func TestAddIPWhitelistStoredAndListed(t *testing.T) {
	home := testHome(t)
	mustInit(t)
	code, out, e := runVault(t, "v\n", "add", "--from-stdin", "--ip-whitelist=10.0.0.0/8,203.0.113.7", "k")
	if code != 0 {
		t.Fatalf("add: %s", e)
	}
	if !strings.Contains(out, "ip-whitelist 10.0.0.0/8, 203.0.113.7") {
		t.Errorf("add confirmation missing whitelist: %q", out)
	}
	s, _ := LoadStore(home)
	a, _ := s.FindAlias("k")
	if a == nil || len(a.IPWhitelist) != 2 || a.IPWhitelist[0] != "10.0.0.0/8" {
		t.Fatalf("stored whitelist wrong: %+v", a)
	}
	code, out, _ = runVault(t, "", "list")
	if code != 0 || !strings.Contains(out, "IP-WHITELIST") || !strings.Contains(out, "10.0.0.0/8,203.0.113.7") {
		t.Errorf("list missing whitelist column/value: %q", out)
	}
}

func TestAddRejectsBadIPWhitelist(t *testing.T) {
	testHome(t)
	mustInit(t)
	code, _, errOut := runVault(t, "v\n", "add", "--from-stdin", "--ip-whitelist=not-an-ip", "k")
	if code == 0 || !strings.Contains(errOut, "ip-whitelist") {
		t.Errorf("bad ip-whitelist should error, got code=%d err=%q", code, errOut)
	}
}

func TestRotateIPWhitelistSetClearPreserve(t *testing.T) {
	home := testHome(t)
	mustInit(t)
	if code, _, e := runVault(t, "v\n", "add", "--from-stdin", "--ip-whitelist=10.0.0.0/8", "k"); code != 0 {
		t.Fatalf("add: %s", e)
	}
	// A bare rotation preserves the whitelist.
	if code, _, e := runVault(t, "v2\n", "rotate", "--from-stdin", "-y", "k"); code != 0 {
		t.Fatalf("rotate: %s", e)
	}
	if s, _ := LoadStore(home); func() bool { a, _ := s.FindAlias("k"); return a == nil || len(a.IPWhitelist) != 1 }() {
		t.Error("bare rotate must preserve the whitelist")
	}
	// --ip-whitelist replaces.
	runVault(t, "v3\n", "rotate", "--from-stdin", "-y", "--ip-whitelist=192.168.0.0/16", "k")
	if s, _ := LoadStore(home); func() bool {
		a, _ := s.FindAlias("k")
		return a == nil || len(a.IPWhitelist) != 1 || a.IPWhitelist[0] != "192.168.0.0/16"
	}() {
		t.Error("rotate --ip-whitelist must replace")
	}
	// --ip-whitelist='' clears (present-but-empty, distinct from absent).
	runVault(t, "v4\n", "rotate", "--from-stdin", "-y", "--ip-whitelist=", "k")
	if s, _ := LoadStore(home); func() bool { a, _ := s.FindAlias("k"); return a == nil || len(a.IPWhitelist) != 0 }() {
		t.Error("rotate --ip-whitelist='' must clear")
	}
}

// TestAddIPWhitelistOverwriteTriState mirrors rotate's tri-state on the add
// overwrite path: absent keeps the prior allowlist, an explicit empty flag
// clears it (so `add --ip-whitelist=` cannot silently leave a stale restriction).
func TestAddIPWhitelistOverwriteTriState(t *testing.T) {
	home := testHome(t)
	mustInit(t)
	if code, _, e := runVault(t, "v1\n", "add", "--from-stdin", "--ip-whitelist=10.0.0.0/8", "k"); code != 0 {
		t.Fatalf("add: %s", e)
	}
	// Overwrite WITHOUT the flag preserves the whitelist.
	runVault(t, "v2\n", "add", "--yes", "--from-stdin", "k")
	if s, _ := LoadStore(home); func() bool { a, _ := s.FindAlias("k"); return a == nil || len(a.IPWhitelist) != 1 }() {
		t.Error("overwrite without --ip-whitelist must preserve the whitelist")
	}
	// Overwrite WITH an explicit empty flag clears it.
	runVault(t, "v3\n", "add", "--yes", "--from-stdin", "--ip-whitelist=", "k")
	if s, _ := LoadStore(home); func() bool { a, _ := s.FindAlias("k"); return a == nil || len(a.IPWhitelist) != 0 }() {
		t.Error("add --ip-whitelist= must clear the prior whitelist")
	}
}

func TestProxyEnforcesIPWhitelist(t *testing.T) {
	testHome(t)
	mustInit(t)
	fu := newFakeUpstream(t)
	registerTestProvider(t, "fake", fu, "bearer")
	// In-test requests come from 127.0.0.1: "denied" excludes it, "allowed"
	// covers it. Host/method allow anything, so only the IP gate differs.
	if code, _, _ := runVault(t, "v\n", "add", "--from-stdin", "--provider=fake", "--ip-whitelist=10.0.0.0/8", "denied"); code != 0 {
		t.Fatal("add denied")
	}
	if code, _, _ := runVault(t, "v\n", "add", "--from-stdin", "--provider=fake", "--ip-whitelist=127.0.0.0/8", "allowed"); code != 0 {
		t.Fatal("add allowed")
	}
	base := startProxy(t)

	req, _ := http.NewRequest("GET", base+"/denied/foo", nil)
	req.Header.Set("Authorization", "Bearer "+grantOK(t, "denied", []string{mustHost(fu.URL)}, nil))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("off-whitelist IP: status=%d, want 403", resp.StatusCode)
	}

	req2, _ := http.NewRequest("GET", base+"/allowed/foo", nil)
	req2.Header.Set("Authorization", "Bearer "+grantOK(t, "allowed", []string{mustHost(fu.URL)}, nil))
	resp2, err := http.DefaultClient.Do(req2)
	if err != nil {
		t.Fatal(err)
	}
	resp2.Body.Close()
	if resp2.StatusCode == http.StatusForbidden {
		t.Errorf("loopback-whitelisted IP wrongly denied: status=%d", resp2.StatusCode)
	}
}

func TestIPWhitelistIPv6(t *testing.T) {
	// Parse: IPv6 literals and CIDRs are valid and canonicalized.
	got, err := parseIPWhitelist("::1, 2001:DB8::1, 2001:db8:abcd::/48, fe80::/10")
	if err != nil {
		t.Fatalf("ipv6 parse: %v", err)
	}
	// 2001:DB8::1 lowercases; the /48 masks to its network.
	want := []string{"::1", "2001:db8::1", "2001:db8:abcd::/48", "fe80::/10"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("ipv6 parse got %v, want %v", got, want)
	}

	// Match: exact v6, v6 CIDR in/out, and IPv6 loopback.
	for _, c := range []struct {
		ip, list string
		want     bool
	}{
		{"2001:db8::5", "2001:db8::/32", true},
		{"2001:db9::5", "2001:db8::/32", false},
		{"::1", "::1", true},
		{"fe80::1234", "fe80::/10", true},
		// Dual-stack: an IPv4-mapped-IPv6 client (::ffff:10.0.0.5, as a
		// dual-stack listener may report) still matches an IPv4 rule.
		{"::ffff:10.0.0.5", "10.0.0.0/8", true},
		{"::ffff:127.0.0.1", "127.0.0.1", true},
	} {
		if got := ipAllowed(c.ip, []string{c.list}); got != c.want {
			t.Errorf("ipAllowed(%q, %q) = %v, want %v", c.ip, c.list, got, c.want)
		}
	}

	// clientIP extracts a bracketed IPv6 host (proxy RemoteAddr form).
	if got := clientIP("[::1]:8787"); got != "::1" {
		t.Errorf("clientIP ipv6 loopback = %q", got)
	}
}

// TestParseIPWhitelistSkipsEmptyParts — an interior/trailing empty part is
// skipped (a trailing comma is not a typo that widens access), and an
// ALL-empty but non-blank csv ("," / ", ,") yields (nil, nil) exactly like
// absent input.
func TestParseIPWhitelistSkipsEmptyParts(t *testing.T) {
	got, err := parseIPWhitelist("203.0.113.7,,")
	if err != nil || len(got) != 1 || got[0] != "203.0.113.7" {
		t.Errorf("trailing empty part must be skipped: got=%v err=%v", got, err)
	}
	if got, err := parseIPWhitelist(", ,"); err != nil || got != nil {
		t.Errorf("all-empty parts must yield (nil, nil): got=%v err=%v", got, err)
	}
}

// TestRotateRejectsBadIPWhitelist — rotate's --ip-whitelist gate matches
// add's: a typo errors (exit 1) instead of silently widening access, and
// the stored whitelist is left untouched.
func TestRotateRejectsBadIPWhitelist(t *testing.T) {
	home := testHome(t)
	mustInit(t)
	if code, _, e := runVault(t, "v\n", "add", "--from-stdin", "--ip-whitelist=10.0.0.0/8", "k"); code != 0 {
		t.Fatalf("add: %s", e)
	}
	code, _, errOut := runVault(t, "v2\n", "rotate", "--from-stdin", "-y", "--ip-whitelist=not-an-ip", "k")
	if code == 0 || !strings.Contains(errOut, "ip-whitelist") {
		t.Errorf("rotate bad ip-whitelist should error, got code=%d err=%q", code, errOut)
	}
	if s, _ := LoadStore(home); func() bool { a, _ := s.FindAlias("k"); return a == nil || len(a.IPWhitelist) != 1 }() {
		t.Error("failed rotate must not touch the stored whitelist")
	}
}
