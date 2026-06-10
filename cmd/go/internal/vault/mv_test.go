package vault

import (
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
)

func TestMvRenameKeyKeepsValueAndMetadata(t *testing.T) {
	testHome(t)
	mustInit(t)
	if code, _, e := runVault(t, "v1\n", "add", "--from-stdin", "--provider=openai", "a"); code != 0 {
		t.Fatalf("add: %s", e)
	}
	code, out, errOut := runVault(t, "", "mv", "a", "b")
	if code != 0 {
		t.Fatalf("mv: %s", errOut)
	}
	if !strings.Contains(out, "moved a -> b") {
		t.Errorf("mv output = %q", out)
	}
	// New name resolves, old is gone — in the store and the keyring.
	if code, out, _ := runVault(t, "", "get", "--reveal", "b"); code != 0 || !strings.Contains(out, "v1") {
		t.Errorf("get b = %q (code %d)", out, code)
	}
	if code, _, _ := runVault(t, "", "get", "a"); code == 0 {
		t.Error("old alias a still resolves")
	}
	s, _ := LoadStore(os.Getenv(EnvHome))
	if a, _ := s.FindAlias("a"); a != nil {
		t.Error("old record still present")
	}
	b, _ := s.FindAlias("b")
	if b == nil || b.Provider != "openai" || b.CreatedAt == "" || b.UpdatedAt == "" {
		t.Errorf("metadata not preserved across rename: %+v", b)
	}
}

func TestMvMoveIntoAndOutOfNamespace(t *testing.T) {
	testHome(t)
	mustInit(t)
	if code, _, e := runVault(t, "v\n", "add", "--from-stdin", "key"); code != 0 {
		t.Fatalf("add: %s", e)
	}
	if code, _, e := runVault(t, "", "mv", "key", "proj:key"); code != 0 {
		t.Fatalf("mv into ns: %s", e)
	}
	if code, out, _ := runVault(t, "", "get", "--reveal", "proj:key"); code != 0 || !strings.Contains(out, "v") {
		t.Errorf("get proj:key = %q (code %d)", out, code)
	}
	s, _ := LoadStore(os.Getenv(EnvHome))
	if a, _ := s.FindAlias("proj:key"); a == nil || a.Namespace != "proj" {
		t.Errorf("namespace metadata not derived: %+v", a)
	}
	// And back to root via the :name escape.
	if code, _, e := runVault(t, "", "mv", "proj:key", ":key"); code != 0 {
		t.Fatalf("mv to root: %s", e)
	}
	if code, _, _ := runVault(t, "", "get", "key"); code != 0 {
		t.Error("key not back at root")
	}
}

func TestMvResolvesDefaultNamespaceOnDestination(t *testing.T) {
	testHome(t)
	mustInit(t)
	if code, _, e := runVault(t, "v\n", "add", "--from-stdin", ":rootkey"); code != 0 {
		t.Fatalf("add: %s", e)
	}
	if code, _, e := runVault(t, "", "config", "--set", "namespace.default=proj"); code != 0 {
		t.Fatalf("config: %s", e)
	}
	// Bare destination resolves into the default namespace.
	if code, _, e := runVault(t, "", "mv", ":rootkey", "rootkey"); code != 0 {
		t.Fatalf("mv: %s", e)
	}
	if code, out, _ := runVault(t, "", "get", "--reveal", "proj:rootkey"); code != 0 || !strings.Contains(out, "v") {
		t.Errorf("get proj:rootkey = %q (code %d)", out, code)
	}
}

func TestMvRefusesCollisionAndSelfMove(t *testing.T) {
	testHome(t)
	mustInit(t)
	for _, ref := range []string{"a", "b"} {
		if code, _, e := runVault(t, ref+"-val\n", "add", "--from-stdin", ref); code != 0 {
			t.Fatalf("add %s: %s", ref, e)
		}
	}
	if code, _, errOut := runVault(t, "", "mv", "a", "b"); code == 0 {
		t.Error("expected collision error")
	} else if !strings.Contains(errOut, "already exists") {
		t.Errorf("collision error = %q", errOut)
	}
	// Nothing changed.
	if code, out, _ := runVault(t, "", "get", "--reveal", "a"); code != 0 || !strings.Contains(out, "a-val") {
		t.Errorf("a was disturbed by failed mv: %q (code %d)", out, code)
	}
	if code, _, errOut := runVault(t, "", "mv", "a", "a"); code == 0 {
		t.Error("expected self-move error")
	} else if !strings.Contains(errOut, "the same alias") {
		t.Errorf("self-move error = %q", errOut)
	}
	if code, _, _ := runVault(t, "", "mv", "nope", "x"); code == 0 {
		t.Error("expected missing-source error")
	}
}

