package aql

import (
	"bufio"
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
	"golang.org/x/term"
)

// User represents a registered user stored server-side.
type User struct {
	Email        string `json:"email"`
	Username     string `json:"username"`
	PasswordHash string `json:"password_hash"`
	CreatedAt    string `json:"created_at"`
}

// TokenInfo represents a server-side auth token.
type TokenInfo struct {
	Username  string `json:"username"`
	CreatedAt string `json:"created_at"`
}

// UserStore manages user and token persistence in a registry directory.
type UserStore struct {
	dir string
}

// ClientUser represents the locally-stored login state (~/.aql/user.jsonic).
type ClientUser struct {
	Username string `json:"username"`
	Email    string `json:"email"`
	Token    string `json:"token"`
	Registry string `json:"registry"`
}

// NewUserStore creates a UserStore backed by the given directory.
func NewUserStore(registryDir string) *UserStore {
	return &UserStore{dir: registryDir}
}

func (s *UserStore) usersPath() string {
	return filepath.Join(s.dir, "users.json")
}

func (s *UserStore) tokensPath() string {
	return filepath.Join(s.dir, "tokens.json")
}

func (s *UserStore) loadUsers() (map[string]*User, error) {
	data, err := os.ReadFile(s.usersPath())
	if err != nil {
		if os.IsNotExist(err) {
			return make(map[string]*User), nil
		}
		return nil, err
	}
	var users map[string]*User
	if err := json.Unmarshal(data, &users); err != nil {
		return nil, err
	}
	return users, nil
}

func (s *UserStore) saveUsers(users map[string]*User) error {
	data, err := json.MarshalIndent(users, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.usersPath(), data, 0600)
}

func (s *UserStore) loadTokens() (map[string]*TokenInfo, error) {
	data, err := os.ReadFile(s.tokensPath())
	if err != nil {
		if os.IsNotExist(err) {
			return make(map[string]*TokenInfo), nil
		}
		return nil, err
	}
	var tokens map[string]*TokenInfo
	if err := json.Unmarshal(data, &tokens); err != nil {
		return nil, err
	}
	return tokens, nil
}

func (s *UserStore) saveTokens(tokens map[string]*TokenInfo) error {
	data, err := json.MarshalIndent(tokens, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.tokensPath(), data, 0600)
}

// Register creates a new user. Returns error if username or email already taken.
func (s *UserStore) Register(email, username, password string) error {
	users, err := s.loadUsers()
	if err != nil {
		return err
	}

	if _, exists := users[username]; exists {
		return fmt.Errorf("username already taken")
	}

	for _, u := range users {
		if u.Email == email {
			return fmt.Errorf("email already registered")
		}
	}

	hash, err := hashPassword(password)
	if err != nil {
		return err
	}

	users[username] = &User{
		Email:        email,
		Username:     username,
		PasswordHash: hash,
		CreatedAt:    time.Now().UTC().Format(time.RFC3339),
	}

	return s.saveUsers(users)
}

// Login validates credentials and returns a token.
func (s *UserStore) Login(username, password string) (string, *User, error) {
	users, err := s.loadUsers()
	if err != nil {
		return "", nil, err
	}

	user, exists := users[username]
	if !exists {
		return "", nil, fmt.Errorf("invalid credentials")
	}

	if err := checkPassword(user.PasswordHash, password); err != nil {
		return "", nil, fmt.Errorf("invalid credentials")
	}

	token, err := generateToken()
	if err != nil {
		return "", nil, err
	}

	tokens, err := s.loadTokens()
	if err != nil {
		return "", nil, err
	}

	tokens[token] = &TokenInfo{
		Username:  username,
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
	}

	if err := s.saveTokens(tokens); err != nil {
		return "", nil, err
	}

	return token, user, nil
}

// ValidateToken checks a token and returns the associated username.
func (s *UserStore) ValidateToken(token string) (string, error) {
	tokens, err := s.loadTokens()
	if err != nil {
		return "", err
	}

	info, exists := tokens[token]
	if !exists {
		return "", fmt.Errorf("invalid token")
	}

	return info.Username, nil
}

// hashPassword hashes a password using bcrypt.
func hashPassword(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(hash), nil
}

// checkPassword verifies a password against a bcrypt hash.
func checkPassword(hash, password string) error {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
}

// generateToken creates a cryptographically random hex token (64 characters).
func generateToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// inputReader wraps an io.Reader with buffered line reading.
type inputReader struct {
	br  *bufio.Reader
	raw io.Reader
}

