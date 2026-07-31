package vault

// mv_expiry_wave4_test.go — error arms for mv.go and expiry.go: usage
// and parse failures, locked/corrupt stores, scope denials, batch
// pre-flight collisions, abort/rollback paths, and the lock-blocked
// mutateStore arms (driven by a directory squatting on the lock path).

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// w4BlockLock replaces the vault lock path with a directory, making
// every subsequent withVaultLock/mutateStore call fail to acquire.
func w4BlockLock(t *testing.T, home string) {
	t.Helper()
	lp := lockPath(home)
	_ = os.Remove(lp)
	if err := os.Mkdir(lp, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Remove(lp) })
}

func TestMvErrorArms(t *testing.T) {
	t.Run("parse and usage", func(t *testing.T) {
		testHome(t)
		if code, _, _ := runVault(t, "", "mv", "-nope", "a", "b"); code == 0 {
			t.Error("mv with a bad flag should fail")
		}
		if code, _, e := runVault(t, "", "mv", "onlyone"); code == 0 || !strings.Contains(e, "usage") {
			t.Errorf("mv with one arg = %d, %q", code, e)
		}
	})
	t.Run("corrupt store", func(t *testing.T) {
		home := testHome(t)
		mustInit(t)
		w4CorruptStore(t, home)
		if code, _, _ := runVault(t, "", "mv", "a", "b"); code == 0 {
			t.Error("mv on a corrupt store should fail")
		}
	})
	t.Run("locked", func(t *testing.T) {
		testHome(t)
		mustInit(t)
		if code, _, e := runVault(t, "", "lock"); code != 0 {
			t.Fatalf("lock: %s", e)
		}
		if code, _, e := runVault(t, "", "mv", "a", "b"); code == 0 || !strings.Contains(e, "locked") {
			t.Errorf("mv on a locked vault = %d, %q", code, e)
		}
	})
	t.Run("bad destination namespace ref", func(t *testing.T) {
		testHome(t)
		mustInit(t)
		if code, _, e := runVault(t, "v\n", "add", "--from-stdin", "a"); code != 0 {
			t.Fatalf("seed: %s", e)
		}
		if code, _, e := runVault(t, "", "mv", "a:", "b!ad:"); code == 0 ||
			!strings.Contains(e, "invalid namespace reference") {
			t.Errorf("mv to an invalid ns ref = %d, %q", code, e)
		}
	})
	t.Run("bad destination alias ref", func(t *testing.T) {
		testHome(t)
		mustInit(t)
		if code, _, e := runVault(t, "v\n", "add", "--from-stdin", "a"); code != 0 {
			t.Fatalf("seed: %s", e)
		}
		if code, _, e := runVault(t, "", "mv", "a", "x:y:z"); code == 0 ||
			!strings.Contains(e, "invalid alias") {
			t.Errorf("mv to an invalid alias ref = %d, %q", code, e)
		}
	})
	t.Run("namespace move collision and re-tag", func(t *testing.T) {
		home := testHome(t)
		mustInit(t)
		if code, _, e := runVault(t, "v1\n", "add", "--from-stdin", "proj:x"); code != 0 {
			t.Fatalf("seed: %s", e)
		}
		// A legacy tag-only alias "x" in the same namespace: both sources
		// would become other:x.
		kr := &fileKeyring{folder: vaultFolder(home), pass: "test-pass"}
		if err := kr.Set("x", "v2"); err != nil {
			t.Fatal(err)
		}
		s, err := LoadStore(home)
		if err != nil {
			t.Fatal(err)
		}
		s.Aliases = append(s.Aliases, Alias{Name: "x", Namespace: "proj", CreatedAt: "2020-01-01T00:00:00Z"})
		if err := SaveStore(home, s); err != nil {
			t.Fatal(err)
		}
		if code, _, e := runVault(t, "", "mv", "proj:", "other:"); code == 0 ||
			!strings.Contains(e, "would both become") {
			t.Errorf("colliding namespace move = %d, %q", code, e)
		}
		// A tag-only alias with no qualified twin re-tags on a move to
		// root (dry-run shows it as a pure re-tag).
		if err := kr.Set("y", "v3"); err != nil {
			t.Fatal(err)
		}
		s, err = LoadStore(home)
		if err != nil {
			t.Fatal(err)
		}
		s.Aliases = append(s.Aliases, Alias{Name: "y", Namespace: "tagns", CreatedAt: "2020-01-01T00:00:00Z"})
		if err := SaveStore(home, s); err != nil {
			t.Fatal(err)
		}
		code, out, e := runVault(t, "", "mv", "--dry-run", "tagns:", ":")
		if code != 0 {
			t.Fatalf("dry-run ns move: %s", e)
		}
		if !strings.Contains(out, "would re-tag y") {
			t.Errorf("dry-run missing the re-tag line:\n%s", out)
		}
	})
	t.Run("wrong passphrase", func(t *testing.T) {
		w4EnvelopeVault(t)
		setPass(t, "wrong")
		if code, _, e := runVault(t, "", "mv", "proj:k", "proj:k2"); code == 0 ||
			!strings.Contains(e, "wrong passphrase") {
			t.Errorf("mv wrong pass = %d, %q", code, e)
		}
	})
	t.Run("scope denials", func(t *testing.T) {
		w4EnvelopeVault(t)
		// A read password cannot move at all.
		setPass(t, "reader-pass")
		if code, _, e := runVault(t, "", "mv", "proj:k", "proj:k2"); code == 0 ||
			!strings.Contains(e, "permission denied") {
			t.Errorf("mv by a read password = %d, %q", code, e)
		}
		// A move password scoped to proj cannot move INTO another namespace.
		mustAddPassword(t, "test-pass", "mover-pass", "mover", "--scope=move", "--namespaces=proj")
		setPass(t, "mover-pass")
		if code, _, e := runVault(t, "", "mv", "proj:k", "other:k"); code == 0 ||
			!strings.Contains(e, "permission denied") {
			t.Errorf("mv outside the mover's namespaces = %d, %q", code, e)
		}
	})
	t.Run("aborts on unreadable source", func(t *testing.T) {
		home := testHome(t)
		mustInit(t)
		s, err := LoadStore(home)
		if err != nil {
			t.Fatal(err)
		}
		s.Aliases = append(s.Aliases, Alias{Name: "ghost", CreatedAt: "2020-01-01T00:00:00Z"})
		if err := SaveStore(home, s); err != nil {
			t.Fatal(err)
		}
		if code, _, e := runVault(t, "", "mv", "ghost", "elsewhere"); code == 0 ||
			!strings.Contains(e, "moving ghost") {
			t.Errorf("mv of a store-only alias = %d, %q", code, e)
		}
	})
	t.Run("aborts when destination write is blocked", func(t *testing.T) {
		home := w4EnvelopeVault(t)
		s, err := LoadStore(home)
		if err != nil {
			t.Fatal(err)
		}
		s.CurrentNDK["dst"] = "aabb00112233ccdd" // an id no session holds
		if err := SaveStore(home, s); err != nil {
			t.Fatal(err)
		}
		if code, _, e := runVault(t, "", "mv", "proj:k", "dst:k"); code == 0 ||
			!strings.Contains(e, "moving proj:k") {
			t.Errorf("mv into a blocked namespace = %d, %q", code, e)
		}
		// The source is untouched by the abort.
		if code, out, _ := runVault(t, "", "get", "--reveal", "proj:k"); code != 0 || strings.TrimSpace(out) != "v" {
			t.Errorf("source lost after aborted mv = %d/%q", code, out)
		}
	})
	t.Run("rolls back when the store commit fails", func(t *testing.T) {
		home := testHome(t)
		mustInit(t)
		if code, _, e := runVault(t, "v\n", "add", "--from-stdin", "src"); code != 0 {
			t.Fatalf("seed: %s", e)
		}
		w4BlockLock(t, home)
		if code, _, e := runVault(t, "", "mv", "src", "dst"); code == 0 || e == "" {
			t.Errorf("mv with a blocked lock = %d, %q", code, e)
		}
	})
}