func TestMvNamespaceRenameRebindsCapabilities(t *testing.T) {
	testHome(t)
	mustInit(t)
	fu := newFakeUpstream(t)
	registerTestProvider(t, "fake", fu, "bearer")
	for _, ref := range []string{"proj:k1", "proj:k2", "other:x"} {
		if code, _, e := runVault(t, ref+"-v\n", "add", "--from-stdin", "--provider=fake", ref); code != 0 {
			t.Fatalf("add %s: %s", ref, e)
		}
	}
	tok := grantOK(t, "proj:k1", nil, nil)

	code, out, errOut := runVault(t, "", "mv", "proj:", "team:")
	if code != 0 {
		t.Fatalf("mv ns: %s", errOut)
	}
	if !strings.Contains(out, "moved proj:k1 -> team:k1") || !strings.Contains(out, "moved proj:k2 -> team:k2") {
		t.Errorf("bulk move output = %q", out)
	}
	if !strings.Contains(out, "re-bound 1 capability") {
		t.Errorf("capability re-bind not reported: %q", out)
	}
	// Untouched namespace stays.
	if code, _, _ := runVault(t, "", "get", "other:x"); code != 0 {
		t.Error("other:x disturbed by namespace rename")
	}
	if code, _, _ := runVault(t, "", "get", "proj:k1"); code == 0 {
		t.Error("proj:k1 still resolves after namespace rename")
	}

	// The original bearer token works against the NEW proxy path: the
	// capability followed the key (its token hash is unchanged).
	base := startProxy(t)
	req, _ := http.NewRequest("GET", base+"/team:k1/v1/z", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("proxy via new path: status %d (%s)", resp.StatusCode, body)
	}
	// The old path no longer authorizes.
	req2, _ := http.NewRequest("GET", base+"/proj:k1/v1/z", nil)
	req2.Header.Set("Authorization", "Bearer "+tok)
	resp2, err := http.DefaultClient.Do(req2)
	if err != nil {
		t.Fatal(err)
	}
	defer resp2.Body.Close()
	if resp2.StatusCode == 200 {
		t.Error("old proxy path still authorizes after move")
	}
}

func TestMvNamespaceRevokeCaps(t *testing.T) {
	testHome(t)
	mustInit(t)
	if code, _, e := runVault(t, "v\n", "add", "--from-stdin", "proj:k"); code != 0 {
		t.Fatalf("add: %s", e)
	}
	_ = grantOK(t, "proj:k", nil, nil)
	code, out, errOut := runVault(t, "", "mv", "--revoke-caps", "proj:k", "team:k")
	if code != 0 {
		t.Fatalf("mv: %s", errOut)
	}
	if !strings.Contains(out, "revoked 1 capability") {
		t.Errorf("revoke not reported: %q", out)
	}
	s, _ := LoadStore(os.Getenv(EnvHome))
	if len(s.Capabilities) != 1 || !s.Capabilities[0].Revoked {
		t.Errorf("capability not revoked: %+v", s.Capabilities)
	}
}