func newInputReader(r io.Reader) *inputReader {
	return &inputReader{br: bufio.NewReader(r), raw: r}
}

func (ir *inputReader) readLine(prompt string, w io.Writer) (string, error) {
	fmt.Fprint(w, prompt)
	line, err := ir.br.ReadString('\n')
	if err != nil && line == "" {
		return "", fmt.Errorf("no input")
	}
	return strings.TrimSpace(line), nil
}

func (ir *inputReader) readPassword(prompt string, w io.Writer) (string, error) {
	fmt.Fprint(w, prompt)
	if f, ok := ir.raw.(*os.File); ok && term.IsTerminal(int(f.Fd())) {
		pw, err := term.ReadPassword(int(f.Fd()))
		fmt.Fprintln(w)
		if err != nil {
			return "", err
		}
		return string(pw), nil
	}
	line, err := ir.br.ReadString('\n')
	if err != nil && line == "" {
		return "", fmt.Errorf("no input")
	}
	return strings.TrimSpace(line), nil
}

// saveClientUser writes user.jsonic to {homeDir}/.aql/user.jsonic.
func saveClientUser(homeDir string, cu *ClientUser) error {
	dir := filepath.Join(homeDir, ".aql")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	content := fmt.Sprintf("username: %s\nemail: %s\ntoken: %s\nregistry: %s\n",
		cu.Username, cu.Email, cu.Token, cu.Registry)
	return os.WriteFile(filepath.Join(dir, "user.jsonic"), []byte(content), 0600)
}

// loadClientUser reads {homeDir}/.aql/user.jsonic.
func loadClientUser(homeDir string) (*ClientUser, error) {
	data, err := os.ReadFile(filepath.Join(homeDir, ".aql", "user.jsonic"))
	if err != nil {
		return nil, err
	}

	cu := &ClientUser{}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		val := strings.TrimSpace(parts[1])
		switch key {
		case "username":
			cu.Username = val
		case "email":
			cu.Email = val
		case "token":
			cu.Token = val
		case "registry":
			cu.Registry = val
		}
	}
	return cu, nil
}

// --- Server-side HTTP handlers ---

// handleRegister handles POST /api/register.
func handleRegister(store *UserStore, w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Email    string `json:"email"`
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	if req.Email == "" || req.Username == "" || req.Password == "" {
		http.Error(w, "email, username, and password are required", http.StatusBadRequest)
		return
	}

	if err := store.Register(req.Email, req.Username, req.Password); err != nil {
		if strings.Contains(err.Error(), "already") {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]string{
		"status":   "registered",
		"username": req.Username,
	})
}

// handleLogin handles POST /api/login.
func handleLogin(store *UserStore, w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	if req.Username == "" || req.Password == "" {
		http.Error(w, "username and password are required", http.StatusBadRequest)
		return
	}

	token, user, err := store.Login(req.Username, req.Password)
	if err != nil {
		http.Error(w, "invalid credentials", http.StatusUnauthorized)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"token":    token,
		"username": user.Username,
		"email":    user.Email,
	})
}

// --- Client-side CLI commands ---

