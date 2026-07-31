// discovery.go writes a small JSON file at startup so local clients
// (ctl, tui) can find the api server without arguments. The file
// holds the URL the api is bound to, the bearer token (if any), and
// the supervisor PID. Mode 0600 so other users on the host can't
// read the token.
//
// Default path is $TMPDIR/boru-api.json. There's deliberately no PID
// in the name: the well-known location is what makes the no-arg
// client UX work. Users running multiple supervisors must pass
// --bind/--token explicitly to one or both clients.

package api

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// supervisorVersion is set by the serve cmd before Start so the
// /v1/server endpoint can report a meaningful version. Stored at
// package scope (rather than a Server field) so the api package
// doesn't have to import cmd/go.
var supervisorVersion = "0.1.0-dev"

// SetSupervisorVersion lets the top-level CLI publish its Version
// constant for the /v1/server endpoint without a circular import.
func SetSupervisorVersion(v string) { supervisorVersion = v }

// discoveryFile is the JSON shape written to disk.
type discoveryFile struct {
	URL   string `json:"url"`
	Token string `json:"token,omitempty"`
	PID   int    `json:"pid"`
}

// tempDirFunc is the source of the tmpdir used by DefaultDiscoveryPath.
// Tests swap it; production uses os.TempDir.
var tempDirFunc = os.TempDir

// jsonMarshal is a test seam (design/TEST-SEAMS.10.md); tests swap it to
// drive writeDiscoveryFile's marshal-failure arm, which is unreachable
// with the plain string/int discoveryFile shape.
var jsonMarshal = json.Marshal

// fileChmod is a test seam (design/TEST-SEAMS.10.md); tests swap it to
// drive writeDiscoveryFile's chmod-failure arm, which cannot be forced
// via file permissions when the suite runs as root (uid 0 ignores modes).
var fileChmod = (*os.File).Chmod

// fileWrite is a test seam (design/TEST-SEAMS.10.md); tests swap it to
// drive writeDiscoveryFile's write-failure arm (same root-only reason).
var fileWrite = (*os.File).Write

// fileClose is a test seam (design/TEST-SEAMS.10.md); tests swap it to
// drive writeDiscoveryFile's close-failure arm (same root-only reason).
var fileClose = (*os.File).Close

// DefaultDiscoveryPath returns the standard location: $TMPDIR/boru-api.json.
func DefaultDiscoveryPath() string {
	return filepath.Join(tempDirFunc(), "boru-api.json")
}

// writeDiscoveryFile drops the file at DefaultDiscoveryPath. Called
// from Start once the listener is bound and the URL is known.
func (s *Server) writeDiscoveryFile() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	path := DefaultDiscoveryPath()
	s.discoverPath = path

	body, err := jsonMarshal(discoveryFile{
		URL:   "http://" + s.Addr(),
		Token: s.token,
		PID:   os.Getpid(),
	})
	if err != nil {
		return err
	}
	// Write atomically: create a temp file with a random, exclusive name
	// (O_EXCL) in the same directory, then rename over the target. A fixed
	// "<path>.tmp" name in a shared temp dir is a symlink-TOCTOU footgun —
	// os.WriteFile would follow a pre-planted symlink and leak the token.
	// os.CreateTemp uses O_CREATE|O_EXCL with an unpredictable suffix.
	f, err := os.CreateTemp(filepath.Dir(path), "boru-api-*.json.tmp")
	if err != nil {
		return err
	}
	tmp := f.Name()
	if err := fileChmod(f, 0600); err != nil {
		_ = f.Close()
		_ = os.Remove(tmp)
		return err
	}
	if _, err := fileWrite(f, body); err != nil {
		_ = f.Close()
		_ = os.Remove(tmp)
		return err
	}
	if err := fileClose(f); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

// removeDiscoveryFile cleans up on shutdown. Errors are swallowed
// because the supervisor is exiting anyway.
func (s *Server) removeDiscoveryFile() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.discoverPath != "" {
		_ = os.Remove(s.discoverPath)
	}
}

// ReadDiscoveryFile reads the default file and returns its contents,
// or an error if the file is missing or malformed. Exported for use
// by the ctl and tui clients.
func ReadDiscoveryFile() (url, token string, pid int, err error) {
	path := DefaultDiscoveryPath()
	data, err := os.ReadFile(path)
	if err != nil {
		return "", "", 0, fmt.Errorf("no api discovery file at %s (is `boru serve ... + api` running?): %w", path, err)
	}
	var f discoveryFile
	if err := json.Unmarshal(data, &f); err != nil {
		return "", "", 0, fmt.Errorf("malformed discovery file %s: %w", path, err)
	}
	return f.URL, f.Token, f.PID, nil
}
