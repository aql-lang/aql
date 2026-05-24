// Package registry is the aql registry HTTP server subcommand —
// `aql registry -r <folder> -p <port>`.
//
// The server speaks three flavours of request:
//
//   - GET /module/<name>-x.y.z — serve a published module zip.
//   - POST /api/register, POST /api/login — bind to UserStore.
//   - POST /api/publish — accept a module zip from a logged-in user.
package registry

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	jsonic "github.com/jsonicjs/jsonic/go"

	"github.com/aql-lang/aql/cmd/go/internal/auth"
	"github.com/aql-lang/aql/cmd/go/internal/command"
)

// cmd is the Command implementation for `aql registry`.
type cmd struct{}

// New returns the registry subcommand.
func New() command.Command { return &cmd{} }

// Name returns "registry".
func (*cmd) Name() string { return "registry" }

// Synopsis returns the one-line help text.
func (*cmd) Synopsis() string {
	return "serve modules and auth endpoints over HTTP"
}

// Mode reports that this is a long-running server.
func (*cmd) Mode() command.Mode { return command.ModeServer }

// Run handles `aql registry -r <folder> -p <port>`.
func (*cmd) Run(args []string, _ io.Reader, stdout, stderr io.Writer) int {
	return run(args, stdout, stderr)
}

// run handles `aql registry -r <folder> -p <port>`.
func run(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("registry", flag.ContinueOnError)
	fs.SetOutput(stderr)

	registryDir := fs.String("r", "", "registry folder containing zip files")
	port := fs.Int("p", 8080, "port to listen on")

	if err := fs.Parse(args); err != nil {
		return 1
	}

	if *registryDir == "" {
		fmt.Fprintf(stderr, "error: -r <folder> is required\n")
		return 1
	}

	info, err := os.Stat(*registryDir)
	if err != nil || !info.IsDir() {
		fmt.Fprintf(stderr, "error: registry folder %q not found\n", *registryDir)
		return 1
	}

	addr := fmt.Sprintf(":%d", *port)
	handler := Handler(*registryDir)

	fmt.Fprintf(stdout, "aql registry serving %s on %s\n", *registryDir, addr)

	if err := http.ListenAndServe(addr, handler); err != nil {
		fmt.Fprintf(stderr, "error: %s\n", err)
		return 1
	}
	return 0
}

// Handler builds the http.Handler for an aql registry serving from
// registryDir. Exposed for tests that spin up an httptest server.
//
// GET /module/<name>-x.y.z returns <registryDir>/<name>-x.y.z.zip.
// POST /api/{register,login,publish} bind to UserStore.
func Handler(registryDir string) http.Handler {
	mux := http.NewServeMux()
	store := auth.NewUserStore(registryDir)

	mux.HandleFunc("/module/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		id := strings.TrimPrefix(r.URL.Path, "/module/")
		if id == "" || strings.Contains(id, "/") || strings.Contains(id, "..") {
			http.NotFound(w, r)
			return
		}

		zipPath := filepath.Join(registryDir, id+".zip")
		data, err := os.ReadFile(zipPath)
		if err != nil {
			http.NotFound(w, r)
			return
		}

		w.Header().Set("Content-Type", "application/zip")
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", id+".zip"))
		w.Write(data)
	})

	mux.HandleFunc("/api/register", func(w http.ResponseWriter, r *http.Request) {
		handleRegister(store, w, r)
	})

	mux.HandleFunc("/api/login", func(w http.ResponseWriter, r *http.Request) {
		handleLogin(store, w, r)
	})

	mux.HandleFunc("/api/publish", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		hdr := r.Header.Get("Authorization")
		token := strings.TrimPrefix(hdr, "Bearer ")
		if token == "" || token == hdr {
			http.Error(w, "authorization required", http.StatusUnauthorized)
			return
		}
		if _, err := store.ValidateToken(token); err != nil {
			http.Error(w, "invalid token", http.StatusUnauthorized)
			return
		}

		handlePublish(registryDir, w, r)
	})

	return mux
}

