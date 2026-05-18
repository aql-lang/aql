package aql

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
)

// registryHandler serves module zip files from a registry directory.
// GET /module/<name>-x.y.z returns <registryDir>/<name>-x.y.z.zip
// POST /api/publish accepts a module zip, validates it, and stores it.
func registryHandler(registryDir string) http.Handler {
	mux := http.NewServeMux()
	store := NewUserStore(registryDir)

	mux.HandleFunc("/module/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		// Extract module identifier from path: /module/<name>-x.y.z
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

		// Validate auth token.
		auth := r.Header.Get("Authorization")
		token := strings.TrimPrefix(auth, "Bearer ")
		if token == "" || token == auth {
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

	// Open as zip archive.
	zr, err := zip.NewReader(bytes.NewReader(body), int64(len(body)))
	if err != nil {
		http.Error(w, "invalid zip archive", http.StatusBadRequest)
		return
	}

	// Index zip entries and reject path traversal.
	zipFiles := make(map[string]*zip.File)
	for _, f := range zr.File {
		if strings.Contains(f.Name, "..") || strings.HasPrefix(f.Name, "/") {
			http.Error(w, fmt.Sprintf("invalid path in zip: %s", f.Name), http.StatusBadRequest)
			return
		}
		zipFiles[f.Name] = f
	}

	// aql.jsonic must be present.
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

	// Parse aql.jsonic. Since it's jsonic format, we need to use doPrep-style
	// parsing. But we only need the JSON equivalent, so we call the jsonic
	// parser. For simplicity we parse it via the same path as prep by writing
	// to a temp dir, running doPrep, and reading the result.
	meta, err := parseJsonic(jdata)
	if err != nil {
		http.Error(w, fmt.Sprintf("invalid aql.jsonic: %s", err), http.StatusBadRequest)
		return
	}

	// Validate required fields.
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

	// Verify all declared files are in the zip.
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

	// Check immutability — reject if version already exists.
	moduleID := fmt.Sprintf("%s-%s", name, version)
	zipPath := filepath.Join(registryDir, moduleID+".zip")
	if _, err := os.Stat(zipPath); err == nil {
		http.Error(w, fmt.Sprintf("version %s@%s already exists", name, version), http.StatusConflict)
		return
	}

	// Write the zip to the registry.
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
// Uses the same jsonic library as doPrep.
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

// runRegistry handles `aql registry -r <folder> -p <port>`.
func runRegistry(args []string, stdout, stderr io.Writer) int {
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
	handler := registryHandler(*registryDir)

	fmt.Fprintf(stdout, "aql registry serving %s on %s\n", *registryDir, addr)

	if err := http.ListenAndServe(addr, handler); err != nil {
		fmt.Fprintf(stderr, "error: %s\n", err)
		return 1
	}
	return 0
}
