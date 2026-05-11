package main

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// --- Unit tests: password hashing ---

func TestHashPasswordAndCheck(t *testing.T) {
	hash, err := hashPassword("mypassword")
	if err != nil {
		t.Fatal(err)
	}
	if hash == "" {
		t.Fatal("hash is empty")
	}
	if err := checkPassword(hash, "mypassword"); err != nil {
		t.Errorf("checkPassword failed for correct password: %s", err)
	}
}

func TestCheckPasswordWrong(t *testing.T) {
	hash, err := hashPassword("correct")
	if err != nil {
		t.Fatal(err)
	}
	if err := checkPassword(hash, "wrong"); err == nil {
		t.Error("checkPassword should fail for wrong password")
	}
}

func TestHashPasswordUniqueness(t *testing.T) {
	h1, _ := hashPassword("same")
	h2, _ := hashPassword("same")
	if h1 == h2 {
		t.Error("same password should produce different hashes (different salt)")
	}
}

// --- Unit tests: token generation ---

func TestGenerateToken(t *testing.T) {
	token, err := generateToken()
	if err != nil {
		t.Fatal(err)
	}
	if len(token) != 64 {
		t.Errorf("token length = %d, want 64", len(token))
	}
	// Verify it's valid hex.
	if _, err := hex.DecodeString(token); err != nil {
		t.Errorf("token is not valid hex: %s", err)
	}
}

func TestGenerateTokenUniqueness(t *testing.T) {
	tokens := make(map[string]bool)
	for i := 0; i < 10; i++ {
		tok, err := generateToken()
		if err != nil {
			t.Fatal(err)
		}
		if tokens[tok] {
			t.Errorf("duplicate token generated: %s", tok)
		}
		tokens[tok] = true
	}
}

// --- Unit tests: UserStore ---

func TestUserStoreRegister(t *testing.T) {
	dir := t.TempDir()
	store := NewUserStore(dir)

	err := store.Register("alice@example.com", "alice", "password123")
	if err != nil {
		t.Fatal(err)
	}

	users, err := store.loadUsers()
	if err != nil {
		t.Fatal(err)
	}
	if len(users) != 1 {
		t.Fatalf("expected 1 user, got %d", len(users))
	}
	u := users["alice"]
	if u.Email != "alice@example.com" {
		t.Errorf("email = %q, want alice@example.com", u.Email)
	}
	if u.Username != "alice" {
		t.Errorf("username = %q, want alice", u.Username)
	}
	if u.PasswordHash == "" {
		t.Error("password hash is empty")
	}
	if u.CreatedAt == "" {
		t.Error("created_at is empty")
	}
}

func TestUserStoreRegisterDuplicateUsername(t *testing.T) {
	dir := t.TempDir()
	store := NewUserStore(dir)

	store.Register("a@example.com", "alice", "pass1")
	err := store.Register("b@example.com", "alice", "pass2")
	if err == nil {
		t.Fatal("expected error for duplicate username")
	}
	if !strings.Contains(err.Error(), "username already taken") {
		t.Errorf("unexpected error: %s", err)
	}
}

func TestUserStoreRegisterDuplicateEmail(t *testing.T) {
	dir := t.TempDir()
	store := NewUserStore(dir)

	store.Register("same@example.com", "alice", "pass1")
	err := store.Register("same@example.com", "bob", "pass2")
	if err == nil {
		t.Fatal("expected error for duplicate email")
	}
	if !strings.Contains(err.Error(), "email already registered") {
		t.Errorf("unexpected error: %s", err)
	}
}

func TestUserStoreLogin(t *testing.T) {
	dir := t.TempDir()
	store := NewUserStore(dir)

	store.Register("alice@example.com", "alice", "password123")
	token, user, err := store.Login("alice", "password123")
	if err != nil {
		t.Fatal(err)
	}
	if token == "" {
		t.Error("token is empty")
	}
	if user.Username != "alice" {
		t.Errorf("username = %q, want alice", user.Username)
	}
}

func TestUserStoreLoginWrongPassword(t *testing.T) {
	dir := t.TempDir()
	store := NewUserStore(dir)

	store.Register("alice@example.com", "alice", "password123")
	_, _, err := store.Login("alice", "wrongpassword")
	if err == nil {
		t.Fatal("expected error for wrong password")
	}
	if !strings.Contains(err.Error(), "invalid credentials") {
		t.Errorf("unexpected error: %s", err)
	}
}

