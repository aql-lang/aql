package vault

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"
)

// TestCapabilityTokenAuth locks in token-hash authentication: the full
// token authenticates, prefixes do not (the old bypass), the plaintext
// token is never stored, and the public ID still prefix-matches for the
// CLI's convenience.
func TestCapabilityTokenAuth(t *testing.T) {
	s := &Store{}
	c, token, err := s.NewCapability("alias", "agent", nil, nil, time.Hour)
	if err != nil {
		t.Fatal(err)
	}

	if got, _ := s.FindCapabilityByToken(token); got == nil {
		t.Fatalf("full token should authenticate")
	}
	// A prefix — even a single character — must NOT authenticate.
	if got, _ := s.FindCapabilityByToken(token[:1]); got != nil {
		t.Errorf("single-char token prefix authenticated")
	}
	if got, _ := s.FindCapabilityByToken(token[:8]); got != nil {
		t.Errorf("8-char token prefix authenticated")
	}
	// The token is never stored in plaintext — only its hash.
	if c.TokenHash == "" || c.TokenHash == token {
		t.Errorf("token stored in plaintext or hash missing: hash=%q", c.TokenHash)
	}
	b, _ := json.Marshal(s)
	if strings.Contains(string(b), token) {
		t.Errorf("serialized store contains the plaintext token")
	}
	// The public ID still prefix-matches for CLI convenience (revoke).
	if got, _ := s.FindCapability(c.ID[:8]); got == nil {
		t.Errorf("FindCapability should accept a short public-ID prefix for CLI use")
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
