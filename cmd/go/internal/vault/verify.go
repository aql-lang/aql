package vault

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// keyringLister is the optional capability a backend implements when it
// can enumerate its stored keys (the file backend does; OS keychains
// and 1Password do not). `vault verify` uses it to find orphaned
// keyring entries; without it, only dangling-metadata detection runs.
type keyringLister interface {
	list() ([]string, error)
}

// runVerify implements `boru vault verify [--prune]`. It reconciles the
// two files the vault depends on — the metadata store (vault.jsonic)
// and the secret keyring — and reports inconsistencies that a crash
// between their separate writes, or a partial import, could leave:
//
//   - dangling metadata: an alias whose secret is missing (get fails)
//   - orphaned keyring entries: a secret with no metadata (file backend)
//   - capabilities bound to an alias that no longer exists
//   - stale .tmp files left by an interrupted atomic write
//
// Without --prune it only reports, exiting non-zero when anything is
// found (for scripts/CI). With --prune it repairs: removes dangling
// records, deletes orphaned secrets, revokes dead capabilities, and
// removes stale temp files.
func runVerify(args []string, homeDir string, stdin io.Reader, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("vault verify", flag.ContinueOnError)
	fs.SetOutput(stderr)
	prune := fs.Bool("prune", false, "repair the issues found (delete orphaned secrets, drop dangling records, revoke dead capabilities, remove stale temp files)")
	if err := fs.Parse(args); err != nil {
		return 1
	}
	s, err := requireStore(homeDir)
	if err != nil {
		fmt.Fprintf(stderr, "error: %s\n", err)
		return 1
	}
	// The passphrase prompt happens here, OUTSIDE the vault lock, so a
	// human at the prompt can never stall the broker's counter writes.
	sess, err := authenticate(s, homeDir, stdin, stdout, "Vault passphrase: ")
	if err != nil {
		fmt.Fprintf(stderr, "error: %s\n", err)
		return 1
	}
	defer sess.Close()
	kr := sess.Keyring()

	issues, pruned := 0, 0
	report := func(repaired bool, format string, a ...any) {
		issues++
		if *prune {
			pruned++
		}
		prefix := ""
		if *prune && repaired {
			prefix = "repaired: "
		}
		fmt.Fprintf(stdout, prefix+format+"\n", a...)
	}

	// The whole check-and-repair runs under the vault lock, against a
	// freshly-loaded store: a writer mid-sequence (add has written the
	// keyring but not yet the store) would otherwise show up as a false
	// orphan, and a --prune save could clobber a concurrent commit. The
	// lock also makes the report mode an actually-consistent snapshot.
	verifyErr := withVaultLock(homeDir, func() error {
		s, err := LoadStore(homeDir)
		if err != nil {
			return err
		}
		if s == nil {
			return errors.New("vault not initialized; run `boru vault init`")
		}

		// Enumerate the keyring once when the backend supports it; this
		// drives both dangling and orphan detection by set difference.
		var keyKeys map[string]bool
		if lister, ok := kr.(keyringLister); ok {
			keys, err := lister.list()
			if err != nil {
				return fmt.Errorf("listing keyring: %w", err)
			}
			keyKeys = make(map[string]bool, len(keys))
			for _, k := range keys {
				keyKeys[k] = true
			}
		}

		// 1. Dangling metadata — an alias with no secret behind it.
		for _, a := range s.SortedAliases() {
			missing := false
			if keyKeys != nil {
				missing = !keyKeys[a.Name]
			} else if _, err := kr.Get(a.Name); err == ErrNotFound {
				missing = true
			} else if err != nil {
				fmt.Fprintf(stderr, "warning: could not check %s: %s\n", a.Name, err)
				continue
			}
			if missing {
				report(true, "dangling metadata (no secret): %s", a.Name)
				if *prune {
					s.RemoveAlias(a.Name)
				}
			}
		}

		// 2. Orphaned keyring entries — a secret with no metadata (only
		//    detectable when the backend can enumerate).
		if keyKeys != nil {
			orphans := make([]string, 0)
			for k := range keyKeys {
				if a, _ := s.FindAlias(k); a == nil {
					orphans = append(orphans, k)
				}
			}
			sort.Strings(orphans)
			for _, k := range orphans {
				report(true, "orphaned keyring entry (no metadata): %s", k)
				if *prune {
					if err := kr.Delete(k); err != nil {
						fmt.Fprintf(stderr, "warning: deleting orphan %s: %s\n", k, err)
						pruned--
					}
				}
			}
		} else {
			fmt.Fprintf(stdout, "note: orphan detection is unavailable for the %s backend\n", kr.Name())
		}

		// 3. Capabilities bound to an alias that no longer exists.
		for i := range s.Capabilities {
			c := &s.Capabilities[i]
			if c.Revoked {
				continue
			}
			if a, _ := s.FindAlias(c.Alias); a == nil {
				report(true, "capability %s references missing alias %q", shortCap(c.ID), c.Alias)
				if *prune {
					c.Revoked = true
				}
			}
		}

		// 4. Stale temp files from an interrupted atomic write. The
		//    in-flight temp of a concurrent writer can't be mistaken for
		//    a stale one: writers hold this same lock.
		dir := vaultFolder(homeDir)
		if entries, err := os.ReadDir(dir); err == nil {
			for _, e := range entries {
				n := e.Name()
				if e.Type().IsRegular() && strings.HasPrefix(n, ".") && strings.HasSuffix(n, ".tmp") {
					report(true, "stale temp file: %s", n)
					if *prune {
						if err := s7osRemove(filepath.Join(dir, n)); err != nil {
							fmt.Fprintf(stderr, "warning: removing %s: %s\n", n, err)
							pruned--
						}
					}
				}
			}
		}

		if *prune && pruned > 0 {
			return SaveStore(homeDir, s)
		}
		return nil
	})
	if verifyErr != nil {
		fmt.Fprintf(stderr, "error: %s\n", verifyErr)
		return 1
	}
	if *prune && pruned > 0 {
		_ = appendAudit(homeDir, AuditEvent{
			Action: "vault.verify", Outcome: "ok",
			Reason: fmt.Sprintf("repaired=%d", pruned),
		})
	}

	// Report the integrity status of an envelope vault (legacy vaults
	// have no integrity layer, so their output is unchanged). A repair
	// re-establishes the keyed sidecar; otherwise we report the detected
	// state (ok / unverified / external-change / hmac-invalid / rollback)
	// without masking it.
	if s.HasPasswordSlots() {
		if *prune && pruned > 0 {
			sealIntegrity(homeDir, sess)
		}
		anchor, _ := readAnchor(sess.Keyring())
		st, _ := checkStoreIntegrity(homeDir, sess.IntegrityKey(), anchor)
		fmt.Fprintf(stdout, "integrity: %s\n", st)
	}

	switch {
	case issues == 0:
		fmt.Fprintln(stdout, "vault verify: OK (store and keyring are consistent)")
		return 0
	case *prune:
		fmt.Fprintf(stdout, "vault verify: repaired %d of %d issue(s)\n", pruned, issues)
		return 0
	default:
		fmt.Fprintf(stdout, "vault verify: %d issue(s) found; re-run with --prune to repair\n", issues)
		return 2
	}
}