func TestUserStoreLoginUnknownUser(t *testing.T) {
	dir := t.TempDir()
	store := NewUserStore(dir)

	_, _, err := store.Login("nobody", "password")
	if err == nil {
		t.Fatal("expected error for unknown user")
	}
	if !strings.Contains(err.Error(), "invalid credentials") {
		t.Errorf("unexpected error: %s", err)
	}
}

func TestUserStoreValidateToken(t *testing.T) {
	dir := t.TempDir()
	store := NewUserStore(dir)

	store.Register("alice@example.com", "alice", "password123")
	token, _, _ := store.Login("alice", "password123")

	username, err := store.ValidateToken(token)
	if err != nil {
		t.Fatal(err)
	}
	if username != "alice" {
		t.Errorf("username = %q, want alice", username)
	}
}

func TestUserStoreValidateTokenInvalid(t *testing.T) {
	dir := t.TempDir()
	store := NewUserStore(dir)

	_, err := store.ValidateToken("nonexistenttoken")
	if err == nil {
		t.Fatal("expected error for invalid token")
	}
}

// --- Unit tests: client user storage ---

func TestClientUserRoundTrip(t *testing.T) {
	homeDir := t.TempDir()

	cu := &ClientUser{
		Username: "alice",
		Email:    "alice@example.com",
		Token:    "abc123def456",
		Registry: "http://localhost:8080",
	}

	if err := saveClientUser(homeDir, cu); err != nil {
		t.Fatal(err)
	}

	loaded, err := loadClientUser(homeDir)
	if err != nil {
		t.Fatal(err)
	}

	if loaded.Username != cu.Username {
		t.Errorf("username = %q, want %q", loaded.Username, cu.Username)
	}
	if loaded.Email != cu.Email {
		t.Errorf("email = %q, want %q", loaded.Email, cu.Email)
	}
	if loaded.Token != cu.Token {
		t.Errorf("token = %q, want %q", loaded.Token, cu.Token)
	}
	if loaded.Registry != cu.Registry {
		t.Errorf("registry = %q, want %q", loaded.Registry, cu.Registry)
	}
}

func TestClientUserFilePermissions(t *testing.T) {
	homeDir := t.TempDir()

	cu := &ClientUser{
		Username: "alice",
		Email:    "alice@example.com",
		Token:    "abc123",
		Registry: "http://localhost:8080",
	}
	saveClientUser(homeDir, cu)

	info, err := os.Stat(filepath.Join(homeDir, ".aql", "user.jsonic"))
	if err != nil {
		t.Fatal(err)
	}
	perm := info.Mode().Perm()
	if perm != 0600 {
		t.Errorf("file permissions = %o, want 0600", perm)
	}
}

// --- Unit tests: readPassword ---

func TestReadPasswordFromReader(t *testing.T) {
	input := strings.NewReader("secret\n")
	ir := newInputReader(input)
	var out bytes.Buffer
	pw, err := ir.readPassword("Password: ", &out)
	if err != nil {
		t.Fatal(err)
	}
	if pw != "secret" {
		t.Errorf("password = %q, want secret", pw)
	}
	if !strings.Contains(out.String(), "Password: ") {
		t.Errorf("expected prompt in output, got %q", out.String())
	}
}

// --- HTTP handler tests ---