func TestMvNamespaceSelectsLegacyTagAndRetags(t *testing.T) {
	home := testHome(t)
	mustInit(t)
	// Construct a legacy alias: unqualified name with a metadata tag
	// (the pre-namespacing shape).
	if code, _, e := runVault(t, "lv\n", "add", "--from-stdin", "legacykey"); code != 0 {
		t.Fatalf("add: %s", e)
	}
	s, _ := LoadStore(home)
	a, _ := s.FindAlias("legacykey")
	a.Namespace = "oldproj"
	if err := SaveStore(home, s); err != nil {
		t.Fatal(err)
	}

	// Moving the namespace to another name qualifies the legacy alias.
	if code, out, e := runVault(t, "", "mv", "oldproj:", "team:"); code != 0 {
		t.Fatalf("mv: %s", e)
	} else if !strings.Contains(out, "moved legacykey -> team:legacykey") {
		t.Errorf("legacy move output = %q", out)
	}
	if code, out, _ := runVault(t, "", "get", "--reveal", "team:legacykey"); code != 0 || !strings.Contains(out, "lv") {
		t.Errorf("get team:legacykey = %q (code %d)", out, code)
	}

	// And a legacy tag moved to root is a pure re-tag (name unchanged).
	if code, _, e := runVault(t, "rv\n", "add", "--from-stdin", "tagonly"); code != 0 {
		t.Fatalf("add tagonly: %s", e)
	}
	s, _ = LoadStore(home)
	ta, _ := s.FindAlias("tagonly")
	ta.Namespace = "tagns"
	if err := SaveStore(home, s); err != nil {
		t.Fatal(err)
	}
	if code, out, e := runVault(t, "", "mv", "tagns:", ":"); code != 0 {
		t.Fatalf("mv retag: %s", e)
	} else if !strings.Contains(out, "re-tagged tagonly") {
		t.Errorf("retag output = %q", out)
	}
	s, _ = LoadStore(home)
	ta, _ = s.FindAlias("tagonly")
	if ta == nil || ta.Namespace != "" {
		t.Errorf("legacy tag not cleared: %+v", ta)
	}
	if code, out, _ := runVault(t, "", "get", "--reveal", "tagonly"); code != 0 || !strings.Contains(out, "rv") {
		t.Errorf("tagonly unreadable after retag: %q (code %d)", out, code)
	}
}

func TestMvDryRunChangesNothing(t *testing.T) {
	testHome(t)
	mustInit(t)
	if code, _, e := runVault(t, "v\n", "add", "--from-stdin", "proj:k"); code != 0 {
		t.Fatalf("add: %s", e)
	}
	code, out, errOut := runVault(t, "", "mv", "--dry-run", "proj:", "team:")
	if code != 0 {
		t.Fatalf("mv --dry-run: %s", errOut)
	}
	if !strings.Contains(out, "would move proj:k -> team:k") {
		t.Errorf("dry-run output = %q", out)
	}
	if code, _, _ := runVault(t, "", "get", "proj:k"); code != 0 {
		t.Error("dry-run moved the key")
	}
	if code, _, _ := runVault(t, "", "get", "team:k"); code == 0 {
		t.Error("dry-run created the destination")
	}
}

func TestMvShapeAndBatchErrors(t *testing.T) {
	testHome(t)
	mustInit(t)
	if code, _, e := runVault(t, "v\n", "add", "--from-stdin", "proj:k"); code != 0 {
		t.Fatalf("add: %s", e)
	}
	// Mixed shapes.
	if code, _, errOut := runVault(t, "", "mv", "proj:", "key"); code == 0 {
		t.Error("expected mixed-shape error")
	} else if !strings.Contains(errOut, "must both be aliases") {
		t.Errorf("mixed-shape error = %q", errOut)
	}
	// Same namespace.
	if code, _, errOut := runVault(t, "", "mv", "proj:", "proj:"); code == 0 {
		t.Error("expected same-namespace error")
	} else if !strings.Contains(errOut, "the same") {
		t.Errorf("same-ns error = %q", errOut)
	}
	// Invalid namespace ref.
	if code, _, _ := runVault(t, "", "mv", "a:b:", "c:"); code == 0 {
		t.Error("expected invalid namespace ref error")
	}
	// Empty source namespace.
	if code, _, errOut := runVault(t, "", "mv", "ghost:", "team:"); code == 0 {
		t.Error("expected empty-namespace error")
	} else if !strings.Contains(errOut, "no aliases in namespace") {
		t.Errorf("empty-ns error = %q", errOut)
	}
	// Batch collision: destination occupied by an existing alias.
	if code, _, e := runVault(t, "w\n", "add", "--from-stdin", "team:k"); code != 0 {
		t.Fatalf("add team:k: %s", e)
	}
	if code, _, errOut := runVault(t, "", "mv", "proj:", "team:"); code == 0 {
		t.Error("expected batch collision error")
	} else if !strings.Contains(errOut, "already exists") {
		t.Errorf("batch collision error = %q", errOut)
	}
	// Nothing moved by the failed batch.
	if code, _, _ := runVault(t, "", "get", "proj:k"); code != 0 {
		t.Error("failed batch disturbed proj:k")
	}
}
