package registry

import (
	"bytes"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// --- Command wrapper (New / Name / Synopsis / Run) ---

func TestRegistryCommandWrapper(t *testing.T) {
	c := New()
	if got := c.Name(); got != "registry" {
		t.Errorf("Name() = %q, want registry", got)
	}
	if c.Synopsis() == "" {
		t.Error("Synopsis() is empty")
	}
	var stdout, stderr bytes.Buffer
	if code := c.Run(nil, nil, &stdout, &stderr); code != 1 {
		t.Fatalf("Run without -r: exit = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "-r") {
		t.Errorf("stderr = %q, want -r error", stderr.String())
	}
}

func TestRunRegistryBadFlag(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := run([]string{"-bogus"}, &stdout, &stderr); code != 1 {
		t.Fatalf("exit = %d, want 1", code)
	}
}

func TestRunRegistryPortInUse(t *testing.T) {
	// Occupy a port, then point run at it: NewServer succeeds (it does not
	// bind), the banner prints, and Start fails with a listen error.
	ln, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	port := ln.Addr().(*net.TCPAddr).Port

	dir := t.TempDir()
	var stdout, stderr bytes.Buffer
	code := run([]string{"-r", dir, "-p", itoa(port)}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("exit = %d, want 1; stderr: %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "aql registry serving") {
		t.Errorf("stdout = %q, want serving banner", stdout.String())
	}
	if !strings.Contains(stderr.String(), "listen") {
		t.Errorf("stderr = %q, want listen error", stderr.String())
	}
}

func itoa(n int) string {
	var buf [8]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}

// --- handleRegister / handleLogin edge branches ---

func TestRegisterEndpointRejectsGet(t *testing.T) {
	srvURL := setupAuthServer(t)
	resp, err := http.Get(srvURL + "/api/register")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want 405", resp.StatusCode)
	}
}

func TestRegisterEndpointRejectsBadJSON(t *testing.T) {
	srvURL := setupAuthServer(t)
	resp, err := http.Post(srvURL+"/api/register", "application/json", strings.NewReader("{not json"))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", resp.StatusCode)
	}
}

func TestLoginEndpointRejectsGet(t *testing.T) {
	srvURL := setupAuthServer(t)
	resp, err := http.Get(srvURL + "/api/login")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want 405", resp.StatusCode)
	}
}

func TestLoginEndpointRejectsBadJSON(t *testing.T) {
	srvURL := setupAuthServer(t)
	resp, err := http.Post(srvURL+"/api/login", "application/json", strings.NewReader("{not json"))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", resp.StatusCode)
	}
}

func TestLoginEndpointMissingFields(t *testing.T) {
	srvURL := setupAuthServer(t)
	for _, body := range []string{
		`{"password":"pass"}`,
		`{"username":"alice"}`,
	} {
		resp, err := http.Post(srvURL+"/api/login", "application/json", strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("body %q: status = %d, want 400", body, resp.StatusCode)
		}
	}
}

func TestRegisterEndpointServerError(t *testing.T) {
	// A corrupt users.json makes UserStore.LoadUsers fail, which surfaces
	// as a 500 (a non-"already" registration error).
	regDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(regDir, "users.json"), []byte("{corrupt"), 0o644); err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(Handler(regDir))
	t.Cleanup(srv.Close)

	body := `{"email":"a@b.com","username":"alice","password":"pass"}`
	resp, err := http.Post(srv.URL+"/api/register", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", resp.StatusCode)
	}
}

// --- handlePublish edge branches (direct calls bypass the auth wrapper) ---

// publishDirect drives handlePublish without the Handler auth wrapper.
func publishDirect(regDir string, body []byte) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/api/publish", bytes.NewReader(body))
	w := httptest.NewRecorder()
	handlePublish(regDir, w, req)
	return w
}

func TestPublishRejectsOversizedBody(t *testing.T) {
	big := bytes.Repeat([]byte{0xAB}, maxPublishSize+2)
	w := publishDirect(t.TempDir(), big)
	if w.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want 413", w.Code)
	}
}

type failingReader struct{}

func (failingReader) Read([]byte) (int, error) { return 0, errors.New("boom") }

