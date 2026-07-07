package vault

import (
	"os"
	"path/filepath"
)

// writeFileAtomic writes data to path atomically and durably. The
// bytes go to a uniquely-named temp file in the same directory, which
// is fsync'd and chmod'd, then renamed over path; finally the
// directory entry is fsync'd. A concurrent reader — or a crash —
// therefore sees either the previous file in full or the new one in
// full, never a torn write, and a successful return means the data
// has reached stable storage.
//
// The caller is responsible for ensuring the parent directory exists
// (the vault writers MkdirAll ~/.aql with 0700 first; the exporter
// writes to a user-chosen directory that must already exist).
func writeFileAtomic(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	// Unique temp name so concurrent writers never share one, and a
	// leading dot keeps a crash-orphaned temp out of casual listings.
	tmp, err := createTemp(dir, "."+filepath.Base(path)+".*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	// Remove the temp if we return before the rename consumes it.
	renamed := false
	defer func() {
		if !renamed {
			_ = tmp.Close()
			_ = os.Remove(tmpName)
		}
	}()

	if err := fileChmod(tmp, perm); err != nil {
		return err
	}
	if _, err := fileWrite(tmp, data); err != nil {
		return err
	}
	// fsync the data before the rename so the rename can never expose a
	// file whose contents have not yet reached disk.
	if err := fileSync(tmp); err != nil {
		return err
	}
	if err := fileClose(tmp); err != nil {
		return err
	}
	if err := osRename(tmpName, path); err != nil {
		return err
	}
	renamed = true
	syncDir(dir)
	return nil
}

// syncDir fsyncs a directory so a freshly-renamed entry survives a
// crash. Best-effort: directory fsync is a POSIX durability nicety
// that some platforms/filesystems (notably Windows) do not support,
// and the file-level fsync plus the atomic rename already guarantee no
// torn or partially-written file regardless.
func syncDir(dir string) {
	d, err := os.Open(dir)
	if err != nil {
		return
	}
	_ = d.Sync()
	_ = d.Close()
}
