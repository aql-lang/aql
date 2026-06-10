package vault

import (
	"net/http"
	"strings"
	"testing"
	"time"
)

// TestFindCapabilityExactRejectsPrefix locks in the distinction between
// the CLI convenience matcher (prefix) and the authentication matcher
// (exact, constant-time).
func TestFindCapabilityExactRejectsPrefix(t *testing.T) {
	s := &Store{}
	c, err := s.NewCapability("alias", "agent", nil, nil, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	full := c.ID

	if got, _ := s.FindCapabilityExact(full); got == nil {
		t.Fatalf("exact id should match")
	}
	// A prefix — even a single character — must NOT authenticate.
	if got, _ := s.FindCapabilityExact(full[:1]); got != nil {
		t.Errorf("single-char prefix authenticated via FindCapabilityExact")
	}
	if got, _ := s.FindCapabilityExact(full[:8]); got != nil {
		t.Errorf("8-char prefix authenticated via FindCapabilityExact")
	}
	// The CLI convenience matcher still accepts the short prefix.
	if got, _ := s.FindCapability(full[:8]); got == nil {
		t.Errorf("FindCapability should still accept a short prefix for CLI use")
	}
}

// TestProxyRejectsTokenPrefix is the end-to-end guard for the auth
// bypass: a prefix of a live capability token must not be accepted by
// the broker, while the full token still works.
func TestProxyRejectsTokenPrefix(t *testing.T) {
	testHome(t)
	mustInit(t)
	fu := newFakeUpstream(t)
	registerTestProvider(t, "fake", fu, "bearer")
	if code, _, _ := runVault(t, "v\n", "add", "--from-stdin", "--provider=fake", "k"); code != 0 {
		t.Fatal("add")
	}
	tok := grantOK(t, "k", nil, nil)

	base := startProxy(t)

	get := func(bearer string) int {
		req, _ := http.NewRequest("GET", base+"/k/foo", nil)
		req.Header.Set("Authorization", "Bearer "+bearer)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		return resp.StatusCode
	}

	if code := get(tok[:8]); code != http.StatusUnauthorized {
		t.Errorf("8-char prefix token: status=%d, want 401", code)
	}
	if code := get(tok[:1]); code != http.StatusUnauthorized {
		t.Errorf("single-char token: status=%d, want 401", code)
	}
	if code := get(tok); code != http.StatusOK {
		t.Errorf("full token: status=%d, want 200", code)
	}
}

// TestProxyRefusesPublicBindWithoutFlag verifies the broker will not
// bind to a non-loopback address unless --allow-public is given.
func TestProxyRefusesPublicBindWithoutFlag(t *testing.T) {
	testHome(t)
	mustInit(t)
	code, _, errOut := runVault(t, "", "proxy", "--listen=0.0.0.0:0")
	if code == 0 {
		t.Fatal("expected non-zero exit binding non-loopback without --allow-public")
	}
	if !strings.Contains(errOut, "allow-public") {
		t.Errorf("error should mention --allow-public, got %q", errOut)
	}
}
