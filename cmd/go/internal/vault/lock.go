package vault

import (
	"errors"
	"os"
	"path/filepath"
)

// errLocked is the shared "vault is locked" error returned from
// mutateStore closures that re-check the lock state under the lock.
var errLocked = errors.New("vault is locked; run `aql vault unlock`")

// lockPath is the advisory lock file guarding store mutations. It is a
// dedicated file (not vault.jsonic itself) so the lock is independent
// of the atomic-rename dance that replaces the store.
func lockPath(homeDir string) string {
	return filepath.Join(fileDir(homeDir), "vault.lock")
}

// withVaultLock runs fn while holding an exclusive cross-process lock
// on the vault, serializing the load-modify-save of vault.jsonic
// against other aql processes (a CLI command vs the broker persisting
// quota counters, two CLI commands, etc.). The lock auto-releases if
// the process dies. Reads (LoadStore) deliberately do NOT take it:
// the atomic rename guarantees a reader always sees a complete file,
// so only writers need to serialize.
//
// Hold the lock for as short a span as possible — never across an
// interactive prompt or network call — so the broker's counter writes
// never stall on human input. mutateStore is the intended entry point;
// it keeps the locked section to a bare load-modify-save.
func withVaultLock(homeDir string, fn func() error) error {
	if err := os.MkdirAll(fileDir(homeDir), 0700); err != nil {
		return err
	}
	f, err := os.OpenFile(lockPath(homeDir), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return err
	}
	defer f.Close()
	if err := lockFile(f); err != nil {
		return err
	}
	defer func() { _ = unlockFile(f) }()
	return fn()
}

// mutateStore applies a change to the metadata store atomically with
// respect to other aql processes: it takes the vault lock, loads the
// LATEST store from disk, runs mutate against it, and saves.
//
// Reloading under the lock is the point — it is what stops a
// concurrent writer's committed change from being clobbered. So mutate
// MUST express its change as a delta on the store it is handed
// (re-find aliases/capabilities by name or ID), never reuse a *Store
// captured before the lock. mutate returning an error aborts the save.
func mutateStore(homeDir string, mutate func(*Store) error) error {
	return withVaultLock(homeDir, func() error {
		s, err := LoadStore(homeDir)
		if err != nil {
			return err
		}
		if s == nil {
			return errors.New("vault not initialized; run `aql vault init`")
		}
		if err := mutate(s); err != nil {
			return err
		}
		return SaveStore(homeDir, s)
	})
}
