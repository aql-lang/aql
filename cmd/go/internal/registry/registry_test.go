package registry

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// --- Module-serving handler ---

func TestRegistryHandlerServesZip(t *testing.T) {
	dir := filepath.Join("../../../../lang/go/test/regsrv/registry")
	srv := httptest.NewServer(Handler(dir))
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/module/color-0.1.0")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "application/zip" {
		t.Errorf("Content-Type = %q, want application/zip", ct)
	}

	body, _ := io.ReadAll(resp.Body)
	zr, err := zip.NewReader(bytes.NewReader(body), int64(len(body)))
	if err != nil {
		t.Fatalf("invalid zip: %s", err)
	}

	names := make(map[string]bool)
	for _, f := range zr.File {
		names[f.Name] = true
	}
	if !names["aql.jsonic"] {
		t.Error("zip missing aql.jsonic")
	}
	if !names["color.aql"] {
		t.Error("zip missing color.aql")
	}
}

func TestRegistryHandlerServesColorScheme(t *testing.T) {
	dir := filepath.Join("../../../../lang/go/test/regsrv/registry")
	srv := httptest.NewServer(Handler(dir))
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/module/color-scheme-0.1.0")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}

	body, _ := io.ReadAll(resp.Body)
	zr, err := zip.NewReader(bytes.NewReader(body), int64(len(body)))
	if err != nil {
		t.Fatalf("invalid zip: %s", err)
	}

	names := make(map[string]bool)
	for _, f := range zr.File {
		names[f.Name] = true
	}
	if !names["aql.jsonic"] {
		t.Error("zip missing aql.jsonic")
	}
	if !names["index.aql"] {
		t.Error("zip missing index.aql")
	}
}

func TestRegistryHandlerNotFound(t *testing.T) {
	dir := filepath.Join("../../../../lang/go/test/regsrv/registry")
	srv := httptest.NewServer(Handler(dir))
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/module/nonexistent-1.0.0")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 404 {
		t.Fatalf("status = %d, want 404", resp.StatusCode)
	}
}

func TestRegistryHandlerEmptyPath(t *testing.T) {
	dir := filepath.Join("../../../../lang/go/test/regsrv/registry")
	srv := httptest.NewServer(Handler(dir))
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/module/")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 404 {
		t.Fatalf("status = %d, want 404", resp.StatusCode)
	}
}

func TestRegistryHandlerRejectsPost(t *testing.T) {
	dir := filepath.Join("../../../../lang/go/test/regsrv/registry")
	srv := httptest.NewServer(Handler(dir))
	defer srv.Close()

	resp, err := http.Post(srv.URL+"/module/color-0.1.0", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 405 {
		t.Fatalf("status = %d, want 405", resp.StatusCode)
	}
}

func TestRegistryHandlerRejectsTraversal(t *testing.T) {
	dir := filepath.Join("../../../../lang/go/test/regsrv/registry")
	srv := httptest.NewServer(Handler(dir))
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/module/../../../etc/passwd")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 404 {
		t.Fatalf("status = %d, want 404", resp.StatusCode)
	}
}

// --- run() flag handling ---

func TestRunRegistryMissingFolder(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"-r", "/nonexistent/dir", "-p", "0"}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
	if !bytes.Contains(stderr.Bytes(), []byte("not found")) {
		t.Errorf("expected 'not found' error, got %q", stderr.String())
	}
}

func TestRunRegistryMissingFlag(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"-p", "0"}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
	if !bytes.Contains(stderr.Bytes(), []byte("-r")) {
		t.Errorf("expected '-r' error, got %q", stderr.String())
	}
}

// --- /api/register endpoint ---

func setupAuthServer(t *testing.T) string {
	t.Helper()
	regDir := t.TempDir()
	srv := httptest.NewServer(Handler(regDir))
	t.Cleanup(srv.Close)
	return srv.URL
}

func TestRegisterEndpointSuccess(t *testing.T) {
	srvURL := setupAuthServer(t)

	body := `{"email":"alice@example.com","username":"alice","password":"password123"}`
	resp, err := http.Post(srvURL+"/api/register", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		respBody, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d, want 201; body: %s", resp.StatusCode, respBody)
	}

	var result map[string]string
	json.NewDecoder(resp.Body).Decode(&result)
	if result["username"] != "alice" {
		t.Errorf("username = %q, want alice", result["username"])
	}
	if result["status"] != "registered" {
		t.Errorf("status = %q, want registered", result["status"])
	}
}