func TestExpiryErrorArms(t *testing.T) {
	t.Run("list parse and filters", func(t *testing.T) {
		testHome(t)
		mustInit(t)
		if code, _, _ := runVault(t, "", "expiry", "-nope"); code == 0 {
			t.Error("expiry with a bad flag should fail")
		}
		if code, _, e := runVault(t, "", "expiry", "--namespace=b!ad"); code == 0 ||
			!strings.Contains(e, "invalid namespace") {
			t.Errorf("expiry bad ns filter = %d, %q", code, e)
		}
		if code, _, e := runVault(t, "", "expiry", "--within=bogus"); code == 0 ||
			!strings.Contains(e, "invalid --within") {
			t.Errorf("expiry bad within = %d, %q", code, e)
		}
	})
	t.Run("list corrupt store", func(t *testing.T) {
		home := testHome(t)
		mustInit(t)
		w4CorruptStore(t, home)
		if code, _, _ := runVault(t, "", "expiry"); code == 0 {
			t.Error("expiry on a corrupt store should fail")
		}
	})
	t.Run("list skips and sorts", func(t *testing.T) {
		home := testHome(t)
		mustInit(t)
		s, err := LoadStore(home)
		if err != nil {
			t.Fatal(err)
		}
		exp := time.Now().Add(24 * time.Hour).UTC().Format(time.RFC3339)
		far := time.Now().Add(90 * 24 * time.Hour).UTC().Format(time.RFC3339)
		s.Aliases = append(s.Aliases,
			Alias{Name: "b2", ExpiresAt: exp, CreatedAt: "2020-01-01T00:00:00Z"},
			Alias{Name: "a1", ExpiresAt: exp, CreatedAt: "2020-01-01T00:00:00Z"},
			Alias{Name: "junk", ExpiresAt: "not-a-time", CreatedAt: "2020-01-01T00:00:00Z"},
			Alias{Name: "far", ExpiresAt: far, CreatedAt: "2020-01-01T00:00:00Z"},
		)
		if err := SaveStore(home, s); err != nil {
			t.Fatal(err)
		}
		// Explicit `expiry list` subcommand, tie-broken by name, window
		// filter excluding the distant key, junk timestamp skipped.
		code, out, e := runVault(t, "", "expiry", "list", "--within=30d")
		if code != 0 {
			t.Fatalf("expiry list: %s", e)
		}
		if strings.Contains(out, "far") || strings.Contains(out, "junk") {
			t.Errorf("window/junk filtering failed:\n%s", out)
		}
		if strings.Index(out, "a1") > strings.Index(out, "b2") {
			t.Errorf("tie-break order wrong:\n%s", out)
		}
	})
	t.Run("set arms", func(t *testing.T) {
		home := testHome(t)
		mustInit(t)
		if code, _, _ := runVault(t, "", "expiry", "set", "-nope", "a", "90d"); code == 0 {
			t.Error("expiry set with a bad flag should fail")
		}
		if code, _, e := runVault(t, "", "expiry", "set", "onlyalias"); code == 0 ||
			!strings.Contains(e, "usage") {
			t.Errorf("expiry set usage = %d, %q", code, e)
		}
		w4CorruptStore(t, home)
		if code, _, _ := runVault(t, "", "expiry", "set", "a", "90d"); code == 0 {
			t.Error("expiry set on a corrupt store should fail")
		}
	})
	t.Run("clear arms", func(t *testing.T) {
		home := testHome(t)
		mustInit(t)
		if code, _, _ := runVault(t, "", "expiry", "clear", "-nope", "a"); code == 0 {
			t.Error("expiry clear with a bad flag should fail")
		}
		if code, _, e := runVault(t, "", "expiry", "clear"); code == 0 || !strings.Contains(e, "usage") {
			t.Errorf("expiry clear usage = %d, %q", code, e)
		}
		w4CorruptStore(t, home)
		if code, _, _ := runVault(t, "", "expiry", "clear", "a"); code == 0 {
			t.Error("expiry clear on a corrupt store should fail")
		}
	})
	t.Run("set and clear blocked lock", func(t *testing.T) {
		home := testHome(t)
		mustInit(t)
		if code, _, e := runVault(t, "v\n", "add", "--from-stdin", "k"); code != 0 {
			t.Fatalf("seed: %s", e)
		}
		w4BlockLock(t, home)
		if code, _, e := runVault(t, "", "expiry", "set", "k", "90d"); code == 0 || e == "" {
			t.Errorf("expiry set with a blocked lock = %d, %q", code, e)
		}
		if code, _, e := runVault(t, "", "expiry", "clear", "k"); code == 0 || e == "" {
			t.Errorf("expiry clear with a blocked lock = %d, %q", code, e)
		}
	})
}