func TestPublishBodyReadError(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/api/publish", failingReader{})
	w := httptest.NewRecorder()
	handlePublish(t.TempDir(), w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
	if !strings.Contains(w.Body.String(), "error reading body") {
		t.Errorf("body = %q, want read error", w.Body.String())
	}
}

func TestPublishRejectsTraversalPathInZip(t *testing.T) {
	zipData := makeModuleZip(t, map[string]string{
		"aql.jsonic": "name: evil\nmajor: 1\nminor: 0\npatch: 0\nfiles: [evil.aql]\n",
		"../evil":    "gotcha",
	})
	w := publishDirect(t.TempDir(), zipData)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
	if !strings.Contains(w.Body.String(), "invalid path in zip") {
		t.Errorf("body = %q, want invalid-path error", w.Body.String())
	}
}

func TestPublishRejectsAbsolutePathInZip(t *testing.T) {
	zipData := makeModuleZip(t, map[string]string{
		"aql.jsonic": "name: evil\nmajor: 1\nminor: 0\npatch: 0\nfiles: [evil.aql]\n",
		"/abs/evil":  "gotcha",
	})
	w := publishDirect(t.TempDir(), zipData)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
	if !strings.Contains(w.Body.String(), "invalid path in zip") {
		t.Errorf("body = %q, want invalid-path error", w.Body.String())
	}
}

func TestPublishRejectsNonMapJsonic(t *testing.T) {
	zipData := makeModuleZip(t, map[string]string{
		"aql.jsonic": "[1, 2, 3]",
	})
	w := publishDirect(t.TempDir(), zipData)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
	if !strings.Contains(w.Body.String(), "invalid aql.jsonic") {
		t.Errorf("body = %q, want invalid aql.jsonic", w.Body.String())
	}
}

func TestPublishRejectsSlashInModuleName(t *testing.T) {
	zipData := makeModuleZip(t, map[string]string{
		"aql.jsonic": "name: 'a/b'\nmajor: 1\nminor: 0\npatch: 0\nfiles: [x.aql]\n",
		"x.aql":      "1",
	})
	w := publishDirect(t.TempDir(), zipData)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
	if !strings.Contains(w.Body.String(), "invalid module name") {
		t.Errorf("body = %q, want invalid module name", w.Body.String())
	}
}

func TestPublishRejectsNonStringFilesEntry(t *testing.T) {
	zipData := makeModuleZip(t, map[string]string{
		"aql.jsonic": "name: numfiles\nmajor: 1\nminor: 0\npatch: 0\nfiles: [1]\n",
	})
	w := publishDirect(t.TempDir(), zipData)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
	if !strings.Contains(w.Body.String(), "non-string") {
		t.Errorf("body = %q, want non-string error", w.Body.String())
	}
}

func TestPublishCannotOpenJsonicEntry(t *testing.T) {
	// Corrupting the local file header signature (the "PK\x03\x04" at
	// offset 0 of a single-entry zip) leaves the central directory — and
	// therefore zip.NewReader — intact, but entry Open fails.
	zipData := makeModuleZip(t, map[string]string{
		"aql.jsonic": "name: mod\nmajor: 1\nminor: 0\npatch: 0\nfiles: [mod.aql]\n",
	})
	zipData[0] = 'X'
	w := publishDirect(t.TempDir(), zipData)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "cannot read aql.jsonic") {
		t.Errorf("body = %q, want cannot-read error", w.Body.String())
	}
}

func TestPublishCannotReadJsonicEntry(t *testing.T) {
	// Corrupting the compressed data (which starts right after the
	// 30-byte local header plus the entry name) keeps Open working but
	// fails the decompressing read.
	name := "aql.jsonic"
	zipData := makeModuleZip(t, map[string]string{
		name: "name: mod\nmajor: 1\nminor: 0\npatch: 0\nfiles: [mod.aql]\n",
	})
	for i := 30 + len(name); i < 30+len(name)+8; i++ {
		zipData[i] ^= 0xFF
	}
	w := publishDirect(t.TempDir(), zipData)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "cannot read aql.jsonic") {
		t.Errorf("body = %q, want cannot-read error", w.Body.String())
	}
}

func TestPublishServerErrorOnUnwritableZipPath(t *testing.T) {
	// A module name longer than the filesystem's NAME_MAX makes the
	// existence probe error (so no 409) and the final write fail: 500.
	longName := strings.Repeat("n", 300)
	zipData := makeModuleZip(t, map[string]string{
		"aql.jsonic": "name: " + longName + "\nmajor: 1\nminor: 0\npatch: 0\nfiles: [mod.aql]\n",
		"mod.aql":    "1",
	})
	w := publishDirect(t.TempDir(), zipData)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500; body: %s", w.Code, w.Body.String())
	}
}

func TestPublishServerErrorWhenRegistryDirUncreatable(t *testing.T) {
	// A registry dir nested under a plain FILE cannot be created:
	// MkdirAll fails and the handler reports a server error.
	dir := t.TempDir()
	blocker := filepath.Join(dir, "blocker")
	if err := os.WriteFile(blocker, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	zipData := makeModuleZip(t, map[string]string{
		"aql.jsonic": "name: mod\nmajor: 1\nminor: 0\npatch: 0\nfiles: [mod.aql]\n",
		"mod.aql":    "1",
	})
	w := publishDirect(filepath.Join(blocker, "sub"), zipData)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500; body: %s", w.Code, w.Body.String())
	}
}

// --- toInt / parseJsonic ---

func TestToInt(t *testing.T) {
	cases := []struct {
		in     any
		want   int
		wantOk bool
	}{
		{float64(7), 7, true},
		{int(9), 9, true},
		{json.Number("11"), 11, true},
		{json.Number("not-a-number"), 0, false},
		{"5", 0, false},
		{nil, 0, false},
	}
	for _, c := range cases {
		got, ok := toInt(c.in)
		if got != c.want || ok != c.wantOk {
			t.Errorf("toInt(%#v) = (%d, %v), want (%d, %v)", c.in, got, ok, c.want, c.wantOk)
		}
	}
}

func TestParseJsonic(t *testing.T) {
	m, err := parseJsonic([]byte("name: hello\nmajor: 1\n"))
	if err != nil {
		t.Fatalf("parseJsonic: %v", err)
	}
	if m["name"] != "hello" {
		t.Errorf("name = %v, want hello", m["name"])
	}

	if _, err := parseJsonic([]byte("[1, 2, 3]")); err == nil {
		t.Error("expected an error for a non-map document")
	} else if !strings.Contains(err.Error(), "expected map") {
		t.Errorf("err = %v, want expected-map error", err)
	}

	if _, err := parseJsonic([]byte(`"unterminated`)); err == nil {
		t.Error("expected a parse error for malformed jsonic")
	}
}