func TestRegisterEndpointDuplicate(t *testing.T) {
	srvURL := setupAuthServer(t)

	body := `{"email":"alice@example.com","username":"alice","password":"password123"}`
	resp, _ := http.Post(srvURL+"/api/register", "application/json", strings.NewReader(body))
	resp.Body.Close()

	resp, err := http.Post(srvURL+"/api/register", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("status = %d, want 409", resp.StatusCode)
	}
}

func TestRegisterEndpointMissingFields(t *testing.T) {
	srvURL := setupAuthServer(t)

	tests := []struct {
		name string
		body string
	}{
		{"missing email", `{"username":"alice","password":"pass"}`},
		{"missing username", `{"email":"a@b.com","password":"pass"}`},
		{"missing password", `{"email":"a@b.com","username":"alice"}`},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			resp, err := http.Post(srvURL+"/api/register", "application/json", strings.NewReader(tc.body))
			if err != nil {
				t.Fatal(err)
			}
			resp.Body.Close()
			if resp.StatusCode != http.StatusBadRequest {
				t.Errorf("status = %d, want 400", resp.StatusCode)
			}
		})
	}
}

// --- /api/login endpoint ---

func TestLoginEndpointSuccess(t *testing.T) {
	srvURL := setupAuthServer(t)

	regBody := `{"email":"alice@example.com","username":"alice","password":"password123"}`
	resp, _ := http.Post(srvURL+"/api/register", "application/json", strings.NewReader(regBody))
	resp.Body.Close()

	loginBody := `{"username":"alice","password":"password123"}`
	resp, err := http.Post(srvURL+"/api/login", "application/json", strings.NewReader(loginBody))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d, want 200; body: %s", resp.StatusCode, respBody)
	}

	var result map[string]string
	json.NewDecoder(resp.Body).Decode(&result)
	if result["token"] == "" {
		t.Error("token is empty")
	}
	if result["username"] != "alice" {
		t.Errorf("username = %q, want alice", result["username"])
	}
	if result["email"] != "alice@example.com" {
		t.Errorf("email = %q, want alice@example.com", result["email"])
	}
}

func TestLoginEndpointWrongPassword(t *testing.T) {
	srvURL := setupAuthServer(t)

	regBody := `{"email":"alice@example.com","username":"alice","password":"password123"}`
	resp, _ := http.Post(srvURL+"/api/register", "application/json", strings.NewReader(regBody))
	resp.Body.Close()

	loginBody := `{"username":"alice","password":"wrongpassword"}`
	resp, err := http.Post(srvURL+"/api/login", "application/json", strings.NewReader(loginBody))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", resp.StatusCode)
	}
}

func TestLoginEndpointUnknownUser(t *testing.T) {
	srvURL := setupAuthServer(t)

	loginBody := `{"username":"nobody","password":"password"}`
	resp, err := http.Post(srvURL+"/api/login", "application/json", strings.NewReader(loginBody))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", resp.StatusCode)
	}
}

// --- /api/publish endpoint (auth + validation) ---

// makeModuleZip is duplicated from testutil here so this package
// does not need to import testutil (which would import this
// package, creating a cycle).
func makeModuleZip(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, content := range files {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		w.Write([]byte(content))
	}
	zw.Close()
	return buf.Bytes()
}

func registerAndLogin(t *testing.T, srvURL string) string {
	t.Helper()
	regBody := `{"email":"test@test.com","username":"testuser","password":"testpass"}`
	resp, err := http.Post(srvURL+"/api/register", "application/json", strings.NewReader(regBody))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("register: status = %d, want 201", resp.StatusCode)
	}

	loginBody := `{"username":"testuser","password":"testpass"}`
	resp, err = http.Post(srvURL+"/api/login", "application/json", strings.NewReader(loginBody))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("login: status = %d, want 200", resp.StatusCode)
	}

	var result map[string]string
	json.NewDecoder(resp.Body).Decode(&result)
	return result["token"]
}

