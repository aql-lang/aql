// Package install implements `aql install <name>-x.y.z [-r <url>]`
// — download a published module zip from the registry, extract it
// into .aql/<name>/, update aql.jsonic deps, and re-prep.
package install

import (
	"archive/zip"
	"bytes"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/aql-lang/aql/cmd/go/internal/command"
	"github.com/aql-lang/aql/cmd/go/internal/prep"
)

// moduleIDPattern matches <name>-<major>.<minor>.<patch>.
var moduleIDPattern = regexp.MustCompile(`^(.+)-(\d+\.\d+\.\d+)$`)

type cmd struct{}

// New returns the install subcommand.
func New() command.Command { return &cmd{} }

func (*cmd) Name() string       { return "install" }
func (*cmd) Synopsis() string   { return "download and install a module from a registry" }
func (*cmd) Mode() command.Mode { return command.ModeSinglePass }
func (*cmd) Run(args []string, _ io.Reader, stdout, stderr io.Writer) int {
	return Run(args, stdout, stderr)
}

// Run handles `aql install <name>-x.y.z [-r <url>]`.
func Run(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("install", flag.ContinueOnError)
	fs.SetOutput(stderr)

	registryURL := fs.String("r", "http://localhost:8080", "registry server URL")

	if err := fs.Parse(args); err != nil {
		return 1
	}

	if fs.NArg() < 1 {
		fmt.Fprintf(stderr, "error: usage: aql install <name>-x.y.z [-r <url>]\n")
		return 1
	}

	moduleID := fs.Arg(0)
	matches := moduleIDPattern.FindStringSubmatch(moduleID)
	if matches == nil {
		fmt.Fprintf(stderr, "error: invalid module identifier %q (expected <name>-x.y.z)\n", moduleID)
		return 1
	}
	name := matches[1]
	version := matches[2]

	aqlJSON := filepath.Join(".aql", "aql.json")
	if _, err := os.Stat(aqlJSON); err != nil {
		fmt.Fprintf(stderr, "error: not a valid module folder (missing .aql/aql.json)\n")
		return 1
	}

	url := strings.TrimRight(*registryURL, "/") + "/module/" + moduleID
	resp, err := http.Get(url)
	if err != nil {
		fmt.Fprintf(stderr, "error: %s\n", err)
		return 1
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		fmt.Fprintf(stderr, "error: module %q not found on registry (status %d)\n", moduleID, resp.StatusCode)
		return 1
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		fmt.Fprintf(stderr, "error: %s\n", err)
		return 1
	}

	destDir := filepath.Join(".aql", name)
	if err := os.MkdirAll(destDir, 0755); err != nil {
		fmt.Fprintf(stderr, "error: %s\n", err)
		return 1
	}

	zr, err := zip.NewReader(bytes.NewReader(body), int64(len(body)))
	if err != nil {
		fmt.Fprintf(stderr, "error: invalid zip: %s\n", err)
		return 1
	}

	for _, f := range zr.File {
		if strings.Contains(f.Name, "..") {
			continue
		}
		destPath := filepath.Join(destDir, f.Name)
		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(destPath, 0755); err != nil {
				fmt.Fprintf(stderr, "error: %s\n", err)
				return 1
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
			fmt.Fprintf(stderr, "error: %s\n", err)
			return 1
		}
		rc, err := f.Open()
		if err != nil {
			fmt.Fprintf(stderr, "error: %s\n", err)
			return 1
		}
		data, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			fmt.Fprintf(stderr, "error: %s\n", err)
			return 1
		}
		if err := os.WriteFile(destPath, data, 0644); err != nil {
			fmt.Fprintf(stderr, "error: %s\n", err)
			return 1
		}
	}

	if err := updateDeps(name, version); err != nil {
		fmt.Fprintf(stderr, "error: updating aql.jsonic: %s\n", err)
		return 1
	}

	if _, err := prep.Do("."); err != nil {
		fmt.Fprintf(stderr, "error: prep: %s\n", err)
		return 1
	}

	fmt.Fprintf(stdout, "installed %s@%s -> .aql/%s/\n", name, version, name)
	return 0
}

// updateDeps reads aql.jsonic, adds/updates deps.<name>=<version>, writes back.
func updateDeps(name, version string) error {
	src := "aql.jsonic"
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}

	content := string(data)
	depEntry := fmt.Sprintf("%s: %s", name, version)

	if strings.Contains(content, "deps:") {
		lines := strings.Split(content, "\n")
		found := false
		inDeps := false
		for i, line := range lines {
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, "deps:") {
				inDeps = true
				if strings.Contains(trimmed, "{") {
					namePattern := regexp.MustCompile(regexp.QuoteMeta(name) + `:\s*[^\s}]+`)
					if namePattern.MatchString(trimmed) {
						lines[i] = namePattern.ReplaceAllString(line, depEntry)
						found = true
					} else {
						lines[i] = strings.Replace(line, "}", " "+depEntry+"}", 1)
						found = true
					}
					inDeps = false
				}
				continue
			}
			if inDeps && trimmed == "}" {
				inDeps = false
				continue
			}
			if inDeps {
				namePrefix := name + ":"
				if strings.HasPrefix(trimmed, namePrefix) {
					lines[i] = strings.Replace(line, trimmed, depEntry, 1)
					found = true
				}
			}
		}
		if found {
			content = strings.Join(lines, "\n")
		} else {
			content = strings.Replace(content, "deps: {", "deps: {"+depEntry+" ", 1)
		}
	} else {
		content = strings.TrimRight(content, "\n") + "\ndeps: {" + depEntry + "}\n"
	}

	return os.WriteFile(src, []byte(content), 0644)
}