// runRegister handles `aql register [-r <url>]`.
func runRegister(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("register", flag.ContinueOnError)
	fs.SetOutput(stderr)
	registryURL := fs.String("r", "http://localhost:8080", "registry server URL")

	if err := fs.Parse(args); err != nil {
		return 1
	}

	ir := newInputReader(stdin)
	email, err := ir.readLine("Email: ", stdout)
	if err != nil {
		fmt.Fprintf(stderr, "error: %s\n", err)
		return 1
	}
	username, err := ir.readLine("Username: ", stdout)
	if err != nil {
		fmt.Fprintf(stderr, "error: %s\n", err)
		return 1
	}
	password, err := ir.readPassword("Password: ", stdout)
	if err != nil {
		fmt.Fprintf(stderr, "error: %s\n", err)
		return 1
	}

	if email == "" || username == "" || password == "" {
		fmt.Fprintf(stderr, "error: email, username, and password are required\n")
		return 1
	}

	body, _ := json.Marshal(map[string]string{
		"email":    email,
		"username": username,
		"password": password,
	})

	resp, err := http.Post(
		strings.TrimRight(*registryURL, "/")+"/api/register",
		"application/json",
		bytes.NewReader(body),
	)
	if err != nil {
		fmt.Fprintf(stderr, "error: %s\n", err)
		return 1
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusCreated {
		fmt.Fprintf(stderr, "error: %s\n", strings.TrimSpace(string(respBody)))
		return 1
	}

	fmt.Fprintf(stdout, "registered %s\n", username)
	return 0
}

// runLogin handles `aql login [-r <url>]`.
func runLogin(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("login", flag.ContinueOnError)
	fs.SetOutput(stderr)
	registryURL := fs.String("r", "http://localhost:8080", "registry server URL")

	if err := fs.Parse(args); err != nil {
		return 1
	}

	ir := newInputReader(stdin)
	username, err := ir.readLine("Username: ", stdout)
	if err != nil {
		fmt.Fprintf(stderr, "error: %s\n", err)
		return 1
	}
	password, err := ir.readPassword("Password: ", stdout)
	if err != nil {
		fmt.Fprintf(stderr, "error: %s\n", err)
		return 1
	}

	if username == "" || password == "" {
		fmt.Fprintf(stderr, "error: username and password are required\n")
		return 1
	}

	body, _ := json.Marshal(map[string]string{
		"username": username,
		"password": password,
	})

	resp, err := http.Post(
		strings.TrimRight(*registryURL, "/")+"/api/login",
		"application/json",
		bytes.NewReader(body),
	)
	if err != nil {
		fmt.Fprintf(stderr, "error: %s\n", err)
		return 1
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		fmt.Fprintf(stderr, "error: %s\n", strings.TrimSpace(string(respBody)))
		return 1
	}

	var result map[string]string
	if err := json.Unmarshal(respBody, &result); err != nil {
		fmt.Fprintf(stderr, "error: invalid response\n")
		return 1
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		fmt.Fprintf(stderr, "error: %s\n", err)
		return 1
	}

	cu := &ClientUser{
		Username: result["username"],
		Email:    result["email"],
		Token:    result["token"],
		Registry: strings.TrimRight(*registryURL, "/"),
	}
	if err := saveClientUser(homeDir, cu); err != nil {
		fmt.Fprintf(stderr, "error: saving credentials: %s\n", err)
		return 1
	}

	fmt.Fprintf(stdout, "logged in as %s\n", cu.Username)
	return 0
}

// runPublish handles `aql publish [-r <url>] [dir]`.
func runPublish(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("publish", flag.ContinueOnError)
	fs.SetOutput(stderr)
	registryURL := fs.String("r", "", "registry server URL")

	if err := fs.Parse(args); err != nil {
		return 1
	}

	dir := "."
	if fs.NArg() > 0 {
		dir = fs.Arg(0)
	}

	// Load credentials.
	homeDir, err := os.UserHomeDir()
	if err != nil {
		fmt.Fprintf(stderr, "error: %s\n", err)
		return 1
	}

	cu, err := loadClientUser(homeDir)
	if err != nil {
		fmt.Fprintf(stderr, "error: not logged in (run 'aql login' first)\n")
		return 1
	}
	if cu.Token == "" {
		fmt.Fprintf(stderr, "error: not logged in (run 'aql login' first)\n")
		return 1
	}

	regURL := *registryURL
	if regURL == "" {
		regURL = cu.Registry
	}
	if regURL == "" {
		regURL = "http://localhost:8080"
	}

	// Run pack to create the zip.
	var packOut, packErr bytes.Buffer
	code := runPack([]string{dir}, &packOut, &packErr)
	if code != 0 {
		fmt.Fprintf(stderr, "error: pack failed: %s", packErr.String())
		return 1
	}

	zipPath := strings.TrimSpace(packOut.String())
	zipData, err := os.ReadFile(zipPath)
	if err != nil {
		fmt.Fprintf(stderr, "error: reading zip: %s\n", err)
		return 1
	}

	// Upload with auth token.
	url := strings.TrimRight(regURL, "/") + "/api/publish"
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(zipData))
	if err != nil {
		fmt.Fprintf(stderr, "error: %s\n", err)
		return 1
	}
	req.Header.Set("Content-Type", "application/zip")
	req.Header.Set("Authorization", "Bearer "+cu.Token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		fmt.Fprintf(stderr, "error: %s\n", err)
		return 1
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusCreated {
		fmt.Fprintf(stderr, "error: %s\n", strings.TrimSpace(string(respBody)))
		return 1
	}

	var result map[string]string
	_ = json.Unmarshal(respBody, &result) // best-effort: a missing field just yields an empty string below
	fmt.Fprintf(stdout, "published %s@%s\n", result["module"], result["version"])
	return 0
}