func authPublish(srvURL string, token string, body []byte) (*http.Response, error) {
	req, err := http.NewRequest(http.MethodPost, srvURL+"/api/publish", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/zip")
	req.Header.Set("Authorization", "Bearer "+token)
	return http.DefaultClient.Do(req)
}

func setupPublishServer(t *testing.T) (srvURL string, regDir string, token string) {
	t.Helper()
	regDir = t.TempDir()
	srv := httptest.NewServer(Handler(regDir))
	t.Cleanup(srv.Close)
	tok := registerAndLogin(t, srv.URL)
	return srv.URL, regDir, tok
}

func TestPublishRequiresAuth(t *testing.T) {
	srvURL := setupAuthServer(t)

	zipData := makeModuleZip(t, map[string]string{
		"aql.jsonic": "name: test\nmajor: 1\nminor: 0\npatch: 0\nfiles: [test.aql]\n",
		"test.aql":   "1",
	})

	resp, err := http.Post(srvURL+"/api/publish", "application/zip", bytes.NewReader(zipData))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", resp.StatusCode)
	}
}

func TestPublishRejectsInvalidToken(t *testing.T) {
	srvURL := setupAuthServer(t)

	zipData := makeModuleZip(t, map[string]string{
		"aql.jsonic": "name: test\nmajor: 1\nminor: 0\npatch: 0\nfiles: [test.aql]\n",
		"test.aql":   "1",
	})

	req, _ := http.NewRequest(http.MethodPost, srvURL+"/api/publish", bytes.NewReader(zipData))
	req.Header.Set("Content-Type", "application/zip")
	req.Header.Set("Authorization", "Bearer invalidtoken")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", resp.StatusCode)
	}
}

func TestPublishValid(t *testing.T) {
	srvURL, regDir, token := setupPublishServer(t)

	zipData := makeModuleZip(t, map[string]string{
		"aql.jsonic": "name: hello\nmain: hello.aql\nmajor: 1\nminor: 0\npatch: 0\nfiles: [hello.aql]\n",
		"hello.aql":  `export Hello {greet: "hi"}`,
	})

	resp, err := authPublish(srvURL, token, zipData)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d, want 201; body: %s", resp.StatusCode, body)
	}

	var result map[string]string
	json.NewDecoder(resp.Body).Decode(&result)
	if result["module"] != "hello" {
		t.Errorf("module = %q, want hello", result["module"])
	}
	if result["version"] != "1.0.0" {
		t.Errorf("version = %q, want 1.0.0", result["version"])
	}

	if _, err := os.Stat(filepath.Join(regDir, "hello-1.0.0.zip")); err != nil {
		t.Errorf("expected hello-1.0.0.zip in registry: %s", err)
	}
}

func TestPublishRejectsOverwrite(t *testing.T) {
	srvURL, _, token := setupPublishServer(t)

	zipData := makeModuleZip(t, map[string]string{
		"aql.jsonic": "name: mymod\nmajor: 0\nminor: 1\npatch: 0\nfiles: [mymod.aql]\n",
		"mymod.aql":  `export Mymod {val: 1}`,
	})

	resp, err := authPublish(srvURL, token, zipData)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("first publish: status = %d, want 201", resp.StatusCode)
	}

	resp, err = authPublish(srvURL, token, zipData)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()

	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("second publish: status = %d, want 409; body: %s", resp.StatusCode, body)
	}
	if !strings.Contains(string(body), "already exists") {
		t.Errorf("expected 'already exists' error, got %q", body)
	}
}

func TestPublishMultipleVersions(t *testing.T) {
	srvURL, regDir, token := setupPublishServer(t)

	versions := []struct {
		major, minor, patch int
		version             string
	}{
		{1, 0, 0, "1.0.0"},
		{1, 0, 1, "1.0.1"},
		{1, 1, 0, "1.1.0"},
		{2, 0, 0, "2.0.0"},
	}

	for _, v := range versions {
		jContent := fmt.Sprintf("name: vmod\nmajor: %d\nminor: %d\npatch: %d\nfiles: [vmod.aql]\n",
			v.major, v.minor, v.patch)

		zd := makeModuleZip(t, map[string]string{
			"aql.jsonic": jContent,
			"vmod.aql":   `export Vmod {v: 1}`,
		})

		resp, err := authPublish(srvURL, token, zd)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()

		if resp.StatusCode != http.StatusCreated {
			t.Fatalf("publish %s: status = %d, want 201", v.version, resp.StatusCode)
		}

		if _, err := os.Stat(filepath.Join(regDir, "vmod-"+v.version+".zip")); err != nil {
			t.Errorf("expected vmod-%s.zip: %s", v.version, err)
		}
	}
}