func setupAuthServer(t *testing.T) string {
	t.Helper()
	regDir := t.TempDir()
	srv := httptest.NewServer(registryHandler(regDir))
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

func TestLoginEndpointSuccess(t *testing.T) {
	srvURL := setupAuthServer(t)

	// Register first.
	regBody := `{"email":"alice@example.com","username":"alice","password":"password123"}`
	resp, _ := http.Post(srvURL+"/api/register", "application/json", strings.NewReader(regBody))
	resp.Body.Close()

	// Login.
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

// --- Publish auth tests ---

func TestPublishRequiresAuth(t *testing.T) {
	srvURL := setupAuthServer(t)

	zipData := makeModuleZip(t, map[string]string{
		"aql.jsonic": "name: test\nmajor: 1\nminor: 0\npatch: 0\nfiles: [test.aql]\n",
		"test.aql":   "1",
	})

	// No auth header.
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

func TestPublishWithValidToken(t *testing.T) {
	srvURL := setupAuthServer(t)

	// Register and login.
	regBody := `{"email":"pub@example.com","username":"publisher","password":"pass123"}`
	resp, _ := http.Post(srvURL+"/api/register", "application/json", strings.NewReader(regBody))
	resp.Body.Close()

	loginBody := `{"username":"publisher","password":"pass123"}`
	resp, _ = http.Post(srvURL+"/api/login", "application/json", strings.NewReader(loginBody))
	var loginResult map[string]string
	json.NewDecoder(resp.Body).Decode(&loginResult)
	resp.Body.Close()
	token := loginResult["token"]

	// Publish with valid token.
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

// --- Integration test: full register -> login -> publish -> install flow ---

func TestRegisterLoginPublishFlow(t *testing.T) {
	regDir := t.TempDir()
	srv := httptest.NewServer(registryHandler(regDir))
	defer srv.Close()
	srvURL := srv.URL

	// 1. Register a user.
	regBody := `{"email":"dev@example.com","username":"dev","password":"devpass"}`
	resp, err := http.Post(srvURL+"/api/register", "application/json", strings.NewReader(regBody))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("register: status = %d", resp.StatusCode)
	}

	// 2. Login.
	loginBody := `{"username":"dev","password":"devpass"}`
	resp, err = http.Post(srvURL+"/api/login", "application/json", strings.NewReader(loginBody))
	if err != nil {
		t.Fatal(err)
	}
	var loginResult map[string]string
	json.NewDecoder(resp.Body).Decode(&loginResult)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("login: status = %d", resp.StatusCode)
	}
	token := loginResult["token"]
	if token == "" {
		t.Fatal("login returned empty token")
	}

	// 3. Create a module and pack it.
	moduleDir := t.TempDir()
	os.WriteFile(filepath.Join(moduleDir, "aql.jsonic"),
		[]byte("name: flowmod\nmajor: 1\nminor: 0\npatch: 0\nfiles: [flowmod.aql]\n"), 0644)
	os.WriteFile(filepath.Join(moduleDir, "flowmod.aql"),
		[]byte(`export Flowmod {val: 42}`), 0644)

	var packOut, packErr bytes.Buffer
	code := runPack([]string{moduleDir}, &packOut, &packErr)
	if code != 0 {
		t.Fatalf("pack failed: %s", packErr.String())
	}
	zipPath := strings.TrimSpace(packOut.String())
	zipData, err := os.ReadFile(zipPath)
	if err != nil {
		t.Fatalf("reading zip: %s", err)
	}

	// 4. Publish with auth token.
	resp, err = authPublish(srvURL, token, zipData)
	if err != nil {
		t.Fatal(err)
	}
	pubBody, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("publish: status = %d; body: %s", resp.StatusCode, pubBody)
	}

	var pubResult map[string]string
	json.Unmarshal(pubBody, &pubResult)
	if pubResult["module"] != "flowmod" {
		t.Errorf("published module = %q, want flowmod", pubResult["module"])
	}
	if pubResult["version"] != "1.0.0" {
		t.Errorf("published version = %q, want 1.0.0", pubResult["version"])
	}

	// 5. Install the published module.
	installDir := t.TempDir()
	os.WriteFile(filepath.Join(installDir, "aql.jsonic"),
		[]byte("name: myapp\nmajor: 0\nminor: 1\npatch: 0\nfiles: [app.aql]\n"), 0644)
	os.WriteFile(filepath.Join(installDir, "app.aql"), []byte("1"), 0644)
	os.MkdirAll(filepath.Join(installDir, ".aql"), 0755)

	orig, _ := os.Getwd()
	os.Chdir(installDir)
	defer os.Chdir(orig)

	var prepOut, prepErr bytes.Buffer
	runPrep(nil, &prepOut, &prepErr)

	var instOut, instErr bytes.Buffer
	code = runInstall([]string{"-r", srvURL, "flowmod-1.0.0"}, &instOut, &instErr)
	if code != 0 {
		t.Fatalf("install failed: %s", instErr.String())
	}
	if !strings.Contains(instOut.String(), "installed flowmod@1.0.0") {
		t.Errorf("unexpected install output: %q", instOut.String())
	}

	// Verify the installed module.
	modAql, _ := os.ReadFile(filepath.Join(".aql", "flowmod", "flowmod.aql"))
	if !strings.Contains(string(modAql), "val: 42") {
		t.Errorf("installed module content wrong: %s", modAql)
	}
}

// --- CLI command tests ---

func TestRunRegisterCLI(t *testing.T) {
	regDir := t.TempDir()
	srv := httptest.NewServer(registryHandler(regDir))
	defer srv.Close()

	stdin := strings.NewReader("test@example.com\ntestuser\ntestpass\n")
	var stdout, stderr bytes.Buffer

	code := runRegister([]string{"-r", srv.URL}, stdin, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("runRegister failed: %s", stderr.String())
	}
	if !strings.Contains(stdout.String(), "registered testuser") {
		t.Errorf("unexpected output: %q", stdout.String())
	}
}

func TestRunLoginCLI(t *testing.T) {
	regDir := t.TempDir()
	srv := httptest.NewServer(registryHandler(regDir))
	defer srv.Close()

	// Register first via API.
	regBody := `{"email":"cli@example.com","username":"cliuser","password":"clipass"}`
	resp, _ := http.Post(srv.URL+"/api/register", "application/json", strings.NewReader(regBody))
	resp.Body.Close()

	// Override home dir.
	homeDir := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", homeDir)
	defer os.Setenv("HOME", origHome)

	stdin := strings.NewReader("cliuser\nclipass\n")
	var stdout, stderr bytes.Buffer

	code := runLogin([]string{"-r", srv.URL}, stdin, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("runLogin failed: %s", stderr.String())
	}
	if !strings.Contains(stdout.String(), "logged in as cliuser") {
		t.Errorf("unexpected output: %q", stdout.String())
	}

	// Verify user.jsonic was created.
	cu, err := loadClientUser(homeDir)
	if err != nil {
		t.Fatal(err)
	}
	if cu.Username != "cliuser" {
		t.Errorf("username = %q, want cliuser", cu.Username)
	}
	if cu.Token == "" {
		t.Error("token is empty")
	}
}

func TestRunPublishNotLoggedIn(t *testing.T) {
	homeDir := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", homeDir)
	defer os.Setenv("HOME", origHome)

	var stdout, stderr bytes.Buffer
	code := runPublish(nil, nil, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("expected exit code 1, got %d", code)
	}
	if !strings.Contains(stderr.String(), "not logged in") {
		t.Errorf("unexpected error: %q", stderr.String())
	}
}

func TestRunPublishCLI(t *testing.T) {
	regDir := t.TempDir()
	srv := httptest.NewServer(registryHandler(regDir))
	defer srv.Close()

	// Register and login via API.
	regBody := `{"email":"pub@example.com","username":"pubuser","password":"pubpass"}`
	resp, _ := http.Post(srv.URL+"/api/register", "application/json", strings.NewReader(regBody))
	resp.Body.Close()

	loginBody := `{"username":"pubuser","password":"pubpass"}`
	resp, _ = http.Post(srv.URL+"/api/login", "application/json", strings.NewReader(loginBody))
	var loginResult map[string]string
	json.NewDecoder(resp.Body).Decode(&loginResult)
	resp.Body.Close()

	// Save credentials.
	homeDir := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", homeDir)
	defer os.Setenv("HOME", origHome)

	saveClientUser(homeDir, &ClientUser{
		Username: "pubuser",
		Email:    "pub@example.com",
		Token:    loginResult["token"],
		Registry: srv.URL,
	})

	// Create a module directory.
	moduleDir := t.TempDir()
	os.WriteFile(filepath.Join(moduleDir, "aql.jsonic"),
		[]byte("name: clipub\nmajor: 1\nminor: 0\npatch: 0\nfiles: [clipub.aql]\n"), 0644)
	os.WriteFile(filepath.Join(moduleDir, "clipub.aql"), []byte("1"), 0644)

	var stdout, stderr bytes.Buffer
	code := runPublish([]string{"-r", srv.URL, moduleDir}, nil, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("runPublish failed: %s", stderr.String())
	}
	if !strings.Contains(stdout.String(), "published clipub@1.0.0") {
		t.Errorf("unexpected output: %q", stdout.String())
	}
}