func TestHumanizeDurNegative(t *testing.T) {
	if got := humanizeDur(-26 * time.Hour); got != "1d" {
		t.Errorf("humanizeDur(-26h) = %q, want 1d", got)
	}
}

// TestMutateBlockedLockArms drives the remaining mutateStore /
// withVaultLock error prints in vault.go with the lock path blocked.
func TestMutateBlockedLockArms(t *testing.T) {
	t.Run("init", func(t *testing.T) {
		home := testHome(t)
		if err := os.MkdirAll(filepath.Join(home, ".boru"), 0o700); err != nil {
			t.Fatal(err)
		}
		w4BlockLock(t, home)
		if code, _, e := runVault(t, "", "init", "--backend=file"); code == 0 || e == "" {
			t.Errorf("init with a blocked lock = %d, %q", code, e)
		}
	})
	t.Run("rm", func(t *testing.T) {
		home := testHome(t)
		mustInit(t)
		if code, _, e := runVault(t, "v\n", "add", "--from-stdin", "k"); code != 0 {
			t.Fatalf("seed: %s", e)
		}
		w4BlockLock(t, home)
		if code, _, e := runVault(t, "", "rm", "--yes", "k"); code == 0 || e == "" {
			t.Errorf("rm with a blocked lock = %d, %q", code, e)
		}
	})
	t.Run("import", func(t *testing.T) {
		home := testHome(t)
		mustInit(t)
		w4BlockLock(t, home)
		if code, _, e := runVault(t, "K=V\n", "import"); code == 0 || e == "" {
			t.Errorf("import with a blocked lock = %d, %q", code, e)
		}
	})
	t.Run("grant", func(t *testing.T) {
		home := testHome(t)
		mustInit(t)
		if code, _, e := runVault(t, "v\n", "add", "--from-stdin", "k"); code != 0 {
			t.Fatalf("seed: %s", e)
		}
		w4BlockLock(t, home)
		if code, _, e := runVault(t, "", "grant", "k"); code == 0 || e == "" {
			t.Errorf("grant with a blocked lock = %d, %q", code, e)
		}
	})
	t.Run("rotate", func(t *testing.T) {
		home := testHome(t)
		mustInit(t)
		if code, _, e := runVault(t, "v\n", "add", "--from-stdin", "k"); code != 0 {
			t.Fatalf("seed: %s", e)
		}
		w4BlockLock(t, home)
		if code, _, e := runVault(t, "v2\n", "rotate", "--yes", "--revoke-caps", "--from-stdin", "k"); code == 0 || e == "" {
			t.Errorf("rotate --revoke-caps with a blocked lock = %d, %q", code, e)
		}
		if code, _, e := runVault(t, "v2\n", "rotate", "--yes", "--from-stdin", "k"); code == 0 || e == "" {
			t.Errorf("rotate with a blocked lock = %d, %q", code, e)
		}
	})
	t.Run("config set on corrupt store", func(t *testing.T) {
		home := testHome(t)
		mustInit(t)
		w4CorruptStore(t, home)
		if code, _, e := runVault(t, "", "config", "--set", "a=b"); code == 0 ||
			!strings.Contains(e, "parsing") {
			t.Errorf("config --set on a corrupt store = %d, %q", code, e)
		}
	})
}