func TestPublishRejectsEmptyBody(t *testing.T) {
	srvURL, _, token := setupPublishServer(t)

	resp, err := authPublish(srvURL, token, nil)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", resp.StatusCode)
	}
}

func TestPublishRejectsInvalidZip(t *testing.T) {
	srvURL, _, token := setupPublishServer(t)

	resp, err := authPublish(srvURL, token, []byte("not a zip"))
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body: %s", resp.StatusCode, body)
	}
}

func TestPublishRejectsMissingAqlJsonic(t *testing.T) {
	srvURL, _, token := setupPublishServer(t)

	zipData := makeModuleZip(t, map[string]string{
		"hello.aql": `export Hello {greet: "hi"}`,
	})

	resp, err := authPublish(srvURL, token, zipData)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body: %s", resp.StatusCode, body)
	}
	if !strings.Contains(string(body), "aql.jsonic") {
		t.Errorf("expected aql.jsonic error, got %q", body)
	}
}

func TestPublishRejectsMissingName(t *testing.T) {
	srvURL, _, token := setupPublishServer(t)

	zipData := makeModuleZip(t, map[string]string{
		"aql.jsonic": "major: 1\nminor: 0\npatch: 0\nfiles: [x.aql]\n",
		"x.aql":      "1",
	})

	resp, err := authPublish(srvURL, token, zipData)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body: %s", resp.StatusCode, body)
	}
	if !strings.Contains(string(body), "name") {
		t.Errorf("expected name error, got %q", body)
	}
}

func TestPublishRejectsMissingVersion(t *testing.T) {
	srvURL, _, token := setupPublishServer(t)

	zipData := makeModuleZip(t, map[string]string{
		"aql.jsonic": "name: noversion\nfiles: [x.aql]\n",
		"x.aql":      "1",
	})

	resp, err := authPublish(srvURL, token, zipData)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body: %s", resp.StatusCode, body)
	}
	if !strings.Contains(string(body), "version") {
		t.Errorf("expected version error, got %q", body)
	}
}

func TestPublishRejectsMissingFiles(t *testing.T) {
	srvURL, _, token := setupPublishServer(t)

	zipData := makeModuleZip(t, map[string]string{
		"aql.jsonic": "name: nofiles\nmajor: 1\nminor: 0\npatch: 0\n",
	})

	resp, err := authPublish(srvURL, token, zipData)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body: %s", resp.StatusCode, body)
	}
	if !strings.Contains(string(body), "files") {
		t.Errorf("expected files error, got %q", body)
	}
}

func TestPublishRejectsMissingDeclaredFile(t *testing.T) {
	srvURL, _, token := setupPublishServer(t)

	zipData := makeModuleZip(t, map[string]string{
		"aql.jsonic": "name: broken\nmajor: 1\nminor: 0\npatch: 0\nfiles: [missing.aql]\n",
	})

	resp, err := authPublish(srvURL, token, zipData)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body: %s", resp.StatusCode, body)
	}
	if !strings.Contains(string(body), "missing.aql") {
		t.Errorf("expected missing file error, got %q", body)
	}
}

func TestPublishRejectsGetMethod(t *testing.T) {
	srvURL, _, _ := setupPublishServer(t)

	resp, err := http.Get(srvURL + "/api/publish")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want 405", resp.StatusCode)
	}
}

func TestPublishWithValidToken(t *testing.T) {
	srvURL, _, token := setupPublishServer(t)

	zipData := makeModuleZip(t, map[string]string{
		"aql.jsonic":  "name: authmod\nmajor: 1\nminor: 0\npatch: 0\nfiles: [authmod.aql]\n",
		"authmod.aql": "1",
	})

	resp, err := authPublish(srvURL, token, zipData)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d, want 201; body: %s", resp.StatusCode, body)
	}

	var result map[string]string
	json.NewDecoder(resp.Body).Decode(&result)
	if result["module"] != "authmod" {
		t.Errorf("module = %q, want authmod", result["module"])
	}
}
