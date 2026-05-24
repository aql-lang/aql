// Package auth holds the auth primitives shared between the
// registry server (UserStore, password hashing, token generation)
// and the client subcommands register/login/publish (ClientUser
// storage, prompt helpers).
//
// HTTP handlers live in internal/registry; client-side commands
// live in internal/{register,login,publish}.
package auth

import (
	"bufio"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
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

// LoadUsers reads the users.json file; returns an empty map if the
// file does not exist.
func (s *UserStore) LoadUsers() (map[string]*User, error) {
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

// LoadTokens reads the tokens.json file; returns an empty map if the
// file does not exist.
func (s *UserStore) LoadTokens() (map[string]*TokenInfo, error) {
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
	users, err := s.LoadUsers()
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

	hash, err := HashPassword(password)
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
	users, err := s.LoadUsers()
	if err != nil {
		return "", nil, err
	}

	user, exists := users[username]
	if !exists {
		return "", nil, fmt.Errorf("invalid credentials")
	}

	if err := CheckPassword(user.PasswordHash, password); err != nil {
		return "", nil, fmt.Errorf("invalid credentials")
	}

	token, err := GenerateToken()
	if err != nil {
		return "", nil, err
	}

	tokens, err := s.LoadTokens()
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
	tokens, err := s.LoadTokens()
	if err != nil {
		return "", err
	}

	info, exists := tokens[token]
	if !exists {
		return "", fmt.Errorf("invalid token")
	}

	return info.Username, nil
}

// HashPassword hashes a password using bcrypt.
func HashPassword(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(hash), nil
}

// CheckPassword verifies a password against a bcrypt hash.
func CheckPassword(hash, password string) error {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
}

// GenerateToken creates a cryptographically random hex token (64 characters).
func GenerateToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// InputReader wraps an io.Reader with buffered line reading and an
// isatty-aware password prompt.
type InputReader struct {
	br  *bufio.Reader
	raw io.Reader
}

// NewInputReader wraps r in an InputReader.
func NewInputReader(r io.Reader) *InputReader {
	return &InputReader{br: bufio.NewReader(r), raw: r}
}

// ReadLine prompts on w and reads one line of input (trimmed).
func (ir *InputReader) ReadLine(prompt string, w io.Writer) (string, error) {
	fmt.Fprint(w, prompt)
	line, err := ir.br.ReadString('\n')
	if err != nil && line == "" {
		return "", fmt.Errorf("no input")
	}
	return strings.TrimSpace(line), nil
}

// ReadPassword prompts on w and reads one line. If the underlying
// reader is a terminal, echoing is suppressed via golang.org/x/term.
func (ir *InputReader) ReadPassword(prompt string, w io.Writer) (string, error) {
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

// SaveClientUser writes user.jsonic to {homeDir}/.aql/user.jsonic.
func SaveClientUser(homeDir string, cu *ClientUser) error {
	dir := filepath.Join(homeDir, ".aql")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	content := fmt.Sprintf("username: %s\nemail: %s\ntoken: %s\nregistry: %s\n",
		cu.Username, cu.Email, cu.Token, cu.Registry)
	return os.WriteFile(filepath.Join(dir, "user.jsonic"), []byte(content), 0600)
}

// LoadClientUser reads {homeDir}/.aql/user.jsonic.
func LoadClientUser(homeDir string) (*ClientUser, error) {
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