// handleRegister handles POST /api/register.
func handleRegister(store *auth.UserStore, w http.ResponseWriter, r *http.Request) {
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
func handleLogin(store *auth.UserStore, w http.ResponseWriter, r *http.Request) {
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

// maxPublishSize limits uploaded zip files to 10 MB.
const maxPublishSize = 10 << 20

// handlePublish validates an uploaded module zip and stores it in the registry.
// The zip must contain aql.jsonic with name, major, minor, patch, and files.
// All files listed in the files property must be present in the zip.
// Versions are immutable — publishing an existing version is rejected.
func handlePublish(registryDir string, w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, maxPublishSize+1))
	if err != nil {
		http.Error(w, "error reading body", http.StatusBadRequest)
		return
	}
	if len(body) > maxPublishSize {
		http.Error(w, "zip exceeds maximum size", http.StatusRequestEntityTooLarge)
		return
	}
	if len(body) == 0 {
		http.Error(w, "empty body", http.StatusBadRequest)
		return
	}

	zr, err := zip.NewReader(bytes.NewReader(body), int64(len(body)))
	if err != nil {
		http.Error(w, "invalid zip archive", http.StatusBadRequest)
		return
	}

	zipFiles := make(map[string]*zip.File)
	for _, f := range zr.File {
		if strings.Contains(f.Name, "..") || strings.HasPrefix(f.Name, "/") {
			http.Error(w, fmt.Sprintf("invalid path in zip: %s", f.Name), http.StatusBadRequest)
			return
		}
		zipFiles[f.Name] = f
	}

	jf, ok := zipFiles["aql.jsonic"]
	if !ok {
		http.Error(w, "zip missing aql.jsonic", http.StatusBadRequest)
		return
	}

	rc, err := jf.Open()
	if err != nil {
		http.Error(w, "cannot read aql.jsonic", http.StatusBadRequest)
		return
	}
	jdata, err := io.ReadAll(rc)
	rc.Close()
	if err != nil {
		http.Error(w, "cannot read aql.jsonic", http.StatusBadRequest)
		return
	}

	meta, err := parseJsonic(jdata)
	if err != nil {
		http.Error(w, fmt.Sprintf("invalid aql.jsonic: %s", err), http.StatusBadRequest)
		return
	}

	name, _ := meta["name"].(string)
	if name == "" {
		http.Error(w, "aql.jsonic missing name", http.StatusBadRequest)
		return
	}
	if strings.Contains(name, "/") || strings.Contains(name, "..") {
		http.Error(w, "invalid module name", http.StatusBadRequest)
		return
	}

	major, majorOk := toInt(meta["major"])
	minor, minorOk := toInt(meta["minor"])
	patch, patchOk := toInt(meta["patch"])
	if !majorOk || !minorOk || !patchOk {
		http.Error(w, "aql.jsonic missing or invalid version (major, minor, patch)", http.StatusBadRequest)
		return
	}
	version := fmt.Sprintf("%d.%d.%d", major, minor, patch)

	rawFiles, ok := meta["files"].([]any)
	if !ok || len(rawFiles) == 0 {
		http.Error(w, "aql.jsonic missing files list", http.StatusBadRequest)
		return
	}

	for _, f := range rawFiles {
		fname, ok := f.(string)
		if !ok {
			http.Error(w, "files list contains non-string entry", http.StatusBadRequest)
			return
		}
		if _, exists := zipFiles[fname]; !exists {
			http.Error(w, fmt.Sprintf("zip missing declared file: %s", fname), http.StatusBadRequest)
			return
		}
	}

	moduleID := fmt.Sprintf("%s-%s", name, version)
	zipPath := filepath.Join(registryDir, moduleID+".zip")
	if _, err := os.Stat(zipPath); err == nil {
		http.Error(w, fmt.Sprintf("version %s@%s already exists", name, version), http.StatusConflict)
		return
	}

	if err := os.MkdirAll(registryDir, 0755); err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	if err := os.WriteFile(zipPath, body, 0644); err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]string{
		"module":  name,
		"version": version,
		"status":  "published",
	})
}

// toInt extracts an integer from a value that may be float64 (from JSON) or int.
func toInt(v any) (int, bool) {
	switch n := v.(type) {
	case float64:
		return int(n), true
	case int:
		return n, true
	case json.Number:
		i, err := n.Int64()
		return int(i), err == nil
	}
	return 0, false
}

// parseJsonic parses jsonic-formatted bytes into a map.
func parseJsonic(data []byte) (map[string]any, error) {
	j := jsonic.Make()
	result, err := j.Parse(string(data))
	if err != nil {
		return nil, err
	}
	m, ok := result.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("expected map, got %T", result)
	}
	return m, nil
}
