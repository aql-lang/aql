package main

import (
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// registryHandler serves module zip files from a registry directory.
// GET /module/<name>-x.y.z returns <registryDir>/<name>-x.y.z.zip
func registryHandler(registryDir string) http.Handler {
	mux := http.NewServeMux()
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
	return mux
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
